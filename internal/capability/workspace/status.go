package workspace

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/jacu-dev/jacu-harness/internal/capability/verify"
	"github.com/jacu-dev/jacu-harness/internal/gitx"
	"github.com/jacu-dev/jacu-harness/internal/runstate"
)

type ListedRun struct {
	RunID        string          `json:"run_id"`
	Status       runstate.Status `json:"status"`
	AgeSeconds   int64           `json:"age_seconds"`
	DiskBytes    int64           `json:"disk_bytes"`
	DiffLines    int             `json:"diff_lines"`
	BaseBehind   int             `json:"base_behind"`
	ProgramID    string          `json:"program_id,omitempty"`
	ProgramIndex int             `json:"program_index,omitempty"`
}

func statusOutputSchema() *jsonschema.Schema {
	schema, err := jsonschema.For[envelope[StatusData]](nil)
	if err != nil {
		panic("workspace: infer status output schema: " + err.Error())
	}
	data, ok := schema.Properties["data"]
	if !ok {
		panic("workspace: status output schema lost data")
	}
	// Task results retain their real structured envelope at runtime. An opaque
	// item schema avoids duplicating verify's command evidence in tools/list.
	data.Properties["tasks"] = &jsonschema.Schema{
		Types: []string{"null", "array"},
		Items: &jsonschema.Schema{Type: "object"},
	}
	return schema
}

func statusInputSchema() *jsonschema.Schema {
	return &jsonschema.Schema{
		Type: "object",
		Properties: map[string]*jsonschema.Schema{
			"task_id": {Type: "string"},
		},
	}
}

type StatusData struct {
	Runs  []ListedRun           `json:"runs"`
	Tasks []verify.TaskSnapshot `json:"tasks,omitempty"`
}

type StatusInput struct {
	TaskID string `json:"task_id,omitempty"`
}

type StatusResult struct {
	Status   string
	Summary  string
	Data     StatusData
	Warnings []string
}

func WorkspaceStatus(ctx context.Context, root string) (StatusResult, error) {
	return WorkspaceStatusWithTasks(ctx, root, nil, StatusInput{})
}

func WorkspaceStatusWithTasks(ctx context.Context, root string, manager *verify.TaskManager, input StatusInput) (StatusResult, error) {
	if err := ctx.Err(); err != nil {
		return StatusResult{}, err
	}
	root = gitx.OwningRepository(root)
	runs, err := runstate.List(root)
	if err != nil {
		return StatusResult{}, err
	}
	git, err := gitx.New()
	if err != nil {
		return StatusResult{}, err
	}
	now := time.Now().UTC()
	data := StatusData{Runs: make([]ListedRun, 0, len(runs))}
	if manager != nil {
		if input.TaskID != "" {
			if retainErr := manager.RetainTasks(); retainErr != nil {
				return StatusResult{}, retainErr
			}
			task, taskErr := manager.Get(input.TaskID, true)
			if taskErr != nil {
				return StatusResult{}, taskErr
			}
			data.Tasks = []verify.TaskSnapshot{task}
		} else {
			tasks, taskErr := manager.List(false)
			if taskErr != nil {
				return StatusResult{}, taskErr
			}
			data.Tasks = tasks
		}
	}
	warnings := []string{}
	for _, run := range runs {
		item := ListedRun{RunID: run.RunID, Status: run.Status, ProgramID: run.ProgramID, ProgramIndex: run.ProgramCursor}
		reportCorrupted := func(cause error) {
			item.Status = runstate.StatusCorrupted
			warnings = append(warnings, fmt.Sprintf("run %s reported as corrupted: %v; use discard --gc", run.RunID, cause))
			data.Runs = append(data.Runs, item)
		}
		if !run.CreatedAt.IsZero() {
			item.AgeSeconds = max(int64(0), int64(now.Sub(run.CreatedAt).Seconds()))
		}
		if run.Status == runstate.StatusCorrupted {
			data.Runs = append(data.Runs, item)
			continue
		}
		_, statErr := os.Stat(run.Worktree)
		if os.IsNotExist(statErr) && (run.Status == runstate.StatusOpen || run.Status == runstate.StatusReviewed) {
			reportCorrupted(errors.New("worktree missing"))
			continue
		}
		if statErr != nil && !os.IsNotExist(statErr) {
			reportCorrupted(statErr)
			continue
		}
		if statErr == nil {
			diskBytes, sizeErr := directoryBytes(ctx, run.Worktree)
			if sizeErr != nil {
				if errors.Is(sizeErr, fs.ErrNotExist) {
					sizeErr = fmt.Errorf("worktree changed during size calculation: %w", sizeErr)
				}
				reportCorrupted(sizeErr)
				continue
			}
			item.DiskBytes = diskBytes
			added, deleted, _, numstatErr := git.ReadOnlyNumstat(ctx, run.Worktree, run.BaseSHA)
			if numstatErr != nil {
				reportCorrupted(numstatErr)
				continue
			}
			item.DiffLines = added + deleted
			if item.DiffLines > 400 {
				warnings = append(warnings, fmt.Sprintf("large diff (%d lines); consider splitting into smaller runs", item.DiffLines))
			}
		}
		baseBehind, aheadErr := git.CountAhead(ctx, root, run.BaseSHA)
		if aheadErr != nil {
			reportCorrupted(aheadErr)
			continue
		}
		item.BaseBehind = baseBehind
		if item.BaseBehind > 0 {
			warnings = append(warnings, fmt.Sprintf("base is stale by %d commits", item.BaseBehind))
		}
		data.Runs = append(data.Runs, item)
	}
	return StatusResult{Status: "ok", Summary: "Workspace status collected.", Data: data, Warnings: warnings}, nil
}

func directoryBytes(ctx context.Context, root string) (int64, error) {
	var total int64
	err := filepath.WalkDir(root, func(_ string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		total += info.Size()
		return nil
	})
	return total, err
}
