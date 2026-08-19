package missioncompile

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLintBlocksEmptyObjective(t *testing.T) {
	got := lint(t.TempDir(), Input{Objective: "   ", RiskHint: "write"}, "light")
	assertLint(t, got, Lint{
		Level: "BLOCK", Rule: "empty_objective", Message: "objective is required", Field: "objective",
	})
}

func TestLintBlocksShellStringCommand(t *testing.T) {
	got := lint(t.TempDir(), Input{
		Objective:            "Run the complete test suite",
		VerificationCommands: [][]string{{"go test ./..."}},
		RiskHint:             "write",
	}, "light")
	assertLint(t, got, Lint{
		Level: "BLOCK", Rule: "shell_string_command", Message: "shell-string command; use argv array", Field: "verification_commands",
	})
}

func TestLintBlocksShellInterpreterCommand(t *testing.T) {
	commands := []struct {
		name string
		argv []string
	}{
		{name: "sh", argv: []string{"sh", "-c", "rm -rf /"}},
		{name: "bash", argv: []string{"bash", "-c", "go test ./..."}},
		{name: "zsh", argv: []string{"zsh", "-c", "go test ./..."}},
		{name: "cmd.exe", argv: []string{"cmd.exe", "/C", "go test ./..."}},
	}

	for _, tt := range commands {
		t.Run(tt.name, func(t *testing.T) {
			got := lint(t.TempDir(), Input{
				Objective:            "Run the complete test suite",
				VerificationCommands: [][]string{tt.argv},
				RiskHint:             "write",
			}, "light")
			assertLint(t, got, Lint{
				Level: "BLOCK", Rule: "shell_interpreter_command", Message: "shell interpreter command is not allowed", Field: "verification_commands",
			})
		})
	}
}

func TestLintBlocksPathOutsideRoot(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	link := filepath.Join(root, "outside-link")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}

	paths := []struct {
		name string
		path string
	}{
		{name: "parent traversal", path: "../outside"},
		{name: "absolute outside", path: outside},
		{name: "existing symlink outside", path: "outside-link"},
	}

	for _, tt := range paths {
		t.Run(tt.name, func(t *testing.T) {
			got := lint(root, Input{
				Objective:    "Update the parser implementation",
				AllowedPaths: []string{tt.path},
				RiskHint:     "write",
			}, "light")
			assertLint(t, got, Lint{
				Level: "BLOCK", Rule: "path_outside_root", Message: "path must stay within project root", Field: "allowed_paths",
			})
		})
	}
}

func TestLintBlocksAllowedForbiddenOverlap(t *testing.T) {
	got := lint(t.TempDir(), Input{
		Objective:      "Update the parser implementation",
		AllowedPaths:   []string{"internal/parser"},
		ForbiddenPaths: []string{"internal/parser"},
		RiskHint:       "write",
	}, "light")
	assertLint(t, got, Lint{
		Level: "BLOCK", Rule: "allowed_forbidden_overlap", Message: "path cannot be both allowed and forbidden", Field: "allowed_paths",
	})
}

func TestLintWarnsAndIgnoresInvalidRiskHint(t *testing.T) {
	got := lint(t.TempDir(), Input{
		Objective: "Update the parser implementation",
		RiskHint:  "banana",
	}, "light")
	assertLint(t, got, Lint{
		Level: "WARN", Rule: "invalid_risk_hint", Message: "unknown risk_hint, ignored", Field: "risk_hint",
	})
	for _, item := range got {
		if item.Level == "BLOCK" {
			t.Fatalf("lint = %#v; a hint must never block the mission", got)
		}
	}
}

func TestLintWarnsFullWithoutCriteria(t *testing.T) {
	got := lint(t.TempDir(), Input{
		Objective:            "Update the parser implementation safely",
		VerificationCommands: [][]string{{"go", "test", "./..."}},
		RiskHint:             "write",
	}, "full")
	assertLint(t, got, Lint{
		Level: "WARN", Rule: "full_without_criteria", Message: "full ceremony requires acceptance criteria", Field: "acceptance_criteria",
	})
}

func TestLintWarnsFullWithoutVerification(t *testing.T) {
	got := lint(t.TempDir(), Input{
		Objective:          "Update the parser implementation safely",
		AcceptanceCriteria: []string{"Parser tests pass"},
		RiskHint:           "write",
	}, "full")
	assertLint(t, got, Lint{
		Level: "WARN", Rule: "full_without_verification", Message: "full ceremony requires verification commands", Field: "verification_commands",
	})
}

func TestLintWarnsAmbiguousObjective(t *testing.T) {
	tests := []struct {
		name      string
		objective string
	}{
		{name: "fewer than four words", objective: "Fix parser"},
		{name: "no action verb", objective: "The parser output is incorrect"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := lint(t.TempDir(), Input{Objective: tt.objective, RiskHint: "write"}, "light")
			assertLint(t, got, Lint{
				Level: "WARN", Rule: "ambiguous_objective", Message: "objective is ambiguous; provide at least four words and an action verb", Field: "objective",
			})
		})
	}
}

func TestLintWarnsProjectIDMismatch(t *testing.T) {
	got := lint(t.TempDir(), Input{
		Objective: "Update the parser implementation safely",
		Context:   Context{ProjectID: "prj_wrong"},
		RiskHint:  "write",
	}, "light")
	assertLint(t, got, Lint{
		Level: "WARN", Rule: "project_id_mismatch", Message: "context.project_id does not match the project root", Field: "context.project_id",
	})
}

func TestLintReportsRiskDerivedWhenHintIsAbsent(t *testing.T) {
	got := lint(t.TempDir(), Input{Objective: "Update the parser implementation safely"}, "light")
	assertLint(t, got, Lint{
		Level: "INFO", Rule: "risk_defaulted", Message: "risk_hint absent; risk derived from the mission", Field: "risk",
	})
}

func TestLintHappyPath(t *testing.T) {
	got := lint(t.TempDir(), Input{
		Objective:            "Update the parser implementation safely",
		AcceptanceCriteria:   []string{"Parser tests pass"},
		VerificationCommands: [][]string{{"go", "test", "./..."}},
		AllowedPaths:         []string{"internal/parser/new.go"},
		ForbiddenPaths:       []string{".env"},
		RiskHint:             "write",
	}, "full")
	if len(got) != 0 {
		t.Fatalf("lint() = %#v; want no lint", got)
	}
}

func assertLint(t *testing.T, got []Lint, want Lint) {
	t.Helper()
	for _, item := range got {
		if item == want {
			return
		}
	}
	t.Fatalf("lint() = %#v; want item %#v", got, want)
}
