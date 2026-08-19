package projectinspect

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestInspectBlocksWhenRootIsNotAGitWorkTree(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	handler := inspectHandler(root)
	result, err := handler(context.Background(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "blocked" {
		t.Fatalf("status = %q; want blocked (must not inspect a non-repo as ok/partial)", result.Status)
	}
	if !strings.Contains(result.Summary, root) {
		t.Fatalf("summary %q does not name the cwd", result.Summary)
	}
	if !strings.Contains(strings.ToLower(result.Summary), "git") {
		t.Fatalf("summary %q does not instruct anchoring to a git work tree", result.Summary)
	}
}

func TestInspectAllowsAGitWorkTree(t *testing.T) {
	root := t.TempDir()
	run := exec.Command("git", "init")
	run.Dir = root
	if out, err := run.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	result, err := inspectHandler(root)(context.Background(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "ok" && result.Status != "partial" {
		t.Fatalf("status = %q; want ok or partial inside a work tree", result.Status)
	}
}
