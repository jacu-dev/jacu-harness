package ledger

import (
	"sort"

	ctxpack "github.com/jacu-dev/jacu-harness/internal/capability/context"
)

const (
	VerdictAdmit   = "admit"
	VerdictRefuse  = "refuse"
	VerdictDegrade = "degrade"
	ReasonFit      = "budget_fit"
	ReasonOverflow = "required_overflow"
	ReasonDropped  = "optional_dropped"
	ReasonAnchors  = "anchors_lost"
)

type Decision struct {
	Verdict          string         `json:"verdict"`
	Reason           string         `json:"reason"`
	BudgetBytes      int64          `json:"budget_bytes"`
	RequestedBytes   int64          `json:"requested_bytes"`
	RemainingBytes   int64          `json:"remaining_bytes"`
	RequiredOverflow bool           `json:"required_overflow"`
	DroppedOptional  int            `json:"dropped_optional,omitempty"`
	ItemsRequired    int            `json:"items_required"`
	ItemsIncluded    int            `json:"items_included"`
	CoverageBPS      int            `json:"coverage_bps"`
	ToolCalls        int            `json:"tool_calls"`
	Included         []ctxpack.Item `json:"included,omitempty"`
}

type Dispatcher func()

func Decide(budget int64, pack ctxpack.Pack, dispatch Dispatcher) Decision {
	if budget <= 0 {
		budget = ctxpack.DefaultBudget
	}
	items := append([]ctxpack.Item{}, pack.Items...)
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Required != items[j].Required {
			return items[i].Required
		}
		return items[i].Path < items[j].Path
	})
	required := 0
	for _, item := range items {
		if item.Required {
			required++
		}
	}
	remaining := budget
	included := make([]ctxpack.Item, 0, len(items))
	droppedOptional := 0
	for _, item := range items {
		if item.Bytes > remaining {
			if item.Required {
				decision := Decision{
					Verdict: VerdictRefuse, Reason: ReasonOverflow, BudgetBytes: budget,
					RequestedBytes: pack.Bytes, RemainingBytes: remaining, RequiredOverflow: true,
					ItemsRequired: required, ItemsIncluded: includedRequired(included), ToolCalls: 0,
					Included: included,
				}
				decision.CoverageBPS = coverageBPS(decision.ItemsIncluded, required)
				return decision
			}
			droppedOptional++
			continue
		}
		included = append(included, item)
		remaining -= item.Bytes
	}
	decision := Decision{
		BudgetBytes: budget, RequestedBytes: pack.Bytes, RemainingBytes: remaining,
		ItemsRequired: required, ItemsIncluded: includedRequired(included), Included: included, ToolCalls: 0,
	}
	decision.CoverageBPS = coverageBPS(decision.ItemsIncluded, required)
	if droppedOptional > 0 {
		decision.Verdict = VerdictDegrade
		decision.Reason = ReasonDropped
		decision.DroppedOptional = droppedOptional
	} else {
		decision.Verdict = VerdictAdmit
		decision.Reason = ReasonFit
	}
	if lost := ctxpack.CheckAnchors(pack); lost > 0 && decision.Verdict != VerdictRefuse {
		decision.Verdict = VerdictRefuse
		decision.Reason = ReasonAnchors
		decision.RequiredOverflow = false
		return decision
	}
	if decision.Verdict != VerdictRefuse && dispatch != nil {
		dispatch()
		decision.ToolCalls = 1
	}
	return decision
}

func includedRequired(items []ctxpack.Item) int {
	count := 0
	for _, item := range items {
		if item.Required {
			count++
		}
	}
	return count
}

func coverageBPS(included, required int) int {
	if required <= 0 {
		return 10000
	}
	return included * 10000 / required
}
