package runstate

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestSaveLoadRoundTripAndCreatedAtRFC3339(t *testing.T) {
	repo := newStateRepo(t)
	want := fixtureRun("run_0000000000000001", time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC))
	want.SchemaVersion = CurrentSchemaVersion

	if err := Save(repo, want); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := Load(repo, want.RunID)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("round trip differs\n got: %#v\nwant: %#v", got, want)
	}
	encoded, err := os.ReadFile(runPath(repo, want.RunID))
	if err != nil {
		t.Fatalf("read state: %v", err)
	}
	if !strings.Contains(string(encoded), `"created_at": "2026-07-30T12:00:00Z"`) {
		t.Fatalf("created_at is not RFC3339 UTC: %s", encoded)
	}
}

func TestSaveRejectsCreatedAtChange(t *testing.T) {
	repo := newStateRepo(t)
	run := fixtureRun("run_0000000000000002", time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC))
	if err := Save(repo, run); err != nil {
		t.Fatalf("initial Save: %v", err)
	}
	run.CreatedAt = run.CreatedAt.Add(time.Minute)

	err := Save(repo, run)
	if err == nil || !strings.Contains(err.Error(), "created_at is immutable") {
		t.Fatalf("Save error = %v; want immutable created_at error", err)
	}
}

func TestListOrdersOldestRunFirst(t *testing.T) {
	repo := newStateRepo(t)
	newer := fixtureRun("run_0000000000000003", time.Date(2026, 7, 30, 13, 0, 0, 0, time.UTC))
	older := fixtureRun("run_0000000000000004", time.Date(2026, 7, 30, 11, 0, 0, 0, time.UTC))
	if err := Save(repo, newer); err != nil {
		t.Fatalf("Save newer: %v", err)
	}
	if err := Save(repo, older); err != nil {
		t.Fatalf("Save older: %v", err)
	}

	runs, err := List(repo)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	got := []string{runs[0].RunID, runs[1].RunID}
	if !slices.Equal(got, []string{"run_0000000000000004", "run_0000000000000003"}) {
		t.Fatalf("List order = %v; want oldest first", got)
	}
}

func TestRunTransitions(t *testing.T) {
	tests := []struct {
		name string
		from Status
		to   Status
		ok   bool
	}{
		{name: "open to reviewed", from: StatusOpen, to: StatusReviewed, ok: true},
		{name: "reviewed to applied", from: StatusReviewed, to: StatusApplied, ok: true},
		{name: "open to discarded", from: StatusOpen, to: StatusDiscarded, ok: true},
		{name: "reviewed to discarded", from: StatusReviewed, to: StatusDiscarded, ok: true},
		{name: "corrupted to discarded", from: StatusCorrupted, to: StatusDiscarded, ok: true},
		{name: "applied to corrupted", from: StatusApplied, to: StatusCorrupted, ok: true},
		{name: "applied to open", from: StatusApplied, to: StatusOpen, ok: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			run := Run{Status: tt.from}
			err := run.Transition(tt.to)
			if (err == nil) != tt.ok {
				t.Fatalf("Transition(%q -> %q) error = %v; ok = %v", tt.from, tt.to, err, tt.ok)
			}
			if tt.ok && run.Status != tt.to {
				t.Fatalf("status = %q; want %q", run.Status, tt.to)
			}
		})
	}
}

func TestCorruptedJSONDoesNotBreakList(t *testing.T) {
	repo := newStateRepo(t)
	valid := fixtureRun("run_0000000000000005", time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC))
	if err := Save(repo, valid); err != nil {
		t.Fatalf("Save valid: %v", err)
	}
	if err := os.WriteFile(runPath(repo, "run_0000000000000006"), []byte(`{"run_id":`), 0o600); err != nil {
		t.Fatalf("write corrupt state: %v", err)
	}

	if _, err := Load(repo, "run_0000000000000006"); err == nil || !strings.Contains(err.Error(), "decode run run_0000000000000006") {
		t.Fatalf("Load corrupt error = %v", err)
	}
	runs, err := List(repo)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(runs) != 2 {
		t.Fatalf("List len = %d; want 2", len(runs))
	}
	for _, run := range runs {
		if run.RunID == "run_0000000000000006" && run.Status != StatusCorrupted {
			t.Fatalf("corrupt run status = %q; want %q", run.Status, StatusCorrupted)
		}
	}
}

func TestListBindsRunIdentityToStateFilename(t *testing.T) {
	repo := newStateRepo(t)
	run := fixtureRun("run_0000000000000007", time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC))
	if err := Save(repo, run); err != nil {
		t.Fatalf("Save: %v", err)
	}
	content, readErr := os.ReadFile(runPath(repo, run.RunID))
	if readErr != nil {
		t.Fatalf("read internal state: %v", readErr)
	}
	const filenameID = "run_0000000000000008"
	// #nosec G703 -- filenameID is a fixed valid test fixture under t.TempDir.
	if writeErr := os.WriteFile(runPath(repo, filenameID), content, 0o600); writeErr != nil {
		t.Fatalf("write mismatched state: %v", writeErr)
	}
	if removeErr := os.Remove(runPath(repo, run.RunID)); removeErr != nil {
		t.Fatalf("remove canonical state fixture: %v", removeErr)
	}
	if _, loadErr := Load(repo, filenameID); loadErr == nil || !strings.Contains(loadErr.Error(), "does not match requested run_id") {
		t.Fatalf("Load mismatch error = %v; want filename/internal run_id mismatch", loadErr)
	}

	runs, listErr := List(repo)
	if listErr != nil {
		t.Fatalf("List: %v", listErr)
	}
	if len(runs) != 1 || runs[0].RunID != filenameID || runs[0].Status != StatusCorrupted {
		t.Fatalf("List mismatch result = %#v; want filename-bound corrupted run", runs)
	}
}

func TestSaveCleansTemporaryFileWhenRenameFails(t *testing.T) {
	repo := newStateRepo(t)
	run := fixtureRun("run_0000000000000009", time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC))
	renameErr := errors.New("rename failed")

	err := saveWithRename(repo, run, func(string, string) error { return renameErr })
	if !errors.Is(err, renameErr) {
		t.Fatalf("saveWithRename error = %v; want %v", err, renameErr)
	}
	temps, err := filepath.Glob(filepath.Join(runsDir(repo), ".*.tmp"))
	if err != nil {
		t.Fatalf("glob temps: %v", err)
	}
	if len(temps) != 0 {
		t.Fatalf("temporary files remain: %v", temps)
	}
	if _, err := os.Stat(runPath(repo, run.RunID)); !os.IsNotExist(err) {
		t.Fatalf("partial run file exists: %v", err)
	}
}

func newStateRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	if err := os.Mkdir(filepath.Join(repo, ".git"), 0o700); err != nil {
		t.Fatalf("mkdir .git: %v", err)
	}
	return repo
}

func fixtureRun(runID string, createdAt time.Time) Run {
	missionInput := MissionInput{
		Objective:            "Update fixture file",
		AcceptanceCriteria:   []string{"fixture updated"},
		VerificationCommands: [][]string{{"go", "test", "./..."}},
		AllowedPaths:         []string{"README.md"},
		RiskHint:             "write",
	}
	mission := MissionSnapshot{
		MissionID:            "msn_fixture",
		Ceremony:             "light",
		Objective:            "Update fixture file",
		AcceptanceCriteria:   []string{"fixture updated"},
		VerificationCommands: [][]string{{"go", "test", "./..."}},
		AllowedPaths:         []string{"README.md"},
		Risk:                 "write",
		Lint:                 []Lint{},
	}
	return Run{
		RunID:        runID,
		MissionID:    mission.MissionID,
		MissionInput: missionInput,
		Mission:      mission,
		Status:       StatusOpen,
		CreatedAt:    createdAt,
		BaseSHA:      "0123456789abcdef",
		Branch:       "jacu/" + runID,
		Worktree:     filepath.Join("/tmp", runID),
		ReviewedAt:   time.Time{},
	}
}
