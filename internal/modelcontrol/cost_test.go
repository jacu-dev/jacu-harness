package modelcontrol

import (
	"math"
	"strings"
	"testing"
)

func TestCostTraceRejectsSubscriptionMoneyAndRawContent(t *testing.T) {
	trace := CostTrace{ProfileID: "claude", BillingMode: BillingSubscription, CostSource: CostSourceAPIDollars, CostUnits: 1}
	if err := trace.Validate(); err == nil || !strings.Contains(err.Error(), "subscription") {
		t.Fatalf("trace error = %v; want subscription block", err)
	}
	if strings.Contains(trace.String(), "prompt") || strings.Contains(trace.String(), "secret") {
		t.Fatalf("trace string exposes raw content: %q", trace.String())
	}
}

func TestMissionLedgerSaturatesAndAdmitsAgainstGlobalCeiling(t *testing.T) {
	ledger := NewMissionLedger(1)
	if !ledger.CanAdmit(1_000_000) || ledger.CanAdmit(1_000_001) {
		t.Fatal("ledger admission boundary is wrong")
	}
	ledger.Absorb(math.MaxUint64, math.MaxUint64)
	ledger.Absorb(1, 1)
	if ledger.Spent() != math.MaxUint64 || ledger.Saved() != math.MaxUint64 || ledger.Remaining() != 0 {
		t.Fatalf("ledger = spent %d saved %d remaining %d; want saturated", ledger.Spent(), ledger.Saved(), ledger.Remaining())
	}
}
