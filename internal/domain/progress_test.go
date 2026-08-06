package domain

import "testing"

func TestProjectProgressSupervisedPlanReview(t *testing.T) {
	run := Run{
		ID: "run_test", Project: "/project", Mode: ModeSupervised, Backend: "direct",
		State: StatePlanReview, Revision: 9,
		RequirementsPath: ".rct/requirements.json", ArchitecturePath: ".rct/architecture.json",
	}
	snapshot := ProjectProgress(run, nil, 12)
	if got := snapshot.Gauges[0]; got.Completed != 2 || got.Total != 8 {
		t.Fatalf("macro gauge = %#v, want 2/8", got)
	}
	if snapshot.Phases[2].Status != "running" {
		t.Fatalf("plan status = %q, want running", snapshot.Phases[2].Status)
	}
}

func TestProjectProgressDesignOnlyHasFixedDenominator(t *testing.T) {
	run := Run{ID: "run_test", Mode: ModeDesignOnly, State: StatePlanApproved, Revision: 4,
		RequirementsPath: "requirements", ArchitecturePath: "architecture", PlanPath: "plan"}
	snapshot := ProjectProgress(run, nil, 4)
	if got := snapshot.Gauges[0]; got.Completed != 3 || got.Total != 3 {
		t.Fatalf("macro gauge = %#v, want 3/3", got)
	}
}

func TestProjectProgressApprovalWaitIncludesCompletedPreflight(t *testing.T) {
	run := Run{ID: "run_test", Mode: ModeSupervised, State: StateAwaitingApproval, Revision: 7,
		RequirementsPath: "requirements", ArchitecturePath: "architecture", PlanPath: "plan", BaseCommit: "abc123"}
	snapshot := ProjectProgress(run, nil, 7)
	if got := snapshot.Gauges[0]; got.Completed != 4 || got.Total != 8 {
		t.Fatalf("macro gauge = %#v, want 4/8", got)
	}
}
