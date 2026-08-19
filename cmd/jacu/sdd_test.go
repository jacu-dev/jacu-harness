package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestSDDCLIExitCodesAndJSONStreams(t *testing.T) {
	binary := buildBinary(t)
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "docs", "sdd", "001-broken"), 0o700); err != nil {
		t.Fatal(err)
	}
	writeNativeSDD(t, root, filepath.Join("docs", "sdd", "001-broken", "sdd.md"), []byte("# broken\n"))
	// #nosec G204 -- binary is built by buildBinary and receives fixed test argv.
	jsonCommand := exec.Command(binary, "sdd", "lint", "--json")
	jsonCommand.Dir = root
	jsonOutput, jsonErr := jsonCommand.Output()
	if jsonErr == nil {
		t.Fatal("lint should block the current unlocked SDD")
	}
	if exitCode := jsonErr.(*exec.ExitError).ExitCode(); exitCode != 1 {
		t.Fatalf("lint exit code = %d; want 1", exitCode)
	}
	var findings []map[string]string
	if err := json.Unmarshal(jsonOutput, &findings); err != nil {
		t.Fatalf("stdout is not JSON: %v; output=%q", err, jsonOutput)
	}
	if len(findings) == 0 {
		t.Fatal("JSON lint output has no findings")
	}

	// #nosec G204 -- binary is built by buildBinary and receives fixed test argv.
	badUsage := exec.Command(binary, "sdd", "lint", "--bad")
	badUsage.Dir = root
	badOutput, badErr := badUsage.CombinedOutput()
	if badErr == nil || badErr.(*exec.ExitError).ExitCode() != 2 {
		t.Fatalf("invalid usage = err %v output %q; want exit 2", badErr, badOutput)
	}
	if !strings.Contains(string(badOutput), "usage") {
		t.Fatalf("invalid usage did not report usage: %q", badOutput)
	}
}

func TestSDDCLIDirectoryTargetAndDerivedStatus(t *testing.T) {
	binary := buildBinary(t)
	root := t.TempDir()
	directory := filepath.Join(root, "docs", "sdd", "001-example")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	writeNativeSDD(t, root, filepath.Join("docs", "sdd", "001-example", "sdd.md"), []byte(validSDDForCLITest()))
	// #nosec G204 -- binary is built by buildBinary and receives fixed test argv.
	writeLock := exec.Command(binary, "sdd", "lint", "--all", "--write-lock")
	writeLock.Dir = root
	if output, err := writeLock.CombinedOutput(); err != nil {
		t.Fatalf("write-lock failed: %v\n%s", err, output)
	}

	// #nosec G204 -- binary is built by buildBinary and receives fixed test argv.
	lint := exec.Command(binary, "sdd", "lint", "docs/sdd/001-example", "--json")
	lint.Dir = root
	jsonOutput, err := lint.Output()
	if err != nil {
		t.Fatalf("directory lint failed: %v\n%s", err, jsonOutput)
	}
	var findings []map[string]string
	if jsonErr := json.Unmarshal(jsonOutput, &findings); jsonErr != nil {
		t.Fatalf("directory lint stdout is not JSON: %v; output=%q", err, jsonOutput)
	}

	// #nosec G204 -- binary is built by buildBinary and receives fixed test argv.
	status := exec.Command(binary, "sdd", "status")
	status.Dir = root
	statusOutput, err := status.Output()
	if err != nil {
		t.Fatalf("status failed: %v", err)
	}
	for _, field := range []string{"repository_runs=0", "tasks_total=1", "tasks_done=1", "tasks_doing=0", "blocks=0", "changed_paths=0"} {
		if !strings.Contains(string(statusOutput), field) {
			t.Fatalf("status %q lacks %s", statusOutput, field)
		}
	}
}

func TestSDDCLILintAllReportsMalformedDirectory(t *testing.T) {
	binary := buildBinary(t)
	root := t.TempDir()
	base := filepath.Join(root, "docs", "sdd")
	if err := os.MkdirAll(filepath.Join(base, "001-example"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(base, "sdd_bad_directory"), 0o700); err != nil {
		t.Fatal(err)
	}
	writeNativeSDD(t, root, filepath.Join("docs", "sdd", "001-example", "sdd.md"), []byte("# broken\n"))
	// #nosec G204 -- binary is built by buildBinary and receives fixed test argv.
	command := exec.Command(binary, "sdd", "lint", "--all", "--json")
	command.Dir = root
	output, err := command.Output()
	if err == nil || err.(*exec.ExitError).ExitCode() != 1 {
		t.Fatalf("malformed directory exit = %v; output=%q", err, output)
	}
	var findings []map[string]string
	if jsonErr := json.Unmarshal(output, &findings); jsonErr != nil {
		t.Fatalf("malformed directory stdout is not JSON: %v; output=%q", jsonErr, output)
	}
	found := false
	for _, finding := range findings {
		if finding["code"] == "sdd_bad_directory" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("malformed directory finding missing: %#v", findings)
	}
}

func TestSDDCLintRejectsSymlinkedDocumentOutsideProject(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink permissions vary on Windows")
	}
	binary := buildBinary(t)
	root := t.TempDir()
	base := filepath.Join(root, "docs", "sdd", "001-example")
	outside := t.TempDir()
	if err := os.MkdirAll(base, 0o700); err != nil {
		t.Fatal(err)
	}
	sentinel := filepath.Join(outside, "sentinel.md")
	if err := os.WriteFile(sentinel, []byte(validSDDForCLITest()), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(sentinel, filepath.Join(base, "sdd.md")); err != nil {
		t.Fatal(err)
	}
	// #nosec G204 -- binary is built by buildBinary and receives fixed test argv.
	command := exec.Command(binary, "sdd", "lint", "docs/sdd/001-example", "--write-lock")
	command.Dir = root
	if output, err := command.CombinedOutput(); err == nil {
		t.Fatalf("symlinked SDD unexpectedly passed: %s", output)
	}
}

func TestSDDCLintRejectsSymlinkedLockOutsideProject(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink permissions vary on Windows")
	}
	binary := buildBinary(t)
	root := t.TempDir()
	base := filepath.Join(root, "docs", "sdd", "001-example")
	outside := t.TempDir()
	if err := os.MkdirAll(base, 0o700); err != nil {
		t.Fatal(err)
	}
	writeNativeSDD(t, root, filepath.Join("docs", "sdd", "001-example", "sdd.md"), []byte(validSDDForCLITest()))
	sentinel := filepath.Join(outside, "sdd.lock.json")
	if err := os.WriteFile(sentinel, []byte(`{"content_sha256":"external-sentinel"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(sentinel, filepath.Join(base, "sdd.lock.json")); err != nil {
		t.Fatal(err)
	}
	// #nosec G204 -- binary is built by buildBinary and receives fixed test argv.
	command := exec.Command(binary, "sdd", "lint", "docs/sdd/001-example", "--write-lock")
	command.Dir = root
	if output, err := command.CombinedOutput(); err == nil {
		t.Fatalf("symlinked lock unexpectedly passed: %s", output)
	}
	// #nosec G304 -- sentinel is an explicit file created under this test's TempDir.
	content, err := os.ReadFile(sentinel)
	if err != nil || string(content) != `{"content_sha256":"external-sentinel"}` {
		t.Fatalf("external lock changed: %q, %v", content, err)
	}
}

func TestSDDCloseReturnsContractExitCodes(t *testing.T) {
	root := t.TempDir()
	directory := filepath.Join(root, "docs", "sdd", "001-example")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	writeNativeSDD(t, root, filepath.Join("docs", "sdd", "001-example", "sdd.md"), []byte(validSDDForCLITest()))
	stdout, err := os.CreateTemp(t.TempDir(), "stdout")
	if err != nil {
		t.Fatal(err)
	}
	stderr, err := os.CreateTemp(t.TempDir(), "stderr")
	if err != nil {
		t.Fatal(err)
	}
	if code := sddClose(root, []string{"docs/sdd/001-example"}, stdout, stderr); code != 2 {
		t.Fatalf("sdd close exit = %d; want 2 for unfinished close contract", code)
	}
}

func validSDDForCLITest() string {
	return `---
sdd: 001-example
branch: test
adr: docs/adr/ADR-019-sdd-nativo.md
---
# Example
## Why
Why.
## Locked decisions
ADR-019.
## Out of scope
None.
## Write scope
**Allowed**
docs/sdd/**
**Forbidden**
outside/**
## Requirements
### Requirement: Safe behavior
The system SHALL be safe.
#### Scenario: Safe
- **WHEN** called
- **THEN** it is safe
Delta: ADDED
## Non-goals
None.
## Open decisions
- none
## Tasks
| # | Task | Files | Verify | Status | Evidence |
|---|---|---|---|---|---|
| T1 | RED: test | docs/sdd/** | go test ./... | done | passed |
## Done
None.
## Follow-ups
None.
`
}

func writeNativeSDD(t *testing.T, root, relative string, content []byte) {
	t.Helper()
	projectRoot, err := os.OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = projectRoot.Close() }()
	if err := projectRoot.WriteFile(relative, content, 0o600); err != nil {
		t.Fatal(err)
	}
}
