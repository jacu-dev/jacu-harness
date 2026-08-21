package ledger

import (
	"time"

	"github.com/jacu-dev/jacu-harness/internal/telemetry"
)

func Emit(root string, decision Decision) {
	status := "ok"
	switch decision.Verdict {
	case VerdictRefuse:
		status = "blocked"
	case VerdictDegrade:
		status = "partial"
	}
	event, err := telemetry.NewEvent(telemetry.EventInput{
		Timestamp: time.Now().UTC(), ProjectID: telemetry.ProjectID(root), TraceID: telemetry.NewTraceID(),
		Module: "ledger", Stage: "decision", Event: telemetry.EventLedgerDecision, Status: status,
		Verdict: decision.Verdict, Reason: decision.Reason,
		BudgetBytes: decision.BudgetBytes, RequestedBytes: decision.RequestedBytes, RemainingBytes: decision.RemainingBytes,
		RequiredOverflow: decision.RequiredOverflow, CoverageBPS: decision.CoverageBPS,
		ItemsRequired: decision.ItemsRequired, ItemsIncluded: decision.ItemsIncluded,
		Measurement: telemetry.NoData,
	})
	if err != nil {
		return
	}
	telemetry.EmitBestEffort(event)
}
