package workspace

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/jacu-dev/jacu-harness/internal/gitx"
	"github.com/jacu-dev/jacu-harness/internal/runner"
	"github.com/jacu-dev/jacu-harness/internal/runstate"
	"github.com/jacu-dev/jacu-harness/internal/telemetry"
)

type autonomyIntegrationResult struct {
	Escalated        bool
	PreserveWorktree bool
	Warning          string
	Evidence         *runner.CheckEvidence
	Remediations     []RemediationPlan
}

// autonomyRunCommand is a narrow seam for deterministic integration tests. The
// production implementation uses direct argv execution; no shell and no tag
// command are involved.
var autonomyRunCommand = func(ctx context.Context, name string, args ...string) error {
	var stderr bytes.Buffer
	// #nosec G204 -- command and argv are fixed integration operations; no shell is used.
	command := exec.CommandContext(ctx, name, args...)
	command.Env = gitx.CleanGitEnv(os.Environ())
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		if stderr.Len() > 0 {
			return fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
		}
		return err
	}
	return nil
}

var autonomyWatchCheckEvidence = runner.WatchCheckEvidence

func autonomyIntegrationCommands(branch, title string) [][]string {
	return [][]string{
		{"gh", "pr", "create", "--base", "main", "--head", branch, "--title", title, "--body-file", "-"},
		{"gh", "pr", "merge", branch, "--auto", "--squash"},
	}
}

func integrateAutonomy(ctx context.Context, root, branch, objective, diffDigest, evidenceDigest, receiptRef string) autonomyIntegrationResult {
	return integrateAutonomyWithHistory(ctx, root, branch, objective, diffDigest, evidenceDigest, receiptRef, nil)
}

func integrateAutonomyWithHistory(ctx context.Context, root, branch, objective, diffDigest, evidenceDigest, receiptRef string, audit *runstate.AuditPackage) autonomyIntegrationResult {
	return integrateAutonomyWithIdentity(ctx, root, branch, objective, diffDigest, evidenceDigest, receiptRef, "", "", audit)
}

func integrateAutonomyWithIdentity(ctx context.Context, root, branch, objective, diffDigest, evidenceDigest, receiptRef, runID, missionID string, audit *runstate.AuditPackage) autonomyIntegrationResult {
	if err := autonomyRunCommand(ctx, "git", "-C", root, "push", "--set-upstream", "origin", branch); err != nil {
		emitAutonomyTelemetry(root, runID, missionID, telemetry.EventEscalation, "escalated", "", 0, "escalated")
		return autonomyIntegrationResult{Escalated: true, PreserveWorktree: true, Warning: "branch push failed; escalation required"}
	}
	title := "JACU autonomy: " + compactTitle(objective)
	body := strings.Join([]string{
		"Automated apply under the approved autonomy policy.",
		"Objective: " + objective,
		"Diff digest: " + diffDigest,
		"Evidence digest: " + evidenceDigest,
		"Receipt: " + receiptRef,
	}, "\n")
	commands := autonomyIntegrationCommands(branch, title)
	if err := autonomyRunCommandWithInput(ctx, commands[0], body); err != nil {
		emitAutonomyTelemetry(root, runID, missionID, telemetry.EventEscalation, "escalated", "", 0, "escalated")
		return autonomyIntegrationResult{Escalated: true, PreserveWorktree: true, Warning: "pull request creation failed; escalation required"}
	}
	if err := autonomyRunCommand(ctx, commands[1][0], commands[1][1:]...); err != nil {
		emitAutonomyTelemetry(root, runID, missionID, telemetry.EventEscalation, "escalated", "", 0, "escalated")
		return autonomyIntegrationResult{Escalated: true, PreserveWorktree: true, Warning: "pull request merge conflict or failure; escalation required"}
	}
	checks, err := autonomyWatchCheckEvidence(ctx, runner.CheckEvidenceRequest{
		PullRequest: branch,
		Directory:   root,
		Timeout:     4 * time.Minute,
	})
	if err != nil {
		emitAutonomyTelemetry(root, runID, missionID, telemetry.EventEscalation, "escalated", "", 0, "escalated")
		return autonomyIntegrationResult{Escalated: true, PreserveWorktree: true, Evidence: &checks, Warning: "CI evidence collection failed; escalation required"}
	}
	result := autonomyIntegrationResult{Evidence: &checks, Remediations: []RemediationPlan{}}
	if checks.Status == runner.CheckStatusPassed {
		emitAutonomyTelemetry(root, runID, missionID, telemetry.EventApply, "applied", "pass", 0, "completed")
		return result
	}
	if checks.Status == runner.CheckStatusPending || checks.Status == runner.CheckStatusTimeout {
		return autonomyEscalation(root, runID, missionID, result, "CI did not reach a terminal green state; worktree preserved")
	}
	for _, failure := range checks.Failures {
		priorFailures := previousCheckFailures(audit, failure.Check.Name)
		plan := BuildRemediationPlan(failure, priorFailures)
		result.Remediations = append(result.Remediations, plan)
		if plan.Decision.Action == RemediationEscalate {
			emitAutonomyTelemetry(root, runID, missionID, telemetry.EventEscalation, "escalated", "", priorFailures+1, "escalated")
		} else {
			emitAutonomyTelemetry(root, runID, missionID, telemetry.EventRemediation, "ok", "", priorFailures+1, "")
		}
		switch plan.Decision.Action {
		case RemediationRerun:
			if failure.RunID == "" {
				return autonomyEscalation(root, runID, missionID, result, "flaky check has no rerunnable workflow; escalation required")
			}
			if err := autonomyRunCommand(ctx, "gh", "run", "rerun", failure.RunID); err != nil {
				return autonomyEscalation(root, runID, missionID, result, "flaky check rerun failed; escalation required")
			}
			return autonomyEscalation(root, runID, missionID, result, "flaky check rerun requested; worktree preserved")
		case RemediationMission:
			return autonomyEscalation(root, runID, missionID, result, "remediation mission compiled; worktree preserved")
		default:
			return autonomyEscalation(root, runID, missionID, result, plan.Decision.Reason)
		}
	}
	return autonomyEscalation(root, runID, missionID, result, "failed CI has no actionable evidence; escalation required")
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

func previousCheckFailures(audit *runstate.AuditPackage, check string) int {
	if audit == nil {
		return 0
	}
	failures := 0
	for _, evidence := range audit.CheckEvidence {
		if evidence.Check == check && evidence.Status == runner.CheckStatusFailed {
			failures++
		}
	}
	return failures
}

func appendIntegrationAudit(run *runstate.Run, integration autonomyIntegrationResult) {
	if run.Audit == nil || integration.Evidence == nil {
		return
	}
	evidence := integration.Evidence
	for _, failure := range evidence.Failures {
		digest := failure.EvidenceDigest
		if digest == "" {
			digest = evidence.Digest
		}
		run.Audit.CheckEvidence = append(run.Audit.CheckEvidence, runstate.CheckEvidenceAudit{
			Check:          failure.Check.Name,
			Status:         evidence.Status,
			Repository:     failure.Repository,
			RunID:          failure.RunID,
			JobID:          failure.JobID,
			LogDigest:      failure.LogDigest,
			LogTruncated:   failure.LogTruncated,
			AnnotationPath: remediationPaths(failure.Annotations),
			EvidenceDigest: digest,
		})
	}
	for _, plan := range integration.Remediations {
		class := CheckOther
		if plan.Mission != nil {
			class = plan.Mission.Class
		}
		audit := runstate.RemediationAudit{
			Action:           string(plan.Decision.Action),
			Check:            plan.Decision.Check,
			Class:            string(class),
			BudgetIterations: plan.Decision.Budget,
			Reason:           plan.Decision.Reason,
		}
		if plan.Mission != nil {
			audit.Objective = plan.Mission.Objective
			audit.AllowedPaths = append([]string{}, plan.Mission.AllowedPaths...)
		}
		run.Audit.Remediations = append(run.Audit.Remediations, audit)
	}
}

func autonomyRunCommandWithInput(ctx context.Context, command []string, input string) error {
	// The injectable runner intentionally has no stdin parameter. Production
	// passes the audit body as a bounded --body value while tests still observe
	// exactly the direct argv operation.
	args := append([]string{}, command...)
	for index, arg := range args {
		if arg == "--body-file" && index+1 < len(args) && args[index+1] == "-" {
			args = append(args[:index], append([]string{"--body", input}, args[index+2:]...)...)
			break
		}
	}
	return autonomyRunCommand(ctx, args[0], args[1:]...)
}

func compactTitle(objective string) string {
	objective = strings.Join(strings.Fields(objective), " ")
	if len(objective) > 100 {
		return objective[:100]
	}
	return objective
}
