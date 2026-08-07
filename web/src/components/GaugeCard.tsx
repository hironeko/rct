import type { Gauge } from "../api/types";
import { useI18n } from "../i18n";

export function GaugeCard({ gauge }: { gauge: Gauge }) {
  const { t } = useI18n();
  const name = t("phasesComplete", { completed: gauge.completed, total: gauge.total });
  return (
    <div className="gauge-card">
      <div className="gauge-copy"><span>{gauge.kind === "macro_phases" ? t("overallProgress") : t("milestones")}</span><strong>{gauge.completed}/{gauge.total}</strong></div>
      <progress value={gauge.completed} max={Math.max(gauge.total, 1)} aria-label={name} />
      <p>{name}</p>
      {gauge.invalidated && <p className="gauge-warning">{gauge.reason ?? "This progress binding must be refreshed"}</p>}
    </div>
  );
}
