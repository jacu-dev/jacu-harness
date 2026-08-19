package runstate_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jacu-dev/jacu-harness/internal/capability/workspace"
	"github.com/jacu-dev/jacu-harness/internal/runstate"
)

func TestLoadValidatesRunIDAndStoredIdentity(t *testing.T) {
	repo := filepath.Join(t.TempDir(), "nested", "project")
	if err := os.MkdirAll(filepath.Join(repo, ".git", "jacu", "runs"), 0o700); err != nil {
		t.Fatalf("create runs directory: %v", err)
	}

	const validRunID = "run_0123456789abcdef"
	const otherRunID = "run_fedcba9876543210"
	validState := marshalRunState(t, runstate.Run{RunID: validRunID})
	otherState := marshalRunState(t, runstate.Run{RunID: otherRunID})
	traversalState := marshalRunState(t, runstate.Run{RunID: "../../../pwn"})

	traversalPath := runStatePath(repo, "../../../pwn")
	if err := os.WriteFile(traversalPath, traversalState, 0o600); err != nil {
		t.Fatalf("plant traversal target %q: %v", traversalPath, err)
	}
	if err := os.WriteFile(runStatePath(repo, validRunID), validState, 0o600); err != nil {
		t.Fatalf("write valid state: %v", err)
	}
	if err := os.WriteFile(runStatePath(repo, otherRunID), otherState, 0o600); err != nil {
		t.Fatalf("write other state: %v", err)
	}

	tests := []struct {
		name       string
		runID      string
		wantErr    string
		wantLoaded bool
	}{
		{name: "relative traversal", runID: "../../../pwn", wantErr: "invalid run_id"},
		{name: "deep traversal", runID: "../../../../../../tmp/pwn", wantErr: "invalid run_id"},
		{name: "embedded separator", runID: "run_aaaa/../bbbb", wantErr: "invalid run_id"},
		{name: "absolute", runID: "/tmp/pwn", wantErr: "invalid run_id"},
		{name: "empty", runID: "", wantErr: "invalid run_id"},
		{name: "stored identity mismatch", runID: validRunID, wantErr: "does not match"},
		{name: "valid coherent", runID: otherRunID, wantLoaded: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.name == "stored identity mismatch" {
				if err := os.WriteFile(runStatePath(repo, validRunID), otherState, 0o600); err != nil {
					t.Fatalf("write mismatched state: %v", err)
				}
			}
			run, err := runstate.Load(repo, tt.runID)
			if tt.wantLoaded {
				if err != nil {
					t.Fatalf("Load(%q): %v", tt.runID, err)
				}
				if run.RunID != tt.runID {
					t.Fatalf("Load(%q) run_id = %q; want %q", tt.runID, run.RunID, tt.runID)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Load(%q) error = %v; want error containing %q", tt.runID, err, tt.wantErr)
			}
		})
	}
}

func TestListReportsInvalidFilenameAndDiscardGCCleansIt(t *testing.T) {
	repo := t.TempDir()
	runsDir := filepath.Join(repo, ".git", "jacu", "runs")
	if err := os.MkdirAll(runsDir, 0o700); err != nil {
		t.Fatalf("create runs directory: %v", err)
	}
	const invalidRunID = "pwn"
	statePath := filepath.Join(runsDir, invalidRunID+".json")
	if err := os.WriteFile(statePath, []byte(`{"run_id":"pwn"}`), 0o600); err != nil {
		t.Fatalf("write invalid run filename: %v", err)
	}

	runs, err := runstate.List(repo)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(runs) != 1 || runs[0].RunID != invalidRunID || runs[0].Status != runstate.StatusCorrupted {
		t.Fatalf("List result = %#v; want invalid filename reported as corrupted", runs)
	}

	result, err := workspace.Discard(context.Background(), repo, workspace.DiscardInput{GC: true})
	if err != nil {
		t.Fatalf("Discard gc: %v", err)
	}
	if result.Status != "ok" || len(result.Data.Runs) != 1 || result.Data.Runs[0].RunID != invalidRunID {
		t.Fatalf("Discard gc result = %#v; want invalid metadata cleaned", result)
	}
	if _, err := os.Stat(statePath); !os.IsNotExist(err) {
		t.Fatalf("invalid run metadata remains after gc: %v", err)
	}
}

func marshalRunState(t *testing.T, run runstate.Run) []byte {
	t.Helper()
	encoded, err := json.Marshal(run)
	if err != nil {
		t.Fatalf("marshal run state: %v", err)
	}
	return encoded
}

func runStatePath(repo, runID string) string {
	return filepath.Join(repo, ".git", "jacu", "runs", runID+".json")
}
