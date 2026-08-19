package verify

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRetainTasksLoadsV1AndPreservesActiveAndCorruptRecords(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, time.August, 15, 12, 0, 0, 0, time.UTC)
	expired := retentionTask("task_0000000000000001", TaskDone, now.Add(-48*time.Hour), now.Add(-time.Hour))
	expired.SchemaVersion = "1"
	queued := retentionTask("task_0000000000000002", TaskQueued, now.Add(-48*time.Hour), now.Add(-time.Hour))
	active := retentionTask("task_0000000000000003", TaskRunning, now.Add(-48*time.Hour), now.Add(-time.Hour))
	fresh := retentionTask("task_0000000000000004", TaskDone, now.Add(-time.Hour), now.Add(time.Hour))
	writeRawRetentionTask(t, root, expired)
	writeRawRetentionTask(t, root, queued)
	writeRawRetentionTask(t, root, active)
	writeRawRetentionTask(t, root, fresh)
	corrupt := filepath.Join(taskDir(root), "task_0000000000000005.json")
	if err := os.WriteFile(corrupt, []byte("not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := RetainTasksAt(root, now); err != nil {
		t.Fatal(err)
	}
	loaded, err := loadTask(root, expired.TaskID)
	if err != nil || loaded.SchemaVersion != CurrentTaskSchemaVersion || !loaded.PayloadPrunedAt.Equal(now) || len(loaded.Input) != 0 || len(loaded.ResultRaw) != 0 {
		t.Fatalf("expired v1 task = %#v, %v; want compact v2", loaded, err)
	}
	if info, statErr := os.Stat(taskPath(root, expired.TaskID)); statErr != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("compacted v2 mode = %v, %v; want 0600", info, statErr)
	}
	encoded, readErr := os.ReadFile(taskPath(root, expired.TaskID))
	if readErr != nil || !json.Valid(encoded) || !strings.Contains(string(encoded), `"schema_version": "2"`) {
		t.Fatalf("compacted record did not round-trip v2: %s, %v", encoded, readErr)
	}
	queuedLoaded, err := loadTask(root, queued.TaskID)
	if err != nil || len(queuedLoaded.Input) == 0 || !queuedLoaded.PayloadPrunedAt.IsZero() {
		t.Fatalf("queued task changed = %#v, %v", queuedLoaded, err)
	}
	activeLoaded, err := loadTask(root, active.TaskID)
	if err != nil || len(activeLoaded.Input) == 0 || len(activeLoaded.ResultRaw) == 0 || !activeLoaded.PayloadPrunedAt.IsZero() {
		t.Fatalf("active task changed = %#v, %v", activeLoaded, err)
	}
	freshLoaded, err := loadTask(root, fresh.TaskID)
	if err != nil || len(freshLoaded.Input) == 0 || len(freshLoaded.ResultRaw) == 0 || !freshLoaded.PayloadPrunedAt.IsZero() {
		t.Fatalf("unexpired terminal task changed = %#v, %v", freshLoaded, err)
	}
	if _, err := os.Stat(corrupt); err != nil {
		t.Fatalf("corrupt task was removed: %v", err)
	}
}

func TestRetainTasksAgeBoundaryAndIdempotence(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, time.August, 15, 12, 0, 0, 0, time.UTC)
	exact := retentionTask("task_0000000000000001", TaskDone, now.Add(-30*24*time.Hour), now)
	old := retentionTask("task_0000000000000002", TaskDone, now.Add(-30*24*time.Hour-time.Nanosecond), now)
	writeRawRetentionTask(t, root, exact)
	writeRawRetentionTask(t, root, old)
	if err := RetainTasksAt(root, now); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(taskPath(root, old.TaskID)); !os.IsNotExist(err) {
		t.Fatalf("older-than-30d task survived: %v", err)
	}
	if _, err := os.Stat(taskPath(root, exact.TaskID)); err != nil {
		t.Fatalf("exact-30d task was deleted: %v", err)
	}
	path := taskPath(root, exact.TaskID)
	before, readErr := os.ReadFile(path) // #nosec G304 -- path is derived from this test's TempDir fixture.
	if readErr != nil {
		t.Fatal(readErr)
	}
	if retainErr := RetainTasksAt(root, now); retainErr != nil {
		t.Fatal(retainErr)
	}
	after, readErr := os.ReadFile(path) // #nosec G304 -- path is derived from this test's TempDir fixture.
	if readErr != nil || string(after) != string(before) {
		t.Fatalf("idempotent compaction changed bytes: %v", readErr)
	}
}

func TestRetainTasksEnforcesTerminalRecordCap(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, time.August, 15, 12, 0, 0, 0, time.UTC)
	for index := 1; index <= terminalRecordCap+1; index++ {
		id := fmt.Sprintf("task_%016x", index)
		task := retentionTask(id, TaskDone, now.Add(-time.Duration(terminalRecordCap-index+1)*time.Second), now)
		writeRawRetentionTask(t, root, task)
	}
	if err := RetainTasksAt(root, now); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(taskDir(root))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != terminalRecordCap {
		t.Fatalf("terminal records = %d; want cap %d", len(entries), terminalRecordCap)
	}
	if _, err := os.Stat(taskPath(root, "task_0000000000000001")); !os.IsNotExist(err) {
		t.Fatalf("oldest terminal record survived cap: %v", err)
	}
}

func TestRetainTasksLeavesOriginalRecordWhenSaveFails(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, time.August, 15, 12, 0, 0, 0, time.UTC)
	task := retentionTask("task_0000000000000001", TaskDone, now.Add(-48*time.Hour), now.Add(-time.Hour))
	writeRawRetentionTask(t, root, task)
	path := taskPath(root, task.TaskID)
	before, readErr := os.ReadFile(path) // #nosec G304 -- path is derived from this test's TempDir fixture.
	if readErr != nil {
		t.Fatal(readErr)
	}
	if retainErr := retainTasks(root, now, func(string, Task) error { return errors.New("save denied") }); retainErr == nil {
		t.Fatal("retention accepted failed save")
	}
	after, readErr := os.ReadFile(path) // #nosec G304 -- path is derived from this test's TempDir fixture.
	if readErr != nil || string(after) != string(before) {
		t.Fatalf("failed compaction changed original: %v", readErr)
	}
}

func TestRetainTasksCompactsEveryTerminalStatus(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, time.August, 15, 12, 0, 0, 0, time.UTC)
	statuses := []TaskStatus{TaskDone, TaskFailed, TaskCancelled, TaskTimeout}
	for index, status := range statuses {
		task := retentionTask(fmt.Sprintf("task_%016x", index+1), status, now.Add(-48*time.Hour), now)
		writeRawRetentionTask(t, root, task)
	}
	if err := RetainTasksAt(root, now); err != nil {
		t.Fatal(err)
	}
	for index, status := range statuses {
		task, err := loadTask(root, fmt.Sprintf("task_%016x", index+1))
		if err != nil || !task.PayloadPrunedAt.Equal(now) || len(task.Input) != 0 || len(task.ResultRaw) != 0 {
			t.Fatalf("%s retention = %#v, %v", status, task, err)
		}
	}
}

func retentionTask(id string, status TaskStatus, finished, expires time.Time) Task {
	return Task{
		SchemaVersion: "1",
		TaskID:        id,
		Capability:    ToolName,
		RunID:         "run_1111111111111111",
		Input:         json.RawMessage(`{"payload":"keep-or-prune"}`),
		Status:        status,
		CreatedAt:     finished.Add(-time.Hour),
		FinishedAt:    finished,
		ResultRaw:     json.RawMessage(`{"ok":true}`),
		ResultDigest:  "sha256:stable",
		ExpiresAt:     expires,
	}
}

func writeRawRetentionTask(t *testing.T, root string, task Task) {
	t.Helper()
	if err := os.MkdirAll(taskDir(root), 0o700); err != nil {
		t.Fatal(err)
	}
	content, err := json.Marshal(task)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(taskPath(root, task.TaskID), content, 0o600); err != nil {
		t.Fatal(err)
	}
}
