import type { PhaseProgress } from "../api/types";

export function PhaseTimeline({ phases }: { phases: PhaseProgress[] }) {
  return (
    <ol className="timeline">
      {phases.map((phase) => (
        <li key={phase.id} className={`timeline-item phase-${phase.status}`}>
          <span className="timeline-icon" aria-hidden="true">{phaseIcon(phase.status)}</span>
          <span><strong>{phase.label}</strong><small>{phaseStatus(phase.status)}</small></span>
        </li>
      ))}
    </ol>
  );
}

function phaseIcon(status: PhaseProgress["status"]): string {
  if (status === "completed" || status === "approved") return "✓";
  if (status === "running") return "●";
  if (status === "waiting") return "Ⅱ";
  if (status === "changes_requested") return "↻";
  if (status === "failed") return "!";
  return "○";
}

function phaseStatus(status: PhaseProgress["status"]): string {
  return status.replaceAll("_", " ");
}
