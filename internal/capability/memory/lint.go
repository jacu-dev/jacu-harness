package memory

import (
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/jacu-dev/jacu-harness/internal/project"
)

var (
	memoryIDPattern = regexp.MustCompile(`^mem_[a-f0-9]{16}$`)
	secretPatterns  = []*regexp.Regexp{
		regexp.MustCompile(`-----BEGIN[^\r\n]*PRIVATE KEY-----`),
		regexp.MustCompile(`(?:ghp_|gho_|github_pat_|sk-|AKIA|xox[bpsa]-)\S+`),
		regexp.MustCompile(`(?i)\bBearer[ \t]+\S+`),
		regexp.MustCompile(`[A-Za-z][A-Za-z0-9+.-]*://[^\s:/@]+:[^\s/@]+@`),
		regexp.MustCompile(`(?i)\b(?:password|passwd|secret|token|api_key)\s*[:=]\s*\S+`),
	}
)

func lint(root string, in Input) []Lint {
	result := []Lint{}
	normalized := normalize(in)

	if normalized.Source == "derived" && !hasEvidence(normalized.Evidence) {
		result = append(result, Lint{
			Level: "BLOCK", Rule: "derived_without_evidence", Message: "derived memory requires evidence", Field: "evidence",
		})
	}
	if normalized.Source != "human" && normalized.Source != "derived" {
		result = append(result, Lint{
			Level: "BLOCK", Rule: "invalid_source", Message: "source must be human or derived", Field: "source",
		})
	}

	if containsSecret(in.Title) || containsSecret(in.Body) {
		result = append(result, Lint{
			Level: "BLOCK", Rule: "secret_content", Message: "content matches secret pattern; not stored", Field: "content",
		})
	}

	if !validKind(normalized.Kind) {
		result = append(result, Lint{
			Level: "BLOCK", Rule: "invalid_kind", Message: "kind must be decision, convention, gotcha, or preference", Field: "kind",
		})
	}

	if normalized.ProjectID == "" {
		if normalized.Kind != "preference" {
			result = append(result, Lint{
				Level: "BLOCK", Rule: "global_scope_restricted", Message: "global scope is restricted to preference records", Field: "project_id",
			})
		}
	} else if expected, err := project.ID(root); err != nil {
		result = append(result, Lint{
			Level: "BLOCK", Rule: "project_root_unresolved", Message: "project root could not be resolved", Field: "project_id",
		})
	} else if normalized.ProjectID != expected {
		result = append(result, Lint{
			Level: "BLOCK", Rule: "project_id_mismatch", Message: "project_id does not match the project root", Field: "project_id",
		})
	}

	if normalized.Title == "" {
		result = append(result, Lint{
			Level: "BLOCK", Rule: "empty_title", Message: "title is required", Field: "title",
		})
	}
	if strings.ContainsAny(in.Title, "\r\n\u0085\u2028\u2029") {
		result = append(result, Lint{
			Level: "BLOCK", Rule: "title_newline", Message: "title must not contain newlines", Field: "title",
		})
	}
	if utf8.RuneCountInString(normalized.Title) > 120 {
		result = append(result, Lint{
			Level: "BLOCK", Rule: "title_too_long", Message: "title exceeds 120 characters", Field: "title",
		})
	}

	if len(normalized.Body) > 4096 {
		result = append(result, Lint{
			Level: "BLOCK", Rule: "body_too_large", Message: "body exceeds 4KB; summarize", Field: "body",
		})
	}

	if normalized.Supersedes != "" && !memoryIDPattern.MatchString(normalized.Supersedes) {
		result = append(result, Lint{
			Level: "BLOCK", Rule: "invalid_supersedes", Message: "supersedes must be a valid memory_id", Field: "supersedes",
		})
	}

	return result
}

func hasEvidence(evidence []string) bool {
	for _, item := range evidence {
		if item != "" {
			return true
		}
	}
	return false
}

func containsSecret(content string) bool {
	for _, pattern := range secretPatterns {
		if pattern.MatchString(content) {
			return true
		}
	}
	return false
}

func validKind(kind string) bool {
	switch kind {
	case "decision", "convention", "gotcha", "preference":
		return true
	default:
		return false
	}
}
