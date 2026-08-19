package cleanexit

import (
	"github.com/jacu-dev/jacu-harness/internal/testgit"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestDetectReportsOrphanedLockedWorktree(t *testing.T) {
	project := t.TempDir()
	runGit(t, project, "init", "-q")
	runGit(t, project, "config", "user.email", "cleanexit@example.invalid")
	runGit(t, project, "config", "user.name", "Clean Exit")
	if err := osWrite(filepath.Join(project, "README"), "fixture\n"); err != nil {
		t.Fatal(err)
	}
	runGit(t, project, "add", "README")
	runGit(t, project, "commit", "-qm", "fixture")
	orphan := filepath.Join(t.TempDir(), "orphan")
	runGit(t, project, "worktree", "add", "--detach", orphan, "HEAD")
	runGit(t, project, "worktree", "lock", orphan, "--reason", "fixture")

	report := Detect(project)
	for _, finding := range report.Findings {
		if finding.Class == "worktree" && strings.Contains(finding.Target, "orphan") {
			if !finding.Locked {
				t.Fatal("orphaned worktree finding lost its lock state")
			}
			return
		}
	}
	t.Fatalf("orphaned locked worktree not reported: %+v", report)
}

func osWrite(path, content string) error {
	return os.WriteFile(path, []byte(content), 0o600)
}

func runGit(t *testing.T, repo string, args ...string) {
	t.Helper()
	// #nosec G204 -- test helper invokes fixed git against a test-owned repository.
	command := exec.Command("git", append([]string{"-C", repo}, args...)...)
	command.Env = testgit.Env()
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", args, err, output)
	}
}
