package workspace

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jacu-dev/jacu-harness/internal/telemetry"
)

func TestAutonomyEscalationAndAutoApplyEmitClosedEvents(t *testing.T) {
	base := t.TempDir()
	t.Setenv("JACU_HOME", base)
	repo := newWorkspaceGitRepo(t)
	checkoutBranch(t, repo, "sdd/023")
	createRunRewrite(t, repo, "jacu/run-conflict", "README.md", "run\n")
	rewriteFile(t, repo, "README.md", "integration\n", "integration rewrite")
	_ = integrateAutonomyWithIdentity(context.Background(), repo, "jacu/run-conflict", "objective", "sha256:diff", "sha256:evidence", "receipt.json", "run_0123456789abcdef", "msn_0123456789abcdef", nil)

	clean := newWorkspaceGitRepo(t)
	checkoutBranch(t, clean, "sdd/023")
	createRunRewrite(t, clean, "jacu/run-ok", "ok.txt", "ok\n")
	runWorkspaceGit(t, clean, "checkout", "sdd/023")
	_ = integrateAutonomyWithIdentity(context.Background(), clean, "jacu/run-ok", "objective", "sha256:diff", "sha256:evidence", "receipt.json", "run_0123456789abcdef", "msn_0123456789abcdef", nil)

	events, err := telemetry.NewStoreAt(base).ReadSince(time.Unix(0, 0).UTC())
	if err != nil {
		t.Fatalf("read telemetry: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("autonomy telemetry events = %+v; want escalation and apply", events)
	}
	seen := map[string]bool{}
	for _, event := range events {
		seen[event.Event] = true
		if event.Tool != "autonomy" || event.RunID != "run_0123456789abcdef" || event.MissionID != "msn_0123456789abcdef" {
			t.Fatalf("autonomy identity = %+v", event)
		}
		if event.Event == telemetry.EventEscalation && event.Status != "escalated" {
			t.Fatalf("escalation event = %+v", event)
		}
		if event.Event == telemetry.EventApply && (event.Status != "applied" || event.Verdict != "pass") {
			t.Fatalf("apply event = %+v", event)
		}
	}
	if !seen[telemetry.EventEscalation] || !seen[telemetry.EventApply] {
		t.Fatalf("autonomy event kinds = %v", seen)
	}
	if _, err := os.Stat(base); err != nil {
		t.Fatalf("telemetry fixture disappeared: %v", err)
	}
}
