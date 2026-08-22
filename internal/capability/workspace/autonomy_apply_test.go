package workspace

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jacu-dev/jacu-harness/internal/runstate"
)

func TestPolicySatisfiedApplyStaysLocalAndNeverInvokesRemote(t *testing.T) {
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
	runGit(t, repo, "checkout", "-b", "sdd/023")
	result, err := Apply(context.Background(), repo, ApplyInput{RunID: opened.RunID}, "Claude Code")
	if err != nil || result.Status != "ok" {
		t.Fatalf("Apply = %#v err %v", result, err)
	}
	wantNext := []string{"jacu deliver --base main"}
	if !reflect.DeepEqual(result.NextActions, wantNext) {
		t.Fatalf("NextActions = %#v, want local merge only", result.NextActions)
	}
	if _, err := os.Stat(opened.WorktreePath); !os.IsNotExist(err) {
		t.Fatalf("worktree remains after local apply: %v", err)
	}
}

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
	runGit(t, repo, "checkout", "-b", "sdd/023")
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
