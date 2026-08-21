package context

import (
	"time"

	"github.com/jacu-dev/jacu-harness/internal/telemetry"
)

func EmitPack(root string, pack Pack, coverage, required, included int) {
	event, err := telemetry.NewEvent(telemetry.EventInput{
		Timestamp: time.Now().UTC(), ProjectID: telemetry.ProjectID(root), TraceID: telemetry.NewTraceID(),
		Module: "context", Stage: "pack", Event: telemetry.EventContextPack, Status: "ok",
		CoverageBPS: coverage, ItemsRequired: required, ItemsIncluded: included,
		Measurement: telemetry.NoData,
	})
	if err != nil {
		return
	}
	telemetry.EmitBestEffort(event)
}

func EmitAnchor(root string, lost int) {
	status, verdict := "ok", "pass"
	if lost > 0 {
		status, verdict = "failed", "fail"
	}
	event, err := telemetry.NewEvent(telemetry.EventInput{
		Timestamp: time.Now().UTC(), ProjectID: telemetry.ProjectID(root), TraceID: telemetry.NewTraceID(),
		Module: "context", Stage: "anchor", Event: telemetry.EventContextAnchor, Status: status, Verdict: verdict,
		AnchorsLost: lost, Measurement: telemetry.NoData,
	})
	if err != nil {
		return
	}
	telemetry.EmitBestEffort(event)
}

func EmitHandoff(root string, included int) {
	event, err := telemetry.NewEvent(telemetry.EventInput{
		Timestamp: time.Now().UTC(), ProjectID: telemetry.ProjectID(root), TraceID: telemetry.NewTraceID(),
		Module: "context", Stage: "handoff", Event: telemetry.EventContextHandoff, Status: "ok",
		ItemsIncluded: included, Measurement: telemetry.NoData,
	})
	if err != nil {
		return
	}
	telemetry.EmitBestEffort(event)
}
