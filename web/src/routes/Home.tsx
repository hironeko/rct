import { useEffect, useState } from "react";
import { Link } from "react-router";
import { ApiError, getRuns } from "../api/client";
import type { RunSnapshot } from "../api/types";
import { StatusPill } from "../components/StatusPill";

export function Home() {
  const [runs, setRuns] = useState<RunSnapshot[]>();
  const [error, setError] = useState<string>();

  useEffect(() => {
    let active = true;
    getRuns()
      .then((value) => { if (active) setRuns(value); })
      .catch((reason: unknown) => {
        if (!active) return;
        setError(reason instanceof ApiError ? `${reason.message} · ${reason.requestId}` : "Unable to load local runs");
      });
    return () => { active = false; };
  }, []);

  return (
    <main className="page home-page">
      <section className="hero">
        <div>
          <p className="eyebrow">AI ENGINEERING, OBSERVED</p>
          <h1>Know what the loop<br />is doing now.</h1>
          <p className="hero-copy">A calm, local view of requirements, architecture, implementation, verification, and independent review.</p>
        </div>
        <div className="hero-orbit" aria-hidden="true">
          <span className="orbit-core">rct</span>
          <span className="orbit-label orbit-label-a">Design</span>
          <span className="orbit-label orbit-label-b">Review</span>
          <span className="orbit-label orbit-label-c">Build</span>
        </div>
      </section>

      <section aria-labelledby="recent-runs-title" className="runs-section">
        <div className="section-heading">
          <div>
            <p className="eyebrow">WORKSPACE</p>
            <h2 id="recent-runs-title">Recent runs</h2>
          </div>
          <span className="read-only-label">Read-only view</span>
        </div>
        {error && <div className="error-banner" role="alert">{error}</div>}
        {!runs && !error && <div className="loading-block">Reading durable run state…</div>}
        {runs?.length === 0 && <div className="empty-state"><h3>No runs found</h3><p>Start a run from the CLI inside this workspace, then refresh this page.</p></div>}
        <div className="run-list">
          {runs?.map((run) => {
            const gauge = run.gauges.find((item) => item.kind === "macro_phases");
            return (
              <Link className="run-row" to={`/runs/${run.run_id}`} key={run.run_id}>
                <span className="run-primary">
                  <strong>{run.project_name}</strong>
                  <code>{shortRunId(run.run_id)}</code>
                </span>
                <span className="run-meta"><StatusPill state={run.state} />{run.mode} · {run.backend}</span>
                <span className="run-progress">{gauge ? `${gauge.completed}/${gauge.total} phases` : "Progress unavailable"}<span aria-hidden="true">→</span></span>
              </Link>
            );
          })}
        </div>
      </section>
    </main>
  );
}

function shortRunId(runId: string): string {
  const parts = runId.split("_");
  return parts.length === 3 ? parts[2].slice(0, 8) : runId.slice(-8);
}
