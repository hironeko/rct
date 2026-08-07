import { NavLink } from "react-router";
import { localizedIdentifier, localizedState, useI18n } from "../i18n";
import { useRunCatalog } from "../runCatalog";

export function Home() {
  const { runs } = useRunCatalog();
  const { locale, t } = useI18n();
  const active = runs?.filter((run) => !["FAILED", "BLOCKED", "CANCELLED", "COMPLETED"].includes(run.state)).slice(0, 3) ?? [];
  return (
    <main className="workspace-page home-page">
      <section className="workspace-hero">
        <p className="eyebrow">{t("observed")}</p>
        <h1>{t("homeTitle")}</h1>
        <p>{t("homeBody")}</p>
      </section>
      <section className="select-agent-card">
        <div><span className="select-symbol" aria-hidden="true">←</span><h2>{t("selectAgent")}</h2><p>{t("selectAgentBody")}</p></div>
        {active.length > 0 && <div className="home-agent-grid">{active.map((run) => (
          <NavLink to={`/runs/${run.run_id}`} key={run.run_id}>
            <span className="agent-avatar" aria-hidden="true">{(run.activity?.provider ?? "rct").slice(0, 1).toUpperCase()}</span>
            <span><strong>{run.project_name}</strong><small>{localizedIdentifier(run.activity?.role ?? "controller", locale)} · {localizedState(run.state, locale)}</small></span>
            <span aria-hidden="true">→</span>
          </NavLink>
        ))}</div>}
      </section>
    </main>
  );
}
