import type { Gauge } from "../api/types";

export function GaugeCard({ gauge }: { gauge: Gauge }) {
  const name = `${gauge.completed} of ${gauge.total} ${gauge.label}`;
  return (
    <div className="gauge-card">
      <div className="gauge-copy"><span>{gauge.kind === "macro_phases" ? "Overall progress" : "Milestones"}</span><strong>{gauge.completed}/{gauge.total}</strong></div>
      <progress value={gauge.completed} max={Math.max(gauge.total, 1)} aria-label={name} />
      <p>{name}</p>
      {gauge.invalidated && <p className="gauge-warning">{gauge.reason ?? "This progress binding must be refreshed"}</p>}
    </div>
  );
}
