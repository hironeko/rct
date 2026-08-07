import type { ProgressEvent, RunSnapshot } from "../api/types";
import { GaugeCard } from "./GaugeCard";
import { PhaseTimeline } from "./PhaseTimeline";
import { StatusPill, stateLabel } from "./StatusPill";

export function RunOverview({ run, events, connection }: { run: RunSnapshot; events: ProgressEvent[]; connection: string }) {
  const macroGauge = run.gauges.find((gauge) => gauge.kind === "macro_phases");
  const milestoneGauge = run.gauges.find((gauge) => gauge.kind === "milestones");
  const activity = run.activity;
  return (
    <>
      <section className="run-header">
        <div>
          <p className="eyebrow">{run.mode} · {run.backend}</p>
          <h1>{run.project_name}</h1>
          <code>{run.run_id}</code>
        </div>
        <div className="run-state"><StatusPill state={run.state} /><span className={`connection connection-${connection.toLowerCase()}`}>{connection}</span></div>
      </section>

      <section className="gauge-grid" aria-label="Objective progress">
        {macroGauge && <GaugeCard gauge={macroGauge} />}
        {milestoneGauge && <GaugeCard gauge={milestoneGauge} />}
      </section>

      <div className="detail-grid">
        <section className="panel activity-panel" aria-labelledby="activity-title">
          <div className="panel-heading"><p className="eyebrow">RIGHT NOW</p><h2 id="activity-title">Current activity</h2></div>
          {activity ? (
            <div className="activity-content">
              <div className="activity-who"><span className="agent-monogram" aria-hidden="true">{(activity.provider ?? "rct").slice(0, 1).toUpperCase()}</span><div><strong>{title(activity.provider ?? "rct Core")}</strong><span>{title(activity.role ?? "controller")}</span></div></div>
              <p className="activity-action">{title(activity.action)} <strong>{title(activity.artifact_kind ?? activity.phase)}</strong></p>
              <dl className="fact-grid">
                {activity.round ? <><dt>Review budget</dt><dd>Round {activity.round} of {activity.max_rounds ?? "—"}</dd></> : null}
                {activity.candidate_version ? <><dt>Candidate</dt><dd>Version {activity.candidate_version}</dd></> : null}
                <dt>Liveness</dt><dd>{activity.status === "stale" ? "Stale — checking required" : title(activity.status)}</dd>
                {activity.job_id ? <><dt>Job</dt><dd><code>{activity.job_id}</code></dd></> : null}
              </dl>
              {activity.previous_verdict && <div className="previous-verdict"><span>Previous review verdict</span><strong>{title(activity.previous_verdict)}</strong>{activity.required_change_count ? <small>{activity.required_change_count} required changes</small> : null}</div>}
              {activity.error && <div className="error-banner" role="alert"><strong>{activity.error.code}</strong><span>{activity.error.summary}</span><small>{activity.error.next_action}</small></div>}
            </div>
          ) : <div className="empty-panel"><strong>No active job</strong><p>{run.waiting_reason || `Workflow state: ${stateLabel(run.state)}`}</p></div>}
        </section>

        <section className="panel timeline-panel" aria-labelledby="timeline-title">
          <div className="panel-heading"><p className="eyebrow">GATES</p><h2 id="timeline-title">Phase timeline</h2></div>
          <PhaseTimeline phases={run.phases} />
        </section>
      </div>

      {(run.waiting_reason || run.next_action) && (
        <section className="attention-card" aria-labelledby="next-action-title">
          <div className="attention-symbol" aria-hidden="true">→</div>
          <div><p className="eyebrow">HUMAN ATTENTION</p><h2 id="next-action-title">Next action</h2><p>{run.waiting_reason}</p><strong>{run.next_action}</strong></div>
        </section>
      )}

      <div className="lower-grid">
        <section className="panel" aria-labelledby="events-title">
          <div className="panel-heading"><p className="eyebrow">AUDIT TRAIL</p><h2 id="events-title">Recent events</h2></div>
          {events.length === 0 ? <p className="muted">No recent semantic events.</p> : (
            <ol className="event-list">{events.slice(-8).reverse().map((event) => <li key={event.sequence}><time dateTime={event.timestamp}>{formatTime(event.timestamp)}</time><span><strong>{splitEventName(event.type)}</strong>{event.phase ? ` · ${title(event.phase)}` : ""}{event.provider ? ` · ${title(event.provider)}` : ""}</span></li>)}</ol>
          )}
        </section>
        <section className="panel" aria-labelledby="artifacts-title">
          <div className="panel-heading"><p className="eyebrow">OUTPUTS</p><h2 id="artifacts-title">Artifacts</h2></div>
          {!run.artifacts || Object.keys(run.artifacts).length === 0 ? <p className="muted">No public artifact references yet.</p> : (
            <dl className="artifact-list">{Object.entries(run.artifacts).map(([kind, reference]) => <div key={kind}><dt>{title(kind)}</dt><dd><code>{reference}</code></dd></div>)}</dl>
          )}
        </section>
      </div>
    </>
  );
}

function title(value: string): string {
  return value.replaceAll("_", " ").replace(/\b\w/g, (character) => character.toUpperCase());
}

function splitEventName(value: string): string {
  return value.replace(/([a-z])([A-Z])/g, "$1 $2");
}

function formatTime(value: string): string {
  return new Intl.DateTimeFormat(undefined, { hour: "2-digit", minute: "2-digit", second: "2-digit" }).format(new Date(value));
}
