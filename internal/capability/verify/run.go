package verify

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jacu-dev/jacu-harness/internal/runstate"
	capabilityruntime "github.com/jacu-dev/jacu-harness/internal/runtime"
	"github.com/jacu-dev/jacu-harness/internal/telemetry"
)

func Run(ctx context.Context, root string, in Input) capabilityruntime.Result {
	manager, err := NewTaskManager(root)
	if err != nil {
		return capabilityruntime.Result{
			Status: "failed", Summary: "capability execution failed",
			Artifacts: []string{}, Warnings: []string{}, NextActions: []string{},
		}
	}
	return RunWithManager(ctx, root, in, manager)
}

func RunWithManager(ctx context.Context, root string, in Input, manager *TaskManager) capabilityruntime.Result {
	emitVerifyRunning(ctx, root, in)
	return capabilityruntime.ExecuteInput(ctx, verifyCapability(root, manager), in)
}

func emitVerifyRunning(ctx context.Context, root string, in Input) {
	telemetry.WriteLiveInput(capabilityruntime.LiveEvents(ctx), telemetry.EventInput{
		Timestamp: time.Now().UTC(),
		ProjectID: telemetry.ProjectID(root),
		TraceID:   telemetry.NewTraceID(),
		RunID:     in.RunID,
		Event:     telemetry.EventVerify,
		Tool:      ToolName,
		Status:    "running",
	})
}

func verifyCapability(root string, manager *TaskManager) capabilityruntime.Capability {
	return capabilityruntime.Capability{
		ProjectID: telemetry.ProjectID(root),
		Spec: capabilityruntime.ToolSpec{
			Name:           ToolName,
			Version:        "1",
			Risk:           capabilityruntime.RiskWrite,
			ReadOnly:       false,
			Idempotent:     false,
			OpenWorld:      false,
			Timeout:        toolTimeout,
			MaxInputBytes:  16 * 1024,
			MaxOutputBytes: maxOutputBytes,
		},
		Handler: workTreeGuardedVerify(root, verifyHandler(root, manager)),
	}
}

func verifyHandler(root string, manager *TaskManager) capabilityruntime.Handler {
	return func(ctx context.Context, rawInput json.RawMessage) (capabilityruntime.Result, error) {
		var input Input
		if err := json.Unmarshal(rawInput, &input); err != nil {
			return capabilityruntime.Result{}, err
		}
		if input.Cancel {
			if input.TaskID == "" || input.Async {
				result := Verify(ctx, root, Input{RunID: input.RunID})
				result.Status = "blocked"
				result.Summary = "task cancellation requires task_id and async=false"
				return capabilityResult(result), nil
			}
			info, err := manager.Cancel(input.TaskID)
			if err != nil {
				result := Verify(ctx, root, Input{RunID: input.RunID})
				result.Status = "blocked"
				result.Summary = "task cancellation blocked: " + err.Error()
				return capabilityResult(result), nil
			}
			return capabilityruntime.Result{
				Status:      "accepted",
				Summary:     "Task cancellation requested.",
				Data:        Data{Verdict: VerdictNotRun, Commands: []Result{}, Task: &info},
				Artifacts:   []string{},
				Warnings:    []string{},
				NextActions: []string{"poll jacu_status with this task_id"},
			}, nil
		}
		if input.Async {
			if err := eligibleRun(root, input.RunID); err != nil {
				result := Verify(ctx, root, Input{RunID: input.RunID, ArgV: input.ArgV})
				return capabilityResult(result), nil
			}
			info, err := manager.Start(ctx, input)
			if err != nil {
				result := Verify(ctx, root, Input{RunID: input.RunID})
				result.Status = "blocked"
				result.Summary = "task start blocked: " + err.Error()
				return capabilityResult(result), nil
			}
			return capabilityruntime.Result{
				Status:      "accepted",
				Summary:     "Verification task accepted.",
				Data:        Data{Verdict: VerdictNotRun, Commands: []Result{}, Task: &info},
				Artifacts:   []string{},
				Warnings:    []string{},
				NextActions: []string{"poll jacu_status with this task_id"},
			}, nil
		}
		if input.TaskID != "" {
			result := Verify(ctx, root, Input{RunID: input.RunID})
			result.Status = "blocked"
			result.Summary = "task_id requires async=true to start or cancel=true to cancel"
			return capabilityResult(result), nil
		}
		result := Verify(ctx, root, input)
		return capabilityResult(result), nil
	}
}

func eligibleRun(root, runID string) error {
	run, err := runstate.Load(root, runID)
	if err != nil {
		return err
	}
	if run.Status != runstate.StatusOpen && run.Status != runstate.StatusReviewed {
		return fmt.Errorf("run %s is not open for verification (status %q)", runID, run.Status)
	}
	return nil
}

func workTreeGuardedVerify(root string, next capabilityruntime.Handler) capabilityruntime.Handler {
	return func(ctx context.Context, input json.RawMessage) (capabilityruntime.Result, error) {
		if blocked, ok := capabilityruntime.WorkTreeBlock(ctx, root); ok {
			blocked.Data = Data{Verdict: VerdictBlocked, Commands: []Result{}}
			blocked.Artifacts = []string{}
			blocked.Warnings = []string{}
			return blocked, nil
		}
		if next == nil {
			return capabilityruntime.Result{Status: "failed", Summary: "capability handler is missing"}, nil
		}
		return next(ctx, input)
	}
}

func capabilityResult(result Envelope) capabilityruntime.Result {
	data, warnings := fitOutputCap(result.Data, result.Warnings)
	return capabilityruntime.Result{
		Status:      result.Status,
		Summary:     result.Summary,
		Data:        data,
		Artifacts:   []string{},
		Warnings:    warnings,
		NextActions: result.NextActions,
	}
}
