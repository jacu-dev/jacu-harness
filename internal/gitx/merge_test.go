package gitx

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMergeFFOnlyAdvancesBranchWithoutMergeCommit(t *testing.T) {
	repo := newTestRepo(t)
	runTestGit(t, repo, "checkout", "-b", "sdd/023")
	runTestGit(t, repo, "checkout", "-b", "jacu/run-ff")
	if err := os.WriteFile(filepath.Join(repo, "ff.txt"), []byte("ff\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runTestGit(t, repo, "add", "ff.txt")
	runTestGit(t, repo, "commit", "-m", "run commit")
	runSHA := runTestGit(t, repo, "rev-parse", "HEAD")
	runTestGit(t, repo, "checkout", "sdd/023")
	git, err := New()
	if err != nil {
		t.Fatal(err)
	}
	if err := git.MergeFFOnly(context.Background(), repo, "jacu/run-ff"); err != nil {
		t.Fatalf("MergeFFOnly: %v", err)
	}
	head := runTestGit(t, repo, "rev-parse", "HEAD")
	if head != runSHA {
		t.Fatalf("HEAD = %s; want run SHA %s", head, runSHA)
	}
	parents := runTestGit(t, repo, "rev-list", "--parents", "-n", "1", "HEAD")
	if len(strings.Fields(parents)) != 2 {
		t.Fatalf("ff merge produced extra parents: %q", parents)
	}
}

func TestMergeNoFFCreatesOneMergeCommitWhenDiverged(t *testing.T) {
	repo := newTestRepo(t)
	runTestGit(t, repo, "checkout", "-b", "sdd/023")
	runTestGit(t, repo, "checkout", "-b", "jacu/run-div")
	if err := os.WriteFile(filepath.Join(repo, "run.txt"), []byte("run\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runTestGit(t, repo, "add", "run.txt")
	runTestGit(t, repo, "commit", "-m", "run side")
	runTestGit(t, repo, "checkout", "sdd/023")
	if err := os.WriteFile(filepath.Join(repo, "base.txt"), []byte("base\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runTestGit(t, repo, "add", "base.txt")
	runTestGit(t, repo, "commit", "-m", "integration side")
	git, err := New()
	if err != nil {
		t.Fatal(err)
	}
	if err := git.MergeFFOnly(context.Background(), repo, "jacu/run-div"); !errors.Is(err, ErrNotFastForward) {
		t.Fatalf("MergeFFOnly = %v; want ErrNotFastForward", err)
	}
	if err := git.MergeNoFF(context.Background(), repo, "jacu/run-div"); err != nil {
		t.Fatalf("MergeNoFF: %v", err)
	}
	parents := runTestGit(t, repo, "rev-list", "--parents", "-n", "1", "HEAD")
	if len(strings.Fields(parents)) != 3 {
		t.Fatalf("want one merge commit with two parents, got %q", parents)
	}
}

func TestMergeNoFFConflictThenAbortLeavesCleanTree(t *testing.T) {
	repo := newTestRepo(t)
	runTestGit(t, repo, "checkout", "-b", "sdd/023")
	runTestGit(t, repo, "checkout", "-b", "jacu/run-conflict")
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("run\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runTestGit(t, repo, "add", "README.md")
	runTestGit(t, repo, "commit", "-m", "run rewrite")
	runTestGit(t, repo, "checkout", "sdd/023")
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("integration\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runTestGit(t, repo, "add", "README.md")
	runTestGit(t, repo, "commit", "-m", "integration rewrite")
	git, err := New()
	if err != nil {
		t.Fatal(err)
	}
	if mergeErr := git.MergeNoFF(context.Background(), repo, "jacu/run-conflict"); !errors.Is(mergeErr, ErrMergeConflict) {
		t.Fatalf("MergeNoFF = %v; want ErrMergeConflict", mergeErr)
	}
	if abortErr := git.MergeAbort(context.Background(), repo); abortErr != nil {
		t.Fatalf("MergeAbort: %v", abortErr)
	}
	clean, err := git.IsClean(context.Background(), repo)
	if err != nil || !clean {
		t.Fatalf("IsClean = %v err %v; want clean after abort", clean, err)
	}
}

func TestCurrentBranchErrorsOnDetachedHEAD(t *testing.T) {
	repo := newTestRepo(t)
	git, err := New()
	if err != nil {
		t.Fatal(err)
	}
	name, err := git.CurrentBranch(context.Background(), repo)
	if err != nil || name == "" {
		t.Fatalf("CurrentBranch on named branch = %q err %v", name, err)
	}
	runTestGit(t, repo, "checkout", "--detach")
	if _, err := git.CurrentBranch(context.Background(), repo); !errors.Is(err, ErrDetachedHEAD) {
		t.Fatalf("detached CurrentBranch = %v; want ErrDetachedHEAD", err)
	}
}

func TestIsCleanFalseWithUntrackedFile(t *testing.T) {
	repo := newTestRepo(t)
	git, err := New()
	if err != nil {
		t.Fatal(err)
	}
	clean, err := git.IsClean(context.Background(), repo)
	if err != nil || !clean {
		t.Fatalf("clean fixture IsClean = %v err %v", clean, err)
	}
	if writeErr := os.WriteFile(filepath.Join(repo, "scratch.txt"), []byte("x\n"), 0o600); writeErr != nil {
		t.Fatal(writeErr)
	}
	clean, err = git.IsClean(context.Background(), repo)
	if err != nil || clean {
		t.Fatalf("untracked IsClean = %v err %v; want dirty", clean, err)
	}
}

// The autonomy checkout always carries untracked JACU state, so the local merge
// asks about tracked changes only. If this ever answers like IsClean, every
// integration escalates instead of merging.
func TestHasTrackedChangesIgnoresUntrackedAndSeesModified(t *testing.T) {
	repo := newTestRepo(t)
	git, err := New()
	if err != nil {
		t.Fatal(err)
	}
	if writeErr := os.WriteFile(filepath.Join(repo, "scratch.txt"), []byte("x\n"), 0o600); writeErr != nil {
		t.Fatal(writeErr)
	}
	dirty, err := git.HasTrackedChanges(context.Background(), repo)
	if err != nil || dirty {
		t.Fatalf("untracked HasTrackedChanges = %v err %v; want clean", dirty, err)
	}
	if writeErr := os.WriteFile(filepath.Join(repo, "README.md"), []byte("edited\n"), 0o600); writeErr != nil {
		t.Fatal(writeErr)
	}
	dirty, err = git.HasTrackedChanges(context.Background(), repo)
	if err != nil || !dirty {
		t.Fatalf("modified HasTrackedChanges = %v err %v; want dirty", dirty, err)
	}
}
