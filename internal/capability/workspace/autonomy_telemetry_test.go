package workspace

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jacu-dev/jacu-harness/internal/runner"
	"github.com/jacu-dev/jacu-harness/internal/telemetry"
)

func TestAutonomyEscalationAndAutoApplyEmitClosedEvents(t *testing.T) {
	base := t.TempDir()
	t.Setenv("JACU_HOME", base)
	root := t.TempDir()
	oldRunner := autonomyRunCommand
	oldWatcher := autonomyWatchCheckEvidence
	defer func() {
		autonomyRunCommand = oldRunner
		autonomyWatchCheckEvidence = oldWatcher
	}()

	autonomyRunCommand = func(_ context.Context, name string, args ...string) error {
		if name == "gh" && len(args) > 1 && args[0] == "pr" && args[1] == "merge" {
			return errors.New("merge conflict")
		}
		return nil
	}
	_ = integrateAutonomyWithIdentity(context.Background(), root, "jacu/run-0123456789abcdef", "objective", "sha256:diff", "sha256:evidence", "receipt.json", "run_0123456789abcdef", "msn_0123456789abcdef", nil)

	autonomyRunCommand = func(context.Context, string, ...string) error { return nil }
	autonomyWatchCheckEvidence = func(context.Context, runner.CheckEvidenceRequest) (runner.CheckEvidence, error) {
		return runner.CheckEvidence{Status: runner.CheckStatusPassed}, nil
	}
	_ = integrateAutonomyWithIdentity(context.Background(), root, "jacu/run-0123456789abcdef", "objective", "sha256:diff", "sha256:evidence", "receipt.json", "run_0123456789abcdef", "msn_0123456789abcdef", nil)

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
