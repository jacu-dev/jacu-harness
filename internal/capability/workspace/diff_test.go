package workspace

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jacu-dev/jacu-harness/internal/capability/missioncompile"
	"github.com/jacu-dev/jacu-harness/internal/gitx"
	"github.com/jacu-dev/jacu-harness/internal/runstate"
)

func TestDiffDigestDeterministicAndChangesWithContent(t *testing.T) {
	repo, opened := openFixture(t)
	defer cleanupWorktree(t, repo, opened.WorktreePath)
	writeWorktreeFile(t, opened.WorktreePath, "README.md", "changed\n")

	first := diffRun(t, repo, opened.RunID)
	second := diffRun(t, repo, opened.RunID)
	if first.Data.Digest != second.Data.Digest {
		t.Fatalf("same state digests differ: %q != %q", first.Data.Digest, second.Data.Digest)
	}

	writeWorktreeFile(t, opened.WorktreePath, "README.md", "changed!\n")
	third := diffRun(t, repo, opened.RunID)
	if first.Data.Digest == third.Data.Digest {
		t.Fatalf("digest did not change after one-byte change: %q", third.Data.Digest)
	}
}

func TestDiffBlocksForgedRunTargetingProjectRoot(t *testing.T) {
	repo := newTestRepo(t)
	writeWorktreeFile(t, repo, "README.md", "forged diff\n")
	run := saveForgedRootRun(t, repo, runstate.StatusOpen)
	headBefore := runGit(t, repo, "rev-parse", "HEAD")

	result, err := WorkspaceDiff(context.Background(), repo, DiffInput{RunID: run.RunID})
	if err != nil {
		t.Fatalf("WorkspaceDiff: %v", err)
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

func TestDiffIncludesNewFileContentInNumstatAndScope(t *testing.T) {
	repo := newTestRepo(t)
	input := validMissionInput(t, repo)
	input.AllowedPaths = []string{"notes/new.txt"}
	opened := openWithMissionInput(t, repo, input)
	defer cleanupWorktree(t, repo, opened.WorktreePath)
	writeWorktreeFile(t, opened.WorktreePath, "notes/new.txt", "first line\nsecond line\n")

	result := diffRun(t, repo, opened.RunID)
	if !strings.Contains(result.Data.Diff, "diff --git a/notes/new.txt b/notes/new.txt") ||
		!strings.Contains(result.Data.Diff, "+first line") ||
		!strings.Contains(result.Data.Diff, "+second line") {
		t.Fatalf("diff missing new file content:\n%s", result.Data.Diff)
	}
	if result.Data.Added != 2 || result.Data.Deleted != 0 || !reflect.DeepEqual(result.Data.Files, []string{"notes/new.txt"}) {
		t.Fatalf("numstat = added %d deleted %d files %v; want 2, 0, [notes/new.txt]", result.Data.Added, result.Data.Deleted, result.Data.Files)
	}
	if !reflect.DeepEqual(result.Data.InScope, []string{"notes/new.txt"}) || len(result.Data.OutOfScope) != 0 {
		t.Fatalf("scope = in %v out %v; want [notes/new.txt], []", result.Data.InScope, result.Data.OutOfScope)
	}
}

func TestDiffReviewIncludesUntrackedAndDeletionWithoutMutatingIndex(t *testing.T) {
	repo := newTestRepo(t)
	writeRepoFileAndCommit(t, repo, "deleted.txt", "delete one\ndelete two\n")
	input := validMissionInput(t, repo)
	input.AllowedPaths = []string{"new.txt", "deleted.txt"}
	opened := openWithMissionInput(t, repo, input)
	defer cleanupWorktree(t, repo, opened.WorktreePath)
	writeWorktreeFile(t, opened.WorktreePath, "new.txt", "new one\nnew two\n")
	if err := os.Remove(filepath.Join(opened.WorktreePath, "deleted.txt")); err != nil {
		t.Fatalf("remove tracked fixture: %v", err)
	}

	porcelainBefore := runGit(t, opened.WorktreePath, "status", "--porcelain=v1", "-z")
	indexPath := runGit(t, opened.WorktreePath, "rev-parse", "--git-path", "index")
	if !filepath.IsAbs(indexPath) {
		indexPath = filepath.Join(opened.WorktreePath, indexPath)
	}
	indexBefore := snapshotPhysicalFile(t, indexPath)

	result := diffRun(t, repo, opened.RunID)
	if !strings.Contains(result.Data.Diff, "+new one") ||
		!strings.Contains(result.Data.Diff, "+new two") ||
		!strings.Contains(result.Data.Diff, "-delete one") ||
		!strings.Contains(result.Data.Diff, "-delete two") {
		t.Fatalf("review patch missing full new/deleted content:\n%s", result.Data.Diff)
	}
	if result.Data.Added != 2 || result.Data.Deleted != 2 {
		t.Fatalf("numstat = added %d deleted %d; want 2, 2", result.Data.Added, result.Data.Deleted)
	}
	for _, path := range []string{"new.txt", "deleted.txt"} {
		if !containsString(result.Data.Files, path) || !containsString(result.Data.InScope, path) {
			t.Fatalf("path %q missing from files/in_scope: files=%q in_scope=%q", path, result.Data.Files, result.Data.InScope)
		}
	}
	if len(result.Data.OutOfScope) != 0 {
		t.Fatalf("out_of_scope = %q; want empty", result.Data.OutOfScope)
	}

	assertPhysicalFileUnchanged(t, "index after WorkspaceDiff", indexBefore, snapshotPhysicalFile(t, indexPath))
	porcelainAfter := runGit(t, opened.WorktreePath, "status", "--porcelain=v1", "-z")
	if porcelainAfter != porcelainBefore {
		t.Fatalf("porcelain status changed:\nbefore: %q\nafter:  %q", porcelainBefore, porcelainAfter)
	}
}

func TestDiffIncludesBinaryPayloadPhysicalPathAndContentDigest(t *testing.T) {
	repo := newTestRepo(t)
	input := validMissionInput(t, repo)
	input.AllowedPaths = []string{"artifact.bin"}
	opened := openWithMissionInput(t, repo, input)
	defer cleanupWorktree(t, repo, opened.WorktreePath)
	path := filepath.Join(opened.WorktreePath, "artifact.bin")
	if err := os.WriteFile(path, []byte{0x00, 0x01, 0x02, 0x03, 0xff}, 0o600); err != nil {
		t.Fatalf("write first binary: %v", err)
	}

	first := diffRun(t, repo, opened.RunID)
	if !reflect.DeepEqual(first.Data.Files, []string{"artifact.bin"}) || !reflect.DeepEqual(first.Data.InScope, []string{"artifact.bin"}) {
		t.Fatalf("binary paths = files %q in_scope %q", first.Data.Files, first.Data.InScope)
	}
	if !strings.Contains(first.Data.Diff, "GIT binary patch") {
		t.Fatalf("binary payload missing from review patch:\n%s", first.Data.Diff)
	}
	git, err := gitx.New()
	if err != nil {
		t.Fatalf("gitx.New: %v", err)
	}
	treeSHA, err := git.StageTree(context.Background(), opened.WorktreePath)
	if err != nil {
		t.Fatalf("StageTree binary: %v", err)
	}
	treePatch, err := git.DiffTree(context.Background(), opened.WorktreePath, opened.BaseSHA, treeSHA)
	if err != nil {
		t.Fatalf("DiffTree binary: %v", err)
	}
	if treeDigest := diffDigest(treePatch); first.Data.Digest != treeDigest {
		t.Fatalf("binary review digest = %q; pinned tree digest = %q", first.Data.Digest, treeDigest)
	}

	if err := os.WriteFile(path, []byte{0x00, 0x01, 0x02, 0x04, 0xff}, 0o600); err != nil {
		t.Fatalf("write second binary: %v", err)
	}
	second := diffRun(t, repo, opened.RunID)
	if first.Data.Digest == second.Data.Digest {
		t.Fatalf("binary digest unchanged after one-byte change: %q", second.Data.Digest)
	}
}

func TestDiffAuditsRenameByExactPhysicalDestination(t *testing.T) {
	repo := newTestRepo(t)
	writeRepoFileAndCommit(t, repo, "safe.txt", "same content\n")
	runGit(t, repo, "config", "diff.renames", "true")
	input := validMissionInput(t, repo)
	input.AllowedPaths = []string{"safe.txt"}
	input.ForbiddenPaths = []string{"secret.txt"}
	opened := openWithMissionInput(t, repo, input)
	defer cleanupWorktree(t, repo, opened.WorktreePath)
	runGit(t, opened.WorktreePath, "mv", "safe.txt", "secret.txt")

	result := diffRun(t, repo, opened.RunID)
	if !containsString(result.Data.Files, "safe.txt") || !containsString(result.Data.Files, "secret.txt") {
		t.Fatalf("rename files = %q; want exact source and destination", result.Data.Files)
	}
	if !containsString(result.Warnings, "FORBIDDEN path modified: secret.txt") {
		t.Fatalf("rename warnings = %q; want exact forbidden destination", result.Warnings)
	}
}

func TestDiffDigestMatchesReviewAndStagedPathsForSameTree(t *testing.T) {
	repo := newTestRepo(t)
	git, gitErr := gitx.New()
	if gitErr != nil {
		t.Fatalf("gitx.New: %v", gitErr)
	}
	baseSHA, headErr := git.RevParseHead(context.Background(), repo)
	if headErr != nil {
		t.Fatalf("RevParseHead: %v", headErr)
	}
	writeWorktreeFile(t, repo, "new.txt", "reviewed content\n")

	reviewSnapshot, snapshotErr := git.DiffSnapshot(context.Background(), repo, baseSHA)
	if snapshotErr != nil {
		t.Fatalf("review DiffSnapshot: %v", snapshotErr)
	}
	reviewPatch := reviewSnapshot.Patch
	if stageErr := git.StageAll(context.Background(), repo); stageErr != nil {
		t.Fatalf("StageAll reviewed tree: %v", stageErr)
	}
	stagedPatch, stagedErr := git.DiffStaged(context.Background(), repo, baseSHA)
	if stagedErr != nil {
		t.Fatalf("DiffStaged reviewed tree: %v", stagedErr)
	}
	if reviewDigest, stagedDigest := diffDigest(reviewPatch), diffDigest(stagedPatch); reviewDigest != stagedDigest {
		t.Fatalf("same tree digests differ: review %q staged %q\nreview patch:\n%s\nstaged patch:\n%s", reviewDigest, stagedDigest, reviewPatch, stagedPatch)
	}

	writeWorktreeFile(t, repo, "new.txt", "reviewed contenT\n")
	if stageChangedErr := git.StageAll(context.Background(), repo); stageChangedErr != nil {
		t.Fatalf("StageAll one-byte change: %v", stageChangedErr)
	}
	changedPatch, changedErr := git.DiffStaged(context.Background(), repo, baseSHA)
	if changedErr != nil {
		t.Fatalf("DiffStaged one-byte change: %v", changedErr)
	}
	if diffDigest(reviewPatch) == diffDigest(changedPatch) {
		t.Fatalf("digest unchanged after one-byte content change: %q", diffDigest(changedPatch))
	}
}

func TestDiffDigestRemovesOnlyIndexMetadataLines(t *testing.T) {
	first := "diff --git a/file b/file\nindex 1111111..2222222 100644\n--- a/file\n+++ b/file\n@@ -1 +1 @@\n-index old content\n+index new content\n"
	metadataChanged := strings.Replace(first, "index 1111111..2222222 100644", "index aaaaaaa..bbbbbbb 100644", 1)
	contentChanged := strings.Replace(first, "+index new content", "+index new contenT", 1)

	if diffDigest(first) != diffDigest(metadataChanged) {
		t.Fatalf("digest changed when only index metadata changed: %q != %q", diffDigest(first), diffDigest(metadataChanged))
	}
	if diffDigest(first) == diffDigest(contentChanged) {
		t.Fatalf("digest ignored one-byte content change: %q", diffDigest(first))
	}
}

func TestDiffPersistsReviewedState(t *testing.T) {
	repo, opened := openFixture(t)
	defer cleanupWorktree(t, repo, opened.WorktreePath)
	writeWorktreeFile(t, opened.WorktreePath, "README.md", "reviewed\n")

	result := diffRun(t, repo, opened.RunID)
	state, err := runstate.Load(repo, opened.RunID)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if state.Status != runstate.StatusReviewed || state.ReviewedDigest != result.Data.Digest || state.ReviewedAt.IsZero() || state.ReviewedAt.Location() != time.UTC {
		t.Fatalf("review state = %#v; digest = %q", state, result.Data.Digest)
	}
}

func TestDiffWarnsForPhysicalPathOutsideAllowedPaths(t *testing.T) {
	repo := newTestRepo(t)
	writeRepoFileAndCommit(t, repo, "outside.txt", "base\n")
	opened := openWithMissionInput(t, repo, validMissionInput(t, repo))
	defer cleanupWorktree(t, repo, opened.WorktreePath)
	writeWorktreeFile(t, opened.WorktreePath, "outside.txt", "changed\n")

	result := diffRun(t, repo, opened.RunID)
	if !containsString(result.Warnings, "out-of-scope change: outside.txt") {
		t.Fatalf("warnings = %v", result.Warnings)
	}
	if len(result.Data.OutOfScope) != 1 || result.Data.OutOfScope[0] != "outside.txt" {
		t.Fatalf("out_of_scope = %v", result.Data.OutOfScope)
	}
}

func TestDiffWarnsForForbiddenPath(t *testing.T) {
	repo := newTestRepo(t)
	writeRepoFileAndCommit(t, repo, "secret.txt", "base\n")
	input := validMissionInput(t, repo)
	input.ForbiddenPaths = []string{"secret.txt"}
	opened := openWithMissionInput(t, repo, input)
	defer cleanupWorktree(t, repo, opened.WorktreePath)
	writeWorktreeFile(t, opened.WorktreePath, "secret.txt", "changed\n")

	result := diffRun(t, repo, opened.RunID)
	if !containsString(result.Warnings, "FORBIDDEN path modified: secret.txt") {
		t.Fatalf("warnings = %v", result.Warnings)
	}
}

func TestDiffEmptyIsOKWithWarning(t *testing.T) {
	repo, opened := openFixture(t)
	defer cleanupWorktree(t, repo, opened.WorktreePath)

	result := diffRun(t, repo, opened.RunID)
	if result.Status != "ok" || !containsString(result.Warnings, "no changes yet") {
		t.Fatalf("result = %#v", result)
	}
	if result.Data.Files == nil || result.Data.InScope == nil || result.Data.OutOfScope == nil || result.Data.Diff != "" {
		t.Fatalf("empty diff data = %#v", result.Data)
	}
}

func TestDiffTruncatesInlineButDigestsFullDiff(t *testing.T) {
	repo, opened := openFixture(t)
	defer cleanupWorktree(t, repo, opened.WorktreePath)
	writeWorktreeFile(t, opened.WorktreePath, "README.md", strings.Repeat("large changed line\n", 2000))

	result := diffRun(t, repo, opened.RunID)
	git, err := gitx.New()
	if err != nil {
		t.Fatalf("gitx.New: %v", err)
	}
	fullDiff, err := git.Diff(context.Background(), opened.WorktreePath, opened.BaseSHA)
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}
	wantDigest := diffDigest(fullDiff)
	if result.Data.Digest != wantDigest {
		t.Fatalf("digest = %q; want full diff digest %q", result.Data.Digest, wantDigest)
	}
	if len(result.Data.Diff) > 16*1024+64 || !strings.Contains(result.Data.Diff, "diff truncated") || !containsString(result.Warnings, "diff exceeds 16KB; inline output truncated") {
		t.Fatalf("truncated diff bytes = %d warnings = %v", len(result.Data.Diff), result.Warnings)
	}
}

func diffRun(t *testing.T, repo, runID string) DiffResult {
	t.Helper()
	result, err := WorkspaceDiff(context.Background(), repo, DiffInput{RunID: runID})
	if err != nil {
		t.Fatalf("WorkspaceDiff: %v", err)
	}
	if result.Status != "ok" {
		t.Fatalf("result = %#v", result)
	}
	return result
}

func saveForgedRootRun(t *testing.T, repo string, status runstate.Status) runstate.Run {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	input := validMissionInput(t, repo)
	input.VerificationCommands = nil
	mission, compileStatus, _ := missioncompile.Compile(repo, input)
	if compileStatus != "ok" {
		t.Fatalf("Compile status = %q; mission = %#v", compileStatus, mission)
	}
	head := runGit(t, repo, "rev-parse", "HEAD")
	run := runstate.Run{
		RunID:        "run_eeeeeeeeeeeeeeee",
		MissionID:    mission.MissionID,
		MissionInput: input,
		Mission:      mission,
		Status:       status,
		CreatedAt:    time.Now().UTC(),
		BaseSHA:      head,
		Branch:       "jacu/run-eeeeeeeeeeeeeeee",
		Worktree:     repo,
	}
	if status == runstate.StatusReviewed {
		git, err := gitx.New()
		if err != nil {
			t.Fatalf("gitx.New: %v", err)
		}
		fullDiff, err := git.Diff(context.Background(), repo, head)
		if err != nil {
			t.Fatalf("Diff: %v", err)
		}
		run.ReviewedDigest = diffDigest(fullDiff)
		run.ReviewedAt = time.Now().UTC()
	}
	if err := runstate.Save(repo, run); err != nil {
		t.Fatalf("Save forged run: %v", err)
	}
	return run
}

func openWithMissionInput(t *testing.T, repo string, input missioncompile.Input) OpenData {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	mission, status, _ := missioncompile.Compile(repo, input)
	if status != "ok" {
		t.Fatalf("Compile status = %q; mission = %#v", status, mission)
	}
	result, err := Open(context.Background(), repo, OpenInput{MissionInput: input, MissionID: mission.MissionID})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if result.Status != "ok" {
		t.Fatalf("Open result = %#v", result)
	}
	return result.Data
}

func writeRepoFileAndCommit(t *testing.T, repo, name, content string) {
	t.Helper()
	writeWorktreeFile(t, repo, name, content)
	runGit(t, repo, "add", name)
	runGit(t, repo, "commit", "-m", "add "+name)
}

func writeWorktreeFile(t *testing.T, worktree, name, content string) {
	t.Helper()
	path := filepath.Join(worktree, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("mkdir %s: %v", name, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}
