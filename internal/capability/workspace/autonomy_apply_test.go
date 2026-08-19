package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jacu-dev/jacu-harness/internal/runner"
	"github.com/jacu-dev/jacu-harness/internal/runstate"
)

func TestPolicySatisfiedApplyPersistsAuditAndIntegrates(t *testing.T) {
	repo, opened := reviewedApplyFixture(t, "write", [][]string{{"git", "status", "--short"}})
	policyPath := filepath.Join(repo, ".jacu", "autonomy-policy.json")
	if err := os.MkdirAll(filepath.Dir(policyPath), 0o700); err != nil {
		t.Fatal(err)
	}
	policy := []byte(`{"policy":{"auto_apply":{"require":["verify_pass","cross_review"],"risk_max":"write","max_iterations":3,"on_violation":"escalate"}}}`)
	if err := os.WriteFile(policyPath, policy, 0o600); err != nil {
		t.Fatal(err)
	}
	key := []byte("local-test-key-012345678901234567890")
	if err := os.WriteFile(filepath.Join(repo, ".git", "jacu", "receipt.key"), key, 0o600); err != nil {
		t.Fatal(err)
	}
	run, err := loadRunForTest(repo, opened.RunID)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := SignReviewReceipt(ReviewReceipt{RunID: run.RunID, DiffDigest: run.ReviewedDigest, Verdict: "approve", CreatedAt: time.Now().UTC()}, key)
	if err != nil {
		t.Fatal(err)
	}
	if _, writeReceiptErr := WriteReviewReceipt(repo, receipt); writeReceiptErr != nil {
		t.Fatal(writeReceiptErr)
	}
	oldRunner := autonomyRunCommand
	defer func() { autonomyRunCommand = oldRunner }()
	autonomyRunCommand = func(context.Context, string, ...string) error { return nil }
	oldWatcher := autonomyWatchCheckEvidence
	defer func() { autonomyWatchCheckEvidence = oldWatcher }()
	autonomyWatchCheckEvidence = func(context.Context, runner.CheckEvidenceRequest) (runner.CheckEvidence, error) {
		return runner.CheckEvidence{Status: runner.CheckStatusPassed, Checks: []runner.CheckRun{{Name: "verify", Bucket: "pass", State: "SUCCESS"}}}, nil
	}
	result, err := Apply(context.Background(), repo, ApplyInput{RunID: opened.RunID}, "Claude Code")
	if err != nil || result.Status != "ok" {
		t.Fatalf("Apply = %#v err %v", result, err)
	}
	state, err := loadRunForTest(repo, opened.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if state.Audit == nil || state.Audit.Verdict != "pass" || state.Audit.EvidenceDigest == "" || state.Audit.ReceiptRef == "" {
		t.Fatalf("audit = %#v", state.Audit)
	}
	if _, err := os.Stat(opened.WorktreePath); !os.IsNotExist(err) {
		t.Fatalf("worktree remains after integrated apply: %v", err)
	}
}

func loadRunForTest(root, runID string) (runstate.Run, error) {
	return runstate.Load(root, runID)
}

func TestAutonomyAuditShapeHasNoSessionClaim(t *testing.T) {
	encoded, err := json.Marshal(runstate.AuditPackage{})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "session") {
		t.Fatalf("audit claims session separation: %s", encoded)
	}
}

func TestAutonomyIntegrationCommandsHaveNoProductionTagMutation(t *testing.T) {
	commands := autonomyIntegrationCommands("jacu/run-0123456789abcdef", "autonomy review")
	if got := commands[1]; !reflect.DeepEqual(got, []string{"gh", "pr", "merge", "jacu/run-0123456789abcdef", "--auto", "--squash"}) {
		t.Fatalf("integration merge command = %q, want squash policy", got)
	}
	for _, command := range commands {
		for _, arg := range command {
			if strings.Contains(arg, "tag") || strings.HasPrefix(arg, "v") {
				t.Fatalf("integration command contains production tag operation: %q", command)
			}
		}
	}
}

func TestAutonomyIntegrationEscalatesOnMergeConflict(t *testing.T) {
	oldRunner := autonomyRunCommand
	defer func() { autonomyRunCommand = oldRunner }()
	var calls [][]string
	autonomyRunCommand = func(_ context.Context, name string, args ...string) error {
		calls = append(calls, append([]string{name}, args...))
		if name == "gh" && len(args) > 1 && args[0] == "pr" && args[1] == "merge" {
			return errors.New("merge conflict")
		}
		return nil
	}
	result := integrateAutonomy(context.Background(), "/repo", "jacu/run-0123456789abcdef", "objective", "sha256:diff", "sha256:evidence", "receipt.json")
	if !result.Escalated || !result.PreserveWorktree || len(calls) != 3 {
		t.Fatalf("integration result = %#v calls = %#v", result, calls)
	}
}

func TestAutonomyIntegrationPreservesWorktreeForPendingChecks(t *testing.T) {
	oldRunner := autonomyRunCommand
	oldWatcher := autonomyWatchCheckEvidence
	defer func() {
		autonomyRunCommand = oldRunner
		autonomyWatchCheckEvidence = oldWatcher
	}()
	autonomyRunCommand = func(context.Context, string, ...string) error { return nil }
	autonomyWatchCheckEvidence = func(context.Context, runner.CheckEvidenceRequest) (runner.CheckEvidence, error) {
		return runner.CheckEvidence{Status: runner.CheckStatusPending}, nil
	}
	result := integrateAutonomy(context.Background(), "/repo", "jacu/run-0123456789abcdef", "objective", "sha256:diff", "sha256:evidence", "receipt.json")
	if !result.Escalated || !result.PreserveWorktree || result.Evidence == nil {
		t.Fatalf("integration result = %#v; pending checks must preserve worktree", result)
	}
}

func TestAutonomyIntegrationCompilesScopedMissionForRealFailure(t *testing.T) {
	oldRunner := autonomyRunCommand
	oldWatcher := autonomyWatchCheckEvidence
	defer func() {
		autonomyRunCommand = oldRunner
		autonomyWatchCheckEvidence = oldWatcher
	}()
	autonomyRunCommand = func(context.Context, string, ...string) error { return nil }
	autonomyWatchCheckEvidence = func(context.Context, runner.CheckEvidenceRequest) (runner.CheckEvidence, error) {
		return runner.CheckEvidence{
			Status: runner.CheckStatusFailed,
			Failures: []runner.CheckFailureEvidence{{
				Check:          runner.CheckRun{Name: "lint", State: "FAILURE", Workflow: "CI"},
				EvidenceDigest: "sha256:failure",
				Annotations:    []runner.CheckAnnotation{{Path: "internal/runner/ci.go"}},
			}},
		}, nil
	}
	result := integrateAutonomy(context.Background(), "/repo", "jacu/run-0123456789abcdef", "objective", "sha256:diff", "sha256:evidence", "receipt.json")
	if !result.Escalated || !result.PreserveWorktree || len(result.Remediations) != 1 || result.Remediations[0].Mission == nil {
		t.Fatalf("integration result = %#v; expected a compiled remediation mission", result)
	}
}
