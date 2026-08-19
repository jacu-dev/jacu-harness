package missioncompile

import (
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/jacu-dev/jacu-harness/internal/project"
)

func lint(root string, in Input, ceremony string) []Lint {
	result := []Lint{}

	if strings.TrimSpace(in.Objective) == "" {
		result = append(result, Lint{
			Level: "BLOCK", Rule: "empty_objective", Message: "objective is required", Field: "objective",
		})
	}

	for _, command := range in.VerificationCommands {
		if len(command) == 1 && strings.ContainsAny(command[0], " \t\r\n") {
			result = append(result, Lint{
				Level: "BLOCK", Rule: "shell_string_command", Message: "shell-string command; use argv array", Field: "verification_commands",
			})
		}
		if isShellInterpreterCommand(command) {
			result = append(result, Lint{
				Level: "BLOCK", Rule: "shell_interpreter_command", Message: "shell interpreter command is not allowed", Field: "verification_commands",
			})
		}
	}

	for _, path := range in.AllowedPaths {
		if !pathStaysWithinRoot(root, path) {
			result = append(result, Lint{
				Level: "BLOCK", Rule: "path_outside_root", Message: "path must stay within project root", Field: "allowed_paths",
			})
		}
	}
	for _, path := range in.ForbiddenPaths {
		if !pathStaysWithinRoot(root, path) {
			result = append(result, Lint{
				Level: "BLOCK", Rule: "path_outside_root", Message: "path must stay within project root", Field: "forbidden_paths",
			})
		}
	}

	forbidden := make(map[string]struct{}, len(in.ForbiddenPaths))
	for _, path := range in.ForbiddenPaths {
		forbidden[filepath.Clean(path)] = struct{}{}
	}
	for _, path := range in.AllowedPaths {
		if _, exists := forbidden[filepath.Clean(path)]; exists {
			result = append(result, Lint{
				Level: "BLOCK", Rule: "allowed_forbidden_overlap", Message: "path cannot be both allowed and forbidden", Field: "allowed_paths",
			})
		}
	}

	// A hint is advisory and can never block. Blocking on it made a malformed
	// value load-bearing enough to stop the mission, and taught the recovering
	// model to resend with the strongest value it knew.
	if in.RiskHint != "" && !validRiskHint(in.RiskHint) {
		result = append(result, Lint{
			Level: "WARN", Rule: "invalid_risk_hint", Message: "unknown risk_hint, ignored", Field: "risk_hint",
		})
	} else if in.RiskHint != "" && riskRank(in.RiskHint) < riskRank(derivedRisk(in)) {
		result = append(result, Lint{
			Level: "WARN", Rule: "risk_hint_below_derived", Message: "risk_hint is weaker than the derived risk; derived risk used", Field: "risk_hint",
		})
	}

	if ceremony == "full" && len(in.AcceptanceCriteria) == 0 {
		result = append(result, Lint{
			Level: "WARN", Rule: "full_without_criteria", Message: "full ceremony requires acceptance criteria", Field: "acceptance_criteria",
		})
	}
	if ceremony == "full" && len(in.VerificationCommands) == 0 {
		result = append(result, Lint{
			Level: "WARN", Rule: "full_without_verification", Message: "full ceremony requires verification commands", Field: "verification_commands",
		})
	}

	if ambiguousObjective(in.Objective) {
		result = append(result, Lint{
			Level: "WARN", Rule: "ambiguous_objective", Message: "objective is ambiguous; provide at least four words and an action verb", Field: "objective",
		})
	}

	if in.Context.ProjectID != "" {
		if expected, err := project.ID(root); err == nil && in.Context.ProjectID != expected {
			result = append(result, Lint{
				Level: "WARN", Rule: "project_id_mismatch", Message: "context.project_id does not match the project root", Field: "context.project_id",
			})
		}
	}

	if in.RiskHint == "" && (ceremony == "light" || ceremony == "full") {
		result = append(result, Lint{
			Level: "INFO", Rule: "risk_defaulted", Message: "risk_hint absent; risk derived from the mission", Field: "risk",
		})
	}

	return result
}

func isShellInterpreterCommand(command []string) bool {
	if len(command) < 2 {
		return false
	}
	interpreter := strings.ToLower(filepath.Base(command[0]))
	if interpreter != "sh" && interpreter != "bash" && interpreter != "zsh" && interpreter != "cmd.exe" {
		return false
	}
	for _, argument := range command[1:] {
		if strings.EqualFold(argument, "-c") || strings.EqualFold(argument, "/c") {
			return true
		}
	}
	return false
}

func pathStaysWithinRoot(root, path string) bool {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return false
	}
	rootResolved, err := filepath.EvalSymlinks(rootAbs)
	if err != nil {
		return false
	}

	candidate := filepath.Clean(path)
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(rootResolved, candidate)
	}
	if !isWithin(rootResolved, candidate) {
		return false
	}
	if _, err := os.Lstat(candidate); err == nil {
		resolved, err := filepath.EvalSymlinks(candidate)
		if err != nil || !isWithin(rootResolved, resolved) {
			return false
		}
	}
	return true
}

func isWithin(root, candidate string) bool {
	relative, err := filepath.Rel(root, candidate)
	if err != nil {
		return false
	}
	return relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func ambiguousObjective(objective string) bool {
	words := strings.FieldsFunc(strings.ToLower(objective), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r)
	})
	if len(words) < 4 {
		return true
	}
	actionVerbs := map[string]struct{}{
		"add": {}, "change": {}, "correct": {}, "create": {}, "fix": {},
		"implement": {}, "refactor": {}, "remove": {}, "rename": {}, "update": {},
		"adicionar": {}, "alterar": {}, "atualizar": {}, "corrigir": {}, "criar": {},
		"implementar": {}, "refatorar": {}, "remover": {}, "renomear": {},
	}
	for _, word := range words {
		if _, exists := actionVerbs[word]; exists {
			return false
		}
	}
	return true
}
