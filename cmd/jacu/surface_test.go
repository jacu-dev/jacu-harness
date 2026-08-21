package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestInspectJSONExitCodes(t *testing.T) {
	binary := buildBinary(t)
	root := initCLIGitRepo(t)

	command := exec.Command(binary, "inspect", "--json")
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

	bad := exec.Command(binary, "inspect", "--bogus")
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

	status := exec.Command(binary, "workspace", "status", "--json")
	status.Dir = root
	status.Env = isolatedUserStateEnv(t)
	output, err := status.Output()
	if err != nil {
		t.Fatalf("workspace status --json: %v\n%s", err, output)
	}
	if !strings.Contains(string(output), `"status"`) {
		t.Fatalf("workspace status JSON missing status: %q", output)
	}

	report := exec.Command(binary, "report", "--json")
	report.Dir = root
	report.Env = isolatedUserStateEnv(t)
	reportOut, err := report.Output()
	if err != nil {
		t.Fatalf("report --json: %v\n%s", err, reportOut)
	}
	var envelope map[string]any
	if err := json.Unmarshal(reportOut, &envelope); err != nil {
		t.Fatalf("report stdout is not JSON: %v; %q", err, reportOut)
	}
	if envelope["status"] != "ok" {
		t.Fatalf("report status = %#v", envelope["status"])
	}
}

func initCLIGitRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
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
