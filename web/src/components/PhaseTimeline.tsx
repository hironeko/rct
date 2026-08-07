import type { PhaseProgress } from "../api/types";
import { localizedIdentifier, useI18n } from "../i18n";

export function PhaseTimeline({ phases }: { phases: PhaseProgress[] }) {
  const { locale, t } = useI18n();
  return (
    <ol className="timeline">
      {phases.map((phase) => (
        <li key={phase.id} className={`timeline-item phase-${phase.status}`}>
          <span className="timeline-icon" aria-hidden="true">{phaseIcon(phase.status)}</span>
          <span><strong>{localizedIdentifier(phase.id || phase.label, locale)}</strong><small>{phaseStatus(phase.status, t)}</small></span>
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

function phaseStatus(status: PhaseProgress["status"], t: (key: string) => string): string {
  const keys: Record<PhaseProgress["status"], string> = { not_started: "notStarted", running: "running", changes_requested: "changesRequested", approved: "approvedState", waiting: "waiting", failed: "failed", completed: "completed" };
  return t(keys[status]);
}
