package workspace

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/jacu-dev/jacu-harness/internal/capability/missioncompile"
	"github.com/jacu-dev/jacu-harness/internal/capability/verify"
	"github.com/jacu-dev/jacu-harness/internal/gitx"
	"github.com/jacu-dev/jacu-harness/internal/runstate"
	"github.com/jacu-dev/jacu-harness/internal/telemetry"
)

type ApplyInput struct {
	RunID              string `json:"run_id"`
	ApproveDestructive bool   `json:"approve_destructive,omitempty"`
}

type ApplyData struct {
	CommitSHA string `json:"commit_sha"`
	Branch    string `json:"branch"`
	Stderr    string `json:"stderr"`
}

type ApplyResult struct {
	Status      string
	Summary     string
	Data        ApplyData
	Warnings    []string
	NextActions []string
}

func Apply(ctx context.Context, root string, in ApplyInput, hostName string) (ApplyResult, error) {
	var result ApplyResult
	err := runstate.WithLock(root, func() error {
		var err error
		result, err = applyUnlocked(ctx, root, in, hostName)
		return err
	})
	return result, err
}

func applyUnlocked(ctx context.Context, root string, in ApplyInput, hostName string) (ApplyResult, error) {
	run, err := runstate.Load(root, in.RunID)
	if err != nil {
		return ApplyResult{}, err
	}
	if identityErr := validateRunIdentity(root, run); identityErr != nil {
		emitWorkspaceGate(root, "block", "jacu_apply", run)
		return blockedApply("run identity check failed: "+identityErr.Error(), ""), nil
	}
	if run.Status != runstate.StatusReviewed {
		emitWorkspaceGate(root, "block", "jacu_apply", run)
		return blockedApply("diff not reviewed; call jacu_diff first", ""), nil
	}

	git, err := gitx.New()
	if err != nil {
		return ApplyResult{}, err
	}
	snapshot, err := git.DiffSnapshot(ctx, run.Worktree, run.BaseSHA)
	if err != nil {
		return ApplyResult{}, err
	}
	fullDiff := snapshot.Patch
	currentDigest := diffDigest(fullDiff)
	if currentDigest != run.ReviewedDigest {
		emitWorkspaceGate(root, "block", "jacu_apply", run)
		return blockedApply("worktree changed after review; review the diff again", ""), nil
	}

	mission, compileStatus, _ := missioncompile.Compile(root, run.MissionInput)
	if compileStatus == "blocked" || mission.MissionID != run.MissionID {
		emitWorkspaceGate(root, "block", "jacu_apply", run)
		return blockedApply("mission integrity check failed", ""), nil
	}
	run.Mission = mission
	for _, path := range snapshot.Files {
		if ScopesConflict(path, run.Mission.AllowedPaths, run.Mission.ForbiddenPaths) {
			emitWorkspaceGate(root, "block", "jacu_apply", run)
			return blockedApply("mission scope mismatch: "+path, ""), nil
		}
	}

	if run.Mission.Risk == "destructive" && !in.ApproveDestructive {
		emitWorkspaceGate(root, "require_approval", "jacu_apply", run)
		return blockedApply("destructive mission requires approve_destructive", ""), nil
	}
	policy, policyPresent, policyErr := LoadAutonomyPolicy(root)
	if policyErr != nil {
		emitWorkspaceGate(root, "block", "jacu_apply", run)
		return blockedApply("autonomy policy unreadable: "+policyErr.Error(), ""), nil
	}

	execution := verify.ExecuteCommands(ctx, root, run, run.Mission.VerificationCommands)
	if execution.Refusal != "" || execution.Data.Verdict == verify.VerdictBlocked {
		emitWorkspaceGate(root, "block", "jacu_apply", run)
	} else if execution.Data.Verdict == verify.VerdictPass {
		emitWorkspaceGate(root, "pass", "jacu_apply", run)
	} else {
		emitWorkspaceGate(root, "warn", "jacu_apply", run)
	}
	emitWorkspaceTelemetry(root, telemetry.EventVerify, "ok", execution.Data.Verdict, 1, "", "jacu_verify", run)
	verified := make([]string, 0, len(execution.Data.Commands))
	for _, outcome := range execution.Data.Commands {
		commandText := strings.Join(outcome.ArgV, " ")
		if outcome.Status != verify.StatusPassed {
			if policyPresent {
				if auditErr := persistAutonomyAudit(root, &run, fullDiff, execution.Data.Verdict, execution.Data.EvidenceDigest, "", 1, []string{verificationDiagnostic(outcome)}); auditErr != nil {
					return ApplyResult{}, auditErr
				}
			}
			return blockedApply("verification failed: "+commandText, verificationDiagnostic(outcome)), nil
		}
		verified = append(verified, commandText+" (exit 0)")
	}
	if execution.Refusal != "" {
		commandText := ""
		if len(run.Mission.VerificationCommands) > 0 {
			commandText = strings.Join(run.Mission.VerificationCommands[0], " ")
		}
		return blockedApply("verification failed: "+commandText, execution.Refusal), nil
	}
	verifiedSummary := "Verified: " + strings.Join(verified, "; ")
	if len(verified) == 0 {
		verifiedSummary = "Verified: no verification commands"
	}
	verifiedHead, err := git.RevParseHead(ctx, run.Worktree)
	if err != nil {
		return ApplyResult{}, err
	}
	if verifiedHead != run.BaseSHA {
		emitWorkspaceGate(root, "block", "jacu_apply", run)
		return blockedApply("verification commands modified the worktree; review the diff again", ""), nil
	}
	validatedTree, err := git.StageTree(ctx, run.Worktree)
	if err != nil {
		return ApplyResult{}, err
	}
	validatedDiff, err := git.DiffTree(ctx, run.Worktree, run.BaseSHA, validatedTree)
	if err != nil {
		return ApplyResult{}, err
	}
	if diffDigest(validatedDiff) != run.ReviewedDigest {
		if resetErr := git.ResetMixed(ctx, run.Worktree); resetErr != nil {
			return ApplyResult{}, resetErr
		}
		emitWorkspaceGate(root, "block", "jacu_apply", run)
		return blockedApply("verification commands modified the worktree; review the diff again", ""), nil
	}

	receiptRef := filepath.Join(".git", "jacu", "receipts", run.RunID+".json")
	var receipt ReviewReceipt
	if policyPresent {
		key, keyErr := LoadOrCreateReceiptKey(root)
		if keyErr != nil {
			return ApplyResult{}, keyErr
		}
		var receiptErr error
		receipt, receiptErr = ConsumeReviewReceipt(root, run.RunID, run.ReviewedDigest, key, time.Now().UTC())
		if receiptErr != nil {
			emitWorkspaceGate(root, "require_approval", "jacu_apply", run)
			if auditErr := persistAutonomyAudit(root, &run, fullDiff, execution.Data.Verdict, execution.Data.EvidenceDigest, receiptRef, 1, []string{receiptErr.Error()}); auditErr != nil {
				return ApplyResult{}, auditErr
			}
			return blockedApply("autonomy policy: valid review receipt required", receiptErr.Error()), nil
		}
		if receipt.Verdict != "approve" {
			emitWorkspaceGate(root, "require_approval", "jacu_apply", run)
			emitReviewDisagreement(root, run, "require_approval")
			decision := EvaluateAutoApplyPolicy(policy, execution.Data.Verdict, run.Mission.Risk, true, 1)
			if auditErr := persistAutonomyAudit(root, &run, fullDiff, execution.Data.Verdict, execution.Data.EvidenceDigest, receiptRef, 1, []string{decision.Reason}); auditErr != nil {
				return ApplyResult{}, auditErr
			}
			return blockedApply("autonomy policy: "+decision.Reason, "receipt verdict "+receipt.Verdict), nil
		}
		decision := EvaluateAutoApplyPolicy(policy, execution.Data.Verdict, run.Mission.Risk, true, 1)
		if !decision.Allowed {
			emitWorkspaceGate(root, "block", "jacu_apply", run)
			if auditErr := persistAutonomyAudit(root, &run, fullDiff, execution.Data.Verdict, execution.Data.EvidenceDigest, receiptRef, 1, []string{decision.Reason}); auditErr != nil {
				return ApplyResult{}, auditErr
			}
			return blockedApply("autonomy policy: "+decision.Reason, ""), nil
		}
		emitWorkspaceGate(root, "pass", "jacu_apply", run)
	}

	message := buildCommitMessage(run, hostName, verifiedSummary)
	commitSHA, err := git.CommitTree(ctx, run.Worktree, run.BaseSHA, validatedTree, message)
	if err != nil {
		return ApplyResult{}, err
	}
	if err := run.Transition(runstate.StatusApplied); err != nil {
		return ApplyResult{}, err
	}
	run.AppliedCommit = commitSHA
	if policyPresent {
		setAutonomyAudit(&run, fullDiff, execution.Data.Verdict, execution.Data.EvidenceDigest, receiptRef, 1, nil)
	}
	if err := runstate.SaveLocked(root, run); err != nil {
		if rollbackErr := git.UpdateHeadCAS(ctx, run.Worktree, run.BaseSHA, commitSHA); rollbackErr != nil {
			return ApplyResult{}, fmt.Errorf(
				"persist applied state after commit %s: %v; rollback HEAD to %s failed: %w; branch may still point to %s and requires manual reconciliation",
				commitSHA, err, run.BaseSHA, rollbackErr, commitSHA,
			)
		}
		return ApplyResult{}, fmt.Errorf(
			"persist applied state after commit %s: %w; HEAD rolled back to %s; retry Apply",
			commitSHA, err, run.BaseSHA,
		)
	}
	result := ApplyResult{
		Status:   "ok",
		Summary:  "Workspace applied.",
		Data:     ApplyData{CommitSHA: commitSHA, Branch: run.Branch},
		Warnings: []string{},
		NextActions: []string{
			"merge " + run.Branch + " into main when ready",
		},
	}
	emitWorkspaceApplyTelemetry(root, run, execution.Data.Verdict, int64(len(fullDiff)), len(snapshot.Files))
	if policyPresent {
		integration := integrateAutonomyWithIdentity(ctx, root, run.Branch, run.Mission.Objective, run.ReviewedDigest, execution.Data.EvidenceDigest, receiptRef, run.RunID, run.MissionID, run.Audit)
		appendIntegrationAudit(&run, integration)
		if auditErr := runstate.SaveLocked(root, run); auditErr != nil {
			return ApplyResult{}, auditErr
		}
		if integration.Escalated {
			result.Status = "escalated"
			result.Summary = "Workspace applied; GitHub integration escalated."
			result.Warnings = append(result.Warnings, integration.Warning)
			result.NextActions = []string{"resolve the integration escalation; the locked worktree is preserved"}
			return result, nil
		}
		result.NextActions = []string{"PR opened with auto-merge for " + run.Branch}
	}
	if err := git.WorktreeUnlock(ctx, root, run.Worktree); err != nil {
		result.Warnings = append(result.Warnings, "worktree cleanup failed: "+err.Error())
		return result, nil
	}
	if err := git.WorktreeRemove(ctx, root, run.Worktree); err != nil {
		result.Warnings = append(result.Warnings, "worktree cleanup failed: "+err.Error())
		return result, nil
	}
	return result, nil
}

func persistAutonomyAudit(root string, run *runstate.Run, fullDiff, verdict, evidenceDigest, receiptRef string, iterations int, warnings []string) error {
	setAutonomyAudit(run, fullDiff, verdict, evidenceDigest, receiptRef, iterations, warnings)
	return runstate.SaveLocked(root, *run)
}

func setAutonomyAudit(run *runstate.Run, fullDiff, verdict, evidenceDigest, receiptRef string, iterations int, warnings []string) {
	run.Audit = &runstate.AuditPackage{
		Objective:      run.Mission.Objective,
		DiffDigest:     diffDigest(fullDiff),
		Verdict:        verdict,
		EvidenceDigest: evidenceDigest,
		ReceiptRef:     receiptRef,
		Iterations:     iterations,
		Warnings:       append([]string{}, warnings...),
	}
}

func verificationDiagnostic(outcome verify.Result) string {
	if outcome.StderrTail != "" {
		return outcome.StderrTail
	}
	if outcome.Reason != "" {
		return outcome.Reason
	}
	return outcome.StdoutTail
}

func blockedApply(summary, stderr string) ApplyResult {
	return ApplyResult{
		Status:      "blocked",
		Summary:     summary,
		Data:        ApplyData{Stderr: stderr},
		Warnings:    []string{},
		NextActions: []string{},
	}
}
