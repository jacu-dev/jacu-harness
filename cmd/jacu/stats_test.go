package main

import (
	"strings"
	"testing"
	"time"

	"github.com/jacu-dev/jacu-harness/internal/telemetry"
)

func TestStatsFullPrintsModuleSectionsAndMeasurements(t *testing.T) {
	if _, _, err := parseStatsArgs([]string{"--full"}); err != nil {
		t.Fatalf("parse stats full: %v", err)
	}
	now := time.Now().UTC()
	stats, err := telemetry.ComputeStats([]telemetry.Event{{
		SchemaVersion: telemetry.CurrentSchemaVersion, Level: telemetry.LevelUser,
		ProjectID: "prj_0123456789abcdef", TraceID: "tr_0123456789abcdef", Timestamp: now,
		Module: "runtime", Stage: "tool_call", Event: telemetry.EventToolCall, Status: "ok",
		InputBytes: 10, OutputBytes: 20, Measurement: "exact_bytes",
	}}, now.Add(-time.Hour), now.Add(time.Hour), nil)
	if err != nil {
		t.Fatalf("compute stats: %v", err)
	}
	output := telemetry.FormatFullStats(stats)
	for _, section := range []string{"MISSION", "VERIFY", "GUARDRAILS", "WORKSPACE", "RUNTIME"} {
		if !strings.Contains(output, section) {
			t.Fatalf("full stats missing %s: %q", section, output)
		}
	}
	if !strings.Contains(output, "measurement=exact_bytes") || !strings.Contains(output, "no-data") {
		t.Fatalf("full stats lacks measurement/no-data: %q", output)
	}
}
