package cleanexit

import (
	"testing"
	"time"

	"github.com/jacu-dev/jacu-harness/internal/telemetry"
)

func TestCleanExitTelemetryCarriesTypedResult(t *testing.T) {
	event, err := telemetry.NewEvent(telemetry.EventInput{
		Timestamp: time.Now().UTC(), ProjectID: "prj_0123456789abcdef", TraceID: "tr_0123456789abcdef",
		Module: "cleanexit", Stage: "close", Event: telemetry.EventCleanExitClose, Status: "done",
		Result: "pass", FailureClass: "main_mismatch",
	})
	if err != nil {
		t.Fatalf("construct cleanexit telemetry: %v", err)
	}
	if event.Result != "pass" || event.FailureClass != "main_mismatch" {
		t.Fatalf("cleanexit telemetry = %+v", event)
	}
}
