package app

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/hironeko/loop-engine/internal/domain"
	"github.com/hironeko/loop-engine/internal/store/filesystem"
)

type ApproveOptions struct {
	Project          string
	Approver         string
	Note             string
	ExpectedRevision uint64
}

func (s *Service) Approve(
	_ context.Context,
	options ApproveOptions,
) (domain.Run, error) {
	project, err := filepath.Abs(options.Project)
	if err != nil {
		return domain.Run{}, fmt.Errorf("resolve project path: %w", err)
	}
	store := filesystem.New(project)
	run, err := store.LoadCurrent()
	if err != nil {
		return domain.Run{}, err
	}
	if run.State != domain.StateAwaitingApproval {
		return run, fmt.Errorf(
			"human approval requires state %s; current state is %s",
			domain.StateAwaitingApproval,
			run.State,
		)
	}
	if options.ExpectedRevision != 0 && run.Revision != options.ExpectedRevision {
		return run, fmt.Errorf(
			"state revision %d does not match expected revision %d",
			run.Revision,
			options.ExpectedRevision,
		)
	}
	if strings.TrimSpace(options.Approver) == "" {
		return run, errors.New("approver is required")
	}
	if run.Approval != nil {
		return run, errors.New("human approval has already been recorded")
	}
	if run.LastVerdict != domain.VerdictApproved {
		return run, fmt.Errorf("latest reviewer verdict is %q, not approved", run.LastVerdict)
	}
	if err := validateIndependentReview(run); err != nil {
		return run, err
	}
	if run.PlanPath == "" || run.PlanReview == "" || run.PlanSHA256 == "" ||
		run.ApprovalTargetHash == "" {
		return run, errors.New("approved plan evidence is incomplete")
	}
	plan, err := store.ReadArtifact(run.ID, run.PlanPath)
	if err != nil {
		return run, err
	}
	currentHash := sha256Hex(plan)
	if currentHash != run.PlanSHA256 || currentHash != run.ApprovalTargetHash {
		return run, fmt.Errorf(
			"plan SHA-256 %q does not match approval target %q",
			currentHash,
			run.ApprovalTargetHash,
		)
	}
	if _, err := domain.ParseImplementationPlan(plan); err != nil {
		return run, err
	}

	random := make([]byte, 6)
	if _, err := io.ReadFull(s.deps.Random, random); err != nil {
		return run, fmt.Errorf("create approval id: %w", err)
	}
	now := s.deps.Now().UTC()
	record := domain.HumanApprovalRecord{
		SchemaVersion: "1.0",
		ID:            "approval_" + hex.EncodeToString(random),
		RunID:         run.ID,
		GateKind:      "implementation_start",
		Phase:         "implementation",
		SubjectPath:   run.PlanPath,
		SubjectSHA256: currentHash,
		Approver:      strings.TrimSpace(options.Approver),
		Note:          strings.TrimSpace(options.Note),
		CreatedAt:     now,
		ConsumedAt:    now,
		StateRevision: run.Revision,
	}
	encoded, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return run, fmt.Errorf("encode approval record: %w", err)
	}
	approvalRelativePath := filepath.Join("approvals", record.ID+".json")
	candidate := run
	candidate.Approval = &record
	candidate.ApprovalPath = filepath.ToSlash(filepath.Join(
		".loop-engine",
		"runs",
		run.ID,
		approvalRelativePath,
	))
	candidate.State = domain.StateImplementationReady
	candidate.UpdatedAt = now
	candidate.Revision++
	if err := store.UpdateExpectedWithRunFile(
		candidate,
		run.State,
		"HumanImplementationApprovalConsumed",
		run.Revision,
		approvalRelativePath,
		encoded,
	); err != nil {
		return run, err
	}
	return candidate, nil
}
