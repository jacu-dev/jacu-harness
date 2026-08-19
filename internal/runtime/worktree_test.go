package runtime

import (
	"context"
	"encoding/json"
	"os/exec"
	"strings"
	"testing"
)

func TestWorkTreeBlockNamesCwdOutsideARepository(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	result, blocked := WorkTreeBlock(context.Background(), root)
	if !blocked {
		t.Fatal("expected a work-tree block outside a repository")
	}
	if result.Status != "blocked" {
		t.Fatalf("status = %q; want blocked", result.Status)
	}
	if !strings.Contains(result.Summary, root) {
		t.Fatalf("summary %q does not name the cwd", result.Summary)
	}
	if !strings.Contains(strings.ToLower(result.Summary), "git") {
		t.Fatalf("summary %q does not instruct anchoring to a git work tree", result.Summary)
	}
	if len(result.NextActions) == 0 {
		t.Fatal("blocked envelope is missing a corrective next action")
	}
}

func TestRequireWorkTreeAllowsAGitWorkTree(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	run := exec.Command("git", "init")
	run.Dir = root
	if out, err := run.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	called := false
	handler := RequireWorkTree(root, func(context.Context, json.RawMessage) (Result, error) {
		called = true
		return Result{Status: "ok", Summary: "ran"}, nil
	})
	result, err := handler(context.Background(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("handler was not called inside a work tree")
	}
	if result.Status != "ok" {
		t.Fatalf("status = %q; want ok", result.Status)
	}
}

func TestRequireWorkTreeDoesNotCallNextOutsideAWorkTree(t *testing.T) {
	t.Parallel()
	handler := RequireWorkTree(t.TempDir(), func(context.Context, json.RawMessage) (Result, error) {
		t.Fatal("next handler must not run outside a work tree")
		return Result{}, nil
	})
	result, err := handler(context.Background(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "blocked" {
		t.Fatalf("status = %q; want blocked", result.Status)
	}
}
