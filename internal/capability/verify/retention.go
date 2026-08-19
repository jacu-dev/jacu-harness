package verify

import (
	"os"
	"sort"
	"time"

	"github.com/jacu-dev/jacu-harness/internal/runstate"
)

const (
	terminalMetadataTTL = 30 * 24 * time.Hour
	terminalRecordCap   = 1000
)

func terminalTask(status TaskStatus) bool {
	return status == TaskDone || status == TaskFailed || status == TaskCancelled || status == TaskTimeout
}

// RetainTasks compacts expired payloads and bounds terminal metadata. It is
// explicitly invoked at startup/list time; no background collector is used.
func (manager *TaskManager) RetainTasks() error {
	return RetainTasksAt(manager.root, manager.clock())
}

// RetainTasksAt applies the durable task retention policy for root at now.
// It is shared with explicit storage lifecycle actions; callers never need to
// reproduce task classification or deletion decisions.
func RetainTasksAt(root string, now time.Time) error {
	return retainTasks(root, now, saveTaskLocked)
}

func retainTasks(root string, now time.Time, save func(string, Task) error) error {
	return runstate.WithLock(root, func() error {
		entries, err := os.ReadDir(taskDir(root))
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return err
		}
		compact := make([]Task, 0)
		for _, entry := range entries {
			if entry.IsDir() || !ValidTaskID(trimTaskExtension(entry.Name())) {
				continue
			}
			task, loadErr := loadTask(root, trimTaskExtension(entry.Name()))
			if loadErr != nil {
				continue // corrupt records remain for operator inspection
			}
			if !terminalTask(task.Status) {
				continue
			}
			if task.PayloadPrunedAt.IsZero() && !task.ExpiresAt.IsZero() && !now.Before(task.ExpiresAt) {
				task.Input = nil
				task.Result = nil
				task.ResultRaw = nil
				task.PayloadPrunedAt = now
				if err := save(root, task); err != nil {
					return err
				}
			}
			if !task.PayloadPrunedAt.IsZero() {
				compact = append(compact, task)
			}
		}
		sort.Slice(compact, func(i, j int) bool {
			if compact[i].FinishedAt.Equal(compact[j].FinishedAt) {
				return compact[i].TaskID < compact[j].TaskID
			}
			return compact[i].FinishedAt.Before(compact[j].FinishedAt)
		})
		for index, task := range compact {
			tooOld := !task.FinishedAt.IsZero() && now.Sub(task.FinishedAt) > terminalMetadataTTL
			overCap := len(compact)-index > terminalRecordCap
			if tooOld || overCap {
				if err := os.Remove(taskPath(root, task.TaskID)); err != nil && !os.IsNotExist(err) {
					return err
				}
			}
		}
		return nil
	})
}

func trimTaskExtension(name string) string {
	if len(name) > 5 && name[len(name)-5:] == ".json" {
		return name[:len(name)-5]
	}
	return name
}
