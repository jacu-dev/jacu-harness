package context

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/jacu-dev/jacu-harness/internal/telemetry"
)

func TestAdmissionEventsHaveNoCounterfactualField(t *testing.T) {
	now := time.Date(2026, 8, 21, 0, 0, 0, 0, time.UTC)
	event, err := telemetry.NewEvent(telemetry.EventInput{
		Timestamp: now, ProjectID: "prj_0123456789abcdef", TraceID: "tr_0123456789abcdef",
		Module: "context", Stage: "pack", Event: telemetry.EventContextPack, Status: "ok",
		CoverageBPS: 5000, ItemsRequired: 2, ItemsIncluded: 1, Measurement: telemetry.NoData,
	})
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	raw := string(encoded)
	for _, forbidden := range []string{"would_have", "saved_bytes", "counterfactual", "tokens_saved"} {
		if strings.Contains(raw, forbidden) {
			t.Fatalf("counterfactual field %s in %s", forbidden, raw)
		}
	}
	anchor, err := telemetry.NewEvent(telemetry.EventInput{
		Timestamp: now, ProjectID: "prj_0123456789abcdef", TraceID: "tr_0123456789abcdef",
		Module: "context", Stage: "anchor", Event: telemetry.EventContextAnchor, Status: "failed", Verdict: "fail",
		AnchorsLost: 1, Measurement: telemetry.NoData,
	})
	if err != nil {
		t.Fatal(err)
	}
	handoff, err := telemetry.NewEvent(telemetry.EventInput{
		Timestamp: now, ProjectID: "prj_0123456789abcdef", TraceID: "tr_0123456789abcdef",
		Module: "context", Stage: "handoff", Event: telemetry.EventContextHandoff, Status: "ok",
		ItemsIncluded: 1, Measurement: telemetry.NoData,
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = anchor
	_ = handoff
}
