import type { ApiEnvelope, EventPage, RunSnapshot } from "./types";

let sessionPromise: Promise<void> | undefined;

async function request<T>(url: string): Promise<T> {
  const response = await fetch(url, {
    credentials: "same-origin",
    headers: { Accept: "application/json" },
  });
  const envelope = (await response.json()) as ApiEnvelope<T>;
  if (!response.ok || envelope.error || envelope.data === undefined) {
    const message = envelope.error?.message ?? "The local rct service could not complete the request";
    throw new ApiError(message, envelope.error?.code ?? "request_failed", envelope.request_id);
  }
  return envelope.data;
}

export class ApiError extends Error {
  constructor(
    message: string,
    readonly code: string,
    readonly requestId: string,
  ) {
    super(message);
    this.name = "ApiError";
  }
}

export function bootstrapSession(): Promise<void> {
  if (sessionPromise) return sessionPromise;
  sessionPromise = (async () => {
    const fragment = new URLSearchParams(window.location.hash.replace(/^#/, ""));
    const token = fragment.get("bootstrap");
    if (!token) return;
    window.history.replaceState(null, "", `${window.location.pathname}${window.location.search}`);
    const response = await fetch("/api/v1/session", {
      method: "POST",
      credentials: "same-origin",
      headers: { "Content-Type": "application/json", Accept: "application/json" },
      body: JSON.stringify({ token }),
    });
    const envelope = (await response.json()) as ApiEnvelope<{ csrf_token: string }>;
    if (!response.ok || envelope.error) {
      throw new ApiError(
        envelope.error?.message ?? "The local browser session could not be established",
        envelope.error?.code ?? "unauthorized",
        envelope.request_id,
      );
    }
  })();
  return sessionPromise;
}

export const getRuns = (): Promise<RunSnapshot[]> => request("/api/v1/runs");

export const getRun = (runId: string): Promise<RunSnapshot> =>
  request(`/api/v1/runs/${encodeURIComponent(runId)}`);

export const getEvents = (runId: string, afterSequence: number, limit = 100): Promise<EventPage> =>
  request(
    `/api/v1/runs/${encodeURIComponent(runId)}/events?after_seq=${afterSequence}&limit=${limit}`,
  );

export const streamURL = (runId: string, afterSequence: number): string =>
  `/api/v1/runs/${encodeURIComponent(runId)}/stream?after_seq=${afterSequence}`;
