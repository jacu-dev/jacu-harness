package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	storagecap "github.com/jacu-dev/jacu-harness/internal/capability/storage"
	"github.com/jacu-dev/jacu-harness/internal/runstate"
)

func statusCLIFixture(t *testing.T) (worktrees string, now time.Time) {
	t.Helper()
	base := t.TempDir()
	repo := filepath.Join(base, "vitrine")
	if err := os.MkdirAll(filepath.Join(repo, ".git", "jacu", "runs"), 0o700); err != nil {
		t.Fatal(err)
	}
	worktrees = filepath.Join(base, "worktrees")
	dir := filepath.Join(worktrees, "prj_abcdef0123456789", "run_abcdef0123456789")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	link := fmt.Sprintf("gitdir: %s\n", filepath.Join(repo, ".git", "worktrees", "run_abcdef0123456789"))
	if err := os.WriteFile(filepath.Join(dir, ".git"), []byte(link), 0o600); err != nil {
		t.Fatal(err)
	}
	now = time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC)
	run := runstate.Run{
		SchemaVersion: runstate.CurrentSchemaVersion,
		RunID:         "run_abcdef0123456789",
		MissionID:     "msn_abcdef0123456789",
		MissionInput:  runstate.MissionInput{Objective: "endurecer a vitrine e os heroes"},
		Status:        runstate.StatusReviewed,
		CreatedAt:     now.Add(-5 * time.Hour),
		BaseSHA:       "0000000000000000000000000000000000000000",
		Branch:        "jacu/run-abcdef0123456789",
	}
	if err := runstate.Save(repo, run); err != nil {
		t.Fatal(err)
	}
	return worktrees, now
}

func TestStatusCommandPrintsParkedRunWithObjective(t *testing.T) {
	worktrees, now := statusCLIFixture(t)
	var stdout, stderr bytes.Buffer

	code := runStatus(nil, &stdout, &stderr, storagecap.StatusOptions{Now: now, WorktreesRoot: worktrees})

	if code != 0 {
		t.Fatalf("exit = %d, want 0; stderr=%q", code, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{"run_abcdef0", "reviewed", "5h", "vitrine", "endurecer a vitrine", "1 parked run"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q:\n%s", want, out)
		}
	}
}

func TestStatusCommandJSONIsMachineReadable(t *testing.T) {
	worktrees, now := statusCLIFixture(t)
	var stdout, stderr bytes.Buffer

	code := runStatus([]string{"--json"}, &stdout, &stderr, storagecap.StatusOptions{Now: now, WorktreesRoot: worktrees})

	if code != 0 {
		t.Fatalf("exit = %d, want 0; stderr=%q", code, stderr.String())
	}
	var report storagecap.StatusReport
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, stdout.String())
	}
	if report.TotalRuns != 1 || report.Runs[0].RunID != "run_abcdef0123456789" {
		t.Fatalf("unexpected report: %+v", report)
	}
	if report.Runs[0].AgeSeconds != int64(5*time.Hour/time.Second) {
		t.Fatalf("age = %d", report.Runs[0].AgeSeconds)
	}
}

// The exit code must not encode how much work is parked: a prompt that runs
// this every command would start reporting failure for a normal state.
func TestStatusCommandExitsZeroWhetherOrNotWorkIsParked(t *testing.T) {
	worktrees, now := statusCLIFixture(t)
	var parked, empty bytes.Buffer
	var ignored bytes.Buffer

	withWork := runStatus(nil, &parked, &ignored, storagecap.StatusOptions{Now: now, WorktreesRoot: worktrees})
	without := runStatus(nil, &empty, &ignored, storagecap.StatusOptions{Now: now, WorktreesRoot: filepath.Join(t.TempDir(), "absent")})

	if withWork != 0 || without != 0 {
		t.Fatalf("exits: with work %d, without %d; both must be 0", withWork, without)
	}
	if !strings.Contains(empty.String(), "no parked runs") {
		t.Fatalf("empty output = %q", empty.String())
	}
}

func TestStatusCommandRejectsUnknownOptionWithTwo(t *testing.T) {
	var stdout, stderr bytes.Buffer

	code := runStatus([]string{"--all-the-things"}, &stdout, &stderr, storagecap.StatusOptions{WorktreesRoot: filepath.Join(t.TempDir(), "worktrees")})

	if code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout polluted on argument error: %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "usage") {
		t.Fatalf("stderr missing usage: %q", stderr.String())
	}
}

func TestTruncateObjectiveCutsOnRuneBoundary(t *testing.T) {
	// Byte slicing would split the ç and produce replacement characters.
	objective := strings.Repeat("ç", 40)

	got := truncateObjective(objective, 10)

	if !strings.HasSuffix(got, "...") {
		t.Fatalf("no ellipsis: %q", got)
	}
	if runes := []rune(strings.TrimSuffix(got, "...")); len(runes) != 10 {
		t.Fatalf("kept %d runes, want 10", len(runes))
	}
	if strings.ContainsRune(got, '\uFFFD') {
		t.Fatalf("split a rune: %q", got)
	}
}

func TestFormatBytesUsesTheUnitAPersonDecidesWith(t *testing.T) {
	cases := map[int64]string{
		512:             "512 B",
		4096:            "4 KB",
		5 * 1 << 20:     "5 MB",
		1<<30 + 1<<29:   "1.5 GB",
		2*1<<30 + 1<<27: "2.1 GB",
	}
	for bytes, want := range cases {
		if got := formatBytes(bytes); got != want {
			t.Errorf("formatBytes(%d) = %q, want %q", bytes, got, want)
		}
	}
}
