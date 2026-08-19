package workspace

import (
	"bytes"
	"context"
	"fmt"
	"github.com/jacu-dev/jacu-harness/internal/testgit"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jacu-dev/jacu-harness/internal/capability/missioncompile"
	"github.com/jacu-dev/jacu-harness/internal/runstate"
)

func TestStatusListsOpenRunFields(t *testing.T) {
	repo, opened := openFixture(t)
	defer cleanupWorktree(t, repo, opened.WorktreePath)

	result, err := WorkspaceStatus(context.Background(), repo)
	if err != nil {
		t.Fatalf("WorkspaceStatus: %v", err)
	}
	if result.Status != "ok" || len(result.Data.Runs) != 1 {
		t.Fatalf("result = %#v", result)
	}
	run := result.Data.Runs[0]
	if run.RunID != opened.RunID || run.Status != runstate.StatusOpen || run.AgeSeconds < 0 || run.DiskBytes <= 0 || run.DiffLines != 0 || run.BaseBehind != 0 {
		t.Fatalf("run status = %#v", run)
	}
}

func TestStatusWarnsForLargeDiff(t *testing.T) {
	repo, opened := openFixture(t)
	defer cleanupWorktree(t, repo, opened.WorktreePath)
	content := strings.Repeat("added line\n", 401)
	if err := os.WriteFile(filepath.Join(opened.WorktreePath, "README.md"), []byte(content), 0o600); err != nil {
		t.Fatalf("write large diff: %v", err)
	}

	result, err := WorkspaceStatus(context.Background(), repo)
	if err != nil {
		t.Fatalf("WorkspaceStatus: %v", err)
	}
	lines := result.Data.Runs[0].DiffLines
	want := fmt.Sprintf("large diff (%d lines); consider splitting into smaller runs", lines)
	if lines <= 400 || !containsString(result.Warnings, want) {
		t.Fatalf("lines = %d warnings = %v; want %q", lines, result.Warnings, want)
	}
}

func TestStatusReportsStaleBase(t *testing.T) {
	repo, opened := openFixture(t)
	defer cleanupWorktree(t, repo, opened.WorktreePath)
	for i := 1; i <= 2; i++ {
		name := fmt.Sprintf("advance-%d.txt", i)
		if err := os.WriteFile(filepath.Join(repo, name), []byte("advance\n"), 0o600); err != nil {
			t.Fatalf("write advance: %v", err)
		}
		runGit(t, repo, "add", name)
		runGit(t, repo, "commit", "-m", "advance main")
	}

	result, err := WorkspaceStatus(context.Background(), repo)
	if err != nil {
		t.Fatalf("WorkspaceStatus: %v", err)
	}
	if result.Data.Runs[0].BaseBehind != 2 || !containsString(result.Warnings, "base is stale by 2 commits") {
		t.Fatalf("result = %#v", result)
	}
}

func TestStatusReportsUnreachableBasePerRunWithoutHidingHealthyRun(t *testing.T) {
	repo := newTestRepo(t)
	t.Setenv("HOME", t.TempDir())
	input := validMissionInput(t, repo)
	mission, status, _ := missioncompile.Compile(repo, input)
	if status != "ok" {
		t.Fatalf("Compile status = %q; want ok", status)
	}

	broken, brokenOpenErr := Open(context.Background(), repo, OpenInput{MissionInput: input, MissionID: mission.MissionID})
	if brokenOpenErr != nil || broken.Status != "ok" {
		t.Fatalf("Open broken fixture = (%#v, %v)", broken, brokenOpenErr)
	}
	defer cleanupWorktree(t, repo, broken.Data.WorktreePath)
	originalBranch := runGit(t, repo, "branch", "--show-current")
	runGit(t, repo, "switch", "--orphan", "replacement")
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("replacement\n"), 0o600); err != nil {
		t.Fatalf("write replacement README: %v", err)
	}
	runGit(t, repo, "add", "README.md")
	runGit(t, repo, "commit", "-m", "replacement root")
	replacementSHA := runGit(t, repo, "rev-parse", "HEAD")
	runGit(t, repo, "branch", "-D", originalBranch)
	runGit(t, broken.Data.WorktreePath, "reset", "--hard", replacementSHA)
	runGit(t, repo, "reflog", "expire", "--expire=now", "--all")
	runGit(t, repo, "gc", "--prune=now")
	// #nosec G204 -- the SHA comes from the test-owned Open result and is passed without a shell.
	checkOldBase := exec.Command("git", "cat-file", "-e", broken.Data.BaseSHA+"^{commit}")
	checkOldBase.Dir = repo
	checkOldBase.Env = testgit.Env()
	if err := checkOldBase.Run(); err == nil {
		t.Fatalf("old base %s remains reachable", broken.Data.BaseSHA)
	}

	input = validMissionInput(t, repo)
	mission, status, _ = missioncompile.Compile(repo, input)
	if status != "ok" {
		t.Fatalf("Compile healthy fixture status = %q; want ok", status)
	}
	healthy, healthyOpenErr := Open(context.Background(), repo, OpenInput{MissionInput: input, MissionID: mission.MissionID})
	if healthyOpenErr != nil || healthy.Status != "ok" {
		t.Fatalf("Open healthy fixture = (%#v, %v)", healthy, healthyOpenErr)
	}
	defer cleanupWorktree(t, repo, healthy.Data.WorktreePath)

	result, err := WorkspaceStatus(context.Background(), repo)
	if err != nil {
		t.Fatalf("WorkspaceStatus: %v; want per-run degradation", err)
	}
	if len(result.Data.Runs) != 2 {
		t.Fatalf("listed runs = %d; want broken and healthy runs", len(result.Data.Runs))
	}
	statuses := map[string]runstate.Status{}
	for _, run := range result.Data.Runs {
		statuses[run.RunID] = run.Status
	}
	if statuses[broken.Data.RunID] != runstate.StatusCorrupted || statuses[healthy.Data.RunID] != runstate.StatusOpen {
		t.Fatalf("run statuses = %v; want broken=corrupted healthy=open", statuses)
	}
	warning := strings.Join(result.Warnings, "\n")
	if !strings.Contains(warning, broken.Data.RunID) || !strings.Contains(warning, "discard --gc") {
		t.Fatalf("warnings = %q; want broken run_id and discard --gc", warning)
	}
}

func TestStatusCountsTrackedAndUntrackedLinesWithoutMutatingRepository(t *testing.T) {
	repo, opened := openFixture(t)
	defer cleanupWorktree(t, repo, opened.WorktreePath)
	if err := os.WriteFile(filepath.Join(opened.WorktreePath, "README.md"), []byte("fixture\ntracked line\n"), 0o600); err != nil {
		t.Fatalf("write tracked change: %v", err)
	}
	if err := os.WriteFile(filepath.Join(opened.WorktreePath, "new file.txt"), []byte("untracked one\nuntracked two\n"), 0o600); err != nil {
		t.Fatalf("write untracked change: %v", err)
	}

	porcelainBefore := runGit(t, opened.WorktreePath, "status", "--porcelain=v1", "-z")
	indexPath := runGit(t, opened.WorktreePath, "rev-parse", "--git-path", "index")
	if !filepath.IsAbs(indexPath) {
		indexPath = filepath.Join(opened.WorktreePath, indexPath)
	}
	indexBefore := snapshotPhysicalFile(t, indexPath)
	statePath := filepath.Join(repo, ".git", "jacu", "runs", opened.RunID+".json")
	stateBefore := snapshotPhysicalFile(t, statePath)

	result, err := WorkspaceStatus(context.Background(), repo)
	if err != nil {
		t.Fatalf("WorkspaceStatus: %v", err)
	}
	if result.Data.Runs[0].DiffLines != 3 {
		t.Fatalf("DiffLines = %d; want tracked 1 + untracked 2", result.Data.Runs[0].DiffLines)
	}

	assertPhysicalFileUnchanged(t, "index after WorkspaceStatus", indexBefore, snapshotPhysicalFile(t, indexPath))
	assertPhysicalFileUnchanged(t, "run state after WorkspaceStatus", stateBefore, snapshotPhysicalFile(t, statePath))
	porcelainAfter := runGit(t, opened.WorktreePath, "status", "--porcelain=v1", "-z")
	if porcelainAfter != porcelainBefore {
		t.Fatalf("porcelain status changed:\nbefore: %q\nafter:  %q", porcelainBefore, porcelainAfter)
	}
}

func TestStatusReportsMissingWorktreeCorruptedWithoutMutatingState(t *testing.T) {
	for _, persistedStatus := range []runstate.Status{runstate.StatusOpen, runstate.StatusReviewed} {
		t.Run(string(persistedStatus), func(t *testing.T) {
			assertStatusReportsMissingWorktreeWithoutMutation(t, persistedStatus)
		})
	}
}

func assertStatusReportsMissingWorktreeWithoutMutation(t *testing.T, persistedStatus runstate.Status) {
	t.Helper()
	repo, opened := openFixture(t)
	if persistedStatus == runstate.StatusReviewed {
		if _, err := WorkspaceDiff(context.Background(), repo, DiffInput{RunID: opened.RunID}); err != nil {
			t.Fatalf("WorkspaceDiff: %v", err)
		}
	}
	statePath := filepath.Join(repo, ".git", "jacu", "runs", opened.RunID+".json")
	// #nosec G304 -- statePath is the runstate file in this test's temporary repository.
	beforeBytes, readErr := os.ReadFile(statePath)
	if readErr != nil {
		t.Fatalf("read run state before status: %v", readErr)
	}
	knownMTime := time.Unix(1_700_000_000, 0).UTC()
	if err := os.Chtimes(statePath, knownMTime, knownMTime); err != nil {
		t.Fatalf("set run state mtime: %v", err)
	}
	beforeInfo, statErr := os.Stat(statePath)
	if statErr != nil {
		t.Fatalf("stat run state before status: %v", statErr)
	}
	if !beforeInfo.ModTime().Equal(knownMTime) {
		t.Fatalf("run state mtime = %v; want forced %v", beforeInfo.ModTime(), knownMTime)
	}
	if err := os.RemoveAll(opened.WorktreePath); err != nil {
		t.Fatalf("remove worktree fixture: %v", err)
	}
	defer runGit(t, repo, "worktree", "prune")

	result, statusErr := WorkspaceStatus(context.Background(), repo)
	if statusErr != nil {
		t.Fatalf("WorkspaceStatus: %v", statusErr)
	}
	wantWarning := fmt.Sprintf("run %s reported as corrupted: worktree missing; use discard --gc", opened.RunID)
	if result.Data.Runs[0].Status != runstate.StatusCorrupted || !containsString(result.Warnings, wantWarning) {
		t.Fatalf("result = %#v", result)
	}
	// #nosec G304 -- statePath is the runstate file in this test's temporary repository.
	afterBytes, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("read run state after status: %v", err)
	}
	if !bytes.Equal(afterBytes, beforeBytes) {
		t.Fatalf("run state bytes changed:\nbefore: %s\nafter:  %s", beforeBytes, afterBytes)
	}
	afterInfo, err := os.Stat(statePath)
	if err != nil {
		t.Fatalf("stat run state after status: %v", err)
	}
	if !afterInfo.ModTime().Equal(beforeInfo.ModTime()) {
		t.Fatalf("run state mtime changed from %v to %v", beforeInfo.ModTime(), afterInfo.ModTime())
	}
	state, err := runstate.Load(repo, opened.RunID)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if state.Status != persistedStatus {
		t.Fatalf("persisted status = %q; want original %q", state.Status, persistedStatus)
	}
}

type physicalFileSnapshot struct {
	Exists  bool
	Content []byte
	Mode    os.FileMode
	MTime   time.Time
}

func snapshotPhysicalFile(t *testing.T, path string) physicalFileSnapshot {
	t.Helper()
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return physicalFileSnapshot{}
	}
	if err != nil {
		t.Fatalf("lstat %q: %v", path, err)
	}
	// #nosec G304 -- path belongs to the test-owned temporary repository.
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %q: %v", path, err)
	}
	return physicalFileSnapshot{Exists: true, Content: content, Mode: info.Mode(), MTime: info.ModTime()}
}

func assertPhysicalFileUnchanged(t *testing.T, label string, before, after physicalFileSnapshot) {
	t.Helper()
	if before.Exists != after.Exists || !bytes.Equal(before.Content, after.Content) || before.Mode != after.Mode || !before.MTime.Equal(after.MTime) {
		t.Fatalf("%s changed:\nbefore: exists=%v mode=%v mtime=%v bytes=%x\nafter:  exists=%v mode=%v mtime=%v bytes=%x", label, before.Exists, before.Mode, before.MTime, before.Content, after.Exists, after.Mode, after.MTime, after.Content)
	}
}

func openFixture(t *testing.T) (string, OpenData) {
	t.Helper()
	repo := newTestRepo(t)
	t.Setenv("HOME", t.TempDir())
	input := validMissionInput(t, repo)
	mission, status, _ := missioncompile.Compile(repo, input)
	if status != "ok" {
		t.Fatalf("Compile status = %q", status)
	}
	result, err := Open(context.Background(), repo, OpenInput{MissionInput: input, MissionID: mission.MissionID})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if result.Status != "ok" {
		t.Fatalf("Open result = %#v", result)
	}
	return repo, result.Data
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
