package workspace

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jacu-dev/jacu-harness/internal/runstate"
	"github.com/jacu-dev/jacu-harness/internal/testgit"
)

func TestAutonomyIntegrationCommandsHaveNoProductionTagMutation(t *testing.T) {
	source, err := os.ReadFile("autonomy_integration.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(source)
	for _, needle := range []string{`"gh"`, "pr create", "pr merge", "push", "--auto", "WatchCheckEvidence"} {
		if strings.Contains(text, needle) {
			t.Fatalf("integration path still contains %q", needle)
		}
	}
}

func TestAutonomyIntegrationEscalatesOnMergeConflict(t *testing.T) {
	repo := newWorkspaceGitRepo(t)
	checkoutBranch(t, repo, "sdd/023")
	createRunRewrite(t, repo, "jacu/run-conflict", "README.md", "run\n")
	rewriteFile(t, repo, "README.md", "integration\n", "integration rewrite")
	result := integrateAutonomy(context.Background(), repo, "jacu/run-conflict", "objective", "sha256:diff", "sha256:evidence", "receipt.json")
	if !result.Escalated || !result.PreserveWorktree || !strings.Contains(result.Warning, "conflict") {
		t.Fatalf("integration result = %#v; want conflict escalation", result)
	}
	status := runWorkspaceGit(t, repo, "status", "--porcelain")
	if status != "" {
		t.Fatalf("checkout dirty after abort: %q", status)
	}
}

func TestAutonomyProgramMergesLocallyWithoutRemote(t *testing.T) {
	repo := newWorkspaceGitRepo(t)
	checkoutBranch(t, repo, "sdd/023")
	createRunRewrite(t, repo, "jacu/run-one", "one.txt", "one\n")
	createRunRewrite(t, repo, "jacu/run-two", "two.txt", "two\n")
	runWorkspaceGit(t, repo, "checkout", "sdd/023")
	missions := []ProgramMission{{Index: 0}, {Index: 1, After: []int{0}}}
	branches := []string{"jacu/run-one", "jacu/run-two"}
	states, err := ExecuteProgram(context.Background(), missions, func(index int) (MissionOutcome, error) {
		result := integrateAutonomy(context.Background(), repo, branches[index], "objective", "sha256:diff", "sha256:evidence", "receipt.json")
		if result.Escalated {
			return MissionOutcome{Status: MissionEscalated, Verdict: "blocked", Warnings: []string{result.Warning}}, nil
		}
		return MissionOutcome{Status: MissionApplied, Verdict: "pass", DiffDigest: "sha256:diff", EvidenceDigest: "sha256:evidence"}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if states[0].Status != MissionApplied || states[1].Status != MissionApplied {
		t.Fatalf("states = %#v", states)
	}
	if _, err := os.Stat(filepath.Join(repo, "one.txt")); err != nil {
		t.Fatalf("first merge missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(repo, "two.txt")); err != nil {
		t.Fatalf("second merge missing: %v", err)
	}
	if remotes := runWorkspaceGit(t, repo, "remote"); remotes != "" {
		t.Fatalf("origin was touched: %q", remotes)
	}
}

func TestExecuteProgramWithDeliveryRunsOnceAfterLastPass(t *testing.T) {
	calls := 0
	states, err := ExecuteProgramWithDelivery(context.Background(), []ProgramMission{{Index: 0}, {Index: 1}}, func(int) (MissionOutcome, error) {
		return MissionOutcome{Status: MissionApplied, Verdict: "pass"}, nil
	}, true, func() error {
		calls++
		return nil
	})
	if err != nil || calls != 1 || states[1].Status != MissionApplied {
		t.Fatalf("states=%#v calls=%d err=%v", states, calls, err)
	}
	calls = 0
	_, err = ExecuteProgramWithDelivery(context.Background(), []ProgramMission{{Index: 0}, {Index: 1}}, func(index int) (MissionOutcome, error) {
		if index == 0 {
			return MissionOutcome{Status: MissionEscalated, Verdict: "blocked"}, nil
		}
		return MissionOutcome{Status: MissionApplied, Verdict: "pass"}, nil
	}, true, func() error {
		calls++
		return nil
	})
	if err != nil || calls != 0 {
		t.Fatalf("deliver ran after escalation: calls=%d err=%v", calls, err)
	}
	calls = 0
	_, err = ExecuteProgramWithDelivery(context.Background(), []ProgramMission{{Index: 0}}, func(int) (MissionOutcome, error) {
		return MissionOutcome{Status: MissionApplied, Verdict: "pass"}, nil
	}, false, func() error {
		calls++
		return nil
	})
	if err != nil || calls != 0 {
		t.Fatalf("deliver ran with flag unset: calls=%d err=%v", calls, err)
	}
}

func TestExecuteCompiledProgramHonorsDeliverAtEnd(t *testing.T) {
	calls := 0
	program := &runstate.Program{DeliverAtEnd: true}
	_, err := ExecuteCompiledProgram(context.Background(), program, []ProgramMission{{Index: 0}}, func(int) (MissionOutcome, error) {
		return MissionOutcome{Status: MissionApplied, Verdict: "pass"}, nil
	}, func() error {
		calls++
		return nil
	})
	if err != nil || calls != 1 {
		t.Fatalf("compiled deliver calls=%d err=%v", calls, err)
	}
	calls = 0
	_, err = ExecuteCompiledProgram(context.Background(), &runstate.Program{}, []ProgramMission{{Index: 0}}, func(int) (MissionOutcome, error) {
		return MissionOutcome{Status: MissionApplied, Verdict: "pass"}, nil
	}, func() error {
		calls++
		return nil
	})
	if err != nil || calls != 0 {
		t.Fatalf("compiled without flag calls=%d err=%v", calls, err)
	}
}

func newWorkspaceGitRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	runWorkspaceGit(t, repo, "init")
	runWorkspaceGit(t, repo, "config", "user.name", "Jacu Test")
	runWorkspaceGit(t, repo, "config", "user.email", "jacu-test@example.invalid")
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("fixture\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runWorkspaceGit(t, repo, "add", "README.md")
	runWorkspaceGit(t, repo, "commit", "-m", "initial")
	return repo
}

func checkoutBranch(t *testing.T, repo, branch string) {
	t.Helper()
	runWorkspaceGit(t, repo, "checkout", "-b", branch)
}

func createRunRewrite(t *testing.T, repo, branch, name, content string) {
	t.Helper()
	runWorkspaceGit(t, repo, "checkout", "-b", branch)
	if err := os.WriteFile(filepath.Join(repo, name), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	runWorkspaceGit(t, repo, "add", name)
	runWorkspaceGit(t, repo, "commit", "-m", branch)
	runWorkspaceGit(t, repo, "checkout", "-")
}

func rewriteFile(t *testing.T, repo, name, content, message string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(repo, name), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	runWorkspaceGit(t, repo, "add", name)
	runWorkspaceGit(t, repo, "commit", "-m", message)
}

func runWorkspaceGit(t *testing.T, repo string, args ...string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", args...) // #nosec G204 -- tests pin git and argv.
	cmd.Dir = repo
	cmd.Env = testgit.Env()
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, output)
	}
	return strings.TrimSpace(string(output))
}
