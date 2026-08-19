package modelcontrol

import (
	"errors"
	"fmt"
	"math"
)

type BillingMode string

const (
	BillingSubscription BillingMode = "subscription"
	BillingLocal        BillingMode = "local"
)

type CostSource string

const (
	CostSourceNone       CostSource = "none"
	CostSourceLocalUnit  CostSource = "local_units"
	CostSourceAPIDollars CostSource = "api_dollars"
)

// CostTrace intentionally has no prompt, output, credential, float or USD
// field. Subscription billing is not a price measurement owned by jacu.
type CostTrace struct {
	ProfileID         string      `json:"profile_id"`
	BaselineProfileID string      `json:"baseline_profile_id,omitempty"`
	BillingMode       BillingMode `json:"billing_mode"`
	CostSource        CostSource  `json:"cost_source"`
	InputTokens       uint32      `json:"input_tokens"`
	OutputTokens      uint32      `json:"output_tokens"`
	DurationMs        uint64      `json:"duration_ms"`
	CostUnits         uint64      `json:"cost_units"`
	SavingsUnits      uint64      `json:"savings_units"`
	CacheHit          bool        `json:"cache_hit"`
}

func (trace CostTrace) Validate() error {
	if trace.ProfileID == "" {
		return errors.New("cost trace profile_id is required")
	}
	if trace.BillingMode != BillingSubscription && trace.BillingMode != BillingLocal {
		return errors.New("cost trace billing_mode is invalid")
	}
	if trace.CostSource != CostSourceNone && trace.CostSource != CostSourceLocalUnit && trace.CostSource != CostSourceAPIDollars {
		return errors.New("cost trace cost_source is invalid")
	}
	if trace.BillingMode == BillingSubscription && (trace.CostSource == CostSourceAPIDollars || trace.CostUnits > 0 || trace.SavingsUnits > 0) {
		return errors.New("subscription cost trace cannot contain USD or API cost")
	}
	if trace.CostSource == CostSourceAPIDollars {
		return errors.New("API dollar cost is outside host-profile model control")
	}
	return nil
}

func (trace CostTrace) String() string {
	return fmt.Sprintf("profile=%s billing=%s source=%s in=%d out=%d duration_ms=%d units=%d saved=%d cache=%t", trace.ProfileID, trace.BillingMode, trace.CostSource, trace.InputTokens, trace.OutputTokens, trace.DurationMs, trace.CostUnits, trace.SavingsUnits, trace.CacheHit)
}

const unitsPerCent uint64 = 1_000_000

type MissionLedger struct {
	ceiling uint64
	spent   uint64
	saved   uint64
}

func NewMissionLedger(budgetCents uint64) MissionLedger {
	ceiling := budgetCents
	if budgetCents > math.MaxUint64/unitsPerCent {
		ceiling = math.MaxUint64
	} else {
		ceiling *= unitsPerCent
	}
	return MissionLedger{ceiling: ceiling}
}

func (ledger *MissionLedger) CanAdmit(projected uint64) bool {
	if ledger == nil {
		return false
	}
	return projected <= ledger.Remaining()
}

func (ledger *MissionLedger) Absorb(costUnits, savedUnits uint64) {
	if ledger == nil {
		return
	}
	ledger.spent = saturatingAdd(ledger.spent, costUnits)
	ledger.saved = saturatingAdd(ledger.saved, savedUnits)
}

func (ledger MissionLedger) Ceiling() uint64 { return ledger.ceiling }
func (ledger MissionLedger) Spent() uint64   { return ledger.spent }
func (ledger MissionLedger) Saved() uint64   { return ledger.saved }

func (ledger MissionLedger) Remaining() uint64 {
	return ledger.ceiling - minUint64(ledger.spent, ledger.ceiling)
}

func saturatingAdd(left, right uint64) uint64 {
	if math.MaxUint64-left < right {
		return math.MaxUint64
	}
	return left + right
}

func minUint64(left, right uint64) uint64 {
	if left < right {
		return left
	}
	return right
}
