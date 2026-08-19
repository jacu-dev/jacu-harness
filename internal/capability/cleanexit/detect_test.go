package cleanexit

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/jacu-dev/jacu-harness/internal/runstate"
	"github.com/jacu-dev/jacu-harness/internal/testgit"
)

func TestDetectClassifiesEveryFailureClass(t *testing.T) {
	tests := []struct {
		name  string
		class string
		setup func(t *testing.T, project string)
	}{
		{name: "branch local", class: "branch_local", setup: func(t *testing.T, project string) { gitFixture(t, project, "checkout", "-b", "feature") }},
		{name: "branch remote", class: "branch_remote", setup: func(t *testing.T, project string) {
			gitFixture(t, project, "checkout", "-b", "feature")
			gitFixture(t, project, "update-ref", "refs/remotes/origin/feature", "HEAD")
		}},
		{name: "worktree", class: "worktree", setup: func(t *testing.T, project string) {
			gitFixture(t, project, "worktree", "add", "--detach", filepath.Join(t.TempDir(), "unreferenced"))
		}},
		{name: "untracked", class: "untracked", setup: func(t *testing.T, project string) {
			if err := os.WriteFile(filepath.Join(project, "user-file"), []byte("preserve"), 0o600); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "stash", class: "stash", setup: func(t *testing.T, project string) {
			if err := os.WriteFile(filepath.Join(project, "tracked"), []byte("changed"), 0o600); err != nil {
				t.Fatal(err)
			}
			gitFixture(t, project, "stash", "push", "-m", "fixture")
		}},
		{name: "open run", class: "run_open", setup: func(t *testing.T, project string) {
			run := runstate.Run{SchemaVersion: runstate.CurrentSchemaVersion, RunID: "run_0123456789abcdef", Status: runstate.StatusOpen, CreatedAt: time.Now().UTC()}
			if err := runstate.Save(project, run); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "main mismatch", class: "main_mismatch", setup: func(t *testing.T, project string) {
			if err := os.RemoveAll(filepath.Join(project, ".git")); err != nil {
				t.Fatal(err)
			}
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			project := testRepository(t)
			test.setup(t, project)
			report := Detect(project)
			if report.Verdict != "fail" || !hasFinding(report, test.class) {
				t.Fatalf("report = %+v; want failure class %q", report, test.class)
			}
		})
	}
}

func TestDetectCleanRepositoryPasses(t *testing.T) {
	if report := Detect(testRepository(t)); report.Verdict != "pass" || len(report.Findings) != 0 {
		t.Fatalf("clean repository report = %+v", report)
	}
}

func TestDetectUnknownStateDoesNotPanic(t *testing.T) {
	report := Detect("/path/that/does/not/exist")
	if report.Verdict != "fail" || !hasFinding(report, "main_mismatch") {
		t.Fatalf("unknown state = %+v", report)
	}
}

func TestDetectFailsClosedWhenGitStateCannotBeRead(t *testing.T) {
	project := t.TempDir()
	if err := os.Mkdir(filepath.Join(project, ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	report := Detect(project)
	if report.Verdict != "fail" || len(report.Findings) != 1 || report.Findings[0].Class != "main_mismatch" {
		t.Fatalf("unavailable git state = %+v; want one main_mismatch failure", report)
	}
}

func testRepository(t *testing.T) string {
	t.Helper()
	project := t.TempDir()
	gitFixture(t, project, "init", "--initial-branch=main")
	gitFixture(t, project, "config", "user.email", "test@example.com")
	gitFixture(t, project, "config", "user.name", "Clean Exit Test")
	if err := os.WriteFile(filepath.Join(project, "tracked"), []byte("initial"), 0o600); err != nil {
		t.Fatal(err)
	}
	gitFixture(t, project, "add", "tracked")
	gitFixture(t, project, "commit", "-m", "fixture")
	return project
}

func gitFixture(t *testing.T, project string, args ...string) {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", project}, args...)...) //nolint:gosec // fixed git argv against test-owned repository
	command.Env = testgit.Env()
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
}

func hasFinding(report Report, want string) bool {
	for _, finding := range report.Findings {
		if finding.Class == want {
			return true
		}
	}
	return false
}
