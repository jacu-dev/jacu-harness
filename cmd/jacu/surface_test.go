package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestInspectJSONExitCodes(t *testing.T) {
	binary := buildBinary(t)
	root := initCLIGitRepo(t)

	command := exec.Command(binary, "inspect", "--json") // #nosec G204 -- binary is built by buildBinary and receives fixed test argv.
	command.Dir = root
	command.Env = isolatedUserStateEnv(t)
	output, err := command.Output()
	if err != nil {
		t.Fatalf("inspect --json: %v\n%s", err, output)
	}
	var envelope map[string]any
	if err := json.Unmarshal(output, &envelope); err != nil {
		t.Fatalf("inspect stdout is not JSON: %v; %q", err, output)
	}
	if envelope["status"] != "ok" && envelope["status"] != "partial" {
		t.Fatalf("inspect status = %#v", envelope["status"])
	}

	bad := exec.Command(binary, "inspect", "--bogus") // #nosec G204 -- binary is built by buildBinary and receives fixed test argv.
	bad.Dir = root
	bad.Env = isolatedUserStateEnv(t)
	_, badErr := bad.CombinedOutput()
	if badErr == nil || badErr.(*exec.ExitError).ExitCode() != 2 {
		t.Fatalf("inspect --bogus exit = %v; want 2", badErr)
	}
}

func TestWorkspaceStatusJSONAndReportJSON(t *testing.T) {
	binary := buildBinary(t)
	root := initCLIGitRepo(t)

	status := exec.Command(binary, "workspace", "status", "--json") // #nosec G204 -- binary is built by buildBinary and receives fixed test argv.
	status.Dir = root
	status.Env = isolatedUserStateEnv(t)
	output, err := status.Output()
	if err != nil {
		t.Fatalf("workspace status --json: %v\n%s", err, output)
	}
	if !strings.Contains(string(output), `"status"`) {
		t.Fatalf("workspace status JSON missing status: %q", output)
	}

	report := exec.Command(binary, "report", "--json") // #nosec G204 -- binary is built by buildBinary and receives fixed test argv.
	report.Dir = root
	report.Env = isolatedUserStateEnv(t)
	reportOut, err := report.Output()
	if err != nil {
		t.Fatalf("report --json: %v\n%s", err, reportOut)
	}
	var quality map[string]any
	if err := json.Unmarshal(reportOut, &quality); err != nil {
		t.Fatalf("report stdout is not JSON: %v; %q", err, reportOut)
	}
	if quality["kind"] != "audit" || quality["schema_version"] != "1" {
		t.Fatalf("report --json is not quality.json: %#v", quality)
	}
}

func TestWorkspaceStatusJSONFromLinkedWorktree(t *testing.T) {
	binary := buildBinary(t)
	repo := initCLIGitRepo(t)
	worktree := filepath.Join(t.TempDir(), "run_linked")
	if err := os.MkdirAll(worktree, 0o700); err != nil {
		t.Fatal(err)
	}
	link := fmt.Sprintf("gitdir: %s\n", filepath.Join(repo, ".git", "worktrees", "run_linked"))
	if err := os.WriteFile(filepath.Join(worktree, ".git"), []byte(link), 0o600); err != nil {
		t.Fatal(err)
	}

	status := exec.Command(binary, "workspace", "status", "--json") // #nosec G204 -- binary is built by buildBinary and receives fixed test argv.
	status.Dir = worktree
	status.Env = isolatedUserStateEnv(t)
	var stderr bytes.Buffer
	status.Stderr = &stderr
	output, err := status.Output()
	if err != nil {
		t.Fatalf("workspace status --json from linked worktree: %v\nstdout=%s\nstderr=%s", err, output, stderr.String())
	}
	var envelope map[string]any
	if err := json.Unmarshal(output, &envelope); err != nil {
		t.Fatalf("stdout is not JSON: %v; stdout=%q stderr=%q", err, output, stderr.String())
	}
	if envelope["status"] != "ok" {
		t.Fatalf("status = %#v summary = %#v; want ok", envelope["status"], envelope["summary"])
	}
}

func initCLIGitRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...) // #nosec G204 -- fixed git binary with test-controlled arguments
		cmd.Dir = root
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=Jacu Test", "GIT_AUTHOR_EMAIL=jacu@example.test", "GIT_COMMITTER_NAME=Jacu Test", "GIT_COMMITTER_EMAIL=jacu@example.test")
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, output)
		}
	}
	run("init", "-b", "main")
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("ok\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	run("add", "README.md")
	run("commit", "-m", "initial")
	return root
}
