package workspace

import (
	"context"
	"errors"
	"regexp"
	"time"

	"github.com/jacu-dev/jacu-harness/internal/gitx"
	"github.com/jacu-dev/jacu-harness/internal/runstate"
	"github.com/jacu-dev/jacu-harness/internal/telemetry"
)

var sddIntegrationBranch = regexp.MustCompile(`^sdd/[0-9]{3}$`)

var newAutonomyGit = gitx.New

type autonomyIntegrationResult struct {
	Escalated        bool
	PreserveWorktree bool
	Warning          string
}

func integrateAutonomy(ctx context.Context, root, branch, objective, diffDigest, evidenceDigest, receiptRef string) autonomyIntegrationResult {
	return integrateAutonomyWithHistory(ctx, root, branch, objective, diffDigest, evidenceDigest, receiptRef, nil)
}

func integrateAutonomyWithHistory(ctx context.Context, root, branch, objective, diffDigest, evidenceDigest, receiptRef string, audit *runstate.AuditPackage) autonomyIntegrationResult {
	return integrateAutonomyWithIdentity(ctx, root, branch, objective, diffDigest, evidenceDigest, receiptRef, "", "", audit)
}

func integrateAutonomyWithIdentity(ctx context.Context, root, branch, objective, diffDigest, evidenceDigest, receiptRef, runID, missionID string, audit *runstate.AuditPackage) autonomyIntegrationResult {
	_ = objective
	_ = diffDigest
	_ = evidenceDigest
	_ = receiptRef
	_ = audit
	git, err := newAutonomyGit()
	if err != nil {
		return autonomyEscalation(root, runID, missionID, autonomyIntegrationResult{}, "git unavailable: "+err.Error())
	}
	current, err := git.CurrentBranch(ctx, root)
	if err != nil || !sddIntegrationBranch.MatchString(current) {
		return autonomyEscalation(root, runID, missionID, autonomyIntegrationResult{}, "checkout is not an sdd/<NNN> branch; merge refused")
	}
	dirty, err := git.HasTrackedChanges(ctx, root)
	if err != nil || dirty {
		return autonomyEscalation(root, runID, missionID, autonomyIntegrationResult{}, "checkout has tracked changes; merge refused")
	}
	if err := git.MergeFFOnly(ctx, root, branch); err != nil {
		if !errors.Is(err, gitx.ErrNotFastForward) {
			return autonomyEscalation(root, runID, missionID, autonomyIntegrationResult{}, "fast-forward merge failed: "+err.Error())
		}
		if mergeErr := git.MergeNoFF(ctx, root, branch); mergeErr != nil {
			if errors.Is(mergeErr, gitx.ErrMergeConflict) {
				if abortErr := git.MergeAbort(ctx, root); abortErr != nil {
					return autonomyEscalation(root, runID, missionID, autonomyIntegrationResult{}, "merge conflict; merge abort failed: "+abortErr.Error())
				}
				return autonomyEscalation(root, runID, missionID, autonomyIntegrationResult{}, "merge conflict; worktree preserved")
			}
			return autonomyEscalation(root, runID, missionID, autonomyIntegrationResult{}, "merge failed: "+mergeErr.Error())
		}
	}
	emitAutonomyTelemetry(root, runID, missionID, telemetry.EventApply, "applied", "pass", 0, "completed")
	return autonomyIntegrationResult{}
}

func autonomyEscalation(root, runID, missionID string, result autonomyIntegrationResult, warning string) autonomyIntegrationResult {
	emitAutonomyTelemetry(root, runID, missionID, telemetry.EventEscalation, "escalated", "", 0, "escalated")
	result.Escalated = true
	result.PreserveWorktree = true
	result.Warning = warning
	return result
}

func emitAutonomyTelemetry(root, runID, missionID, eventName, status, verdict string, iteration int, exitReason string) {
	telemetry.EmitBestEffortInput(telemetry.EventInput{
		Timestamp: time.Now().UTC(), ProjectID: telemetry.ProjectID(root), TraceID: telemetry.NewTraceID(),
		RunID: runID, MissionID: missionID, Event: eventName, Tool: "autonomy", Status: status,
		Verdict: verdict, Iteration: iteration, ExitReason: exitReason,
	})
}
