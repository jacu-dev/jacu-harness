package cleanexit

import (
	"time"

	"github.com/jacu-dev/jacu-harness/internal/telemetry"
)

func emitCleanExitTelemetry(root string, result RemovalReport) {
	if len(result.Findings) == 0 {
		telemetry.EmitBestEffortInput(telemetry.EventInput{
			Timestamp: time.Now().UTC(), ProjectID: telemetry.ProjectID(root), TraceID: telemetry.NewTraceID(),
			Module: "cleanexit", Stage: "close", Event: telemetry.EventCleanExitClose, Status: "done", Result: result.Verdict,
		})
		return
	}
	for _, finding := range result.Findings {
		telemetry.EmitBestEffortInput(telemetry.EventInput{
			Timestamp: time.Now().UTC(), ProjectID: telemetry.ProjectID(root), TraceID: telemetry.NewTraceID(),
			Module: "cleanexit", Stage: "close", Event: telemetry.EventCleanExitClose, Status: "blocked",
			Result: result.Verdict, FailureClass: finding.Class,
		})
	}
}

func EmitTelemetry(root string, result RemovalReport) {
	emitCleanExitTelemetry(root, result)
}
