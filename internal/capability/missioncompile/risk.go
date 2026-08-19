package missioncompile

import (
	"strings"
	"unicode"
)

// Risk is derived from what the mission actually declares, and the caller's
// hint may only raise it. Before this, `risk = RiskHint` or "write": the object
// being governed graded its own risk, and a malformed hint could stop the
// mission outright.
//
// The tiers are the runtime enum — safe | write | destructive. No fourth tier:
// the policy of the autonomy phase needs the write/destructive cut, and a tier
// nobody can derive is a tier nobody can enforce.

// riskRank orders the enum so risks can be compared. Unknown values rank below
// everything, which is what makes an invalid hint harmless.
func riskRank(risk string) int {
	switch risk {
	case "safe":
		return 1
	case "write":
		return 2
	case "destructive":
		return 3
	default:
		return 0
	}
}

func validRiskHint(hint string) bool {
	return riskRank(hint) > 0
}

// derivedRisk classifies a mission by its own content.
//
// The criterion is deliberately explicit rather than clever: a destructive verb
// in the objective means destructive, declaring paths or a mutation verb means
// write, and a mission that declares neither is a question, not a change.
// Breadth of allowed_paths is not part of this: it already drives ceremony, and
// a wide refactor is not the same kind of danger as a deletion.
func derivedRisk(in Input) string {
	words := objectiveWords(in.Objective)
	if containsAny(words, destructiveVerbs) {
		return "destructive"
	}
	if containsAny(words, mutationVerbs) || len(in.AllowedPaths) > 0 {
		return "write"
	}
	if len(in.VerificationCommands) > 0 {
		return "write"
	}
	return "safe"
}

// effectiveRisk applies the composition rule: max(derived, hint). A hint may
// only raise the risk — a model can incriminate itself, never absolve itself.
func effectiveRisk(derived, hint string) string {
	if riskRank(hint) > riskRank(derived) {
		return hint
	}
	return derived
}

var destructiveVerbs = map[string]struct{}{
	"remove": {}, "delete": {}, "drop": {}, "truncate": {}, "purge": {},
	"wipe": {}, "clean": {}, "cleanup": {}, "clear": {}, "prune": {}, "reset": {},
	"remover": {}, "apagar": {}, "excluir": {}, "deletar": {}, "limpar": {},
	"zerar": {}, "resetar": {}, "podar": {},
}

var mutationVerbs = map[string]struct{}{
	"add": {}, "create": {}, "fix": {}, "refactor": {}, "rename": {},
	"update": {}, "change": {}, "implement": {}, "migrate": {},
	"adicionar": {}, "corrigir": {}, "criar": {}, "refatorar": {},
	"renomear": {}, "atualizar": {}, "alterar": {}, "implementar": {},
}

func objectiveWords(objective string) []string {
	return strings.FieldsFunc(strings.ToLower(objective), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r)
	})
}

func containsAny(words []string, set map[string]struct{}) bool {
	for _, word := range words {
		if _, exists := set[word]; exists {
			return true
		}
	}
	return false
}
