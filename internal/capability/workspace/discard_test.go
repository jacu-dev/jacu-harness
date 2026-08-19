package workspace

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jacu-dev/jacu-harness/internal/capability/missioncompile"
	"github.com/jacu-dev/jacu-harness/internal/gitx"
	"github.com/jacu-dev/jacu-harness/internal/project"
	"github.com/jacu-dev/jacu-harness/internal/runstate"
)

func TestDiscardArchivesRecoverableRawDiffBeforeRemovingDirtyWorktree(t *testing.T) {
	repo := newTestRepo(t)
	t.Setenv("HOME", t.TempDir())
	opened := openTestRun(t, repo)

	if err := os.WriteFile(filepath.Join(opened.Data.WorktreePath, "README.md"), []byte("fixture changed\n"), 0o600); err != nil {
		t.Fatalf("write README: %v", err)
	}
	binaryContent := []byte{0x00, 0x01, 0xff, 0x10, 0x00, 0xfe}
	if err := os.WriteFile(filepath.Join(opened.Data.WorktreePath, "asset.bin"), binaryContent, 0o600); err != nil {
		t.Fatalf("write binary: %v", err)
	}

	git, err := gitx.New()
	if err != nil {
		t.Fatalf("gitx.New: %v", err)
	}
	expectedPatch, err := git.Diff(context.Background(), opened.Data.WorktreePath, opened.Data.BaseSHA)
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}
	if expectedPatch == "" || !strings.Contains(expectedPatch, "GIT binary patch") {
		t.Fatalf("raw diff does not cover dirty binary worktree:\n%s", expectedPatch)
	}
	runGit(t, opened.Data.WorktreePath, "add", "-A")
	expectedTree := runGit(t, opened.Data.WorktreePath, "write-tree")
	runGit(t, opened.Data.WorktreePath, "reset", "--mixed", "--quiet", "HEAD")

	result, err := Discard(context.Background(), repo, DiscardInput{RunID: opened.Data.RunID})
	if err != nil {
		t.Fatalf("Discard: %v", err)
	}
	if result.Status != "ok" || len(result.Data.Runs) != 1 {
		t.Fatalf("result = %#v", result)
	}
	discarded := result.Data.Runs[0]
	if discarded.RunID != opened.Data.RunID || discarded.ArchivePatch == "" {
		t.Fatalf("discarded run = %#v", discarded)
	}
	if result.Warnings == nil || result.NextActions == nil || discarded.Actions == nil {
		t.Fatalf("output slices must be non-nil: %#v", result)
	}
	// #nosec G304 -- the archive path is produced by Discard inside the temporary repository.
	archived, archiveReadErr := os.ReadFile(filepath.Join(repo, discarded.ArchivePatch))
	if archiveReadErr != nil {
		t.Fatalf("read archive: %v", archiveReadErr)
	}
	if string(archived) != expectedPatch {
		t.Fatalf("archive differs from raw gitx diff\nwant:\n%s\ngot:\n%s", expectedPatch, archived)
	}
	if _, statErr := os.Stat(opened.Data.WorktreePath); !os.IsNotExist(statErr) {
		t.Fatalf("worktree still exists or stat failed: %v", statErr)
	}

	recovery := filepath.Join(t.TempDir(), "recovery")
	runGit(t, repo, "clone", "--no-local", repo, recovery)
	runGit(t, recovery, "checkout", "--detach", opened.Data.BaseSHA)
	runGit(t, recovery, "apply", "--index", "--binary", filepath.Join(repo, discarded.ArchivePatch))
	if got := runGit(t, recovery, "write-tree"); got != expectedTree {
		t.Fatalf("recovered tree = %s; want %s", got, expectedTree)
	}
	// #nosec G304 -- recovery is a test-owned directory under t.TempDir.
	recoveredBinary, recoveryReadErr := os.ReadFile(filepath.Join(recovery, "asset.bin"))
	if recoveryReadErr != nil {
		t.Fatalf("read recovered binary: %v", recoveryReadErr)
	}
	if !reflect.DeepEqual(recoveredBinary, binaryContent) {
		t.Fatalf("recovered binary = %v; want %v", recoveredBinary, binaryContent)
	}

	state, err := runstate.Load(repo, opened.Data.RunID)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if state.Status != runstate.StatusDiscarded || state.ArchivePatch != discarded.ArchivePatch {
		t.Fatalf("state = %#v", state)
	}
}

func TestDiscardCleanRunCreatesNoPatch(t *testing.T) {
	repo := newTestRepo(t)
	t.Setenv("HOME", t.TempDir())
	opened := openTestRun(t, repo)

	result, err := Discard(context.Background(), repo, DiscardInput{RunID: opened.Data.RunID})
	if err != nil {
		t.Fatalf("Discard: %v", err)
	}
	if result.Status != "ok" || len(result.Data.Runs) != 1 {
		t.Fatalf("result = %#v", result)
	}
	discarded := result.Data.Runs[0]
	if discarded.ArchivePatch != "" {
		t.Fatalf("archive_patch = %q; want empty", discarded.ArchivePatch)
	}
	archive := filepath.Join(repo, ".git", "jacu", "archive", opened.Data.RunID+".patch")
	if _, archiveStatErr := os.Stat(archive); !os.IsNotExist(archiveStatErr) {
		t.Fatalf("clean discard archive exists or stat failed: %v", archiveStatErr)
	}
	if _, worktreeStatErr := os.Stat(opened.Data.WorktreePath); !os.IsNotExist(worktreeStatErr) {
		t.Fatalf("worktree still exists or stat failed: %v", worktreeStatErr)
	}
	if branches := runGit(t, repo, "branch", "--list", opened.Data.Branch); branches != "" {
		t.Fatalf("discarded branch still exists: %q", branches)
	}
	state, err := runstate.Load(repo, opened.Data.RunID)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if state.Status != runstate.StatusDiscarded || state.ArchivePatch != "" {
		t.Fatalf("state = %#v", state)
	}
}

func TestDiscardRetryRecapturesCurrentPatchBeforeRemovingWorktree(t *testing.T) {
	repo := newTestRepo(t)
	t.Setenv("HOME", t.TempDir())
	opened := openTestRun(t, repo)
	if err := os.WriteFile(filepath.Join(opened.Data.WorktreePath, "README.md"), []byte("dirty recovery\n"), 0o600); err != nil {
		t.Fatalf("write dirty worktree: %v", err)
	}
	realGit, err := exec.LookPath("git")
	if err != nil {
		t.Fatalf("git executable: %v", err)
	}
	wrapperDir := t.TempDir()
	marker := filepath.Join(t.TempDir(), "worktree-remove-failed")
	wrapper := filepath.Join(wrapperDir, "git")
	script := fmt.Sprintf(`#!/bin/sh
if [ "$1" = "worktree" ] && [ "$2" = "remove" ] && [ ! -f %q ]; then
  : > %q
  exit 91
fi
exec %q "$@"
`, marker, marker, realGit)
	// #nosec G306 -- the test wrapper must be executable by the test process.
	if writeErr := os.WriteFile(wrapper, []byte(script), 0o700); writeErr != nil {
		t.Fatalf("write git wrapper: %v", writeErr)
	}
	t.Setenv("PATH", wrapperDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	_, discardErr := Discard(context.Background(), repo, DiscardInput{RunID: opened.Data.RunID})
	if discardErr == nil || !strings.Contains(discardErr.Error(), "worktree remove") {
		t.Fatalf("first Discard error = %v; want injected worktree remove failure", discardErr)
	}
	wantArchive := filepath.Join(".git", "jacu", "archive", opened.Data.RunID+".patch")
	stateAfterFailure, loadFailureErr := runstate.Load(repo, opened.Data.RunID)
	if loadFailureErr != nil {
		t.Fatalf("Load after failure: %v", loadFailureErr)
	}
	if stateAfterFailure.Status != runstate.StatusOpen || stateAfterFailure.ArchivePatch != wantArchive {
		t.Fatalf("state after failure = %#v; want open with archive %q", stateAfterFailure, wantArchive)
	}
	// #nosec G304 -- wantArchive is the canonical path produced for this temporary run.
	archivedBeforeRetry, readBeforeErr := os.ReadFile(filepath.Join(repo, wantArchive))
	if readBeforeErr != nil {
		t.Fatalf("read archive before retry: %v", readBeforeErr)
	}
	if len(archivedBeforeRetry) == 0 {
		t.Fatal("archive before retry is empty")
	}
	if _, statErr := os.Stat(opened.Data.WorktreePath); statErr != nil {
		t.Fatalf("worktree missing after injected pre-remove failure: %v", statErr)
	}
	if writeErr := os.WriteFile(filepath.Join(opened.Data.WorktreePath, "README.md"), []byte("dirty recovery byte!\n"), 0o600); writeErr != nil {
		t.Fatalf("mutate worktree before retry: %v", writeErr)
	}
	runGit(t, opened.Data.WorktreePath, "add", "-A")
	expectedTree := runGit(t, opened.Data.WorktreePath, "write-tree")
	runGit(t, opened.Data.WorktreePath, "reset", "--mixed", "--quiet", "HEAD")

	result, err := Discard(context.Background(), repo, DiscardInput{RunID: opened.Data.RunID})
	if err != nil {
		t.Fatalf("retry Discard: %v", err)
	}
	if len(result.Data.Runs) != 1 || result.Data.Runs[0].ArchivePatch != wantArchive {
		t.Fatalf("retry result = %#v; want preserved archive %q", result, wantArchive)
	}
	stateAfterRetry, err := runstate.Load(repo, opened.Data.RunID)
	if err != nil {
		t.Fatalf("Load after retry: %v", err)
	}
	if stateAfterRetry.Status != runstate.StatusDiscarded || stateAfterRetry.ArchivePatch != wantArchive {
		t.Fatalf("state after retry = %#v", stateAfterRetry)
	}
	// #nosec G304 -- wantArchive is the canonical path produced for this temporary run.
	archivedAfterRetry, err := os.ReadFile(filepath.Join(repo, wantArchive))
	if err != nil {
		t.Fatalf("read archive after retry: %v", err)
	}
	if bytes.Equal(archivedAfterRetry, archivedBeforeRetry) {
		t.Fatal("retry kept a stale recovery archive")
	}
	recovery := filepath.Join(t.TempDir(), "retry-recovery")
	runGit(t, repo, "clone", "--no-local", repo, recovery)
	runGit(t, recovery, "checkout", "--detach", opened.Data.BaseSHA)
	runGit(t, recovery, "apply", "--index", "--binary", filepath.Join(repo, wantArchive))
	if got := runGit(t, recovery, "write-tree"); got != expectedTree {
		t.Fatalf("retry archive recovered tree %s; want removed tree %s", got, expectedTree)
	}
}

func TestDiscardRetryRecreatesUnavailableArchiveWhileWorktreeExists(t *testing.T) {
	for _, test := range []struct {
		name   string
		damage func(*testing.T, string)
	}{
		{
			name: "missing",
			damage: func(t *testing.T, path string) {
				t.Helper()
				if err := os.Remove(path); err != nil {
					t.Fatalf("remove archive: %v", err)
				}
			},
		},
		{
			name: "corrupt",
			damage: func(t *testing.T, path string) {
				t.Helper()
				if err := os.WriteFile(path, []byte("not a git patch"), 0o600); err != nil {
					t.Fatalf("corrupt archive: %v", err)
				}
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			repo := newTestRepo(t)
			t.Setenv("HOME", t.TempDir())
			opened := openTestRun(t, repo)
			if err := os.WriteFile(filepath.Join(opened.Data.WorktreePath, "README.md"), []byte("retry recovery\n"), 0o600); err != nil {
				t.Fatalf("write dirty worktree: %v", err)
			}
			installOneShotGitFailure(t, "worktree", "remove")
			if _, err := Discard(context.Background(), repo, DiscardInput{RunID: opened.Data.RunID}); err == nil {
				t.Fatal("first Discard unexpectedly succeeded")
			}
			state, err := runstate.Load(repo, opened.Data.RunID)
			if err != nil {
				t.Fatalf("Load after failure: %v", err)
			}
			archive := filepath.Join(repo, state.ArchivePatch)
			test.damage(t, archive)
			git, err := gitx.New()
			if err != nil {
				t.Fatalf("gitx.New: %v", err)
			}
			expectedPatch, err := git.Diff(context.Background(), opened.Data.WorktreePath, opened.Data.BaseSHA)
			if err != nil {
				t.Fatalf("Diff current worktree: %v", err)
			}

			if _, retryErr := Discard(context.Background(), repo, DiscardInput{RunID: opened.Data.RunID}); retryErr != nil {
				t.Fatalf("retry Discard: %v", retryErr)
			}
			// #nosec G304 -- archive is the canonical path loaded from this test-owned run state.
			got, err := os.ReadFile(archive)
			if err != nil {
				t.Fatalf("read recreated archive: %v", err)
			}
			if string(got) != expectedPatch {
				t.Fatalf("recreated archive differs from current diff\nwant:\n%s\ngot:\n%s", expectedPatch, got)
			}
		})
	}
}

func TestDiscardStateSaveFailureBeforeCleanupRemovesNothing(t *testing.T) {
	repo := newTestRepo(t)
	t.Setenv("HOME", t.TempDir())
	opened := openTestRun(t, repo)
	if err := os.WriteFile(filepath.Join(opened.Data.WorktreePath, "README.md"), []byte("dirty save failure\n"), 0o600); err != nil {
		t.Fatalf("write dirty worktree: %v", err)
	}
	runsDir := filepath.Join(repo, ".git", "jacu", "runs")
	// Discard loads the run before archiving the patch and saves it right after,
	// so blocking on the first "git diff" lands between the two: the load that
	// selects the run succeeds and the save that precedes cleanup fails.
	installOneShotRunsDirBlock(t, "diff", runsDir)
	defer func() { _ = restoreBlockedRunsDir(t, runsDir) }()

	_, discardErr := Discard(context.Background(), repo, DiscardInput{RunID: opened.Data.RunID})
	if restoreErr := restoreBlockedRunsDir(t, runsDir); restoreErr != nil {
		t.Fatalf("restore runs directory: %v", restoreErr)
	}
	if discardErr == nil {
		t.Fatal("Discard succeeded despite the run state directory being unwritable")
	}
	if !strings.Contains(discardErr.Error(), filepath.Join(runsDir, opened.Data.RunID+".json")) {
		t.Fatalf("Discard error = %v; want the failure to name the unwritable run state file", discardErr)
	}
	if _, statErr := os.Stat(opened.Data.WorktreePath); statErr != nil {
		t.Fatalf("worktree changed after pre-cleanup save failure: %v", statErr)
	}
	if branches := runGit(t, repo, "branch", "--list", opened.Data.Branch); branches == "" {
		t.Fatal("branch removed after pre-cleanup save failure")
	}
	state, err := runstate.Load(repo, opened.Data.RunID)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if state.Status != runstate.StatusOpen || state.ArchivePatch != "" {
		t.Fatalf("state changed after failed save: %#v", state)
	}
	cleanupWorktree(t, repo, opened.Data.WorktreePath)
	runGit(t, repo, "branch", "-D", opened.Data.Branch)
}

func TestDiscardRejectsTraversalRunIDBeforeLoadingState(t *testing.T) {
	repo := newTestRepo(t)
	t.Setenv("HOME", t.TempDir())
	opened := openTestRun(t, repo)
	traversal := filepath.Join("..", "runs", opened.Data.RunID)

	_, err := Discard(context.Background(), repo, DiscardInput{RunID: traversal})
	if err == nil || !strings.Contains(err.Error(), "invalid run_id") {
		t.Fatalf("Discard error = %v; want invalid run_id before Load", err)
	}
	if _, statErr := os.Stat(opened.Data.WorktreePath); statErr != nil {
		t.Fatalf("traversal input touched target worktree: %v", statErr)
	}
	state, err := runstate.Load(repo, opened.Data.RunID)
	if err != nil {
		t.Fatalf("Load target: %v", err)
	}
	if state.Status != runstate.StatusOpen {
		t.Fatalf("traversal input changed target status to %q", state.Status)
	}
	cleanupWorktree(t, repo, opened.Data.WorktreePath)
	runGit(t, repo, "branch", "-D", opened.Data.Branch)
}

func TestDiscardRejectsStateFilenameAliasForAnotherRun(t *testing.T) {
	repo := newTestRepo(t)
	t.Setenv("HOME", t.TempDir())
	opened := openTestRun(t, repo)
	alias := "run_aaaaaaaaaaaaaaaa"
	if alias == opened.Data.RunID {
		alias = "run_bbbbbbbbbbbbbbbb"
	}
	// #nosec G304 -- opened is a run created inside the temporary repository.
	content, readErr := os.ReadFile(filepath.Join(repo, ".git", "jacu", "runs", opened.Data.RunID+".json"))
	if readErr != nil {
		t.Fatalf("read canonical state: %v", readErr)
	}
	// #nosec G703 -- alias is a fixed test fixture under the temporary repository.
	if writeErr := os.WriteFile(filepath.Join(repo, ".git", "jacu", "runs", alias+".json"), content, 0o600); writeErr != nil {
		t.Fatalf("write alias state: %v", writeErr)
	}

	_, discardErr := Discard(context.Background(), repo, DiscardInput{RunID: alias})
	if discardErr == nil || !strings.Contains(discardErr.Error(), "does not match requested") {
		t.Fatalf("Discard error = %v; want filename/internal ID mismatch", discardErr)
	}
	if _, statErr := os.Stat(opened.Data.WorktreePath); statErr != nil {
		t.Fatalf("alias input touched canonical worktree: %v", statErr)
	}
	state, err := runstate.Load(repo, opened.Data.RunID)
	if err != nil {
		t.Fatalf("Load canonical state: %v", err)
	}
	if state.Status != runstate.StatusOpen {
		t.Fatalf("alias input changed canonical status to %q", state.Status)
	}
	cleanupWorktree(t, repo, opened.Data.WorktreePath)
	runGit(t, repo, "branch", "-D", opened.Data.Branch)
}

func TestDiscardRejectsDetachedRegisteredWorktree(t *testing.T) {
	repo := newTestRepo(t)
	t.Setenv("HOME", t.TempDir())
	opened := openTestRun(t, repo)
	runGit(t, opened.Data.WorktreePath, "checkout", "--detach")

	_, err := Discard(context.Background(), repo, DiscardInput{RunID: opened.Data.RunID})
	if err == nil || !strings.Contains(err.Error(), "worktree branch") {
		t.Fatalf("Discard error = %v; want detached branch identity rejection", err)
	}
	if _, err := os.Stat(opened.Data.WorktreePath); err != nil {
		t.Fatalf("detached worktree was changed: %v", err)
	}
	cleanupWorktree(t, repo, opened.Data.WorktreePath)
	runGit(t, repo, "branch", "-D", opened.Data.Branch)
}

func TestDiscardRejectsCrossRunWorktreeAndBranchMetadata(t *testing.T) {
	repo := newTestRepo(t)
	t.Setenv("HOME", t.TempDir())
	first := openTestRun(t, repo)
	second := openTestRun(t, repo)
	state, loadErr := runstate.Load(repo, first.Data.RunID)
	if loadErr != nil {
		t.Fatalf("Load first: %v", loadErr)
	}
	state.Worktree = second.Data.WorktreePath
	state.Branch = second.Data.Branch
	if saveErr := runstate.Save(repo, state); saveErr != nil {
		t.Fatalf("Save crossed state: %v", saveErr)
	}

	_, discardErr := Discard(context.Background(), repo, DiscardInput{RunID: first.Data.RunID})
	if discardErr == nil || !strings.Contains(discardErr.Error(), "identity") {
		t.Fatalf("Discard error = %v; want cross-run identity rejection", discardErr)
	}
	for _, opened := range []OpenResult{first, second} {
		if _, err := os.Stat(opened.Data.WorktreePath); err != nil {
			t.Fatalf("cross-run metadata touched %s: %v", opened.Data.RunID, err)
		}
		if branches := runGit(t, repo, "branch", "--list", opened.Data.Branch); branches == "" {
			t.Fatalf("cross-run metadata deleted branch %s", opened.Data.Branch)
		}
		cleanupWorktree(t, repo, opened.Data.WorktreePath)
		runGit(t, repo, "branch", "-D", opened.Data.Branch)
	}
}

func TestDiscardRetryFailsClosedWhenPromisedArchiveIsMissing(t *testing.T) {
	repo := newTestRepo(t)
	t.Setenv("HOME", t.TempDir())
	opened := openTestRun(t, repo)
	if err := os.WriteFile(filepath.Join(opened.Data.WorktreePath, "README.md"), []byte("promised recovery\n"), 0o600); err != nil {
		t.Fatalf("write dirty worktree: %v", err)
	}
	installOneShotGitFailure(t, "branch", "-D")

	_, err := Discard(context.Background(), repo, DiscardInput{RunID: opened.Data.RunID})
	if err == nil || !strings.Contains(err.Error(), "branch -D") {
		t.Fatalf("first Discard error = %v; want branch failure", err)
	}
	state, err := runstate.Load(repo, opened.Data.RunID)
	if err != nil {
		t.Fatalf("Load after failure: %v", err)
	}
	archive := filepath.Join(repo, state.ArchivePatch)
	if removeErr := os.Remove(archive); removeErr != nil {
		t.Fatalf("remove promised archive fixture: %v", removeErr)
	}

	_, err = Discard(context.Background(), repo, DiscardInput{RunID: opened.Data.RunID})
	if err == nil || !strings.Contains(err.Error(), "archive") {
		t.Fatalf("retry error = %v; want missing archive failure", err)
	}
	if branches := runGit(t, repo, "branch", "--list", opened.Data.Branch); branches == "" {
		t.Fatal("retry deleted branch without promised recovery")
	}
	state, err = runstate.Load(repo, opened.Data.RunID)
	if err != nil {
		t.Fatalf("Load after retry: %v", err)
	}
	if state.Status != runstate.StatusOpen {
		t.Fatalf("status after closed retry = %q; want open", state.Status)
	}
	runGit(t, repo, "branch", "-D", opened.Data.Branch)
}

func TestDiscardRetryRejectsNoncanonicalArchiveReference(t *testing.T) {
	repo := newTestRepo(t)
	t.Setenv("HOME", t.TempDir())
	opened := openTestRun(t, repo)
	if err := os.WriteFile(filepath.Join(opened.Data.WorktreePath, "README.md"), []byte("canonical recovery\n"), 0o600); err != nil {
		t.Fatalf("write dirty worktree: %v", err)
	}
	installOneShotGitFailure(t, "branch", "-D")
	_, firstDiscardErr := Discard(context.Background(), repo, DiscardInput{RunID: opened.Data.RunID})
	if firstDiscardErr == nil {
		t.Fatal("first Discard unexpectedly succeeded")
	}
	state, loadErr := runstate.Load(repo, opened.Data.RunID)
	if loadErr != nil {
		t.Fatalf("Load after failure: %v", loadErr)
	}
	// #nosec G304 -- ArchivePatch was produced by Discard for this temporary run.
	canonicalContent, readErr := os.ReadFile(filepath.Join(repo, state.ArchivePatch))
	if readErr != nil {
		t.Fatalf("read canonical archive: %v", readErr)
	}
	tampered := filepath.Join(".git", "jacu", "archive", "other.patch")
	// #nosec G703 -- tampered is a fixed test fixture under the temporary repository.
	if writeErr := os.WriteFile(filepath.Join(repo, tampered), canonicalContent, 0o600); writeErr != nil {
		t.Fatalf("write tampered archive fixture: %v", writeErr)
	}
	state.ArchivePatch = tampered
	if saveErr := runstate.Save(repo, state); saveErr != nil {
		t.Fatalf("Save tampered state: %v", saveErr)
	}

	_, retryErr := Discard(context.Background(), repo, DiscardInput{RunID: opened.Data.RunID})
	if retryErr == nil || !strings.Contains(retryErr.Error(), "archive_patch") {
		t.Fatalf("retry error = %v; want noncanonical archive rejection", retryErr)
	}
	if branches := runGit(t, repo, "branch", "--list", opened.Data.Branch); branches == "" {
		t.Fatal("retry deleted branch with noncanonical recovery reference")
	}
	runGit(t, repo, "branch", "-D", opened.Data.Branch)
}

func TestDiscardRetryRejectsSemanticallyTamperedArchive(t *testing.T) {
	repo := newTestRepo(t)
	t.Setenv("HOME", t.TempDir())
	opened := openTestRun(t, repo)
	if err := os.WriteFile(filepath.Join(opened.Data.WorktreePath, "README.md"), []byte("integrity protected\n"), 0o600); err != nil {
		t.Fatalf("write dirty worktree: %v", err)
	}
	installOneShotGitFailure(t, "branch", "-D")
	if _, err := Discard(context.Background(), repo, DiscardInput{RunID: opened.Data.RunID}); err == nil {
		t.Fatal("first Discard unexpectedly succeeded")
	}
	state, err := runstate.Load(repo, opened.Data.RunID)
	if err != nil {
		t.Fatalf("Load after failure: %v", err)
	}
	if state.ArchiveDigest == "" {
		t.Fatalf("archive integrity metadata missing from state: %#v", state)
	}
	expectedDigest := state.ArchiveDigest
	archive := filepath.Join(repo, state.ArchivePatch)
	recovery := filepath.Join(t.TempDir(), "integrity-recovery")
	runGit(t, repo, "clone", "--no-local", repo, recovery)
	runGit(t, recovery, "checkout", "--detach", opened.Data.BaseSHA)
	runGit(t, recovery, "apply", "--index", "--binary", archive)
	if got := runGit(t, recovery, "show", ":README.md"); got != "integrity protected" {
		t.Fatalf("untampered archive recovered README %q", got)
	}
	// #nosec G304 -- archive is the canonical path produced for this temporary run.
	file, err := os.OpenFile(archive, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatalf("open archive for tamper: %v", err)
	}
	if _, writeErr := file.WriteString("\n# tampered\n"); writeErr != nil {
		_ = file.Close()
		t.Fatalf("tamper archive: %v", writeErr)
	}
	if closeErr := file.Close(); closeErr != nil {
		t.Fatalf("close tampered archive: %v", closeErr)
	}

	_, err = Discard(context.Background(), repo, DiscardInput{RunID: opened.Data.RunID})
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "integrity") {
		t.Fatalf("retry error = %v; want archive integrity failure", err)
	}
	if branches := runGit(t, repo, "branch", "--list", opened.Data.Branch); branches == "" {
		t.Fatal("retry deleted branch after archive integrity failure")
	}
	state, err = runstate.Load(repo, opened.Data.RunID)
	if err != nil {
		t.Fatalf("Load after rejected retry: %v", err)
	}
	if state.Status != runstate.StatusOpen || state.ArchiveDigest != expectedDigest {
		t.Fatalf("state after rejected retry = %#v; want open with digest %q", state, expectedDigest)
	}
	runGit(t, repo, "branch", "-D", opened.Data.Branch)
}

func TestDiscardRetryFailsClosedForLegacyArchiveWithoutIntegrityMetadata(t *testing.T) {
	repo := newTestRepo(t)
	t.Setenv("HOME", t.TempDir())
	opened := openTestRun(t, repo)
	if err := os.WriteFile(filepath.Join(opened.Data.WorktreePath, "README.md"), []byte("legacy recovery\n"), 0o600); err != nil {
		t.Fatalf("write dirty worktree: %v", err)
	}
	installOneShotGitFailure(t, "branch", "-D")
	if _, err := Discard(context.Background(), repo, DiscardInput{RunID: opened.Data.RunID}); err == nil {
		t.Fatal("first Discard unexpectedly succeeded")
	}
	statePath := filepath.Join(repo, ".git", "jacu", "runs", opened.Data.RunID+".json")
	// #nosec G304 -- statePath is under the temporary repository created by this test.
	content, readErr := os.ReadFile(statePath)
	if readErr != nil {
		t.Fatalf("read state: %v", readErr)
	}
	var encoded map[string]any
	if decodeErr := json.Unmarshal(content, &encoded); decodeErr != nil {
		t.Fatalf("decode state: %v", decodeErr)
	}
	delete(encoded, "archive_digest")
	content, encodeErr := json.MarshalIndent(encoded, "", "  ")
	if encodeErr != nil {
		t.Fatalf("encode legacy state: %v", encodeErr)
	}
	content = append(content, '\n')
	if writeErr := os.WriteFile(statePath, content, 0o600); writeErr != nil {
		t.Fatalf("write legacy state: %v", writeErr)
	}

	_, retryErr := Discard(context.Background(), repo, DiscardInput{RunID: opened.Data.RunID})
	if retryErr == nil || !strings.Contains(strings.ToLower(retryErr.Error()), "archive integrity") {
		t.Fatalf("legacy retry error = %v; want missing integrity metadata failure", retryErr)
	}
	if branches := runGit(t, repo, "branch", "--list", opened.Data.Branch); branches == "" {
		t.Fatal("legacy retry deleted branch without integrity metadata")
	}
	runGit(t, repo, "branch", "-D", opened.Data.Branch)
}

func TestArchiveDiffRejectsSymlinkDirectoryAndDestination(t *testing.T) {
	t.Run("archive directory", func(t *testing.T) {
		repo := newTestRepo(t)
		jacuDir := filepath.Join(repo, ".git", "jacu")
		if err := os.MkdirAll(jacuDir, 0o700); err != nil {
			t.Fatalf("mkdir jacu: %v", err)
		}
		victimDir := t.TempDir()
		if err := os.Symlink(victimDir, filepath.Join(jacuDir, "archive")); err != nil {
			t.Fatalf("symlink archive dir: %v", err)
		}

		_, err := archiveDiff(repo, "run_1111111111111111", "patch bytes")
		if err == nil || !strings.Contains(err.Error(), "symlink") {
			t.Fatalf("archiveDiff error = %v; want symlink rejection", err)
		}
		entries, err := os.ReadDir(victimDir)
		if err != nil {
			t.Fatalf("read victim dir: %v", err)
		}
		if len(entries) != 0 {
			t.Fatalf("archive escaped through symlink: %v", entries)
		}
	})

	t.Run("archive destination", func(t *testing.T) {
		repo := newTestRepo(t)
		archiveDir := filepath.Join(repo, ".git", "jacu", "archive")
		if err := os.MkdirAll(archiveDir, 0o700); err != nil {
			t.Fatalf("mkdir archive: %v", err)
		}
		victim := filepath.Join(t.TempDir(), "victim")
		if err := os.WriteFile(victim, []byte("keep me"), 0o600); err != nil {
			t.Fatalf("write victim: %v", err)
		}
		destination := filepath.Join(archiveDir, "run_2222222222222222.patch")
		if err := os.Symlink(victim, destination); err != nil {
			t.Fatalf("symlink destination: %v", err)
		}

		_, err := archiveDiff(repo, "run_2222222222222222", "replacement")
		if err == nil || !strings.Contains(err.Error(), "symlink") {
			t.Fatalf("archiveDiff error = %v; want symlink rejection", err)
		}
		// #nosec G304 -- victim is a fixed fixture under the test's temporary directories.
		content, err := os.ReadFile(victim)
		if err != nil {
			t.Fatalf("read victim: %v", err)
		}
		if string(content) != "keep me" {
			t.Fatalf("victim content = %q; want unchanged", content)
		}
	})
}

func TestArchiveDiffFailurePreservesExistingBytesAndPermissions(t *testing.T) {
	repo := newTestRepo(t)
	archiveDir := filepath.Join(repo, ".git", "jacu", "archive")
	if err := os.MkdirAll(archiveDir, 0o700); err != nil {
		t.Fatalf("mkdir archive: %v", err)
	}
	path := filepath.Join(archiveDir, "run_3333333333333333.patch")
	if err := os.WriteFile(path, []byte("old recovery"), 0o400); err != nil {
		t.Fatalf("write old archive: %v", err)
	}
	// The failure is injected at the last moment before the rename, which is the
	// only window where the new bytes already exist on disk and could still
	// clobber the recoverable archive. A read-only mode cannot be used for this:
	// the suite also runs as root, and root writes straight through it.
	writeFailure := errors.New("simulated archive write failure")
	_, archiveErr := archiveDiffWithHook(repo, "run_3333333333333333", "new recovery", func() error {
		return writeFailure
	})
	if !errors.Is(archiveErr, writeFailure) {
		t.Fatalf("archiveDiff error = %v; want the injected write failure", archiveErr)
	}
	// #nosec G304 -- path is the canonical archive created in the temporary repository.
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read preserved archive: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat preserved archive: %v", err)
	}
	if string(content) != "old recovery" || info.Mode().Perm() != 0o400 {
		t.Fatalf("existing archive changed: content=%q mode=%#o", content, info.Mode().Perm())
	}
	entries, err := os.ReadDir(archiveDir)
	if err != nil {
		t.Fatalf("read archive directory: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != filepath.Base(path) {
		t.Fatalf("archive directory = %v; want only the preserved archive", entries)
	}
}

func TestArchiveDiffRootConfinementRejectsDirectorySwapBeforeRename(t *testing.T) {
	repo := newTestRepo(t)
	runID := "run_6666666666666666"
	relative, archiveErr := archiveDiff(repo, runID, "old recovery")
	if archiveErr != nil {
		t.Fatalf("initial archiveDiff: %v", archiveErr)
	}
	archiveDir := filepath.Dir(filepath.Join(repo, relative))
	movedDir := filepath.Join(filepath.Dir(archiveDir), "archive-moved")
	victimDir := t.TempDir()
	swap := func() error {
		if renameErr := os.Rename(archiveDir, movedDir); renameErr != nil {
			return renameErr
		}
		return os.Symlink(victimDir, archiveDir)
	}

	_, swapErr := archiveDiffWithHook(repo, runID, "new recovery", swap)
	if swapErr == nil || !strings.Contains(strings.ToLower(swapErr.Error()), "identity") {
		t.Fatalf("archiveDiffWithHook error = %v; want directory identity failure", swapErr)
	}
	victimEntries, readErr := os.ReadDir(victimDir)
	if readErr != nil {
		t.Fatalf("read victim dir: %v", readErr)
	}
	if len(victimEntries) != 0 {
		t.Fatalf("root-confined write escaped into victim: %v", victimEntries)
	}
	// #nosec G304 -- movedDir and relative are test-owned archive fixtures.
	preserved, err := os.ReadFile(filepath.Join(movedDir, filepath.Base(relative)))
	if err != nil {
		t.Fatalf("read preserved archive: %v", err)
	}
	if string(preserved) != "old recovery" {
		t.Fatalf("preserved archive = %q; want old recovery", preserved)
	}
	temps, err := filepath.Glob(filepath.Join(movedDir, ".*.tmp"))
	if err != nil {
		t.Fatalf("glob temps: %v", err)
	}
	if len(temps) != 0 {
		t.Fatalf("root-confined temp files remain: %v", temps)
	}
}

func TestDiscardGCCleansLockedOrphanAndPreservesHealthyOpenRun(t *testing.T) {
	repo := newTestRepo(t)
	t.Setenv("HOME", t.TempDir())
	orphan := openTestRun(t, repo)
	healthy := openTestRun(t, repo)
	if err := os.RemoveAll(orphan.Data.WorktreePath); err != nil {
		t.Fatalf("remove orphan fixture: %v", err)
	}

	result, err := Discard(context.Background(), repo, DiscardInput{GC: true})
	if err != nil {
		t.Fatalf("Discard gc: %v", err)
	}
	if result.Status != "ok" || len(result.Data.Runs) != 1 || result.Data.Runs[0].RunID != orphan.Data.RunID {
		t.Fatalf("gc result = %#v", result)
	}
	if !containsAction(result.Data.Runs[0].Actions, "confirmed worktree absent") {
		t.Fatalf("orphan actions = %#v", result.Data.Runs[0].Actions)
	}
	orphanState, err := runstate.Load(repo, orphan.Data.RunID)
	if err != nil {
		t.Fatalf("Load orphan: %v", err)
	}
	if orphanState.Status != runstate.StatusDiscarded {
		t.Fatalf("orphan status = %q; want discarded", orphanState.Status)
	}
	if branches := runGit(t, repo, "branch", "--list", orphan.Data.Branch); branches != "" {
		t.Fatalf("orphan branch still exists: %q", branches)
	}
	healthyState, err := runstate.Load(repo, healthy.Data.RunID)
	if err != nil {
		t.Fatalf("Load healthy: %v", err)
	}
	if healthyState.Status != runstate.StatusOpen {
		t.Fatalf("healthy status = %q; want open", healthyState.Status)
	}
	if _, err := os.Stat(healthy.Data.WorktreePath); err != nil {
		t.Fatalf("healthy worktree changed: %v", err)
	}
	if branches := runGit(t, repo, "branch", "--list", healthy.Data.Branch); branches == "" {
		t.Fatal("healthy branch was removed")
	}
	cleanupWorktree(t, repo, healthy.Data.WorktreePath)
	runGit(t, repo, "branch", "-D", healthy.Data.Branch)
}

func TestDiscardGCHandlesOrphanWithMetadataAndBranchAlreadyAbsent(t *testing.T) {
	repo := newTestRepo(t)
	t.Setenv("HOME", t.TempDir())
	orphan := openTestRun(t, repo)
	if err := os.RemoveAll(orphan.Data.WorktreePath); err != nil {
		t.Fatalf("remove orphan fixture: %v", err)
	}
	runGit(t, repo, "worktree", "unlock", orphan.Data.WorktreePath)
	runGit(t, repo, "worktree", "prune")
	runGit(t, repo, "branch", "-D", orphan.Data.Branch)

	result, err := Discard(context.Background(), repo, DiscardInput{GC: true})
	if err != nil {
		t.Fatalf("Discard gc: %v", err)
	}
	if len(result.Data.Runs) != 1 || result.Data.Runs[0].RunID != orphan.Data.RunID {
		t.Fatalf("result = %#v", result)
	}
	if !containsAction(result.Data.Runs[0].Actions, "branch already absent") {
		t.Fatalf("actions = %#v", result.Data.Runs[0].Actions)
	}
	state, err := runstate.Load(repo, orphan.Data.RunID)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if state.Status != runstate.StatusDiscarded {
		t.Fatalf("status = %q; want discarded", state.Status)
	}
}

func TestDiscardGCPreservesAppliedAndDiscardedRunsWithExpectedAbsentWorktrees(t *testing.T) {
	repo := newTestRepo(t)
	baseSHA := runGit(t, repo, "rev-parse", "HEAD")
	appliedBranch := "jacu/run-applied"
	runGit(t, repo, "branch", appliedBranch)
	applied := runstate.Run{
		RunID:         "run_000000000000000b",
		Status:        runstate.StatusApplied,
		CreatedAt:     time.Now().UTC().Add(-time.Minute),
		BaseSHA:       baseSHA,
		Branch:        appliedBranch,
		Worktree:      filepath.Join(t.TempDir(), "expected-absent-applied"),
		AppliedCommit: baseSHA,
	}
	discarded := runstate.Run{
		RunID:     "run_000000000000000c",
		Status:    runstate.StatusDiscarded,
		CreatedAt: time.Now().UTC(),
		BaseSHA:   baseSHA,
		Branch:    "jacu/run-discarded",
		Worktree:  filepath.Join(t.TempDir(), "expected-absent-discarded"),
	}
	if err := runstate.Save(repo, applied); err != nil {
		t.Fatalf("Save applied: %v", err)
	}
	if err := runstate.Save(repo, discarded); err != nil {
		t.Fatalf("Save discarded: %v", err)
	}

	result, err := Discard(context.Background(), repo, DiscardInput{GC: true})
	if err != nil {
		t.Fatalf("Discard gc: %v", err)
	}
	if len(result.Data.Runs) != 0 {
		t.Fatalf("gc touched terminal runs: %#v", result.Data.Runs)
	}
	if branches := runGit(t, repo, "branch", "--list", appliedBranch); branches == "" {
		t.Fatal("applied branch was removed")
	}
	appliedState, err := runstate.Load(repo, applied.RunID)
	if err != nil {
		t.Fatalf("Load applied: %v", err)
	}
	if appliedState.Status != runstate.StatusApplied {
		t.Fatalf("applied status = %q", appliedState.Status)
	}
}

func TestDiscardGCReportsPartialFailuresAndPreservesCompletedRuns(t *testing.T) {
	repo := newTestRepo(t)
	t.Setenv("HOME", t.TempDir())
	success := openTestRun(t, repo)
	failure := openTestRun(t, repo)
	if err := os.RemoveAll(success.Data.WorktreePath); err != nil {
		t.Fatalf("remove successful orphan fixture: %v", err)
	}
	failureState, loadFailureErr := runstate.Load(repo, failure.Data.RunID)
	if loadFailureErr != nil {
		t.Fatalf("Load failure fixture: %v", loadFailureErr)
	}
	if transitionErr := failureState.Transition(runstate.StatusCorrupted); transitionErr != nil {
		t.Fatalf("mark failure corrupted: %v", transitionErr)
	}
	failureState.Branch = "jacu/run-0000000000000000"
	if saveErr := runstate.Save(repo, failureState); saveErr != nil {
		t.Fatalf("Save failure fixture: %v", saveErr)
	}

	result, err := Discard(context.Background(), repo, DiscardInput{GC: true})
	if err != nil {
		t.Fatalf("Discard gc returned top-level error: %v", err)
	}
	if result.Status != "partial" || !strings.Contains(strings.ToLower(result.Summary), "partial") {
		t.Fatalf("partial result = %#v", result)
	}
	if len(result.Data.Runs) != 1 || result.Data.Runs[0].RunID != success.Data.RunID {
		t.Fatalf("completed runs lost from partial result: %#v", result.Data.Runs)
	}
	if result.Data.Failures == nil || len(result.Data.Failures) != 1 {
		t.Fatalf("typed non-nil failures missing from result data: %#v", result.Data)
	}
	failureItem := result.Data.Failures[0]
	if failureItem.RunID != failure.Data.RunID || failureItem.Error == "" {
		t.Fatalf("failure item = %#v", failureItem)
	}
	if len(result.Warnings) == 0 || !strings.Contains(strings.ToLower(strings.Join(result.Warnings, " ")), "failed") {
		t.Fatalf("partial warnings = %#v", result.Warnings)
	}
	successState, err := runstate.Load(repo, success.Data.RunID)
	if err != nil {
		t.Fatalf("Load successful run: %v", err)
	}
	if successState.Status != runstate.StatusDiscarded {
		t.Fatalf("successful run status = %q", successState.Status)
	}

	retry, err := Discard(context.Background(), repo, DiscardInput{GC: true})
	if err != nil {
		t.Fatalf("retry gc returned top-level error: %v", err)
	}
	if len(retry.Data.Failures) != 1 || retry.Data.Failures[0].RunID != failure.Data.RunID {
		t.Fatalf("failed run not visible on retry: %#v", retry.Data)
	}
	cleanupWorktree(t, repo, failure.Data.WorktreePath)
	runGit(t, repo, "branch", "-D", failure.Data.Branch)
}

func TestDiscardGCReportsPerRunProgressAfterBranchDeleteFailure(t *testing.T) {
	repo := newTestRepo(t)
	t.Setenv("HOME", t.TempDir())
	opened := openTestRun(t, repo)
	if err := os.WriteFile(filepath.Join(opened.Data.WorktreePath, "README.md"), []byte("partial progress\n"), 0o600); err != nil {
		t.Fatalf("write dirty worktree: %v", err)
	}
	state, loadErr := runstate.Load(repo, opened.Data.RunID)
	if loadErr != nil {
		t.Fatalf("Load: %v", loadErr)
	}
	if transitionErr := state.Transition(runstate.StatusCorrupted); transitionErr != nil {
		t.Fatalf("mark corrupted: %v", transitionErr)
	}
	if saveErr := runstate.Save(repo, state); saveErr != nil {
		t.Fatalf("Save corrupted: %v", saveErr)
	}
	installOneShotGitFailure(t, "branch", "-D")

	result, err := Discard(context.Background(), repo, DiscardInput{GC: true})
	if err != nil {
		t.Fatalf("Discard gc: %v", err)
	}
	if result.Status != "partial" || len(result.Data.Runs) != 0 || len(result.Data.Failures) != 1 {
		t.Fatalf("partial result = %#v", result)
	}
	failureProgress := result.Data.Failures[0]
	if failureProgress.ArchivePatch == "" || failureProgress.ArchiveDigest == "" ||
		failureProgress.Actions == nil {
		t.Fatalf("typed partial progress missing: %#v", failureProgress)
	}
	wantArchive := filepath.Join(".git", "jacu", "archive", opened.Data.RunID+".patch")
	if failureProgress.ArchivePatch != wantArchive {
		t.Fatalf("failure archive = %q; want %q", failureProgress.ArchivePatch, wantArchive)
	}
	wantActions := []string{
		"archived current patch to " + wantArchive,
		"unlocked worktree " + opened.Data.WorktreePath,
		"removed worktree via git " + opened.Data.WorktreePath,
	}
	if !reflect.DeepEqual(failureProgress.Actions, wantActions) {
		t.Fatalf("partial actions = %#v; want %#v", failureProgress.Actions, wantActions)
	}
	if _, statErr := os.Stat(opened.Data.WorktreePath); !os.IsNotExist(statErr) {
		t.Fatalf("worktree progress not reflected on disk: %v", statErr)
	}
	state, err = runstate.Load(repo, opened.Data.RunID)
	if err != nil {
		t.Fatalf("Load partial state: %v", err)
	}
	if state.Status != runstate.StatusCorrupted || state.ArchivePatch != wantArchive ||
		state.ArchiveDigest != failureProgress.ArchiveDigest {
		t.Fatalf("partial state = %#v", state)
	}

	retry, err := Discard(context.Background(), repo, DiscardInput{GC: true})
	if err != nil {
		t.Fatalf("retry gc: %v", err)
	}
	if retry.Status != "ok" || len(retry.Data.Runs) != 1 ||
		retry.Data.Runs[0].RunID != opened.Data.RunID ||
		retry.Data.Runs[0].ArchivePatch != wantArchive ||
		retry.Data.Runs[0].ArchiveDigest != failureProgress.ArchiveDigest {
		t.Fatalf("retry result lost partial progress: %#v", retry)
	}
}

func TestDiscardGCReportsUnreadableCorruptedRunAsTypedFailure(t *testing.T) {
	repo := newTestRepo(t)
	runID := "run_5555555555555555"
	runsDir := filepath.Join(repo, ".git", "jacu", "runs")
	if err := os.MkdirAll(runsDir, 0o700); err != nil {
		t.Fatalf("mkdir runs: %v", err)
	}
	if err := os.WriteFile(filepath.Join(runsDir, runID+".json"), []byte(`{"run_id":`), 0o600); err != nil {
		t.Fatalf("write unreadable run: %v", err)
	}

	result, err := Discard(context.Background(), repo, DiscardInput{GC: true})
	if err != nil {
		t.Fatalf("Discard gc: %v", err)
	}
	if result.Status != "partial" || len(result.Data.Failures) != 1 {
		t.Fatalf("unreadable result = %#v", result)
	}
	failure := result.Data.Failures[0]
	if failure.RunID != runID || !strings.Contains(strings.ToLower(failure.Error), "unreadable") {
		t.Fatalf("unreadable failure = %#v", failure)
	}
	if failure.Actions == nil || len(failure.Actions) != 0 {
		t.Fatalf("unreadable failure actions = %#v", failure)
	}
	if len(result.Warnings) == 0 || !strings.Contains(strings.ToLower(strings.Join(result.Warnings, " ")), "unreadable") {
		t.Fatalf("unreadable warnings = %#v", result.Warnings)
	}
}

func TestDiscardExistingUnregisteredDirectoryFailsThroughGitAndPreservesIt(t *testing.T) {
	repo := newTestRepo(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	runID := "run_4444444444444444"
	projectID, projectErr := project.ID(repo)
	if projectErr != nil {
		t.Fatalf("project.ID: %v", projectErr)
	}
	clone := filepath.Join(home, ".jacu-harness", "worktrees", projectID, runID)
	if mkdirErr := os.MkdirAll(filepath.Dir(clone), 0o700); mkdirErr != nil {
		t.Fatalf("mkdir expected worktree parent: %v", mkdirErr)
	}
	runGit(t, repo, "clone", "--no-local", repo, clone)
	sentinel := filepath.Join(clone, "sentinel.txt")
	if writeErr := os.WriteFile(sentinel, []byte("must remain\n"), 0o600); writeErr != nil {
		t.Fatalf("write sentinel: %v", writeErr)
	}
	branch := "jacu/run-4444444444444444"
	runGit(t, repo, "branch", branch)
	state := runstate.Run{
		RunID:     runID,
		Status:    runstate.StatusOpen,
		CreatedAt: time.Now().UTC(),
		BaseSHA:   runGit(t, repo, "rev-parse", "HEAD"),
		Branch:    branch,
		Worktree:  clone,
	}
	if saveErr := runstate.Save(repo, state); saveErr != nil {
		t.Fatalf("Save: %v", saveErr)
	}
	indexPath := filepath.Join(clone, ".git", "index")
	// #nosec G304 -- indexPath belongs to the test-owned temporary clone.
	indexBefore, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatalf("read foreign index before: %v", err)
	}
	statusBefore := gitStatusBytes(t, clone)
	archive := filepath.Join(repo, ".git", "jacu", "archive", runID+".patch")

	_, err = Discard(context.Background(), repo, DiscardInput{RunID: runID})
	if err == nil || !strings.Contains(err.Error(), "not registered") {
		t.Fatalf("Discard error = %v; want unregistered-worktree rejection", err)
	}
	// #nosec G304 -- sentinel is a fixed fixture in the test-owned temporary clone.
	if content, readErr := os.ReadFile(sentinel); readErr != nil || string(content) != "must remain\n" {
		t.Fatalf("existing target was changed: content=%q err=%v", content, readErr)
	}
	// #nosec G304 -- indexPath belongs to the test-owned temporary clone.
	indexAfter, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatalf("read foreign index after: %v", err)
	}
	if !bytes.Equal(indexAfter, indexBefore) {
		t.Fatal("foreign repository index changed")
	}
	if statusAfter := gitStatusBytes(t, clone); !bytes.Equal(statusAfter, statusBefore) {
		t.Fatalf("foreign status changed byte-for-byte\nbefore: %q\nafter:  %q", statusBefore, statusAfter)
	}
	if _, err := os.Stat(archive); !os.IsNotExist(err) {
		t.Fatalf("archive created for unregistered target or stat failed: %v", err)
	}
	if branches := runGit(t, repo, "branch", "--list", branch); branches == "" {
		t.Fatal("branch was removed after worktree remove failure")
	}
	loaded, loadErr := runstate.Load(repo, runID)
	if loadErr != nil {
		t.Fatalf("Load: %v", loadErr)
	}
	if loaded.Status != runstate.StatusOpen {
		t.Fatalf("status = %q; want open", loaded.Status)
	}
}

func openTestRun(t *testing.T, repo string) OpenResult {
	t.Helper()
	input := validMissionInput(t, repo)
	mission, _, _ := missioncompile.Compile(repo, input)
	result, err := Open(context.Background(), repo, OpenInput{MissionInput: input, MissionID: mission.MissionID})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if result.Status != "ok" {
		t.Fatalf("Open result = %#v", result)
	}
	return result
}

func containsAction(actions []string, fragment string) bool {
	for _, action := range actions {
		if strings.Contains(action, fragment) {
			return true
		}
	}
	return false
}

func gitStatusBytes(t *testing.T, repo string) []byte {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "status", "--porcelain=v1", "-z")
	cmd.Dir = repo
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("git status --porcelain=v1 -z: %v", err)
	}
	return output
}

func installOneShotGitFailure(t *testing.T, firstArg, secondArg string) {
	t.Helper()
	realGit, err := exec.LookPath("git")
	if err != nil {
		t.Fatalf("git executable: %v", err)
	}
	wrapperDir := t.TempDir()
	marker := filepath.Join(t.TempDir(), "git-failure-used")
	wrapper := filepath.Join(wrapperDir, "git")
	script := fmt.Sprintf(`#!/bin/sh
if [ "$1" = %q ] && [ "$2" = %q ] && [ ! -f %q ]; then
  : > %q
  exit 91
fi
exec %q "$@"
`, firstArg, secondArg, marker, marker, realGit)
	// #nosec G306 -- the test wrapper must be executable by the test process.
	if err := os.WriteFile(wrapper, []byte(script), 0o700); err != nil {
		t.Fatalf("write git wrapper: %v", err)
	}
	t.Setenv("PATH", wrapperDir+string(os.PathListSeparator)+os.Getenv("PATH"))
}
