package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jacu-dev/jacu-harness/internal/capability/workspace"
	"github.com/jacu-dev/jacu-harness/internal/gitx"
	"github.com/jacu-dev/jacu-harness/internal/testgit"
)

func TestDeliverRefusesDirtyOrNonIntegrationBranch(t *testing.T) {
	repo := newDeliverRepo(t)
	pushed := 0
	oldPush := deliverPush
	oldPR := deliverCreatePR
	defer func() {
		deliverPush = oldPush
		deliverCreatePR = oldPR
	}()
	deliverPush = func(context.Context, *gitx.Git, string, string) error {
		pushed++
		return nil
	}
	deliverCreatePR = func(context.Context, []string) (string, error) {
		t.Fatal("gh must not run")
		return "", nil
	}
	var stdout, stderr bytes.Buffer
	if code := runDeliver(repo, nil, &stdout, &stderr); code != 2 || pushed != 0 {
		t.Fatalf("main branch code=%d pushed=%d stderr=%q", code, pushed, stderr.String())
	}
	runDeliverGit(t, repo, "checkout", "-b", "sdd/023")
	if err := os.WriteFile(filepath.Join(repo, "dirty.txt"), []byte("x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	stderr.Reset()
	if code := runDeliver(repo, []string{"--json"}, &stdout, &stderr); code != 2 || pushed != 0 {
		t.Fatalf("dirty tree code=%d pushed=%d stderr=%q", code, pushed, stderr.String())
	}
}

func TestDeliverPushesAndCreatesOnePullRequest(t *testing.T) {
	repo := newDeliverRepo(t)
	runDeliverGit(t, repo, "checkout", "-b", "sdd/023")
	var pushArgs []string
	var prArgs [][]string
	oldPush := deliverPush
	oldPR := deliverCreatePR
	defer func() {
		deliverPush = oldPush
		deliverCreatePR = oldPR
	}()
	deliverPush = func(_ context.Context, _ *gitx.Git, _, branch string) error {
		pushArgs = append(pushArgs, branch)
		return nil
	}
	deliverCreatePR = func(_ context.Context, argv []string) (string, error) {
		prArgs = append(prArgs, append([]string{}, argv...))
		return "https://github.com/jacu-dev/jacu-harness/pull/99", nil
	}
	var stdout, stderr bytes.Buffer
	code := runDeliver(repo, []string{"--base", "main", "--title", "SDD-023", "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("deliver exit=%d stderr=%q", code, stderr.String())
	}
	if len(pushArgs) != 1 || pushArgs[0] != "sdd/023" {
		t.Fatalf("push args = %#v", pushArgs)
	}
	if len(prArgs) != 1 {
		t.Fatalf("gh invocations = %#v", prArgs)
	}
	got := prArgs[0]
	if got[0] != "gh" || got[1] != "pr" || got[2] != "create" {
		t.Fatalf("gh argv = %#v", got)
	}
	for _, arg := range got {
		if arg == "--auto" || arg == "merge" {
			t.Fatalf("deliver armed merge: %#v", got)
		}
	}
	var payload deliverResult
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.URL != "https://github.com/jacu-dev/jacu-harness/pull/99" || payload.Branch != "sdd/023" {
		t.Fatalf("json = %#v", payload)
	}
}

// A program with deliver_at_end reaches delivery through the workspace seam.
// If main stops wiring it, every such program merges locally and then warns
// "deliver is not wired" instead of opening the pull request.
func TestWireAutonomyDeliverGivesWorkspaceTheOnlyPushPath(t *testing.T) {
	wireAutonomyDeliver()
	if !workspace.AutonomyDeliverConfigured() {
		t.Fatal("autonomy delivery is not wired")
	}
	repo := newDeliverRepo(t)
	runDeliverGit(t, repo, "checkout", "-b", "sdd/023")
	var prArgs [][]string
	oldPush := deliverPush
	oldPR := deliverCreatePR
	defer func() {
		deliverPush = oldPush
		deliverCreatePR = oldPR
	}()
	deliverPush = func(context.Context, *gitx.Git, string, string) error { return nil }
	deliverCreatePR = func(_ context.Context, argv []string) (string, error) {
		prArgs = append(prArgs, append([]string{}, argv...))
		return "https://github.com/jacu-dev/jacu-harness/pull/1", nil
	}
	if err := deliverForAutonomy(context.Background(), repo); err != nil {
		t.Fatalf("deliverForAutonomy: %v", err)
	}
	if len(prArgs) != 1 {
		t.Fatalf("gh invocations = %#v", prArgs)
	}
	for _, arg := range prArgs[0] {
		if arg == "--auto" || arg == "merge" {
			t.Fatalf("autonomy armed merge: %#v", prArgs[0])
		}
	}
}

func TestDeliverForAutonomyRefusesOutsideIntegrationBranch(t *testing.T) {
	repo := newDeliverRepo(t)
	oldPush := deliverPush
	defer func() { deliverPush = oldPush }()
	deliverPush = func(context.Context, *gitx.Git, string, string) error {
		t.Fatal("push must not run off an sdd/<NNN> branch")
		return nil
	}
	err := deliverForAutonomy(context.Background(), repo)
	if !errors.Is(err, errDeliverPrecondition) {
		t.Fatalf("err = %v, want precondition", err)
	}
}

func newDeliverRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	runDeliverGit(t, repo, "init")
	runDeliverGit(t, repo, "config", "user.name", "Jacu Test")
	runDeliverGit(t, repo, "config", "user.email", "jacu-test@example.invalid")
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("fixture\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runDeliverGit(t, repo, "add", "README.md")
	runDeliverGit(t, repo, "commit", "-m", "initial")
	return repo
}

func runDeliverGit(t *testing.T, repo string, args ...string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", args...) // #nosec G204 -- tests pin git and argv.
	cmd.Dir = repo
	cmd.Env = testgit.Env()
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, output)
	}
}
