import { NavLink } from "react-router";
import { localizedIdentifier, localizedState, useI18n } from "../i18n";
import { useRunCatalog } from "../runCatalog";
import type { RunSnapshot } from "../api/types";
import { stateKind } from "./StatusPill";

export function RunSidebar() {
  const { runs, error } = useRunCatalog();
  const { locale, setLocale, t } = useI18n();
  const active = runs?.filter((run) => !terminalGroup(run)) ?? [];
  const completed = runs?.filter((run) => run.state === "COMPLETED") ?? [];
  const failed = runs?.filter((run) => terminalGroup(run) === "failed") ?? [];

  return (
    <aside className="run-sidebar" aria-label={t("status")}>
      <div className="sidebar-head">
        <NavLink to="/" className="brand" aria-label="rct home"><span className="brand-mark" aria-hidden="true">rct</span><span className="brand-copy">{t("runControl")}</span></NavLink>
        <span className="local-badge"><span aria-hidden="true">●</span>{t("localOnly")}</span>
      </div>
      <div className="language-switch" aria-label="Language">
        <button type="button" aria-pressed={locale === "ja"} onClick={() => setLocale("ja")}>日本語</button>
        <button type="button" aria-pressed={locale === "en"} onClick={() => setLocale("en")}>EN</button>
      </div>
      <nav className="agent-nav" aria-label={t("activeAgents")}>
        <RunGroup title={t("activeAgents")} runs={active} empty={t("noActiveAgents")} />
        {completed.length > 0 && <RunGroup title={t("completedRuns")} runs={completed} compact />}
        <div className="failed-run-group"><RunGroup title={t("failedRuns")} runs={failed} empty={t("noFailedRuns")} compact /></div>
      </nav>
      {error && <p className="sidebar-error" role="alert">{error}</p>}
      <p className="sidebar-footer">{t("footer")}</p>
    </aside>
  );
}

function RunGroup({ title, runs, empty, compact = false }: { title: string; runs: RunSnapshot[]; empty?: string; compact?: boolean }) {
  return (
    <section className={`run-group${compact ? " run-group-compact" : ""}`}>
      <h2>{title}<span>{runs.length}</span></h2>
      {runs.length === 0 && empty && <p className="run-group-empty">{empty}</p>}
      <div className="agent-list">
        {runs.map((run) => <AgentLink run={run} key={run.run_id} />)}
      </div>
    </section>
  );
}

function AgentLink({ run }: { run: RunSnapshot }) {
  const { locale } = useI18n();
  const provider = run.activity?.provider ?? "rct";
  const role = run.activity?.role ?? "controller";
  const kind = stateKind(run.state);
  return (
    <NavLink to={`/runs/${run.run_id}`} className={({ isActive }) => `agent-link agent-${kind}${isActive ? " selected" : ""}`}>
      <span className="agent-avatar" aria-hidden="true">{provider.slice(0, 1).toUpperCase()}</span>
      <span className="agent-link-copy">
        <strong>{run.project_name}</strong>
        <span>{localizedIdentifier(provider, locale)} · {localizedIdentifier(role, locale)}</span>
        <small>{localizedState(run.state, locale)}</small>
      </span>
      <span className="agent-state-dot" aria-hidden="true">{kind === "failed" ? "!" : kind === "waiting" ? "Ⅱ" : kind === "completed" ? "✓" : "●"}</span>
    </NavLink>
  );
}

function terminalGroup(run: RunSnapshot): false | "failed" | "completed" {
  if (run.state === "COMPLETED") return "completed";
  if (run.state === "FAILED" || run.state === "BLOCKED" || run.state === "CANCELLED") return "failed";
  return false;
}
