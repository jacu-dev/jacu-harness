package missioncompile

import (
	ctxpack "github.com/jacu-dev/jacu-harness/internal/capability/context"
	"github.com/jacu-dev/jacu-harness/internal/capability/ledger"
)

func admitMissionContext(root string, in Input, mission *Mission) (status string, blocked bool) {
	spec := ctxpack.Spec{
		Objective:      in.Objective,
		Acceptance:     in.AcceptanceCriteria,
		AllowedPaths:   in.AllowedPaths,
		ForbiddenPaths: in.ForbiddenPaths,
		RequiredPaths:  append([]string{}, in.Context.Refs...),
		Verification:   in.VerificationCommands,
		BudgetBytes:    ctxpack.DefaultBudget,
	}
	pack, err := ctxpack.PackRoot(root, spec)
	if err != nil {
		mission.Lint = append(mission.Lint, Lint{Level: "BLOCK", Rule: "context_pack", Message: "context pack failed"})
		return "blocked", true
	}
	lost := ctxpack.CheckAnchors(pack)
	ctxpack.EmitAnchor(root, lost)
	decision := ledger.Decide(spec.BudgetBytes, pack, nil)
	ledger.Emit(root, decision)
	ctxpack.EmitPack(root, pack, decision.CoverageBPS, decision.ItemsRequired, decision.ItemsIncluded)
	if lost > 0 {
		mission.Lint = append(mission.Lint, Lint{Level: "BLOCK", Rule: "context_anchor", Message: "required context anchor missing from pack"})
		return "blocked", true
	}
	if decision.Verdict == ledger.VerdictRefuse {
		mission.Lint = append(mission.Lint, Lint{Level: "BLOCK", Rule: "ledger", Message: "required context does not fit the budget"})
		return "blocked", true
	}
	ctxpack.EmitHandoff(root, decision.ItemsIncluded)
	return "ok", false
}
