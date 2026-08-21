package clarity

import (
	"time"

	"github.com/jacu-dev/jacu-harness/internal/telemetry"
)

func ProbeEvent(projectID string, report Report) (telemetry.Event, error) {
	status := "done"
	if report.Verdict != "pass" {
		status = "failed"
	}
	return telemetry.NewEvent(telemetry.EventInput{
		Timestamp:       time.Now().UTC(),
		ProjectID:       projectID,
		TraceID:         telemetry.NewTraceID(),
		Module:          "clarity",
		Stage:           "probe",
		Event:           telemetry.EventClarityProbe,
		Status:          status,
		Verdict:         report.Verdict,
		Round:           report.Round,
		Divergences:     report.Divergences,
		DivergenceField: report.DivergenceField,
		VarianceRuns:    report.VarianceRuns,
		SpecBytes:       report.SpecBytes,
		SpecBytesDelta:  report.SpecBytesDelta,
		Measurement:     telemetry.NoData,
	})
}

func Emit(root string, report Report) {
	event, err := ProbeEvent(telemetry.ProjectID(root), report)
	if err != nil {
		return
	}
	telemetry.EmitBestEffort(event)
}
