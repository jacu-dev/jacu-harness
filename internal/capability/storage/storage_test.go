package storage

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/jacu-dev/jacu-harness/internal/runstate"
	"github.com/jacu-dev/jacu-harness/internal/telemetry"
)

func TestArchiveRequiresCanonicalTerminalRunReferenceAndDigest(t *testing.T) {
	root, options, now := storageFixture(t)
	patch := []byte("diff --git a/a b/a\n")
	run := writeTerminalRun(t, root, options, "run_0123456789abcdef", now, patch)
	unknown := filepath.Join(root, ".git", "jacu", "archive", "archive-unknown.patch")
	writeOldFile(t, unknown, []byte("unknown"), now)

	preview := PruneWithOptions(root, options, false)
	assertActions(t, preview, "archive/"+run.RunID+".patch", "runs/"+run.RunID)
	if _, err := os.Stat(unknown); err != nil {
		t.Fatalf("dry-run changed unknown archive: %v", err)
	}

	result := PruneWithOptions(root, options, true)
	if len(result.Failed) != 0 || !result.Applied {
		t.Fatalf("apply failed: %+v", result)
	}
	if _, err := os.Stat(filepath.Join(root, run.ArchivePatch)); !os.IsNotExist(err) {
		t.Fatalf("canonical archive survived: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".git", "jacu", "runs", run.RunID+".json")); !os.IsNotExist(err) {
		t.Fatalf("terminal run survived: %v", err)
	}
	if _, err := os.Stat(unknown); err != nil {
		t.Fatalf("unknown archive removed: %v", err)
	}
}

func TestArchiveAndRunPreservedOnDigestMismatchOrCorruptState(t *testing.T) {
	root, options, now := storageFixture(t)
	run := writeTerminalRun(t, root, options, "run_1111111111111111", now, []byte("canonical"))
	archive := filepath.Join(root, run.ArchivePatch)
	if err := os.WriteFile(archive, []byte("tampered"), 0o600); err != nil {
		t.Fatal(err)
	}
	old := now.Add(-31 * 24 * time.Hour)
	if err := os.Chtimes(archive, old, old); err != nil {
		t.Fatal(err)
	}
	corrupt := filepath.Join(root, ".git", "jacu", "runs", "run_2222222222222222.json")
	writeOldFile(t, corrupt, []byte(`{"status":`), now)

	report := PruneWithOptions(root, options, true)
	if len(report.Skipped) == 0 {
		t.Fatal("archive swap was not reported as skipped")
	}
	if len(report.Actions) != 0 {
		t.Fatalf("unsafe records received actions: %+v", report.Actions)
	}
	for _, path := range []string{archive, filepath.Join(root, ".git", "jacu", "runs", run.RunID+".json"), corrupt} {
		if _, err := os.Lstat(path); err != nil {
			t.Fatalf("unsafe evidence removed %s: %v", filepath.Base(path), err)
		}
	}
}

func TestApplyRevalidatesArchiveAfterPlanBeforeRemove(t *testing.T) {
	root, options, now := storageFixture(t)
	run := writeTerminalRun(t, root, options, "run_3333333333333333", now, []byte("canonical"))
	archive := filepath.Join(root, run.ArchivePatch)
	target := filepath.Join(root, "user-data")
	if err := os.WriteFile(target, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	options.beforeAction = func(action Action) {
		if action.Owner != "archive" {
			return
		}
		if err := os.Remove(archive); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(target, archive); err != nil {
			t.Fatal(err)
		}
	}

	report := PruneWithOptions(root, options, true)
	if len(report.Skipped) == 0 {
		t.Fatal("archive swap was not reported as skipped")
	}
	if _, err := os.Lstat(archive); err != nil {
		t.Fatalf("swapped archive removed: %v", err)
	}
	content, err := readConfinedFile(root, "user-data")
	if err != nil || string(content) != "keep" {
		t.Fatalf("symlink target changed: %q %v", content, err)
	}
	if _, err := os.Stat(filepath.Join(root, ".git", "jacu", "runs", run.RunID+".json")); err != nil {
		t.Fatalf("run proof removed after archive swap: %v", err)
	}
}

func TestApplyRejectsArchiveDirectorySwap(t *testing.T) {
	root, options, now := storageFixture(t)
	run := writeTerminalRun(t, root, options, "run_4444444444444444", now, []byte("canonical"))
	archiveDir := filepath.Join(root, ".git", "jacu", "archive")
	moved := archiveDir + "-moved"
	options.beforeAction = func(action Action) {
		if action.Owner != "archive" {
			return
		}
		if err := os.Rename(archiveDir, moved); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(moved, archiveDir); err != nil {
			t.Fatal(err)
		}
	}
	report := PruneWithOptions(root, options, true)
	if len(report.Skipped) == 0 {
		t.Fatal("archive directory swap was not reported")
	}
	if _, err := os.Stat(filepath.Join(moved, run.RunID+".patch")); err != nil {
		t.Fatalf("archive removed through swapped directory: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".git", "jacu", "runs", run.RunID+".json")); err != nil {
		t.Fatalf("run proof removed after directory swap: %v", err)
	}
}

func TestRunPreservedWhenArchiveDisappearsBeforeItsAction(t *testing.T) {
	root, options, now := storageFixture(t)
	run := writeTerminalRun(t, root, options, "run_5555555555555555", now, []byte("canonical"))
	archive := filepath.Join(root, run.ArchivePatch)
	options.beforeAction = func(action Action) {
		if action.Owner == "archive" {
			if err := os.Remove(archive); err != nil {
				t.Fatal(err)
			}
		}
	}
	report := PruneWithOptions(root, options, true)
	if len(report.Skipped) < 2 {
		t.Fatalf("archive and dependent run were not both skipped: %+v", report)
	}
	if _, err := os.Stat(filepath.Join(root, ".git", "jacu", "runs", run.RunID+".json")); err != nil {
		t.Fatalf("run proof removed after external archive removal: %v", err)
	}
}

func TestApplyRevalidatesEmptyParentAfterPlan(t *testing.T) {
	root, options, _ := storageFixture(t)
	if err := os.MkdirAll(options.WorktreeDir, 0o700); err != nil {
		t.Fatal(err)
	}
	options.beforeAction = func(action Action) {
		if action.Owner == "worktrees" {
			if err := os.WriteFile(filepath.Join(options.WorktreeDir, "unknown"), []byte("keep"), 0o600); err != nil {
				t.Fatal(err)
			}
		}
	}
	report := PruneWithOptions(root, options, true)
	if _, err := os.Stat(filepath.Join(options.WorktreeDir, "unknown")); err != nil {
		t.Fatalf("changed parent removed: %v; report=%+v", err, report)
	}
}

func TestTasksDelegateRetentionAndTelemetryRemainsReportOnly(t *testing.T) {
	root, options, now := storageFixture(t)
	tasks := filepath.Join(root, ".git", "jacu", "tasks")
	writeOldFile(t, filepath.Join(tasks, "task_0123456789abcdef.json"), []byte(`{"schema_version":"2","task_id":"task_0123456789abcdef","capability":"jacu_verify","run_id":"run_0123456789abcdef","status":"done","created_at":"2026-01-01T00:00:00Z","finished_at":"2026-01-01T01:00:00Z","payload_pruned_at":"2026-01-02T00:00:00Z"}`), now)
	event, err := telemetry.NewEvent(telemetry.EventInput{Timestamp: now.Add(-40 * 24 * time.Hour), ProjectID: telemetry.ProjectID(root), TraceID: "tr_0123456789abcdef", Module: "runtime", Stage: "tool_call", Event: telemetry.EventToolCall, Status: "completed"})
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	encoded = append(encoded, '\n')
	segment := filepath.Join(options.TelemetryDir, "events-2026-01.jsonl")
	writeOldFile(t, segment, encoded, now)

	preview := PruneWithOptions(root, options, false)
	assertActions(t, preview, "tasks/shared-retention")
	if _, err := os.Stat(filepath.Join(tasks, "task_0123456789abcdef.json")); err != nil {
		t.Fatalf("dry-run mutated task retention: %v", err)
	}
	result := PruneWithOptions(root, options, true)
	if len(result.Failed) != 0 {
		t.Fatalf("task retention failed: %+v", result)
	}
	if _, err := os.Stat(filepath.Join(tasks, "task_0123456789abcdef.json")); !os.IsNotExist(err) {
		t.Fatalf("shared task retention did not remove candidate: %v", err)
	}
	if _, err := os.Stat(segment); err != nil {
		t.Fatalf("storage duplicated telemetry retention: %v", err)
	}
}

func TestTaskRetentionApplyDoesNotNestRunstateLock(t *testing.T) {
	root, options, now := storageFixture(t)
	task := filepath.Join(root, ".git", "jacu", "tasks", "task_aaaaaaaaaaaaaaaa.json")
	writeOldFile(t, task, []byte(`{"schema_version":"2","task_id":"task_aaaaaaaaaaaaaaaa","capability":"jacu_verify","run_id":"run_0123456789abcdef","status":"done","created_at":"2026-01-01T00:00:00Z","finished_at":"2026-01-01T01:00:00Z","payload_pruned_at":"2026-01-02T00:00:00Z"}`), now)
	done := make(chan Report, 1)
	go func() { done <- PruneWithOptions(root, options, true) }()
	select {
	case report := <-done:
		if len(report.Failed) != 0 {
			t.Fatalf("task retention failed: %+v", report)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("storage apply deadlocked by nesting the runstate lock")
	}
}

func TestToolchainPreservedWhenTaskIsActive(t *testing.T) {
	root, options, now := storageFixture(t)
	cache := filepath.Join(options.ToolchainDir, "cache.bin")
	writeOldFile(t, cache, []byte("cache"), now)
	old := now.Add(-31 * 24 * time.Hour)
	if err := os.Chtimes(options.ToolchainDir, old, old); err != nil {
		t.Fatal(err)
	}
	active := filepath.Join(root, ".git", "jacu", "tasks", "task_bbbbbbbbbbbbbbbb.json")
	writeOldFile(t, active, []byte(`{"schema_version":"2","task_id":"task_bbbbbbbbbbbbbbbb","capability":"jacu_verify","run_id":"run_0123456789abcdef","status":"running","created_at":"2026-01-01T00:00:00Z"}`), now)

	report := PruneWithOptions(root, options, true)
	for _, action := range report.Actions {
		if action.Owner == "toolchain" {
			t.Fatalf("active task allowed toolchain removal: %+v", report)
		}
	}
	if _, err := os.Stat(cache); err != nil {
		t.Fatalf("active task cache removed: %v", err)
	}
}

func TestToolchainRemovalUsesConfinedRoot(t *testing.T) {
	root, options, now := storageFixture(t)
	cache := filepath.Join(options.ToolchainDir, "nested", "cache.bin")
	writeOldFile(t, cache, []byte("cache"), now)
	old := now.Add(-31 * 24 * time.Hour)
	for _, path := range []string{filepath.Dir(cache), options.ToolchainDir} {
		if err := os.Chtimes(path, old, old); err != nil {
			t.Fatal(err)
		}
	}
	report := PruneWithOptions(root, options, true)
	if len(report.Failed) != 0 {
		t.Fatalf("confined toolchain removal failed: %+v", report)
	}
	if _, err := os.Stat(options.ToolchainDir); !os.IsNotExist(err) {
		t.Fatalf("toolchain survived successful removal: %v", err)
	}
}

func TestToolchainPreservedForUnknownOrInvalidTaskState(t *testing.T) {
	fixtures := map[string]string{
		"unknown-status": `{"schema_version":"2","task_id":"task_cccccccccccccccc","capability":"jacu_verify","run_id":"run_0123456789abcdef","status":"mystery"}`,
		"invalid-schema": `{"schema_version":"999","task_id":"task_cccccccccccccccc","capability":"jacu_verify","run_id":"run_0123456789abcdef","status":"done"}`,
		"corrupt-json":   `{"schema_version":`,
	}
	for name, content := range fixtures {
		t.Run(name, func(t *testing.T) {
			root, options, now := storageFixture(t)
			cache := filepath.Join(options.ToolchainDir, "cache.bin")
			writeOldFile(t, cache, []byte("cache"), now)
			old := now.Add(-31 * 24 * time.Hour)
			if err := os.Chtimes(options.ToolchainDir, old, old); err != nil {
				t.Fatal(err)
			}
			writeOldFile(t, filepath.Join(root, ".git", "jacu", "tasks", "task_cccccccccccccccc.json"), []byte(content), now)
			report := PruneWithOptions(root, options, true)
			for _, action := range report.Actions {
				if action.Owner == "toolchain" {
					t.Fatalf("unsafe task allowed toolchain removal: %+v", report)
				}
			}
			if _, err := os.Stat(cache); err != nil {
				t.Fatalf("cache removed with unsafe task: %v", err)
			}
		})
	}
}

func TestDistributedToolchainTreeCannotBypassGlobalInventoryCap(t *testing.T) {
	root, options, now := storageFixture(t)
	old := now.Add(-31 * 24 * time.Hour)
	for directory := range 65 {
		dir := filepath.Join(options.ToolchainDir, fmt.Sprintf("d-%02d", directory))
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		for file := range 32 {
			path := filepath.Join(dir, fmt.Sprintf("f-%02d", file))
			if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.Chtimes(path, old, old); err != nil {
				t.Fatal(err)
			}
		}
		if err := os.Chtimes(dir, old, old); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Chtimes(options.ToolchainDir, old, old); err != nil {
		t.Fatal(err)
	}
	report := PruneWithOptions(root, options, false)
	for _, action := range report.Actions {
		if action.Owner == "toolchain" {
			t.Fatalf("over-cap distributed tree became removable: %+v", action)
		}
	}
}

func TestToolchainRemovalCannotEscapeAfterChildDirectorySwap(t *testing.T) {
	root, options, now := storageFixture(t)
	child := filepath.Join(options.ToolchainDir, "child")
	cache := filepath.Join(child, "cache.bin")
	writeOldFile(t, cache, []byte("owned"), now)
	external := t.TempDir()
	externalFile := filepath.Join(external, "user-data")
	if err := os.WriteFile(externalFile, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	old := now.Add(-31 * 24 * time.Hour)
	for _, path := range []string{child, options.ToolchainDir} {
		if err := os.Chtimes(path, old, old); err != nil {
			t.Fatal(err)
		}
	}
	moved := child + "-moved"
	options.duringRemove = func(reference string) {
		if reference != "child" {
			return
		}
		if err := os.Rename(child, moved); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(external, child); err != nil {
			t.Fatal(err)
		}
	}
	report := PruneWithOptions(root, options, true)
	if len(report.Failed) == 0 {
		t.Fatalf("directory swap was not reported as failure: %+v", report)
	}
	content, err := readConfinedFile(external, "user-data")
	if err != nil || string(content) != "keep" {
		t.Fatalf("external file changed through swapped child: %q %v", content, err)
	}
	if _, err := os.Stat(filepath.Join(moved, "cache.bin")); err != nil {
		t.Fatalf("original owned child was unexpectedly traversed: %v", err)
	}
}

func TestInventoryAgeAndJSONAreDeterministic(t *testing.T) {
	root, options, now := storageFixture(t)
	first := writeTerminalRun(t, root, options, "run_6666666666666666", now, []byte("first"))
	second := writeTerminalRun(t, root, options, "run_7777777777777777", now, []byte("second"))
	for _, run := range []runstate.Run{first, second} {
		if err := os.Remove(filepath.Join(root, run.ArchivePatch)); err != nil {
			t.Fatal(err)
		}
	}
	want, err := Encode(PruneWithOptions(root, options, false))
	if err != nil {
		t.Fatal(err)
	}
	for range 20 {
		got, encodeErr := Encode(PruneWithOptions(root, options, false))
		if encodeErr != nil {
			t.Fatal(encodeErr)
		}
		if string(got) != string(want) {
			t.Fatalf("inventory JSON is not deterministic:\nwant %s\ngot  %s", want, got)
		}
	}
	report := PruneWithOptions(root, options, false)
	for _, item := range report.Items {
		if item.Owner == "runs" && (item.OldestAgeSeconds == 0 || item.NewestAgeSeconds == 0) {
			t.Fatalf("run ages not reported from fake clock: %+v", item)
		}
	}
}

func storageFixture(t *testing.T) (string, Options, time.Time) {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".git", "jacu", "runs"), 0o700); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC)
	return root, Options{Now: now, TelemetryDir: filepath.Join(root, "telemetry"), ToolchainDir: filepath.Join(root, "toolchain"), WorktreeDir: filepath.Join(root, "worktrees")}, now
}

func writeTerminalRun(t *testing.T, root string, options Options, runID string, now time.Time, patch []byte) runstate.Run {
	t.Helper()
	digest := sha256.Sum256(patch)
	run := runstate.Run{SchemaVersion: runstate.CurrentSchemaVersion, RunID: runID, Status: runstate.StatusDiscarded, CreatedAt: now.Add(-40 * 24 * time.Hour), Branch: "jacu/run-" + runID[4:], Worktree: filepath.Join(options.WorktreeDir, runID), BaseSHA: "0000000000000000000000000000000000000000", ArchivePatch: filepath.Join(".git", "jacu", "archive", runID+".patch"), ArchiveDigest: "sha256:" + hex.EncodeToString(digest[:])}
	if err := runstate.Save(root, run); err != nil {
		t.Fatal(err)
	}
	runPath := filepath.Join(root, ".git", "jacu", "runs", runID+".json")
	old := now.Add(-31 * 24 * time.Hour)
	if err := os.Chtimes(runPath, old, old); err != nil {
		t.Fatal(err)
	}
	writeOldFile(t, filepath.Join(root, run.ArchivePatch), patch, now)
	return run
}

func writeOldFile(t *testing.T, path string, content []byte, now time.Time) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
	old := now.Add(-31 * 24 * time.Hour)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
}

func assertActions(t *testing.T, report Report, want ...string) {
	t.Helper()
	got := make([]string, 0, len(report.Actions))
	for _, action := range report.Actions {
		got = append(got, action.Ref)
	}
	if !slices.Equal(got, want) {
		t.Fatalf("actions=%v want=%v report=%+v", got, want, report)
	}
}
