export function StatusPill({ state }: { state: string }) {
  const kind = stateKind(state);
  return <span className={`status-pill status-${kind}`}><span aria-hidden="true">{statusIcon(kind)}</span>{stateLabel(state)}</span>;
}

export function stateKind(state: string): "running" | "waiting" | "completed" | "failed" | "idle" {
  if (state === "COMPLETED") return "completed";
  if (state === "FAILED" || state === "BLOCKED") return "failed";
  if (state.includes("AWAITING") || state === "WAITING_FOR_HUMAN") return "waiting";
  if (state === "INTAKE") return "idle";
  return "running";
}

function statusIcon(kind: ReturnType<typeof stateKind>): string {
  return { running: "●", waiting: "Ⅱ", completed: "✓", failed: "!", idle: "○" }[kind];
}

export function stateLabel(state: string): string {
  return state.toLowerCase().split("_").map((part) => part.charAt(0).toUpperCase() + part.slice(1)).join(" ");
}
