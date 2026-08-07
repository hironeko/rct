import { useEffect, useRef, useState } from "react";
import { Link, useParams } from "react-router";
import { ApiError, getEvents, getRun, streamURL } from "../api/client";
import type { ProgressEvent, RunSnapshot } from "../api/types";
import { RunOverview } from "../components/RunOverview";

type ConnectionState = "Connecting" | "Live" | "Polling" | "Current";

export function RunDetail() {
  const { runId = "" } = useParams();
  const [run, setRun] = useState<RunSnapshot>();
  const [events, setEvents] = useState<ProgressEvent[]>([]);
  const [connection, setConnection] = useState<ConnectionState>("Connecting");
  const [error, setError] = useState<string>();
  const [announcement, setAnnouncement] = useState("");
  const sequence = useRef(0);

  useEffect(() => {
    let active = true;
    let source: EventSource | undefined;
    let polling: number | undefined;

    const refresh = async (announce = false) => {
      const nextRun = await getRun(runId);
      if (!active) return;
      setRun((previous) => {
        if (announce && previous && previous.state !== nextRun.state) setAnnouncement(`Run state changed to ${nextRun.state.replaceAll("_", " ")}`);
        return nextRun;
      });
      const after = Math.max(0, sequence.current || nextRun.last_event_seq - 24);
      const page = await getEvents(runId, after, 100);
      if (!active) return;
      sequence.current = Math.max(sequence.current, page.next_after_seq);
      setEvents((current) => deduplicate([...current, ...page.events]));
    };

    const beginPolling = () => {
      if (!active || polling !== undefined) return;
      source?.close();
      setConnection("Polling");
      polling = window.setInterval(() => {
        refresh(true).catch(() => undefined);
      }, 2000);
    };

    refresh()
      .then(() => {
        if (!active) return;
        source = new EventSource(streamURL(runId, sequence.current));
        source.addEventListener("open", () => setConnection("Live"));
        source.addEventListener("progress", (message) => {
          const event = JSON.parse((message as MessageEvent<string>).data) as ProgressEvent;
          if (event.sequence <= sequence.current) return;
          sequence.current = event.sequence;
          setEvents((current) => deduplicate([...current, event]));
          if (attentionEvent(event.type)) setAnnouncement(splitEventName(event.type));
          refresh().catch(beginPolling);
        });
        source.addEventListener("activity", () => refresh().catch(beginPolling));
        source.addEventListener("terminal", () => {
          setConnection("Current");
          source?.close();
          refresh(true).catch(() => undefined);
        });
        source.addEventListener("resync_required", () => refresh().catch(beginPolling));
        source.onerror = beginPolling;
      })
      .catch((reason: unknown) => {
        if (!active) return;
        setError(reason instanceof ApiError ? `${reason.message} · ${reason.requestId}` : "Unable to load this run");
      });

    return () => {
      active = false;
      source?.close();
      if (polling !== undefined) window.clearInterval(polling);
    };
  }, [runId]);

  return (
    <main className="page run-page">
      <Link to="/" className="back-link">← All runs</Link>
      <div className="sr-only" aria-live="polite" aria-atomic="true">{announcement}</div>
      {error && <div className="error-page"><p className="eyebrow">RUN UNAVAILABLE</p><h1>Unable to display this run</h1><p>{error}</p></div>}
      {!run && !error && <div className="run-loading"><span aria-hidden="true">rct</span><p>Rebuilding progress from durable state…</p></div>}
      {run && <RunOverview run={run} events={events} connection={connection} />}
    </main>
  );
}

function deduplicate(events: ProgressEvent[]): ProgressEvent[] {
  const bySequence = new Map<number, ProgressEvent>();
  for (const event of events) bySequence.set(event.sequence, event);
  return [...bySequence.values()].sort((left, right) => left.sequence - right.sequence).slice(-100);
}

function attentionEvent(type: string): boolean {
  return type === "RunFailed" || type === "RunCompleted" || type.includes("ApprovalRequested") || type.includes("LimitReached");
}

function splitEventName(value: string): string {
  return value.replace(/([a-z])([A-Z])/g, "$1 $2");
}
