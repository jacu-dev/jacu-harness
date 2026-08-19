package verify

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/jacu-dev/jacu-harness/internal/runstate"
)

const CurrentTaskSchemaVersion = "2"

type TaskStatus string

const (
	TaskQueued    TaskStatus = "queued"
	TaskRunning   TaskStatus = "running"
	TaskDone      TaskStatus = "done"
	TaskFailed    TaskStatus = "failed"
	TaskCancelled TaskStatus = "cancelled"
	TaskTimeout   TaskStatus = "timeout"
)

const (
	defaultTaskConcurrency = 1
	defaultTaskTimeout     = 15 * time.Minute
	defaultResultTTL       = 24 * time.Hour
)

// Task is the durable record. Input is normalized before persistence: the
// worker must never re-enter the async/cancel dispatch path when it resumes.
type Task struct {
	SchemaVersion   string          `json:"schema_version"`
	TaskID          string          `json:"task_id"`
	Capability      string          `json:"capability"`
	RunID           string          `json:"run_id"`
	Input           json.RawMessage `json:"input,omitempty"`
	Status          TaskStatus      `json:"status"`
	CreatedAt       time.Time       `json:"created_at"`
	StartedAt       time.Time       `json:"started_at,omitempty"`
	FinishedAt      time.Time       `json:"finished_at,omitempty"`
	Reason          string          `json:"reason,omitempty"`
	Result          *Envelope       `json:"result,omitempty"`
	ResultRaw       json.RawMessage `json:"result_raw,omitempty"`
	ResultDigest    string          `json:"result_digest,omitempty"`
	ExpiresAt       time.Time       `json:"expires_at,omitempty"`
	PayloadPrunedAt time.Time       `json:"payload_pruned_at,omitempty"`
}

// TaskInfo is the bounded metadata returned by async start and cancellation.
// The result is intentionally separate so callers can distinguish task state
// from the verdict produced by verify.
type TaskInfo struct {
	TaskID       string     `json:"task_id"`
	RunID        string     `json:"run_id"`
	Status       TaskStatus `json:"status"`
	Reason       string     `json:"reason,omitempty"`
	ResultDigest string     `json:"result_digest,omitempty"`
}

type TaskSnapshot struct {
	TaskInfo
	// Result is intentionally unrestricted in the advertised status schema: it
	// is the already-versioned verify envelope, and duplicating all command
	// evidence here would consume the global tools/list budget.
	Result any `json:"result,omitempty"`
}

type TaskExecutor func(context.Context, Input) Envelope

// RawTaskExecutor is used by a registered capability that has its own stable
// JSON result. Registration is explicit; this is not a generic dispatch or
// shell execution hook.
type RawTaskExecutor func(context.Context, json.RawMessage) (json.RawMessage, error)

type TaskConfig struct {
	MaxConcurrent int
	TaskTimeout   time.Duration
	ResultTTL     time.Duration
	Executor      TaskExecutor
	Now           func() time.Time
	ID            func() (string, error)
}

type TaskManager struct {
	root          string
	maxConcurrent int
	taskTimeout   time.Duration
	resultTTL     time.Duration
	executor      TaskExecutor
	now           func() time.Time
	newID         func() (string, error)

	mu     sync.Mutex
	queue  []string
	active map[string]struct{}
	cancel map[string]context.CancelFunc
	raw    map[string]RawTaskExecutor
}

func NewTaskManager(root string) (*TaskManager, error) {
	return NewTaskManagerWithConfig(root, TaskConfig{})
}

func NewTaskManagerWithConfig(root string, config TaskConfig) (*TaskManager, error) {
	if root == "" {
		return nil, errors.New("task manager root is empty")
	}
	if config.MaxConcurrent < 0 {
		return nil, errors.New("task concurrency must not be negative")
	}
	manager := &TaskManager{
		root:          root,
		maxConcurrent: config.MaxConcurrent,
		taskTimeout:   config.TaskTimeout,
		resultTTL:     config.ResultTTL,
		executor:      config.Executor,
		now:           config.Now,
		newID:         config.ID,
		active:        make(map[string]struct{}),
		cancel:        make(map[string]context.CancelFunc),
		raw:           make(map[string]RawTaskExecutor),
	}
	if manager.maxConcurrent == 0 {
		manager.maxConcurrent = defaultTaskConcurrency
	}
	if manager.taskTimeout <= 0 {
		manager.taskTimeout = defaultTaskTimeout
	}
	if manager.resultTTL <= 0 {
		manager.resultTTL = defaultResultTTL
	}
	if manager.now == nil {
		manager.now = func() time.Time { return time.Now().UTC() }
	}
	if manager.newID == nil {
		manager.newID = newTaskID
	}
	if manager.executor == nil {
		manager.executor = func(ctx context.Context, input Input) Envelope {
			input.Async = false
			input.TaskID = ""
			input.Cancel = false
			result := Verify(ctx, root, input)
			result.Data, result.Warnings = fitOutputCap(result.Data, result.Warnings)
			return result
		}
	}
	if err := manager.recoverOrphans(); err != nil {
		return nil, err
	}
	if err := manager.RetainTasks(); err != nil {
		return nil, err
	}
	return manager, nil
}

func (task *Task) Transition(next TaskStatus) error {
	if task.Status == next {
		return nil
	}
	valid := (task.Status == TaskQueued && (next == TaskRunning || next == TaskFailed || next == TaskCancelled)) ||
		(task.Status == TaskRunning && (next == TaskDone || next == TaskFailed || next == TaskCancelled || next == TaskTimeout))
	if !valid {
		return fmt.Errorf("invalid task transition %q to %q", task.Status, next)
	}
	task.Status = next
	return nil
}

func (manager *TaskManager) Start(ctx context.Context, input Input) (TaskInfo, error) {
	if err := ctx.Err(); err != nil {
		return TaskInfo{}, err
	}
	if !runstate.ValidRunID(input.RunID) {
		return TaskInfo{}, fmt.Errorf("invalid run_id %q", input.RunID)
	}
	if input.TaskID != "" || input.Cancel {
		return TaskInfo{}, errors.New("task start input cannot request task control")
	}
	taskID, err := manager.newID()
	if err != nil {
		return TaskInfo{}, fmt.Errorf("create task id: %w", err)
	}
	if !ValidTaskID(taskID) {
		return TaskInfo{}, fmt.Errorf("task id generator returned invalid id %q", taskID)
	}
	input.Async = false
	encoded, err := json.Marshal(input)
	if err != nil {
		return TaskInfo{}, fmt.Errorf("encode task input: %w", err)
	}
	now := manager.clock()
	task := Task{
		SchemaVersion: CurrentTaskSchemaVersion,
		TaskID:        taskID,
		Capability:    ToolName,
		RunID:         input.RunID,
		Input:         encoded,
		Status:        TaskQueued,
		CreatedAt:     now,
	}
	if err := runstate.WithLock(manager.root, func() error { return saveTaskLocked(manager.root, task) }); err != nil {
		return TaskInfo{}, err
	}
	manager.mu.Lock()
	manager.queue = append(manager.queue, taskID)
	manager.pumpLocked()
	manager.mu.Unlock()
	return task.info(), nil
}

// RegisterRawExecutor installs one named internal capability executor. The
// name is still persisted in the task record and is never supplied as an MCP
// command by the caller.
func (manager *TaskManager) RegisterRawExecutor(capability string, executor RawTaskExecutor) error {
	if !strings.HasPrefix(capability, "jacu_") || executor == nil {
		return fmt.Errorf("invalid raw task executor %q", capability)
	}
	manager.mu.Lock()
	defer manager.mu.Unlock()
	manager.raw[capability] = executor
	return nil
}

func (manager *TaskManager) StartRaw(ctx context.Context, capability, runID string, input json.RawMessage) (TaskInfo, error) {
	if err := ctx.Err(); err != nil {
		return TaskInfo{}, err
	}
	if !strings.HasPrefix(capability, "jacu_") || capability == ToolName {
		return TaskInfo{}, fmt.Errorf("invalid raw task capability %q", capability)
	}
	if runID != "" && !runstate.ValidRunID(runID) {
		return TaskInfo{}, fmt.Errorf("invalid run_id %q", runID)
	}
	manager.mu.Lock()
	_, registered := manager.raw[capability]
	manager.mu.Unlock()
	if !registered {
		return TaskInfo{}, fmt.Errorf("raw task capability %q is not registered", capability)
	}
	taskID, err := manager.newID()
	if err != nil {
		return TaskInfo{}, fmt.Errorf("create task id: %w", err)
	}
	if !ValidTaskID(taskID) {
		return TaskInfo{}, fmt.Errorf("task id generator returned invalid id %q", taskID)
	}
	if len(input) == 0 {
		input = json.RawMessage("{}")
	}
	now := manager.clock()
	task := Task{
		SchemaVersion: CurrentTaskSchemaVersion,
		TaskID:        taskID,
		Capability:    capability,
		RunID:         runID,
		Input:         append(json.RawMessage{}, input...),
		Status:        TaskQueued,
		CreatedAt:     now,
	}
	if err := runstate.WithLock(manager.root, func() error { return saveTaskLocked(manager.root, task) }); err != nil {
		return TaskInfo{}, err
	}
	manager.mu.Lock()
	manager.queue = append(manager.queue, taskID)
	manager.pumpLocked()
	manager.mu.Unlock()
	return task.info(), nil
}

func (manager *TaskManager) Cancel(taskID string) (TaskInfo, error) {
	if !ValidTaskID(taskID) {
		return TaskInfo{}, fmt.Errorf("invalid task_id %q", taskID)
	}
	manager.mu.Lock()
	defer manager.mu.Unlock()
	task, err := loadTask(manager.root, taskID)
	if err != nil {
		return TaskInfo{}, err
	}
	switch task.Status {
	case TaskQueued:
		if err := task.Transition(TaskCancelled); err != nil {
			return TaskInfo{}, err
		}
		task.FinishedAt = manager.clock()
		task.Reason = "cancelled before execution"
		if err := runstate.WithLock(manager.root, func() error { return saveTaskLocked(manager.root, task) }); err != nil {
			return TaskInfo{}, err
		}
		manager.removeQueued(taskID)
		manager.pumpLocked()
		return task.info(), nil
	case TaskRunning:
		if cancel := manager.cancel[taskID]; cancel != nil {
			cancel()
		}
		return task.info(), nil
	default:
		return task.info(), nil
	}
}

func (manager *TaskManager) Get(taskID string, includeResult bool) (TaskSnapshot, error) {
	if !ValidTaskID(taskID) {
		return TaskSnapshot{}, fmt.Errorf("invalid task_id %q", taskID)
	}
	task, err := loadTask(manager.root, taskID)
	if err != nil {
		return TaskSnapshot{}, err
	}
	return task.snapshot(manager.clock(), includeResult), nil
}

func (manager *TaskManager) List(includeResult bool) ([]TaskSnapshot, error) {
	if err := manager.RetainTasks(); err != nil {
		return nil, err
	}
	tasks, err := listTasks(manager.root)
	if err != nil {
		return nil, err
	}
	now := manager.clock()
	result := make([]TaskSnapshot, 0, len(tasks))
	for _, task := range tasks {
		result = append(result, task.snapshot(now, includeResult))
	}
	return result, nil
}

func (manager *TaskManager) recoverOrphans() error {
	return runstate.WithLock(manager.root, func() error {
		tasks, err := listTasks(manager.root)
		if err != nil {
			return err
		}
		for _, task := range tasks {
			if task.Status != TaskQueued && task.Status != TaskRunning {
				continue
			}
			if err := task.Transition(TaskFailed); err != nil {
				return err
			}
			task.FinishedAt = manager.clock()
			task.Reason = "orphaned: task had no live executor at server startup"
			if err := saveTaskLocked(manager.root, task); err != nil {
				return err
			}
		}
		return nil
	})
}

func (manager *TaskManager) pumpLocked() {
	for len(manager.active) < manager.maxConcurrent && len(manager.queue) > 0 {
		taskID := manager.queue[0]
		manager.queue = manager.queue[1:]
		task, err := loadTask(manager.root, taskID)
		if err != nil || task.Status != TaskQueued {
			continue
		}
		if err := task.Transition(TaskRunning); err != nil {
			continue
		}
		task.StartedAt = manager.clock()
		if err := runstate.WithLock(manager.root, func() error { return saveTaskLocked(manager.root, task) }); err != nil {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), manager.taskTimeout)
		manager.active[taskID] = struct{}{}
		manager.cancel[taskID] = cancel
		go manager.execute(taskID, ctx, cancel)
	}
}

func (manager *TaskManager) execute(taskID string, ctx context.Context, cancel context.CancelFunc) {
	defer cancel()
	task, err := loadTask(manager.root, taskID)
	if err == nil {
		if task.Capability == ToolName {
			var input Input
			if decodeErr := json.Unmarshal(task.Input, &input); decodeErr != nil {
				err = decodeErr
			} else {
				result := manager.executor(ctx, input)
				status, reason := taskTerminalStatus(ctx, manager.taskTimeout)
				err = manager.finish(taskID, status, reason, &result)
			}
		} else {
			manager.mu.Lock()
			rawExecutor := manager.raw[task.Capability]
			manager.mu.Unlock()
			if rawExecutor == nil {
				err = fmt.Errorf("no executor registered for task capability %q", task.Capability)
			} else {
				result, execErr := rawExecutor(ctx, task.Input)
				if execErr != nil {
					err = execErr
				} else {
					status, reason := taskTerminalStatus(ctx, manager.taskTimeout)
					err = manager.finishRaw(taskID, status, reason, result)
				}
			}
		}
	}
	if err != nil {
		_ = manager.finish(taskID, TaskFailed, err.Error(), nil)
	}
	manager.mu.Lock()
	delete(manager.active, taskID)
	delete(manager.cancel, taskID)
	manager.pumpLocked()
	manager.mu.Unlock()
}

func taskTerminalStatus(ctx context.Context, timeout time.Duration) (TaskStatus, string) {
	switch {
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		return TaskTimeout, "task timeout after " + timeout.String()
	case errors.Is(ctx.Err(), context.Canceled):
		return TaskCancelled, "cancelled"
	default:
		return TaskDone, ""
	}
}

func (manager *TaskManager) finish(taskID string, status TaskStatus, reason string, result *Envelope) error {
	return runstate.WithLock(manager.root, func() error {
		task, err := loadTask(manager.root, taskID)
		if err != nil {
			return err
		}
		if task.Status != TaskRunning {
			return nil
		}
		if err := task.Transition(status); err != nil {
			return err
		}
		task.FinishedAt = manager.clock()
		task.Reason = reason
		task.Result = result
		task.ResultRaw = nil
		task.ExpiresAt = task.FinishedAt.Add(manager.resultTTL)
		if result != nil {
			encoded, marshalErr := json.Marshal(result)
			if marshalErr != nil {
				return marshalErr
			}
			digest := sha256.Sum256(encoded)
			task.ResultDigest = "sha256:" + hex.EncodeToString(digest[:])
		}
		return saveTaskLocked(manager.root, task)
	})
}

func (manager *TaskManager) finishRaw(taskID string, status TaskStatus, reason string, result json.RawMessage) error {
	return runstate.WithLock(manager.root, func() error {
		task, err := loadTask(manager.root, taskID)
		if err != nil {
			return err
		}
		if task.Status != TaskRunning {
			return nil
		}
		if err := task.Transition(status); err != nil {
			return err
		}
		task.FinishedAt = manager.clock()
		task.Reason = reason
		task.Result = nil
		task.ResultRaw = append(json.RawMessage{}, result...)
		task.ExpiresAt = task.FinishedAt.Add(manager.resultTTL)
		digest := sha256.Sum256(result)
		task.ResultDigest = "sha256:" + hex.EncodeToString(digest[:])
		return saveTaskLocked(manager.root, task)
	})
}

func (manager *TaskManager) removeQueued(taskID string) {
	for index, queuedID := range manager.queue {
		if queuedID == taskID {
			manager.queue = append(manager.queue[:index], manager.queue[index+1:]...)
			return
		}
	}
}

func (manager *TaskManager) clock() time.Time { return manager.now().UTC() }

func (task Task) info() TaskInfo {
	return TaskInfo{
		TaskID:       task.TaskID,
		RunID:        task.RunID,
		Status:       task.Status,
		Reason:       task.Reason,
		ResultDigest: task.ResultDigest,
	}
}

func (task Task) snapshot(now time.Time, includeResult bool) TaskSnapshot {
	snapshot := TaskSnapshot{TaskInfo: task.info()}
	if includeResult && task.Result != nil && (task.ExpiresAt.IsZero() || now.Before(task.ExpiresAt)) {
		snapshot.Result = task.Result
	} else if includeResult && len(task.ResultRaw) > 0 && (task.ExpiresAt.IsZero() || now.Before(task.ExpiresAt)) {
		var raw any
		if err := json.Unmarshal(task.ResultRaw, &raw); err == nil {
			snapshot.Result = raw
		}
	}
	return snapshot
}

func newTaskID() (string, error) {
	var bytes [8]byte
	if _, err := io.ReadFull(rand.Reader, bytes[:]); err != nil {
		return "", err
	}
	return "task_" + hex.EncodeToString(bytes[:]), nil
}

func ValidTaskID(taskID string) bool {
	if len(taskID) != len("task_")+16 || !strings.HasPrefix(taskID, "task_") {
		return false
	}
	for _, char := range strings.TrimPrefix(taskID, "task_") {
		if (char >= 'a' && char <= 'f') || (char >= '0' && char <= '9') {
			continue
		}
		return false
	}
	return true
}

func taskDir(root string) string { return filepath.Join(root, ".git", "jacu", "tasks") }

func taskPath(root, taskID string) string { return filepath.Join(taskDir(root), taskID+".json") }

func saveTaskLocked(root string, task Task) error {
	if !ValidTaskID(task.TaskID) {
		return fmt.Errorf("invalid task_id %q", task.TaskID)
	}
	if task.SchemaVersion == "" || task.SchemaVersion == "1" {
		task.SchemaVersion = CurrentTaskSchemaVersion
	}
	if task.SchemaVersion != CurrentTaskSchemaVersion {
		return fmt.Errorf("unsupported task schema version %q", task.SchemaVersion)
	}
	if !strings.HasPrefix(task.Capability, "jacu_") {
		return fmt.Errorf("unsupported task capability %q", task.Capability)
	}
	if task.Capability == ToolName && (task.RunID == "" || !runstate.ValidRunID(task.RunID)) {
		return fmt.Errorf("invalid task run_id %q", task.RunID)
	}
	if task.Capability != ToolName && task.RunID != "" && !runstate.ValidRunID(task.RunID) {
		return fmt.Errorf("invalid task run_id %q", task.RunID)
	}
	dir := taskDir(root)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("prepare task directory: %w", err)
	}
	encoded, err := json.MarshalIndent(task, "", "  ")
	if err != nil {
		return fmt.Errorf("encode task: %w", err)
	}
	file, err := os.CreateTemp(dir, ".task-*")
	if err != nil {
		return fmt.Errorf("create task temporary file: %w", err)
	}
	tempPath := file.Name()
	defer func() { _ = os.Remove(tempPath) }()
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		return fmt.Errorf("protect task temporary file: %w", err)
	}
	if _, err := file.Write(encoded); err != nil {
		_ = file.Close()
		return fmt.Errorf("write task: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close task: %w", err)
	}
	// #nosec G703 -- taskPath is derived from the validated task id and the
	// server-resolved repository root; rename is the atomic persistence boundary.
	if err := os.Rename(tempPath, taskPath(root, task.TaskID)); err != nil {
		return fmt.Errorf("persist task: %w", err)
	}
	return nil
}

func loadTask(root, taskID string) (Task, error) {
	if !ValidTaskID(taskID) {
		return Task{}, fmt.Errorf("invalid task_id %q", taskID)
	}
	// #nosec G304,G703 -- taskID is validated and the directory is fixed below
	// the server-resolved repository root.
	content, err := os.ReadFile(taskPath(root, taskID))
	if err != nil {
		return Task{}, err
	}
	var task Task
	if err := json.Unmarshal(content, &task); err != nil {
		return Task{}, fmt.Errorf("decode task %s: %w", taskID, err)
	}
	if task.TaskID != taskID {
		return Task{}, fmt.Errorf("loaded task_id %q does not match requested task_id %q", task.TaskID, taskID)
	}
	if task.SchemaVersion == "" || task.SchemaVersion == "1" {
		task.SchemaVersion = CurrentTaskSchemaVersion
	}
	if task.SchemaVersion != CurrentTaskSchemaVersion {
		return Task{}, fmt.Errorf("unsupported task schema version %q", task.SchemaVersion)
	}
	return task, nil
}

func listTasks(root string) ([]Task, error) {
	entries, err := os.ReadDir(taskDir(root))
	if os.IsNotExist(err) {
		return []Task{}, nil
	}
	if err != nil {
		return nil, err
	}
	tasks := make([]Task, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		taskID := strings.TrimSuffix(entry.Name(), ".json")
		task, err := loadTask(root, taskID)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, task)
	}
	sort.Slice(tasks, func(i, j int) bool {
		if tasks[i].CreatedAt.Equal(tasks[j].CreatedAt) {
			return tasks[i].TaskID < tasks[j].TaskID
		}
		return tasks[i].CreatedAt.Before(tasks[j].CreatedAt)
	})
	return tasks, nil
}
