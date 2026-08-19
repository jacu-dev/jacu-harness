package sdd

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jacu-dev/jacu-harness/internal/testgit"
)

func TestLintReportsEveryDeclaredFindingCode(t *testing.T) {
	base := `---
sdd: 001-example
branch: 001-example
adr: docs/adr/ADR-019-sdd-nativo.md
---
# Example
## Why
why
## Locked decisions
1. Rule — ADR-019
## Out of scope
- outside.txt
## Write scope
**Allowed**
` + "```" + `
internal/**
` + "```" + `
**Forbidden**
` + "```" + `
outside.txt
` + "```" + `
## Requirements
### Requirement: Safe behavior
The system SHALL be safe.
Delta: ADDED
## Open decisions
- none
## Tasks
| # | Task | Files | Verify | Status | Evidence |
|---|---|---|---|---|---|
| T1 | RED: test | internal/a.go | go test ./... | done | output |
`
	cases := []struct {
		name    string
		content string
		changes []string
		code    string
		level   string
	}{
		{name: "missing section", content: "---\nsdd: 001-example\n---\n# Only\n", code: "sdd_missing_section", level: SeverityBlock},
		{name: "task verify", content: base + "| T2 | RED: test | internal/b.go |  | todo |  |\n", code: "sdd_task_without_verify", level: SeverityBlock},
		{name: "done evidence", content: base + "| T2 | RED: test | internal/b.go | go test ./... | done |  |\n", code: "sdd_task_done_without_evidence", level: SeverityBlock},
		{name: "two in flight", content: base + "| T2 | RED: test | internal/b.go | go test ./... | doing |  |\n| T3 | RED: test | internal/c.go | go test ./... | doing |  |\n", code: "sdd_two_tasks_in_flight", level: SeverityBlock},
		{name: "out of scope", content: base, changes: []string{"outside.txt"}, code: "sdd_out_of_scope_touched", level: SeverityBlock},
		{name: "stale lock", content: base, code: "sdd_stale_lock", level: SeverityBlock},
		{name: "open decision", content: strings.Replace(base, "- none\n## Tasks", "- [ ] owner choice\n## Tasks", 1), code: "sdd_open_decision", level: SeverityBlock},
		{name: "requirement scenario", content: base[:len(base)-len("Delta: ADDED\n## Open decisions\n- none\n## Tasks\n| # | Task | Files | Verify | Status | Evidence |\n|---|---|---|---|---|---|\n| T1 | RED: test | internal/a.go | go test ./... | done | output |\n")] + "Delta: ADDED\n## Open decisions\n- none\n## Tasks\n| # | Task | Files | Verify | Status | Evidence |\n|---|---|---|---|---|---|\n| T1 | RED: test | internal/a.go | go test ./... | done | output |\n", code: "sdd_requirement_without_scenario", level: SeverityWarn},
		{name: "locked ADR", content: "---\nsdd: 001-example\n---\n# Example\n## Why\nwhy\n## Locked decisions\n1. Rule\n## Out of scope\n- none\n## Write scope\n**Allowed**\n```\ninternal/**\n```\n**Forbidden**\n```\noutside/**\n```\n## Requirements\n### Requirement: Safe\nThe system SHALL be safe.\n#### Scenario: Works\n- **WHEN** input\n- **THEN** output\n## Open decisions\n- none\n## Tasks\n| # | Task | Files | Verify | Status | Evidence |\n|---|---|---|---|---|---|\n| T1 | RED: test | internal/a.go | go test ./... | done | output |\n", code: "sdd_locked_decision_without_adr", level: SeverityWarn},
		{name: "task red", content: strings.Replace(base, "| T1 | RED: test |", "| T1 | GREEN: implementation |", 1), code: "sdd_task_without_red", level: SeverityWarn},
		{name: "language", content: base + "Este sistema deve funcionar e precisa de uma resposta.\n", code: "sdd_language_not_english", level: SeverityWarn},
		{name: "delta", content: base, code: "sdd_delta_summary", level: SeverityInfo},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			directory := filepath.Join(t.TempDir(), "001-example")
			if err := os.Mkdir(directory, 0o700); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(directory, "sdd.md")
			if err := os.WriteFile(path, []byte(testCase.content), 0o600); err != nil {
				t.Fatal(err)
			}
			document, err := Parse([]byte(testCase.content))
			if err != nil {
				t.Fatal(err)
			}
			findings := lintDocumentWithLock(document, directory, testCase.changes, nil)
			if finding := findCode(findings, testCase.code); finding == nil {
				t.Fatalf("missing %s in %#v", testCase.code, findings)
			} else if finding.Severity != testCase.level {
				t.Fatalf("%s severity = %s; want %s", testCase.code, finding.Severity, testCase.level)
			}
		})
	}
}

func TestLintReportsBadDirectory(t *testing.T) {
	document, err := Parse([]byte("# malformed\n"))
	if err != nil {
		t.Fatal(err)
	}
	findings := lintDocumentWithLock(document, filepath.Join(t.TempDir(), "not-a-change"), nil, nil)
	if finding := findCode(findings, "sdd_bad_directory"); finding == nil || finding.Severity != SeverityBlock {
		t.Fatalf("bad directory finding = %#v", finding)
	}
}

func TestLintSDDChecksWorkingTreeScope(t *testing.T) {
	root := t.TempDir()
	directory := filepath.Join(root, "docs", "sdd", "001-example")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	content := []byte("---\nsdd: 001-example\n---\n# Example\n## Write scope\n**Allowed**\ninternal/**\n**Forbidden**\noutside/**\n")
	rootHandle, openErr := os.OpenRoot(directory)
	if openErr != nil {
		t.Fatal(openErr)
	}
	defer func() { _ = rootHandle.Close() }()
	if writeErr := rootHandle.WriteFile("sdd.md", content, 0o600); writeErr != nil {
		t.Fatal(writeErr)
	}
	document, err := Parse(content)
	if err != nil {
		t.Fatal(err)
	}
	if lockErr := WriteLockRoot(rootHandle, ".", document); lockErr != nil {
		t.Fatal(lockErr)
	}
	for _, args := range [][]string{{"init"}, {"config", "user.email", "test@example.com"}, {"config", "user.name", "SDD Test"}, {"add", "."}, {"commit", "-m", "fixture"}} {
		// #nosec G204 -- git is fixed and args are the test-owned repository setup.
		command := exec.Command("git", args...)
		command.Dir = root
		command.Env = testgit.Env()
		if output, commandErr := command.CombinedOutput(); commandErr != nil {
			t.Fatalf("git %v: %v\n%s", args, commandErr, output)
		}
	}
	if writeErr := os.WriteFile(filepath.Join(root, "README.md"), []byte("outside scope\n"), 0o600); writeErr != nil {
		t.Fatal(writeErr)
	}
	lock, err := rootHandle.ReadFile("sdd.lock.json")
	if err != nil {
		t.Fatal(err)
	}
	if finding := findCode(LintSDDContentWithLock(directory, content, lock), "sdd_out_of_scope_touched"); finding == nil || finding.Target != "README.md" {
		t.Fatalf("working-tree scope finding = %#v", finding)
	}
}

func TestLintSelectedSDDChecksScopeAfterDocumentIsCommitted(t *testing.T) {
	root := t.TempDir()
	directory := filepath.Join(root, "docs", "sdd", "001-example")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	content := []byte("---\nsdd: 001-example\n---\n# Example\n## Write scope\n**Allowed**\ninternal/**\n**Forbidden**\noutside/**\n")
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/sddfixture\n\ngo 1.26\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	rootHandle, openErr := os.OpenRoot(directory)
	if openErr != nil {
		t.Fatal(openErr)
	}
	defer func() { _ = rootHandle.Close() }()
	if writeErr := rootHandle.WriteFile("sdd.md", content, 0o600); writeErr != nil {
		t.Fatal(writeErr)
	}
	document, err := Parse(content)
	if err != nil {
		t.Fatal(err)
	}
	if lockErr := WriteLockRoot(rootHandle, ".", document); lockErr != nil {
		t.Fatal(lockErr)
	}
	for _, args := range [][]string{{"init"}, {"config", "user.email", "test@example.com"}, {"config", "user.name", "SDD Test"}, {"add", "."}, {"commit", "-m", "fixture"}} {
		// #nosec G204 -- git is fixed and args are the test-owned repository setup.
		command := exec.Command("git", args...)
		command.Dir = root
		command.Env = testgit.Env()
		if output, commandErr := command.CombinedOutput(); commandErr != nil {
			t.Fatalf("git %v: %v\n%s", args, commandErr, output)
		}
	}
	if writeErr := os.WriteFile(filepath.Join(root, "README.md"), []byte("outside scope\n"), 0o600); writeErr != nil {
		t.Fatal(writeErr)
	}
	lock, err := rootHandle.ReadFile("sdd.lock.json")
	if err != nil {
		t.Fatal(err)
	}
	if finding := findCode(LintSelectedSDDContentWithLock(directory, content, lock), "sdd_out_of_scope_touched"); finding == nil || finding.Target != "README.md" {
		t.Fatalf("selected SDD scope finding = %#v", finding)
	}
}

func TestLintSDDBlocksWhenGitStateIsUnavailable(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "001-example")
	if err := os.MkdirAll(filepath.Join(directory, ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	rootHandle, err := os.OpenRoot(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rootHandle.Close() }()
	if err := rootHandle.WriteFile("sdd.md", []byte("---\nsdd: 001-example\n---\n# Example\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if findCode(LintSDDContentWithLock(directory, []byte("---\nsdd: 001-example\n---\n# Example\n"), nil), "sdd_git_state_unavailable") == nil {
		t.Fatalf("unavailable git state was not blocked")
	}
}

func findCode(findings []Finding, code string) *Finding {
	for index := range findings {
		if findings[index].Code == code {
			return &findings[index]
		}
	}
	return nil
}
