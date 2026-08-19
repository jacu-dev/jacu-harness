package workspace

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jacu-dev/jacu-harness/internal/capability/missioncompile"
	"github.com/jacu-dev/jacu-harness/internal/runstate"
)

func TestApplyCommitsReviewedDiffRemovesWorktreeAndKeepsBranch(t *testing.T) {
	repo, opened := reviewedApplyFixture(t, "write", [][]string{{"git", "status", "--short"}})

	result, applyErr := Apply(context.Background(), repo, ApplyInput{RunID: opened.RunID}, "Claude Code")
	if applyErr != nil {
		t.Fatalf("Apply: %v", applyErr)
	}
	if result.Status != "ok" || result.Data.CommitSHA == "" || result.Data.Branch != opened.Branch {
		t.Fatalf("result = %#v", result)
	}
	if _, err := os.Stat(opened.WorktreePath); !os.IsNotExist(err) {
		t.Fatalf("worktree still exists or stat failed unexpectedly: %v", err)
	}
	runGit(t, repo, "show-ref", "--verify", "refs/heads/"+opened.Branch)
	if parent := runGit(t, repo, "rev-parse", result.Data.CommitSHA+"^"); parent != opened.BaseSHA {
		t.Fatalf("commit parent = %q; want reviewed base %q", parent, opened.BaseSHA)
	}
	state, err := runstate.Load(repo, opened.RunID)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if state.Status != runstate.StatusApplied || state.AppliedCommit != result.Data.CommitSHA {
		t.Fatalf("state = %#v", state)
	}
	message := runGit(t, repo, "log", "-1", "--format=%B", opened.Branch)
	for _, trailer := range []string{
		"Jacu-Run: " + opened.RunID,
		"Jacu-Mission: " + state.MissionID,
		"Jacu-Base: " + opened.BaseSHA,
		"Assisted-by: Claude Code",
	} {
		if !strings.Contains(message, trailer) {
			t.Fatalf("commit message missing %q:\n%s", trailer, message)
		}
	}
	wantNext := "merge " + opened.Branch + " into main when ready"
	if len(result.NextActions) != 1 || result.NextActions[0] != wantNext {
		t.Fatalf("next_actions = %v; want %q", result.NextActions, wantNext)
	}
}

func TestApplyBlocksForgedRunTargetingProjectRoot(t *testing.T) {
	repo := newTestRepo(t)
	writeWorktreeFile(t, repo, "README.md", "forged apply\n")
	run := saveForgedRootRun(t, repo, runstate.StatusReviewed)
	headBefore := runGit(t, repo, "rev-parse", "HEAD")

	result, err := Apply(context.Background(), repo, ApplyInput{RunID: run.RunID}, "Claude Code")
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	headAfter := runGit(t, repo, "rev-parse", "HEAD")
	if result.Status != "blocked" || !strings.Contains(result.Summary, "identity") || headAfter != headBefore {
		t.Fatalf(
			"forged run result = %#v; HEAD before = %s; HEAD after = %s; want blocked identity and unchanged HEAD",
			result,
			headBefore,
			headAfter,
		)
	}
}

func TestApplyRecompilesMissionBeforeRunningTamperedVerification(t *testing.T) {
	sentinel := filepath.Join(t.TempDir(), "verification-ran")
	command := []string{"sh", "-c", fmt.Sprintf("printf pwned > %q", sentinel)}
	repo, opened := reviewedApplyFixture(t, "write", [][]string{{"git", "status", "--short"}})
	defer func() {
		if _, err := os.Stat(opened.WorktreePath); err == nil {
			cleanupWorktree(t, repo, opened.WorktreePath)
		}
	}()

	run, loadErr := runstate.Load(repo, opened.RunID)
	if loadErr != nil {
		t.Fatalf("Load reviewed run: %v", loadErr)
	}
	run.MissionInput.VerificationCommands = [][]string{command}
	run.Mission.VerificationCommands = [][]string{command}
	if err := runstate.Save(repo, run); err != nil {
		t.Fatalf("Save tampered run: %v", err)
	}

	result, err := Apply(context.Background(), repo, ApplyInput{RunID: opened.RunID}, "Claude Code")
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	// #nosec G304 -- sentinel is created under this test's t.TempDir.
	sentinelContents, sentinelErr := os.ReadFile(sentinel)
	if result.Status != "blocked" ||
		result.Summary != "mission integrity check failed" ||
		!os.IsNotExist(sentinelErr) {
		t.Fatalf(
			"result = %#v; sentinel contents = %q, error = %v; want blocked mission integrity and no executed sentinel",
			result,
			sentinelContents,
			sentinelErr,
		)
	}
}

func TestApplyBlocksWhenRecompiledMissionIDDiffers(t *testing.T) {
	repo, opened := reviewedApplyFixture(t, "write", [][]string{{"git", "status", "--short"}})
	defer func() {
		if _, err := os.Stat(opened.WorktreePath); err == nil {
			cleanupWorktree(t, repo, opened.WorktreePath)
		}
	}()

	run, loadErr := runstate.Load(repo, opened.RunID)
	if loadErr != nil {
		t.Fatalf("Load reviewed run: %v", loadErr)
	}
	run.MissionInput.Objective = "fix a different mission objective before apply"
	if err := runstate.Save(repo, run); err != nil {
		t.Fatalf("Save mismatched mission input: %v", err)
	}

	result, applyErr := Apply(context.Background(), repo, ApplyInput{RunID: opened.RunID}, "Claude Code")
	if applyErr != nil {
		t.Fatalf("Apply: %v", applyErr)
	}
	if result.Status != "blocked" || result.Summary != "mission integrity check failed" {
		t.Fatalf("result = %#v; want blocked mission integrity", result)
	}
}

func TestApplyPreservesAppliedStateWhenWorktreeRemovalFails(t *testing.T) {
	repo, opened := reviewedApplyFixture(t, "write", [][]string{{"git", "status", "--short"}})
	defer func() {
		runGit(t, repo, "worktree", "remove", "--force", opened.WorktreePath)
	}()

	installApplyGitWrapper(t, `#!/bin/sh
if [ "$1" = "worktree" ] && [ "$2" = "remove" ] && [ ! -f "$TMPDIR/worktree-remove-failed" ]; then
  : > "$TMPDIR/worktree-remove-failed"
  printf 'simulated worktree remove failure\n' >&2
  exit 1
fi
exec "$TMPDIR/git" "$@"
`)

	result, applyErr := Apply(context.Background(), repo, ApplyInput{RunID: opened.RunID}, "Claude Code")
	if applyErr != nil {
		t.Fatalf("Apply: %v", applyErr)
	}
	if result.Status != "ok" || result.Data.CommitSHA == "" || result.Data.Branch != opened.Branch {
		t.Fatalf("result = %#v", result)
	}
	warnings := strings.Join(result.Warnings, "\n")
	if !strings.Contains(warnings, "worktree cleanup failed") ||
		!strings.Contains(warnings, "simulated worktree remove failure") {
		t.Fatalf("warnings = %q; want cleanup failure details", warnings)
	}
	if _, err := os.Stat(opened.WorktreePath); err != nil {
		t.Fatalf("worktree missing after failed removal: %v", err)
	}
	branchHead := runGit(t, repo, "rev-parse", opened.Branch)
	worktreeHead := runGit(t, opened.WorktreePath, "rev-parse", "HEAD")
	if result.Data.CommitSHA != branchHead || result.Data.CommitSHA != worktreeHead {
		t.Fatalf("commit mismatch: result=%q branch=%q worktree=%q", result.Data.CommitSHA, branchHead, worktreeHead)
	}
	state, err := runstate.Load(repo, opened.RunID)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if state.Status != runstate.StatusApplied || state.AppliedCommit != result.Data.CommitSHA {
		t.Fatalf("state = %#v; want applied commit %q", state, result.Data.CommitSHA)
	}
}

func TestApplyPreservesAppliedStateWhenWorktreeUnlockFails(t *testing.T) {
	repo, opened := reviewedApplyFixture(t, "write", [][]string{{"git", "status", "--short"}})
	defer cleanupWorktree(t, repo, opened.WorktreePath)

	installApplyGitWrapper(t, `#!/bin/sh
if [ "$1" = "worktree" ] && [ "$2" = "unlock" ] && [ ! -f "$TMPDIR/worktree-unlock-failed" ]; then
  : > "$TMPDIR/worktree-unlock-failed"
  printf 'simulated worktree unlock failure\n' >&2
  exit 1
fi
exec "$TMPDIR/git" "$@"
`)

	result, err := Apply(context.Background(), repo, ApplyInput{RunID: opened.RunID}, "Claude Code")
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if result.Status != "ok" || result.Data.CommitSHA == "" || result.Data.Branch != opened.Branch {
		t.Fatalf("result = %#v", result)
	}
	warnings := strings.Join(result.Warnings, "\n")
	if !strings.Contains(warnings, "worktree cleanup failed") ||
		!strings.Contains(warnings, "simulated worktree unlock failure") {
		t.Fatalf("warnings = %q; want unlock failure details", warnings)
	}
	if _, statErr := os.Stat(opened.WorktreePath); statErr != nil {
		t.Fatalf("worktree missing after failed unlock: %v", statErr)
	}
	worktrees := runGit(t, repo, "worktree", "list", "--porcelain")
	if !strings.Contains(worktrees, opened.WorktreePath) || !strings.Contains(worktrees, "locked") {
		t.Fatalf("worktree is not retained locked:\n%s", worktrees)
	}
	branchHead := runGit(t, repo, "rev-parse", opened.Branch)
	if result.Data.CommitSHA != branchHead {
		t.Fatalf("commit mismatch: result=%q branch=%q", result.Data.CommitSHA, branchHead)
	}
	state, err := runstate.Load(repo, opened.RunID)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if state.Status != runstate.StatusApplied || state.AppliedCommit != result.Data.CommitSHA {
		t.Fatalf("state = %#v; want applied commit %q", state, result.Data.CommitSHA)
	}
}

func TestApplyRollsBackCommitWhenAppliedStateCannotPersist(t *testing.T) {
	repo, opened := reviewedApplyFixture(t, "write", [][]string{{"git", "status", "--short"}})
	runsDir := filepath.Join(repo, ".git", "jacu", "runs")
	defer func() {
		_ = restoreBlockedRunsDir(t, runsDir)
		if _, err := os.Stat(opened.WorktreePath); err == nil {
			cleanupWorktree(t, repo, opened.WorktreePath)
		}
	}()
	originalPath, failedCommitPath := installAppliedStateSaveFailureWrapper(t, opened.BaseSHA, runsDir, false)

	_, applyErr := Apply(context.Background(), repo, ApplyInput{RunID: opened.RunID}, "Claude Code")
	if applyErr == nil || !strings.Contains(applyErr.Error(), "persist applied state") ||
		!strings.Contains(applyErr.Error(), "HEAD rolled back to "+opened.BaseSHA) ||
		!strings.Contains(applyErr.Error(), "retry Apply") {
		t.Fatalf("Apply error = %v; want persisted-state failure with successful rollback action", applyErr)
	}
	if !strings.Contains(applyErr.Error(), filepath.Join(runsDir, opened.RunID+".json")) {
		t.Fatalf("Apply error = %v; want the failure to name the unwritable run state file", applyErr)
	}
	if err := restoreBlockedRunsDir(t, runsDir); err != nil {
		t.Fatalf("restore runs directory: %v", err)
	}
	// #nosec G304 -- failedCommitPath is under this test's t.TempDir.
	failedCommitBytes, readErr := os.ReadFile(failedCommitPath)
	if readErr != nil {
		t.Fatalf("read failed commit SHA: %v", readErr)
	}
	failedCommit := strings.TrimSpace(string(failedCommitBytes))
	if failedCommit == "" {
		t.Fatal("wrapper did not capture post-CommitTree SHA")
	}
	if branchHead := runGit(t, repo, "rev-parse", opened.Branch); branchHead != opened.BaseSHA {
		t.Fatalf("branch HEAD = %q; want rolled back base %q", branchHead, opened.BaseSHA)
	}
	if worktreeHead := runGit(t, opened.WorktreePath, "rev-parse", "HEAD"); worktreeHead != opened.BaseSHA {
		t.Fatalf("worktree HEAD = %q; want rolled back base %q", worktreeHead, opened.BaseSHA)
	}
	if branches := runGit(t, repo, "branch", "--contains", failedCommit, "--format=%(refname:short)"); branches != "" {
		t.Fatalf("branches still retain failed applied commit %q: %s", failedCommit, branches)
	}
	if _, err := os.Stat(opened.WorktreePath); err != nil {
		t.Fatalf("worktree missing after applied-state Save failure: %v", err)
	}
	state, err := runstate.Load(repo, opened.RunID)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if state.Status != runstate.StatusReviewed || state.AppliedCommit != "" {
		t.Fatalf("state = %#v; want reviewed without applied commit", state)
	}

	t.Setenv("PATH", originalPath)
	retried, err := Apply(context.Background(), repo, ApplyInput{RunID: opened.RunID}, "Claude Code")
	if err != nil || retried.Status != "ok" || retried.Data.CommitSHA == "" {
		t.Fatalf("retry Apply = %#v, %v", retried, err)
	}
}

func TestApplyFailsClosedWhenAppliedStateSaveAndRollbackFail(t *testing.T) {
	repo, opened := reviewedApplyFixture(t, "write", [][]string{{"git", "status", "--short"}})
	runsDir := filepath.Join(repo, ".git", "jacu", "runs")
	defer func() {
		_ = restoreBlockedRunsDir(t, runsDir)
		if _, err := os.Stat(opened.WorktreePath); err == nil {
			cleanupWorktree(t, repo, opened.WorktreePath)
		}
	}()
	_, failedCommitPath := installAppliedStateSaveFailureWrapper(t, opened.BaseSHA, runsDir, true)

	_, applyErr := Apply(context.Background(), repo, ApplyInput{RunID: opened.RunID}, "Claude Code")
	if applyErr == nil || !strings.Contains(applyErr.Error(), "persist applied state") ||
		!strings.Contains(applyErr.Error(), "simulated rollback failure") ||
		!strings.Contains(applyErr.Error(), "requires manual reconciliation") {
		t.Fatalf("Apply error = %v; want combined save/rollback failure with reconciliation action", applyErr)
	}
	if restoreErr := restoreBlockedRunsDir(t, runsDir); restoreErr != nil {
		t.Fatalf("restore runs directory: %v", restoreErr)
	}
	// #nosec G304 -- failedCommitPath is under this test's t.TempDir.
	failedCommitBytes, readErr := os.ReadFile(failedCommitPath)
	if readErr != nil {
		t.Fatalf("read failed commit SHA: %v", readErr)
	}
	failedCommit := strings.TrimSpace(string(failedCommitBytes))
	if branchHead := runGit(t, repo, "rev-parse", opened.Branch); branchHead != failedCommit {
		t.Fatalf("branch HEAD = %q; want unreconciled commit %q", branchHead, failedCommit)
	}
	if !strings.Contains(applyErr.Error(), failedCommit) || !strings.Contains(applyErr.Error(), opened.BaseSHA) {
		t.Fatalf("Apply error = %v; want actionable commit and base SHAs", applyErr)
	}
	state, err := runstate.Load(repo, opened.RunID)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if state.Status != runstate.StatusReviewed || state.AppliedCommit != "" {
		t.Fatalf("state = %#v; want unchanged reviewed state", state)
	}
	if _, err := os.Stat(opened.WorktreePath); err != nil {
		t.Fatalf("worktree missing after failed rollback: %v", err)
	}
}

func TestApplyBlocksBeforeDiffReview(t *testing.T) {
	repo, opened := openApplyFixture(t, "write", [][]string{{"git", "status", "--short"}})
	defer cleanupWorktree(t, repo, opened.WorktreePath)
	writeWorktreeFile(t, opened.WorktreePath, "README.md", "changed\n")

	result, err := Apply(context.Background(), repo, ApplyInput{RunID: opened.RunID}, "Claude Code")
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if result.Status != "blocked" || result.Summary != "diff not reviewed; call jacu_diff first" {
		t.Fatalf("result = %#v", result)
	}
}

func TestApplyBlocksWhenWorktreeChangesAfterReview(t *testing.T) {
	repo, opened := reviewedApplyFixture(t, "write", [][]string{{"git", "status", "--short"}})
	defer cleanupWorktree(t, repo, opened.WorktreePath)
	writeWorktreeFile(t, opened.WorktreePath, "README.md", "changed after review\n")

	result, err := Apply(context.Background(), repo, ApplyInput{RunID: opened.RunID}, "Claude Code")
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if result.Status != "blocked" || result.Summary != "worktree changed after review; review the diff again" {
		t.Fatalf("result = %#v", result)
	}
}

func TestApplyBlocksReviewedDiffOutsideMissionScope(t *testing.T) {
	repo := newTestRepo(t)
	input := validMissionInput(t, repo)
	input.AllowedPaths = []string{"allowed.txt"}
	input.VerificationCommands = [][]string{{"git", "status", "--short"}}
	opened := openWithMissionInput(t, repo, input)
	defer cleanupWorktree(t, repo, opened.WorktreePath)
	writeWorktreeFile(t, opened.WorktreePath, "README.md", "outside mission scope\n")
	if reviewed := diffRun(t, repo, opened.RunID); reviewed.Status != "ok" {
		t.Fatalf("diff review = %#v", reviewed)
	}

	result, err := Apply(context.Background(), repo, ApplyInput{RunID: opened.RunID}, "Claude Code")
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if result.Status != "blocked" || !strings.Contains(result.Summary, "scope") {
		t.Fatalf("result = %#v; want scope refusal before commit", result)
	}
	if head := runGit(t, repo, "rev-parse", opened.Branch); head != opened.BaseSHA {
		t.Fatalf("branch HEAD = %q; want unchanged base %q", head, opened.BaseSHA)
	}
}

func TestApplyBlocksWhenNewFileAppearsAfterReview(t *testing.T) {
	repo, opened := reviewedApplyFixture(t, "write", [][]string{{"git", "status", "--short"}})
	writeWorktreeFile(t, opened.WorktreePath, "created-after-review.txt", "not reviewed\n")

	result, err := Apply(context.Background(), repo, ApplyInput{RunID: opened.RunID}, "Claude Code")
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if result.Status != "blocked" || result.Summary != "worktree changed after review; review the diff again" {
		t.Fatalf("result = %#v", result)
	}
	cleanupWorktree(t, repo, opened.WorktreePath)
}

func TestApplyBlocksWhenVerificationCreatesFileWithoutCommit(t *testing.T) {
	command := []string{"git", "mv", "README.md", "verification-created.txt"}
	repo, opened := reviewedApplyFixtureAllowingProgram(t, "write", [][]string{command}, "git")

	result, err := Apply(context.Background(), repo, ApplyInput{RunID: opened.RunID}, "Claude Code")
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if result.Status != "blocked" || result.Summary != "verification commands modified the worktree; review the diff again" {
		t.Fatalf("result = %#v", result)
	}
	if head := runGit(t, opened.WorktreePath, "rev-parse", "HEAD"); head != opened.BaseSHA {
		t.Fatalf("worktree HEAD = %q; want unchanged base %q", head, opened.BaseSHA)
	}
	if branchHead := runGit(t, repo, "rev-parse", opened.Branch); branchHead != opened.BaseSHA {
		t.Fatalf("branch HEAD = %q; want unchanged base %q", branchHead, opened.BaseSHA)
	}
	state, err := runstate.Load(repo, opened.RunID)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if state.Status != runstate.StatusReviewed || state.AppliedCommit != "" {
		t.Fatalf("state = %#v; want reviewed without applied commit", state)
	}
	if cached := runGit(t, opened.WorktreePath, "diff", "--cached", "--name-only"); cached != "" {
		t.Fatalf("blocked Apply left staged paths: %q", cached)
	}
	if _, err := os.Stat(filepath.Join(opened.WorktreePath, "verification-created.txt")); err != nil {
		t.Fatalf("verification mutation was not preserved in worktree: %v", err)
	}
	cleanupWorktree(t, repo, opened.WorktreePath)
}

func TestApplyBlocksWhenVerificationAdvancesHead(t *testing.T) {
	command := []string{"git", "commit", "-am", "verification injected commit"}
	repo, opened := reviewedApplyFixtureAllowingProgram(t, "write", [][]string{command}, "git")
	defer func() {
		if _, err := os.Stat(opened.WorktreePath); err == nil {
			cleanupWorktree(t, repo, opened.WorktreePath)
		}
	}()

	result, err := Apply(context.Background(), repo, ApplyInput{RunID: opened.RunID}, "Claude Code")
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if result.Status != "blocked" || result.Summary != "verification commands modified the worktree; review the diff again" {
		t.Fatalf("result = %#v", result)
	}
	if head := runGit(t, opened.WorktreePath, "rev-parse", "HEAD"); head == opened.BaseSHA {
		t.Fatalf("verification command did not advance HEAD from base %q", opened.BaseSHA)
	}
	state, err := runstate.Load(repo, opened.RunID)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if state.Status != runstate.StatusReviewed || state.AppliedCommit != "" {
		t.Fatalf("state = %#v; want reviewed without applied commit", state)
	}
}

func TestApplyCommitsPinnedReviewedTreeWhenIndexChangesAfterValidation(t *testing.T) {
	repo, opened := reviewedApplyFixture(t, "write", [][]string{{"git", "status", "--short"}})

	installApplyGitWrapper(t, `#!/bin/sh
if [ "$1" = "diff" ]; then
  late=0
  if [ -f "$TMPDIR/first-diff-seen" ]; then
    late=1
  else
    : > "$TMPDIR/first-diff-seen"
  fi
  "$TMPDIR/git" "$@"
  status=$?
  if [ "$late" -eq 1 ]; then
    printf 'changed after tree validation\n' > README.md
    "$TMPDIR/git" add -A
  fi
  exit "$status"
fi
exec "$TMPDIR/git" "$@"
`)

	result, err := Apply(context.Background(), repo, ApplyInput{RunID: opened.RunID}, "Claude Code")
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if result.Status != "ok" {
		t.Fatalf("result = %#v", result)
	}
	if committed := runGit(t, repo, "show", result.Data.CommitSHA+":README.md"); committed != "changed" {
		t.Fatalf("committed README = %q; want pinned reviewed content", committed)
	}
	if _, err := os.Stat(opened.WorktreePath); !os.IsNotExist(err) {
		t.Fatalf("dirty worktree remains after Apply: %v", err)
	}
}

func TestApplyCommitsOnlyFilesPresentInReviewedDiff(t *testing.T) {
	repo := newTestRepo(t)
	input := validMissionInput(t, repo)
	input.AllowedPaths = []string{"reviewed-new.txt"}
	input.VerificationCommands = [][]string{{"git", "status", "--short"}}
	opened := openWithMissionInput(t, repo, input)
	writeWorktreeFile(t, opened.WorktreePath, "reviewed-new.txt", "reviewed content\n")
	reviewed := diffRun(t, repo, opened.RunID)

	result, err := Apply(context.Background(), repo, ApplyInput{RunID: opened.RunID}, "Claude Code")
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if result.Status != "ok" {
		t.Fatalf("result = %#v", result)
	}
	committed := strings.Fields(runGit(t, repo, "diff-tree", "--no-commit-id", "--name-only", "-r", result.Data.CommitSHA))
	for _, path := range committed {
		if !containsString(reviewed.Data.Files, path) {
			t.Fatalf("committed file %q was absent from reviewed files %v; reviewed diff:\n%s", path, reviewed.Data.Files, reviewed.Data.Diff)
		}
	}
}

func TestApplyDestructiveRequiresExplicitApproval(t *testing.T) {
	repo, opened := reviewedApplyFixture(t, "destructive", [][]string{{"git", "status", "--short"}})

	blocked, err := Apply(context.Background(), repo, ApplyInput{RunID: opened.RunID}, "Claude Code")
	if err != nil {
		t.Fatalf("Apply without approval: %v", err)
	}
	if blocked.Status != "blocked" || blocked.Summary != "destructive mission requires approve_destructive" {
		cleanupWorktree(t, repo, opened.WorktreePath)
		t.Fatalf("blocked result = %#v", blocked)
	}

	applied, err := Apply(context.Background(), repo, ApplyInput{RunID: opened.RunID, ApproveDestructive: true}, "Claude Code")
	if err != nil {
		t.Fatalf("Apply with approval: %v", err)
	}
	if applied.Status != "ok" {
		t.Fatalf("applied result = %#v", applied)
	}
}

func TestApplyBlocksOnVerificationFailureWithoutCommit(t *testing.T) {
	command := []string{"git", "rev-parse", "--verify", "refs/heads/does-not-exist"}
	repo, opened := reviewedApplyFixtureAllowingProgram(t, "write", [][]string{command}, "git")
	defer cleanupWorktree(t, repo, opened.WorktreePath)

	result, err := Apply(context.Background(), repo, ApplyInput{RunID: opened.RunID}, "Claude Code")
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	wantSummary := "verification failed: " + strings.Join(command, " ")
	if result.Status != "blocked" || result.Summary != wantSummary || result.Data.Stderr == "" {
		t.Fatalf("result = %#v", result)
	}
	if head := runGit(t, opened.WorktreePath, "rev-parse", "HEAD"); head != opened.BaseSHA {
		t.Fatalf("HEAD = %q; want unchanged base %q", head, opened.BaseSHA)
	}
	state, err := runstate.Load(repo, opened.RunID)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if state.Status != runstate.StatusReviewed || state.AppliedCommit != "" {
		t.Fatalf("state = %#v", state)
	}
}

func TestMissionCompilePreflightsEntireVerificationBatchBeforeDispatch(t *testing.T) {
	probeDir := t.TempDir()
	marker := filepath.Join(probeDir, "verification-spawned")
	writeExecutable(t, filepath.Join(probeDir, "apply-probe"), fmt.Sprintf("#!/bin/sh\nprintf spawned > %q\n", marker))
	repo := newTestRepo(t)
	writeVerifyPolicy(t, repo, probeDir, "apply-probe")
	input := validMissionInput(t, repo)
	input.RiskHint = "write"
	input.VerificationCommands = [][]string{{"apply-probe"}, {"definitely-not-allowlisted"}}
	mission, status, _ := missioncompile.Compile(repo, input)
	if status != "blocked" || !hasPreflightLint(mission.Lint) {
		t.Fatalf("mission = %#v status = %q; want preflight block before dispatch", mission, status)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("earlier verification command executed before batch refusal: %v", err)
	}
}

func hasPreflightLint(lints []runstate.Lint) bool {
	for _, lint := range lints {
		if lint.Level == "BLOCK" && lint.Rule == "preflight" {
			return true
		}
	}
	return false
}

func TestApplyVerificationUsesSyntheticHome(t *testing.T) {
	probeDir := t.TempDir()
	observedHome := filepath.Join(probeDir, "observed-home")
	writeExecutable(t, filepath.Join(probeDir, "home-probe"), fmt.Sprintf("#!/bin/sh\nprintf '%%s' \"$HOME\" > %q\n", observedHome))
	realHome := t.TempDir()
	t.Setenv("HOME", realHome)
	repo := newTestRepo(t)
	writeVerifyPolicy(t, repo, probeDir, "home-probe")
	input := validMissionInput(t, repo)
	input.RiskHint = "write"
	input.VerificationCommands = [][]string{{"home-probe"}}
	opened := openWithMissionInput(t, repo, input)
	writeWorktreeFile(t, opened.WorktreePath, "README.md", "changed\n")
	diffRun(t, repo, opened.RunID)
	t.Setenv("PATH", probeDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	result, err := Apply(context.Background(), repo, ApplyInput{RunID: opened.RunID}, "Claude Code")
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if result.Status != "ok" {
		t.Fatalf("result = %#v", result)
	}
	// #nosec G304 -- observedHome is under this test's t.TempDir.
	content, err := os.ReadFile(observedHome)
	if err != nil {
		t.Fatalf("read observed HOME: %v", err)
	}
	got := string(content)
	if got == realHome || !strings.Contains(got, filepath.Join(".jacu-harness", "toolchain-home")) {
		t.Fatalf("verification HOME = %q; real HOME = %q; want synthetic toolchain HOME", got, realHome)
	}
}

func TestApplyTwiceRefusesAppliedRun(t *testing.T) {
	repo, opened := reviewedApplyFixture(t, "write", [][]string{{"git", "status", "--short"}})
	first, err := Apply(context.Background(), repo, ApplyInput{RunID: opened.RunID}, "Claude Code")
	if err != nil || first.Status != "ok" {
		t.Fatalf("first Apply = %#v, %v", first, err)
	}

	second, err := Apply(context.Background(), repo, ApplyInput{RunID: opened.RunID}, "Claude Code")
	if err != nil {
		t.Fatalf("second Apply: %v", err)
	}
	if second.Status != "blocked" || second.Summary != "diff not reviewed; call jacu_diff first" {
		t.Fatalf("second result = %#v", second)
	}
}

func reviewedApplyFixture(t *testing.T, risk string, commands [][]string) (string, OpenData) {
	t.Helper()
	repo, opened := openApplyFixture(t, risk, commands)
	writeWorktreeFile(t, opened.WorktreePath, "README.md", "changed\n")
	diffRun(t, repo, opened.RunID)
	return repo, opened
}

func reviewedApplyFixtureAllowingProgram(t *testing.T, risk string, commands [][]string, program string) (string, OpenData) {
	t.Helper()
	repo := newTestRepo(t)
	writeVerifyPolicy(t, repo, "", program)
	input := validMissionInput(t, repo)
	input.RiskHint = risk
	input.VerificationCommands = commands
	opened := openWithMissionInput(t, repo, input)
	writeWorktreeFile(t, opened.WorktreePath, "README.md", "changed\n")
	diffRun(t, repo, opened.RunID)
	return repo, opened
}

func openApplyFixture(t *testing.T, risk string, commands [][]string) (string, OpenData) {
	t.Helper()
	repo := newTestRepo(t)
	input := validMissionInput(t, repo)
	input.RiskHint = risk
	input.VerificationCommands = commands
	return repo, openWithMissionInput(t, repo, input)
}

func installAppliedStateSaveFailureWrapper(t *testing.T, baseSHA, runsDir string, failRollback bool) (string, string) {
	t.Helper()
	originalPath, dataDir := installApplyGitWrapper(t, fmt.Sprintf(`#!/bin/sh
if [ "$1" = "update-ref" ] && [ "$2" = "HEAD" ]; then
  base_sha=$(cat "$TMPDIR/base-sha") || exit 96
  if [ "$3" = "$base_sha" ] && [ -f "$TMPDIR/fail-rollback" ]; then
    printf 'simulated rollback failure\n' >&2
    exit 1
  fi
  "$TMPDIR/git" "$@"
  status=$?
  if [ "$status" -ne 0 ]; then
    exit "$status"
  fi
  if [ "$3" != "$base_sha" ]; then
    printf '%%s\n' "$3" > "$TMPDIR/failed-commit-sha"
    mv %q %q || exit 97
    : > %q || exit 98
  fi
  exit 0
fi
exec "$TMPDIR/git" "$@"
`, runsDir, runsDir+blockedDirSuffix, runsDir))
	if err := os.WriteFile(filepath.Join(dataDir, "base-sha"), []byte(baseSHA+"\n"), 0o600); err != nil {
		t.Fatalf("write base SHA fixture: %v", err)
	}
	if failRollback {
		if err := os.WriteFile(filepath.Join(dataDir, "fail-rollback"), nil, 0o600); err != nil {
			t.Fatalf("write rollback failure marker: %v", err)
		}
	}
	return originalPath, filepath.Join(dataDir, "failed-commit-sha")
}

func installApplyGitWrapper(t *testing.T, script string) (string, string) {
	t.Helper()
	realGit, err := exec.LookPath("git")
	if err != nil {
		t.Fatalf("git executable: %v", err)
	}
	dataDir := t.TempDir()
	if err := os.Symlink(realGit, filepath.Join(dataDir, "git")); err != nil {
		t.Fatalf("symlink real git: %v", err)
	}
	wrapperDir := t.TempDir()
	wrapper := filepath.Join(wrapperDir, "git")
	// #nosec G306 -- the test wrapper must be executable by the test process.
	if err := os.WriteFile(wrapper, []byte(script), 0o700); err != nil {
		t.Fatalf("write git wrapper: %v", err)
	}
	originalPath := os.Getenv("PATH")
	t.Setenv("TMPDIR", dataDir)
	t.Setenv("PATH", wrapperDir+string(os.PathListSeparator)+originalPath)
	return originalPath, dataDir
}

func writeExecutable(t *testing.T, path, content string) {
	t.Helper()
	// #nosec G306 -- the test probe must be executable by the test process.
	if err := os.WriteFile(path, []byte(content), 0o700); err != nil {
		t.Fatalf("write executable probe: %v", err)
	}
}

func writeVerifyPolicy(t *testing.T, repo, pathDir, program string) {
	t.Helper()
	policyDir := filepath.Join(repo, ".jacu")
	if err := os.MkdirAll(policyDir, 0o700); err != nil {
		t.Fatalf("mkdir verify policy: %v", err)
	}
	policy := fmt.Sprintf("{\n  \"allow\": [{\"program\": %q}],\n  \"path_dirs\": [%q]\n}\n", program, pathDir)
	if err := os.WriteFile(filepath.Join(policyDir, "verify-allowlist.json"), []byte(policy), 0o600); err != nil {
		t.Fatalf("write verify policy: %v", err)
	}
}
