package missioncompile

import (
	"strings"
	"unicode"
)

func classifyCeremony(in Input) string {
	if in.Program != nil {
		return "full"
	}
	// Ceremony reads the effective risk, not the raw hint. An invalid hint is
	// ignored everywhere or nowhere; letting it push ceremony up would be the
	// same defect in a second place.
	risk := effectiveRisk(derivedRisk(in), in.RiskHint)
	if len(in.AllowedPaths) == 0 && len(in.VerificationCommands) == 0 && risk == "safe" && !hasMutationVerb(in.Objective) {
		return "direct"
	}

	if risk == "destructive" || len(in.AcceptanceCriteria) >= 3 || hasBroadAllowedPaths(in.AllowedPaths) {
		return "full"
	}

	return "light"
}

func hasMutationVerb(objective string) bool {
	words := strings.FieldsFunc(strings.ToLower(objective), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r)
	})
	mutationVerbs := map[string]struct{}{
		"add": {}, "create": {}, "fix": {}, "refactor": {}, "remove": {}, "rename": {},
		"adicionar": {}, "corrigir": {}, "criar": {}, "refatorar": {}, "remover": {}, "renomear": {},
	}
	for _, word := range words {
		if _, exists := mutationVerbs[word]; exists {
			return true
		}
	}
	return false
}

func hasBroadAllowedPaths(paths []string) bool {
	distinct := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		if path == "." || path == "**" {
			return true
		}
		distinct[path] = struct{}{}
	}
	return len(distinct) >= 3
}
