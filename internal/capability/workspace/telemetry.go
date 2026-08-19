package workspace

import (
	"time"

	"github.com/jacu-dev/jacu-harness/internal/runstate"
	"github.com/jacu-dev/jacu-harness/internal/telemetry"
)

func emitWorkspaceTelemetry(root, eventName, status, verdict string, iteration int, exitReason, tool string, run runstate.Run) {
	stage := eventName
	if eventName == telemetry.EventGateDecision {
		stage = "gate"
	}
	telemetry.EmitBestEffortInput(telemetry.EventInput{
		Timestamp: time.Now().UTC(), ProjectID: telemetry.ProjectID(root), TraceID: telemetry.NewTraceID(),
		RunID: run.RunID, MissionID: run.MissionID, Module: "workspace", Stage: stage, Event: eventName, Tool: tool,
		Status: status, Verdict: verdict, Iteration: iteration, ExitReason: exitReason,
	})
}

func emitWorkspaceGate(root, verdict, tool string, run runstate.Run) {
	telemetry.EmitBestEffortInput(telemetry.EventInput{
		Timestamp: time.Now().UTC(), ProjectID: telemetry.ProjectID(root), TraceID: telemetry.NewTraceID(),
		RunID: run.RunID, MissionID: run.MissionID, Module: "workspace", Stage: "gate",
		Event: telemetry.EventGateDecision, Tool: tool, Status: "ok", Verdict: verdict,
	})
}

func emitWorkspaceApplyTelemetry(root string, run runstate.Run, verdict string, diffBytes int64, filesChanged int) {
	telemetry.EmitBestEffortInput(telemetry.EventInput{
		Timestamp: time.Now().UTC(), ProjectID: telemetry.ProjectID(root), TraceID: telemetry.NewTraceID(),
		RunID: run.RunID, MissionID: run.MissionID, Module: "workspace", Stage: "apply",
		Event: telemetry.EventApply, Tool: "jacu_apply", Status: "applied", Verdict: verdict,
		Auto: true, Intervention: false, DiffBytes: diffBytes, FilesChanged: filesChanged,
	})
}

func emitReviewDisagreement(root string, run runstate.Run, resolved string) {
	telemetry.EmitBestEffortInput(telemetry.EventInput{
		Timestamp: time.Now().UTC(), ProjectID: telemetry.ProjectID(root), TraceID: telemetry.NewTraceID(),
		RunID: run.RunID, MissionID: run.MissionID, Module: "workspace", Stage: "review",
		Event: telemetry.EventReviewDisagreement, Tool: "jacu_apply", Status: "blocked", Resolved: resolved,
	})
}
