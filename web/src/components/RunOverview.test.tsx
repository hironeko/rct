import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import type { RunSnapshot } from "../api/types";
import { I18nProvider, type Locale } from "../i18n";
import { RunOverview } from "./RunOverview";

const fixture: RunSnapshot = {
  schema_version: "progress-v1",
  run_id: "run_20260807T010203Z_abcdef123456",
  project_name: "new-ios-game-app",
  backend: "direct",
  mode: "supervised",
  state: "PLAN_REVIEW",
  state_revision: 8,
  roles: { designer: "codex", reviewer: "claude", implementer: "codex" },
  activity: {
    revision: 3,
    status: "running",
    phase: "plan",
    action: "reviewing",
    role: "reviewer",
    provider: "claude",
    backend: "direct",
    job_id: "plan-r02-reviewer",
    round: 2,
    max_rounds: 3,
    artifact_kind: "plan",
    candidate_version: 2,
    previous_verdict: "changes_requested",
    required_change_count: 3,
    started_at: "2026-08-07T01:02:03Z",
    last_heartbeat_at: "2026-08-07T01:02:08Z",
  },
  phases: [
    { id: "requirements", label: "Requirements", status: "completed" },
    { id: "architecture", label: "Architecture", status: "completed" },
    { id: "plan", label: "Implementation Plan", status: "running" },
    { id: "implementation_preflight", label: "Implementation preflight", status: "not_started" },
    { id: "implementation_approval", label: "Human approval", status: "not_started" },
    { id: "implementation", label: "Implementation", status: "not_started" },
    { id: "final_verification", label: "Final verification", status: "not_started" },
    { id: "final_review", label: "Final review", status: "not_started" },
  ],
  gauges: [{ kind: "macro_phases", revision: 8, completed: 2, total: 8, label: "phases complete" }],
  artifacts: { plan: "artifacts/plan/v001.json" },
  last_event_seq: 16,
  updated_at: "2026-08-07T01:02:08Z",
};

describe("RunOverview", () => {
  it("separates objective progress from review budget and previous verdict", () => {
    renderOverview("en");
    expect(screen.getByLabelText("2 of 8 phases complete")).toHaveAttribute("value", "2");
    expect(screen.getByText("Round 2 of 3")).toBeInTheDocument();
    expect(screen.getByText("Previous review")).toBeInTheDocument();
    expect(screen.getByText("Changes Requested")).toBeInTheDocument();
    expect(screen.queryByText(/67%/)).not.toBeInTheDocument();
  });

  it("does not render private fields that are absent from the public DTO", () => {
    const { container } = renderOverview("en");
    expect(container).not.toHaveTextContent("/Users/");
    expect(container).not.toHaveTextContent("stdout");
    expect(container).not.toHaveTextContent("prompt");
  });

  it("renders the same workflow in Japanese", () => {
    renderOverview("ja");
    expect(screen.getByText("工程の会話")).toBeInTheDocument();
    expect(screen.getByText("レビュー回数")).toBeInTheDocument();
    expect(screen.getByText("計画レビュー")).toBeInTheDocument();
  });

  it("requires an explicit confirmation before approving", () => {
    const onApprove = vi.fn().mockResolvedValue(undefined);
    const awaiting = { ...fixture, state: "AWAITING_IMPLEMENTATION_APPROVAL", activity: undefined };
    render(<I18nProvider initialLocale="en"><RunOverview run={awaiting} events={[]} connection="Current" onApprove={onApprove} /></I18nProvider>);
    fireEvent.click(screen.getByRole("button", { name: "Review approval" }));
    fireEvent.change(screen.getByLabelText("Approval note (optional)"), { target: { value: "Looks good" } });
    expect(onApprove).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "Approve this Plan" }));
    expect(onApprove).toHaveBeenCalledWith("Looks good");
  });
});

function renderOverview(locale: Locale) {
  return render(<I18nProvider initialLocale={locale}><RunOverview run={fixture} events={[]} connection="Live" /></I18nProvider>);
}
