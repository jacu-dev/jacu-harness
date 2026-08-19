package preflight

import (
	"sort"
	"time"

	"github.com/jacu-dev/jacu-harness/internal/telemetry"
)

func preflightTelemetryEvents(report Report) ([]telemetry.Event, error) {
	result, status := "pass", "done"
	classes := failureClasses(report)
	if len(classes) > 0 || report.Verdict != "pass" {
		result, status = "fail", "blocked"
	}
	events := make([]telemetry.Event, 0, 1+len(classes))
	checkClasses := classes
	if len(checkClasses) == 0 {
		checkClasses = []string{""}
	}
	for _, class := range checkClasses {
		check, err := telemetry.NewEvent(telemetry.EventInput{
			Timestamp: time.Now().UTC(), ProjectID: "prj_unknown", TraceID: telemetry.NewTraceID(),
			Module: "preflight", Stage: "preflight", Event: telemetry.EventPreflightCheck,
			Status: status, Result: result, FailureClass: class,
		})
		if err != nil {
			return nil, err
		}
		events = append(events, check)
	}
	for _, class := range classes {
		interruption, err := telemetry.NewEvent(telemetry.EventInput{
			Timestamp: time.Now().UTC(), ProjectID: "prj_unknown", TraceID: telemetry.NewTraceID(),
			Module: "mission", Stage: "interruption", Event: telemetry.EventMissionInterruption,
			Status: "blocked", Result: "fail", FailureClass: class,
		})
		if err != nil {
			return nil, err
		}
		events = append(events, interruption)
	}
	return events, nil
}

func failureClasses(report Report) []string {
	seen := map[string]struct{}{}
	classes := make([]string, 0, len(report.Findings))
	for _, finding := range report.Findings {
		if finding.Class == "" {
			continue
		}
		if _, ok := seen[finding.Class]; ok {
			continue
		}
		seen[finding.Class] = struct{}{}
		classes = append(classes, finding.Class)
	}
	sort.Strings(classes)
	return classes
}

func EmitTelemetry(root string, report Report) {
	events, err := preflightTelemetryEvents(report)
	if err != nil {
		return
	}
	projectID := telemetry.ProjectID(root)
	for _, event := range events {
		event.ProjectID = projectID
		telemetry.EmitBestEffort(event)
	}
}
