package verify

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jacu-dev/jacu-harness/internal/runstate"
)

func TestTaskTransitionIsForwardOnly(t *testing.T) {
	for _, item := range []struct {
		from TaskStatus
		to   TaskStatus
		ok   bool
	}{
		{TaskQueued, TaskRunning, true},
		{TaskRunning, TaskDone, true},
		{TaskRunning, TaskFailed, true},
		{TaskRunning, TaskCancelled, true},
		{TaskRunning, TaskTimeout, true},
		{TaskQueued, TaskDone, false},
		{TaskDone, TaskRunning, false},
		{TaskCancelled, TaskFailed, false},
	} {
		t.Run(string(item.from)+"-"+string(item.to), func(t *testing.T) {
			task := Task{Status: item.from}
			err := task.Transition(item.to)
			if (err == nil) != item.ok {
				t.Fatalf("transition %q -> %q error = %v; want ok=%v", item.from, item.to, err, item.ok)
			}
		})
	}
}

func TestTaskManagerPersistsTerminalResultAndDigest(t *testing.T) {
	root := t.TempDir()
	manager, err := NewTaskManagerWithConfig(root, TaskConfig{
		Executor: func(context.Context, Input) Envelope {
			return Envelope{Status: "ok", Summary: "verified", Data: Data{Verdict: VerdictPass, Commands: []Result{}}}
		},
	})
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	info, err := manager.Start(context.Background(), Input{RunID: "run_1111111111111111", Async: true})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if !strings.HasPrefix(info.TaskID, "task_") {
		t.Fatalf("task id = %q; want task_ prefix", info.TaskID)
	}
	finished := waitForTask(t, manager, info.TaskID, TaskDone)
	finishedResult, ok := finished.Result.(*Envelope)
	if !ok || finishedResult.Data.Verdict != VerdictPass {
		t.Fatalf("result = %#v; want persisted pass envelope", finished.Result)
	}
	if finished.ResultDigest == "" || !strings.HasPrefix(finished.ResultDigest, "sha256:") {
		t.Fatalf("result digest = %q; want sha256", finished.ResultDigest)
	}
	path := filepath.Join(root, ".git", "jacu", "tasks", info.TaskID+".json")
	if _, statErr := os.Stat(path); statErr != nil {
		t.Fatalf("task file missing: %v", statErr)
	}

	reloaded, err := NewTaskManagerWithConfig(root, TaskConfig{Executor: func(context.Context, Input) Envelope {
		return Envelope{Status: "bad", Summary: "must not run"}
	}})
	if err != nil {
		t.Fatalf("reload manager: %v", err)
	}
	recovered, err := reloaded.Get(info.TaskID, true)
	if err != nil {
		t.Fatalf("get reloaded task: %v", err)
	}
	recoveredResult, ok := recovered.Result.(*Envelope)
	if recovered.Status != TaskDone || !ok || recoveredResult.Data.Verdict != VerdictPass {
		t.Fatalf("reloaded task = %#v; terminal result changed", recovered)
	}
}

func TestRetentionCompactsExpiredPayloadAndKeepsDigest(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC)
	task := Task{SchemaVersion: "1", TaskID: "task_0123456789abcdef", Capability: ToolName, RunID: "run_1111111111111111", Status: TaskDone, CreatedAt: now.Add(-48 * time.Hour), FinishedAt: now.Add(-47 * time.Hour), Input: []byte(`{"secret":"payload"}`), ResultRaw: []byte(`{"ok":true}`), ResultDigest: "sha256:stable", ExpiresAt: now.Add(-time.Hour)}
	if err := runstate.WithLock(root, func() error { return saveTaskLocked(root, task) }); err != nil {
		t.Fatal(err)
	}
	manager, err := NewTaskManagerWithConfig(root, TaskConfig{Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	if retainErr := manager.RetainTasks(); retainErr != nil {
		t.Fatal(retainErr)
	}
	loaded, err := loadTask(root, task.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Input) != 0 || len(loaded.ResultRaw) != 0 || loaded.ResultDigest != task.ResultDigest || loaded.PayloadPrunedAt.IsZero() {
		t.Fatalf("compacted task input=%d raw=%d result=%v digest=%q pruned=%v", len(loaded.Input), len(loaded.ResultRaw), loaded.Result != nil, loaded.ResultDigest, loaded.PayloadPrunedAt)
	}
}

func TestTaskManagerUsesFIFOWithOneActiveTask(t *testing.T) {
	root := t.TempDir()
	firstStarted := make(chan struct{})
	releaseFirst := make(chan struct{})
	var mu sync.Mutex
	order := []string{}
	manager, err := NewTaskManagerWithConfig(root, TaskConfig{
		MaxConcurrent: 1,
		Executor: func(ctx context.Context, input Input) Envelope {
			mu.Lock()
			order = append(order, input.RunID)
			mu.Unlock()
			if input.RunID == "run_1111111111111111" {
				close(firstStarted)
				select {
				case <-releaseFirst:
				case <-ctx.Done():
				}
			}
			return Envelope{Status: "ok", Data: Data{Verdict: VerdictPass, Commands: []Result{}}}
		},
	})
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	first, err := manager.Start(context.Background(), Input{RunID: "run_1111111111111111", Async: true})
	if err != nil {
		t.Fatalf("start first: %v", err)
	}
	select {
	case <-firstStarted:
	case <-time.After(time.Second):
		t.Fatal("first task did not start")
	}
	second, err := manager.Start(context.Background(), Input{RunID: "run_2222222222222222", Async: true})
	if err != nil {
		t.Fatalf("start second: %v", err)
	}
	queued, err := manager.Get(second.TaskID, false)
	if err != nil {
		t.Fatalf("get queued task: %v", err)
	}
	if queued.Status != TaskQueued {
		t.Fatalf("second status = %q; want queued", queued.Status)
	}
	close(releaseFirst)
	waitForTask(t, manager, first.TaskID, TaskDone)
	waitForTask(t, manager, second.TaskID, TaskDone)
	mu.Lock()
	defer mu.Unlock()
	if len(order) != 2 || order[0] != "run_1111111111111111" || order[1] != "run_2222222222222222" {
		t.Fatalf("execution order = %v; want FIFO", order)
	}
}

func TestTaskManagerCancelsQueuedTaskWithoutExecutingIt(t *testing.T) {
	root := t.TempDir()
	firstStarted := make(chan struct{})
	releaseFirst := make(chan struct{})
	var mu sync.Mutex
	started := map[string]bool{}
	manager, err := NewTaskManagerWithConfig(root, TaskConfig{
		MaxConcurrent: 1,
		Executor: func(ctx context.Context, input Input) Envelope {
			mu.Lock()
			started[input.RunID] = true
			mu.Unlock()
			if input.RunID == "run_1111111111111111" {
				close(firstStarted)
				select {
				case <-releaseFirst:
				case <-ctx.Done():
				}
			}
			return Envelope{Status: "ok", Data: Data{Verdict: VerdictPass, Commands: []Result{}}}
		},
	})
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	first, err := manager.Start(context.Background(), Input{RunID: "run_1111111111111111", Async: true})
	if err != nil {
		t.Fatalf("start first: %v", err)
	}
	<-firstStarted
	second, err := manager.Start(context.Background(), Input{RunID: "run_2222222222222222", Async: true})
	if err != nil {
		t.Fatalf("start second: %v", err)
	}
	cancelled, err := manager.Cancel(second.TaskID)
	if err != nil {
		t.Fatalf("cancel queued: %v", err)
	}
	if cancelled.Status != TaskCancelled {
		t.Fatalf("cancelled status = %q; want cancelled", cancelled.Status)
	}
	close(releaseFirst)
	waitForTask(t, manager, first.TaskID, TaskDone)
	mu.Lock()
	defer mu.Unlock()
	if started["run_2222222222222222"] {
		t.Fatal("queued cancelled task was executed")
	}
}

func TestTaskManagerRecoversQueuedAndRunningTasksAsOrphaned(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, ".git", "jacu", "tasks")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir tasks: %v", err)
	}
	for _, status := range []TaskStatus{TaskQueued, TaskRunning} {
		task := Task{
			SchemaVersion: CurrentTaskSchemaVersion,
			TaskID:        "task_1111111111111111",
			Capability:    ToolName,
			RunID:         "run_1111111111111111",
			Status:        status,
			CreatedAt:     time.Now().UTC(),
		}
		if status == TaskRunning {
			task.TaskID = "task_2222222222222222"
		}
		content, err := json.Marshal(task)
		if err != nil {
			t.Fatalf("marshal orphan: %v", err)
		}
		if err := os.WriteFile(filepath.Join(dir, task.TaskID+".json"), content, 0o600); err != nil {
			t.Fatalf("write orphan: %v", err)
		}
	}
	manager, err := NewTaskManagerWithConfig(root, TaskConfig{})
	if err != nil {
		t.Fatalf("recover manager: %v", err)
	}
	for _, taskID := range []string{"task_1111111111111111", "task_2222222222222222"} {
		snapshot, err := manager.Get(taskID, false)
		if err != nil {
			t.Fatalf("get %s: %v", taskID, err)
		}
		if snapshot.Status != TaskFailed || !strings.Contains(snapshot.Reason, "orphaned") {
			t.Fatalf("recovered %s = %#v; want orphaned failure", taskID, snapshot)
		}
	}
}

func TestTaskManagerTimeoutIsTerminal(t *testing.T) {
	root := t.TempDir()
	manager, err := NewTaskManagerWithConfig(root, TaskConfig{
		TaskTimeout: 20 * time.Millisecond,
		Executor: func(ctx context.Context, _ Input) Envelope {
			<-ctx.Done()
			return Envelope{Status: "ok", Data: Data{Verdict: VerdictNotRun, Commands: []Result{}}}
		},
	})
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	info, err := manager.Start(context.Background(), Input{RunID: "run_1111111111111111", Async: true})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	result := waitForTask(t, manager, info.TaskID, TaskTimeout)
	if !strings.Contains(result.Reason, "timeout") {
		t.Fatalf("timeout task = %#v; want timeout reason", result)
	}
}

func TestTaskManagerExpiresResultButKeepsDigest(t *testing.T) {
	root := t.TempDir()
	now := time.Now().UTC()
	manager, err := NewTaskManagerWithConfig(root, TaskConfig{
		ResultTTL: time.Second,
		Now:       func() time.Time { return now },
		Executor: func(context.Context, Input) Envelope {
			return Envelope{Status: "ok", Data: Data{Verdict: VerdictPass, Commands: []Result{}}}
		},
	})
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	info, err := manager.Start(context.Background(), Input{RunID: "run_1111111111111111", Async: true})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	waitForTask(t, manager, info.TaskID, TaskDone)
	now = now.Add(2 * time.Second)
	expired, err := manager.Get(info.TaskID, true)
	if err != nil {
		t.Fatalf("get expired: %v", err)
	}
	if expired.Result != nil || expired.ResultDigest == "" || expired.Status != TaskDone {
		t.Fatalf("expired snapshot = %#v; want status/digest without result", expired)
	}
}

func TestTaskManagerStressOneHundredShortTasks(t *testing.T) {
	root := t.TempDir()
	manager, err := NewTaskManagerWithConfig(root, TaskConfig{
		MaxConcurrent: 4,
		Executor: func(context.Context, Input) Envelope {
			return Envelope{Status: "ok", Data: Data{Verdict: VerdictPass, Commands: []Result{}}}
		},
	})
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	ids := make([]string, 0, 100)
	for index := 0; index < 100; index++ {
		info, startErr := manager.Start(context.Background(), Input{RunID: "run_1111111111111111", Async: true})
		if startErr != nil {
			t.Fatalf("start task %d: %v", index, startErr)
		}
		ids = append(ids, info.TaskID)
	}
	for _, taskID := range ids {
		waitForTask(t, manager, taskID, TaskDone)
	}
}

func TestTaskManagerRunsRegisteredRawCapabilityAndPersistsJSONResult(t *testing.T) {
	root := t.TempDir()
	manager, err := NewTaskManagerWithConfig(root, TaskConfig{})
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	if registerErr := manager.RegisterRawExecutor("jacu_flow_run", func(_ context.Context, input json.RawMessage) (json.RawMessage, error) {
		var request map[string]string
		if unmarshalErr := json.Unmarshal(input, &request); unmarshalErr != nil {
			return nil, unmarshalErr
		}
		return json.Marshal(map[string]any{"status": "ok", "echo": request["echo"]})
	}); registerErr != nil {
		t.Fatalf("register raw: %v", registerErr)
	}
	info, err := manager.StartRaw(context.Background(), "jacu_flow_run", "", json.RawMessage(`{"echo":"flow"}`))
	if err != nil {
		t.Fatalf("start raw: %v", err)
	}
	result := waitForTask(t, manager, info.TaskID, TaskDone)
	data, ok := result.Result.(map[string]any)
	if !ok || data["echo"] != "flow" {
		t.Fatalf("raw result = %#v; want persisted JSON object", result.Result)
	}
}

func waitForTask(t *testing.T, manager *TaskManager, taskID string, want TaskStatus) TaskSnapshot {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		snapshot, err := manager.Get(taskID, true)
		if err == nil && snapshot.Status == want {
			return snapshot
		}
		time.Sleep(5 * time.Millisecond)
	}
	snapshot, err := manager.Get(taskID, true)
	if err != nil {
		t.Fatalf("wait for %s: %v", want, err)
	}
	t.Fatalf("task %s status = %q; want %q", taskID, snapshot.Status, want)
	return TaskSnapshot{}
}
