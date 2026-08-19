package verify

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jacu-dev/jacu-harness/internal/runstate"
)

func TestRunCommandExecutesInsideTheRunWorktree(t *testing.T) {
	root, worktree := seedRun(t, "run_6666666666666666", nil, runstate.StatusOpen)

	result := RunCommand(context.Background(), root, RunCommandInput{
		RunID: "run_6666666666666666",
		ArgV:  []string{"touch", "diagnostic"},
	})
	if result.Status != "ok" || result.Data.Verdict != VerdictPass {
		t.Fatalf("status = %q verdict = %q (%s)", result.Status, result.Data.Verdict, result.Summary)
	}
	if _, err := os.Stat(filepath.Join(worktree, "diagnostic")); err != nil {
		t.Fatalf("the command did not run in the worktree: %v", err)
	}
}

// The ad-hoc door faces the same allowlist as verification. If it did not, it
// would be execute_shell wearing a different name.
func TestRunCommandFacesTheSameAllowlist(t *testing.T) {
	root, worktree := seedRun(t, "run_7777777777777777", nil, runstate.StatusOpen)

	for _, tt := range []struct {
		name string
		argv []string
		want string
	}{
		{name: "shell", argv: []string{"sh", "-c", "touch pwned"}, want: "shell invocation blocked"},
		{name: "not allowlisted", argv: []string{"curl", "https://example.invalid"}, want: "command not in allowlist"},
		{name: "metachar", argv: []string{"printf", "a; touch pwned"}, want: "shell metachar in arg"},
		{name: "absolute path", argv: []string{"touch", "/tmp/pwned"}, want: "absolute path in arg blocked"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			result := RunCommand(context.Background(), root, RunCommandInput{
				RunID: "run_7777777777777777", ArgV: tt.argv,
			})
			if result.Status != "blocked" || !strings.Contains(result.Summary, tt.want) {
				t.Fatalf("status = %q summary = %q; want blocked with %q", result.Status, result.Summary, tt.want)
			}
			if _, err := os.Stat(filepath.Join(worktree, "pwned")); !os.IsNotExist(err) {
				t.Fatalf("a refused command still ran (stat err = %v)", err)
			}
		})
	}
}

func TestRunCommandBlocksWithoutAnOpenRun(t *testing.T) {
	root, _ := seedRun(t, "run_8888888888888888", nil, runstate.StatusDiscarded)
	for _, runID := range []string{"run_9999999999999999", "../escape", "run_8888888888888888"} {
		result := RunCommand(context.Background(), root, RunCommandInput{
			RunID: runID, ArgV: []string{"printf", "x"},
		})
		if result.Status != "blocked" {
			t.Fatalf("run %q: status = %q; want blocked", runID, result.Status)
		}
	}
}
