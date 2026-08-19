package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jacu-dev/jacu-harness/internal/runstate"
)

// statusFixture builds a repository with a real linked-worktree layout: the
// worktree carries a `.git` file pointing back at the repository, which is the
// only thing that ties a directory under ~/.jacu-harness to a project.
func statusFixture(t *testing.T) (repo, worktrees string, now time.Time) {
	t.Helper()
	base := t.TempDir()
	repo = filepath.Join(base, "myproject")
	if err := os.MkdirAll(filepath.Join(repo, ".git", "jacu", "runs"), 0o700); err != nil {
		t.Fatal(err)
	}
	worktrees = filepath.Join(base, "worktrees")
	return repo, worktrees, time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC)
}

func linkWorktree(t *testing.T, repo, worktrees, projectID, runID string) string {
	t.Helper()
	dir := filepath.Join(worktrees, projectID, runID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	link := fmt.Sprintf("gitdir: %s\n", filepath.Join(repo, ".git", "worktrees", runID))
	if err := os.WriteFile(filepath.Join(dir, ".git"), []byte(link), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

func saveRun(t *testing.T, repo, runID string, status runstate.Status, objective string, created time.Time) {
	t.Helper()
	run := runstate.Run{
		SchemaVersion: runstate.CurrentSchemaVersion,
		RunID:         runID,
		MissionID:     "msn_" + runID[4:],
		MissionInput:  runstate.MissionInput{Objective: objective},
		Status:        status,
		CreatedAt:     created,
		BaseSHA:       "0000000000000000000000000000000000000000",
		Branch:        "jacu/run-" + runID[4:],
	}
	if err := runstate.Save(repo, run); err != nil {
		t.Fatal(err)
	}
}

func TestStatusReportsParkedRunWithObjectiveAndAge(t *testing.T) {
	repo, worktrees, now := statusFixture(t)
	dir := linkWorktree(t, repo, worktrees, "prj_1111111111111111", "run_1111111111111111")
	if err := os.WriteFile(filepath.Join(dir, "payload"), make([]byte, 4096), 0o600); err != nil {
		t.Fatal(err)
	}
	saveRun(t, repo, "run_1111111111111111", runstate.StatusReviewed, "endurecer a vitrine", now.Add(-3*time.Hour))

	report := Status(StatusOptions{Now: now, WorktreesRoot: worktrees})

	if report.TotalRuns != 1 {
		t.Fatalf("expected one parked run, got %d: %+v", report.TotalRuns, report.Runs)
	}
	run := report.Runs[0]
	if run.Status != string(runstate.StatusReviewed) {
		t.Fatalf("status = %q, want reviewed", run.Status)
	}
	if run.Objective != "endurecer a vitrine" {
		t.Fatalf("objective = %q", run.Objective)
	}
	if run.RepoName != "myproject" {
		t.Fatalf("repo name = %q, want myproject", run.RepoName)
	}
	if run.AgeSeconds != int64(3*time.Hour/time.Second) {
		t.Fatalf("age = %ds, want %d", run.AgeSeconds, int64(3*time.Hour/time.Second))
	}
	if run.Bytes < 4096 {
		t.Fatalf("bytes = %d, want at least the 4096 written", run.Bytes)
	}
	if run.BytesTruncated || report.Truncated {
		t.Fatal("a four-kilobyte tree was reported as truncated")
	}
}

// The bug this guards: measurement stopped at 2048 visited paths and reported
// the partial total as if it were the answer. 88 MB was printed for 2.1 GB.
func TestStatusMeasuresTreeWiderThanTheInventoryBudget(t *testing.T) {
	repo, worktrees, now := statusFixture(t)
	dir := linkWorktree(t, repo, worktrees, "prj_2222222222222222", "run_2222222222222222")
	saveRun(t, repo, "run_2222222222222222", runstate.StatusOpen, "instala tudo", now)

	const files = maxInventory * 2
	const size = 64
	deep := filepath.Join(dir, "node_modules", "pkg")
	if err := os.MkdirAll(deep, 0o700); err != nil {
		t.Fatal(err)
	}
	for i := range files {
		name := filepath.Join(deep, fmt.Sprintf("f%05d", i))
		if err := os.WriteFile(name, make([]byte, size), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	report := Status(StatusOptions{Now: now, WorktreesRoot: worktrees})

	if report.TotalRuns != 1 {
		t.Fatalf("expected one run, got %d", report.TotalRuns)
	}
	want := int64(files * size)
	if report.Runs[0].Bytes < want {
		t.Fatalf("bytes = %d, want at least %d — the walk stopped early and said nothing", report.Runs[0].Bytes, want)
	}
	if report.Runs[0].BytesTruncated {
		t.Fatal("tree within the deep budget was flagged truncated")
	}
}

// The bug that actually printed 46 MB for 1.1 GB: the size walk gave up at the
// first symlink and reported the partial sum as the total. `node_modules/.bin`
// is entirely symlinks, so every real worktree hit this immediately.
func TestStatusKeepsMeasuringPastASymlink(t *testing.T) {
	repo, worktrees, now := statusFixture(t)
	dir := linkWorktree(t, repo, worktrees, "prj_8888888888888888", "run_8888888888888888")
	saveRun(t, repo, "run_8888888888888888", runstate.StatusOpen, "with dependencies", now)

	bin := filepath.Join(dir, "node_modules", ".bin")
	if err := os.MkdirAll(bin, 0o700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(dir, "node_modules", "real.js")
	if err := os.WriteFile(target, make([]byte, 2048), 0o600); err != nil {
		t.Fatal(err)
	}
	// The symlink is enumerated before `real.js` alphabetically inside
	// node_modules, so an aborting walk never reaches the payload.
	if err := os.Symlink(target, filepath.Join(bin, "cli")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "zzz.bin"), make([]byte, 4096), 0o600); err != nil {
		t.Fatal(err)
	}

	report := Status(StatusOptions{Now: now, WorktreesRoot: worktrees})

	if report.TotalRuns != 1 {
		t.Fatalf("expected one run, got %d", report.TotalRuns)
	}
	run := report.Runs[0]
	if run.BytesTruncated {
		t.Fatal("a symlink marked the measurement truncated; it is not an early stop")
	}
	if run.Bytes < 2048+4096 {
		t.Fatalf("bytes = %d, want at least 6144 — the walk stopped at the symlink", run.Bytes)
	}
}

func TestStatusFlagsWorktreeWithNoRunStateAsOrphan(t *testing.T) {
	repo, worktrees, now := statusFixture(t)
	linkWorktree(t, repo, worktrees, "prj_3333333333333333", "run_3333333333333333")
	// No saveRun: the directory exists and nothing claims it.

	report := Status(StatusOptions{Now: now, WorktreesRoot: worktrees})

	if report.TotalRuns != 1 {
		t.Fatalf("orphan not reported: %+v", report)
	}
	if got := report.Runs[0].Status; got != string(runstate.StatusCorrupted) && got != StatusOrphan {
		t.Fatalf("status = %q, want corrupted or orphan", got)
	}
}

func TestStatusFlagsWorktreeWithoutGitLinkAsOrphan(t *testing.T) {
	_, worktrees, now := statusFixture(t)
	dir := filepath.Join(worktrees, "prj_4444444444444444", "run_4444444444444444")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}

	report := Status(StatusOptions{Now: now, WorktreesRoot: worktrees})

	if report.TotalRuns != 1 || report.Runs[0].Status != StatusOrphan {
		t.Fatalf("detached directory not reported as orphan: %+v", report.Runs)
	}
	if report.Runs[0].Repo != "" {
		t.Fatalf("repo resolved for a directory with no git link: %q", report.Runs[0].Repo)
	}
}

// A worktree that survives a terminal run is a leak, and hiding it is how the
// disk fills. Terminal() is what a caller uses to say so.
func TestStatusReportsTerminalRunThatKeptItsWorktree(t *testing.T) {
	repo, worktrees, now := statusFixture(t)
	linkWorktree(t, repo, worktrees, "prj_5555555555555555", "run_5555555555555555")
	saveRun(t, repo, "run_5555555555555555", runstate.StatusApplied, "ja aplicado", now.Add(-time.Hour))

	report := Status(StatusOptions{Now: now, WorktreesRoot: worktrees})

	if report.TotalRuns != 1 {
		t.Fatalf("applied run with a live worktree was hidden: %+v", report)
	}
	if !report.Runs[0].Terminal() {
		t.Fatalf("Terminal() false for status %q", report.Runs[0].Status)
	}
}

func TestStatusSortsBiggestFirstAndTotalsBytes(t *testing.T) {
	repo, worktrees, now := statusFixture(t)
	small := linkWorktree(t, repo, worktrees, "prj_6666666666666666", "run_6666666666666666")
	big := linkWorktree(t, repo, worktrees, "prj_7777777777777777", "run_7777777777777777")
	saveRun(t, repo, "run_6666666666666666", runstate.StatusOpen, "pequeno", now)
	saveRun(t, repo, "run_7777777777777777", runstate.StatusOpen, "grande", now)
	if err := os.WriteFile(filepath.Join(small, "a"), make([]byte, 1024), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(big, "a"), make([]byte, 8192), 0o600); err != nil {
		t.Fatal(err)
	}

	report := Status(StatusOptions{Now: now, WorktreesRoot: worktrees})

	if report.TotalRuns != 2 {
		t.Fatalf("expected two runs, got %d", report.TotalRuns)
	}
	if report.Runs[0].RunID != "run_7777777777777777" {
		t.Fatalf("biggest run not first: %s", report.Runs[0].RunID)
	}
	if report.TotalBytes != report.Runs[0].Bytes+report.Runs[1].Bytes {
		t.Fatalf("total %d does not equal the sum of parts", report.TotalBytes)
	}
}

func TestStatusOnMissingRootIsEmptyAndNotAFailure(t *testing.T) {
	report := Status(StatusOptions{Now: time.Now(), WorktreesRoot: filepath.Join(t.TempDir(), "never-created")})

	if report.TotalRuns != 0 || len(report.Failed) != 0 {
		t.Fatalf("absent root should be silent: %+v", report)
	}
}
