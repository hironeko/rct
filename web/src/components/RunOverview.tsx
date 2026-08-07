import { useState } from "react";
import type { ProgressEvent, RunSnapshot } from "../api/types";
import { localizedEvent, localizedIdentifier, useI18n } from "../i18n";
import { GaugeCard } from "./GaugeCard";
import { PhaseTimeline } from "./PhaseTimeline";
import { StatusPill } from "./StatusPill";

interface RunOverviewProps {
  run: RunSnapshot;
  events: ProgressEvent[];
  connection: string;
  onApprove?: (note: string) => Promise<void>;
  approving?: boolean;
  approvalError?: string;
}

export function RunOverview({ run, events, connection, onApprove, approving = false, approvalError }: RunOverviewProps) {
  const { locale, t } = useI18n();
  const macroGauge = run.gauges.find((gauge) => gauge.kind === "macro_phases");
  const milestoneGauge = run.gauges.find((gauge) => gauge.kind === "milestones");
  const activity = run.activity;
  const connectionLabels: Record<string, string> = {
    Connecting: t("connectionConnecting"), Live: t("connectionLive"), Polling: t("connectionPolling"), Current: t("connectionCurrent"),
  };
  return (
    <>
      <header className="conversation-header">
        <div><p className="eyebrow">{t("workflow")} · {run.mode} · {run.backend}</p><h1>{run.project_name}</h1><code>{run.run_id}</code></div>
        <div className="run-state"><StatusPill state={run.state} /><span className={`connection connection-${connection.toLowerCase()}`}>{connectionLabels[connection] ?? connection}</span></div>
      </header>

      <section className="run-gauges" aria-label={t("overallProgress")}>
        {macroGauge && <GaugeCard gauge={macroGauge} />}{milestoneGauge && <GaugeCard gauge={milestoneGauge} />}
      </section>

      <div className="run-workspace-grid">
        <section className="conversation-panel" aria-labelledby="conversation-title">
          <div className="conversation-title"><div><p className="eyebrow">LIVE THREAD</p><h2 id="conversation-title">{t("conversation")}</h2></div><span>{events.length}</span></div>
          <div className="message-thread">
            {events.length === 0 && !activity && <p className="conversation-empty">{t("conversationEmpty")}</p>}
            {events.slice(-14).map((event) => <EventMessage event={event} key={event.sequence} />)}
            {activity && <ActivityMessage run={run} />}
            {!activity && run.waiting_reason && <SystemMessage title={t("humanAttention")} body={waitingReason(run.state, t, run.waiting_reason)} />}
          </div>

          {run.state === "AWAITING_IMPLEMENTATION_APPROVAL" && onApprove ? (
            <ApprovalDock onApprove={onApprove} approving={approving} error={approvalError} />
          ) : (run.waiting_reason || run.next_action) && (
            <div className="next-action-dock"><span aria-hidden="true">→</span><div><strong>{t("nextAction")}</strong><p>{waitingReason(run.state, t, run.waiting_reason)}</p>{run.next_action && <code>{run.next_action}</code>}</div></div>
          )}
        </section>

        <aside className="run-inspector" aria-label={t("status")}>
          <section className="inspector-panel" aria-labelledby="timeline-title"><div className="panel-heading"><p className="eyebrow">GATES</p><h2 id="timeline-title">{t("phaseTimeline")}</h2></div><PhaseTimeline phases={run.phases} /></section>
          <section className="inspector-panel" aria-labelledby="artifacts-title"><div className="panel-heading"><p className="eyebrow">OUTPUTS</p><h2 id="artifacts-title">{t("artifacts")}</h2></div>
            {!run.artifacts || Object.keys(run.artifacts).length === 0 ? <p className="muted">{t("noArtifacts")}</p> : <dl className="artifact-list">{Object.entries(run.artifacts).map(([kind, reference]) => <div key={kind}><dt>{localizedIdentifier(kind, locale)}</dt><dd><code>{reference}</code></dd></div>)}</dl>}
          </section>
        </aside>
      </div>
    </>
  );
}

function EventMessage({ event }: { event: ProgressEvent }) {
  const { locale } = useI18n();
  const human = event.type === "ImplementationApprovalRequested" || event.type.startsWith("Human");
  const provider = human ? "human" : (event.provider || (event.role ? "rct" : "rct"));
  const role = human ? "human" : (event.role || "controller");
  return (
    <article className={`chat-message message-${human ? "human" : role}`}>
      <span className="message-avatar" aria-hidden="true">{provider === "human" ? "Y" : provider.slice(0, 1).toUpperCase()}</span>
      <div className="message-body">
        <header><strong>{localizedIdentifier(provider === "human" ? "human" : provider, locale)}</strong><span>{localizedIdentifier(role, locale)}</span><time dateTime={event.timestamp}>{formatTime(event.timestamp, locale)}</time></header>
        <p>{localizedEvent(event.type, locale)}</p>
        <small>{[event.phase && localizedIdentifier(event.phase, locale), event.artifact_kind && localizedIdentifier(event.artifact_kind, locale), event.round ? `R${event.round}` : ""].filter(Boolean).join(" · ")}</small>
      </div>
    </article>
  );
}

function ActivityMessage({ run }: { run: RunSnapshot }) {
  const { locale, t } = useI18n();
  const activity = run.activity;
  if (!activity) return null;
  const provider = activity.provider ?? "rct";
  return (
    <article className="chat-message message-current">
      <span className="message-avatar" aria-hidden="true">{provider.slice(0, 1).toUpperCase()}</span>
      <div className="message-body">
        <header><strong>{localizedIdentifier(provider, locale)}</strong><span>{localizedIdentifier(activity.role ?? "controller", locale)}</span><em>{t("currentActivity")}</em></header>
        <p>{localizedIdentifier(activity.action, locale)} · <strong>{localizedIdentifier(activity.artifact_kind ?? activity.phase, locale)}</strong></p>
        <dl className="message-facts">
          {activity.round ? <><dt>{t("reviewBudget")}</dt><dd>{t("roundOf", { round: activity.round, max: activity.max_rounds ?? "—" })}</dd></> : null}
          {activity.candidate_version ? <><dt>{t("candidate")}</dt><dd>{t("version", { version: activity.candidate_version })}</dd></> : null}
          <dt>{t("liveness")}</dt><dd>{localizedIdentifier(activity.status, locale)}</dd>
          {activity.job_id ? <><dt>{t("job")}</dt><dd><code>{activity.job_id}</code></dd></> : null}
        </dl>
        {activity.previous_verdict && <div className="message-verdict"><span>{t("previousVerdict")}</span><strong>{localizedIdentifier(activity.previous_verdict, locale)}</strong>{activity.required_change_count ? <small>{t("requiredChanges", { count: activity.required_change_count })}</small> : null}</div>}
        {activity.error && <div className="error-banner" role="alert"><strong>{activity.error.code}</strong><span>{activity.error.summary}</span></div>}
      </div>
    </article>
  );
}

function SystemMessage({ title, body }: { title: string; body: string }) {
  return <article className="chat-message message-system"><span className="message-avatar" aria-hidden="true">r</span><div className="message-body"><header><strong>rct Core</strong></header><p><strong>{title}</strong></p><small>{body}</small></div></article>;
}

function ApprovalDock({ onApprove, approving, error }: { onApprove: (note: string) => Promise<void>; approving: boolean; error?: string }) {
  const { t } = useI18n();
  const [confirming, setConfirming] = useState(false);
  const [note, setNote] = useState("");
  return (
    <section className="approval-dock" aria-labelledby="approval-title">
      <span className="approval-icon" aria-hidden="true">✓</span>
      <div className="approval-copy"><p className="eyebrow">HUMAN GATE</p><h2 id="approval-title">{t("approveTitle")}</h2><p>{t("approveBody")}</p>
        {error && <div className="error-banner" role="alert">{error}</div>}
        {!confirming ? <button className="primary-action" type="button" onClick={() => setConfirming(true)}>{t("reviewApproval")}</button> : (
          <div className="approval-confirmation">
            <label htmlFor="approval-note">{t("approvalNote")}</label><textarea id="approval-note" value={note} maxLength={1000} rows={3} onChange={(event) => setNote(event.target.value)} disabled={approving} />
            <div><button className="secondary-action" type="button" onClick={() => setConfirming(false)} disabled={approving}>{t("cancel")}</button><button className="primary-action" type="button" onClick={() => void onApprove(note)} disabled={approving}>{approving ? t("approving") : t("confirmApproval")}</button></div>
          </div>
        )}
      </div>
    </section>
  );
}

function formatTime(value: string, locale: string): string {
  return new Intl.DateTimeFormat(locale === "ja" ? "ja-JP" : "en", { hour: "2-digit", minute: "2-digit" }).format(new Date(value));
}

function waitingReason(state: string, t: (key: string) => string, fallback = ""): string {
  if (state === "AWAITING_IMPLEMENTATION_APPROVAL") return t("waitingApproval");
  if (state === "WAITING_FOR_HUMAN") return t("waitingHuman");
  if (state === "BLOCKED") return t("runBlocked");
  if (state === "FAILED") return t("runFailed");
  return fallback;
}
