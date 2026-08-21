package modelcontrol

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/jacu-dev/jacu-harness/internal/telemetry"
)

func TestCostTraceEventHasNoInventedDollarPrice(t *testing.T) {
	trace := CostTrace{ProfileID: "cheap", BillingMode: BillingSubscription, CostSource: CostSourceNone, InputTokens: 3, OutputTokens: 1, DurationMs: 12}
	if err := trace.Validate(); err != nil {
		t.Fatal(err)
	}
	event, err := telemetry.NewEvent(telemetry.EventInput{
		Timestamp: time.Now().UTC(), ProjectID: "prj_0123456789abcdef", TraceID: telemetry.NewTraceID(),
		Module: "modelcontrol", Stage: "cost", Event: telemetry.EventCostTrace, Tool: "cheap",
		Status: "ok", Measurement: telemetry.NoData, Duration: 12 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	raw := strings.ToLower(string(encoded))
	for _, forbidden := range []string{"usd", "$", "dollar", "api_dollars"} {
		if strings.Contains(raw, forbidden) {
			t.Fatalf("invented price %q in %s", forbidden, encoded)
		}
	}
}
