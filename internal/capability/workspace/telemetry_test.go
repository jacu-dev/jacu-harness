package workspace

import (
	"testing"
	"time"

	"github.com/jacu-dev/jacu-harness/internal/runstate"
	"github.com/jacu-dev/jacu-harness/internal/telemetry"
)

func TestGateDecisionTelemetryRecordsWorkspaceDecision(t *testing.T) {
	base := t.TempDir()
	t.Setenv("JACU_HOME", base)
	run := runstate.Run{RunID: "run_0123456789abcdef", MissionID: "msn_0123456789abcdef"}
	emitWorkspaceTelemetry(t.TempDir(), telemetry.EventGateDecision, "ok", "pass", 1, "", "jacu_diff", run)

	events, err := telemetry.NewStoreAt(base).ReadSince(time.Unix(0, 0).UTC())
	if err != nil {
		t.Fatalf("read gate telemetry: %v", err)
	}
	if len(events) != 1 || events[0].Event != telemetry.EventGateDecision || events[0].Module != "workspace" || events[0].Stage != "gate" || events[0].Verdict != "pass" {
		t.Fatalf("gate telemetry = %+v; want workspace gate decision", events)
	}
}

func TestGateDecisionVerdictOrder(t *testing.T) {
	got := []string{"pass", "warn", "require_approval", "block"}
	for index, verdict := range got {
		if telemetry.GateVerdictRank(verdict) != index {
			t.Fatalf("rank(%q) = %d; want %d", verdict, telemetry.GateVerdictRank(verdict), index)
		}
	}
}

func TestWorkspaceTelemetryCarriesTypedApplyAndReviewDetail(t *testing.T) {
	base := t.TempDir()
	t.Setenv("JACU_HOME", base)
	run := runstate.Run{RunID: "run_0123456789abcdef", MissionID: "msn_0123456789abcdef"}
	telemetry.EmitBestEffortInput(telemetry.EventInput{
		Timestamp: time.Now().UTC(), ProjectID: telemetry.ProjectID(t.TempDir()), TraceID: telemetry.NewTraceID(),
		RunID: run.RunID, MissionID: run.MissionID, Module: "workspace", Stage: "apply", Event: telemetry.EventApply,
		Status: "applied", Auto: true, Intervention: false, DiffBytes: 42, FilesChanged: 2,
	})
	telemetry.EmitBestEffortInput(telemetry.EventInput{
		Timestamp: time.Now().UTC(), ProjectID: telemetry.ProjectID(t.TempDir()), TraceID: telemetry.NewTraceID(),
		RunID: run.RunID, MissionID: run.MissionID, Module: "workspace", Stage: "review", Event: telemetry.EventReviewDisagreement,
		Status: "blocked", Resolved: "require_approval",
	})
	events, err := telemetry.NewStoreAt(base).ReadSince(time.Unix(0, 0).UTC())
	if err != nil {
		t.Fatalf("read typed detail telemetry: %v", err)
	}
	if len(events) != 2 || !events[0].Auto || events[0].DiffBytes != 42 || events[0].FilesChanged != 2 || events[1].Resolved != "require_approval" {
		t.Fatalf("typed detail events = %+v", events)
	}
}
