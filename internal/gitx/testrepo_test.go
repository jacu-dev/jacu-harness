package gitx

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jacu-dev/jacu-harness/internal/testgit"
)

func newTestRepo(t *testing.T) string {
	t.Helper()
	repo := newEmptyTestRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("fixture\n"), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	runTestGit(t, repo, "add", "README.md")
	runTestGit(t, repo, "commit", "-m", "initial commit")
	return repo
}

func newEmptyTestRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	runTestGit(t, repo, "init")
	runTestGit(t, repo, "config", "user.name", "Jacu Test")
	runTestGit(t, repo, "config", "user.email", "jacu-test@example.invalid")
	return repo
}

func runTestGit(t *testing.T, repo string, args ...string) string {
	t.Helper()
	return strings.TrimSpace(runTestGitRaw(t, repo, args...))
}

func runTestGitRaw(t *testing.T, repo string, args ...string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", args...) // #nosec G204 -- fixed git binary and test-controlled arguments.
	cmd.Dir = repo
	cmd.Env = testgit.Env()
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, output)
	}
	return string(output)
}
