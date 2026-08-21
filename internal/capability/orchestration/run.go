package orchestration

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jacu-dev/jacu-harness/internal/capability/verify"
	capabilityruntime "github.com/jacu-dev/jacu-harness/internal/runtime"
	"github.com/jacu-dev/jacu-harness/internal/telemetry"
)

func Run(ctx context.Context, root string, in Input) capabilityruntime.Result {
	manager, err := verify.NewTaskManager(root)
	if err != nil {
		return capabilityruntime.Result{
			Status: "failed", Summary: "capability execution failed",
			Artifacts: []string{}, Warnings: []string{}, NextActions: []string{},
		}
	}
	bindFlowExecutor(root, manager)
	return RunWithManager(ctx, root, in, manager)
}

func RunWithManager(ctx context.Context, root string, in Input, manager *verify.TaskManager) capabilityruntime.Result {
	return capabilityruntime.ExecuteInput(ctx, flowCapability(root, manager), in)
}

func bindFlowExecutor(root string, manager *verify.TaskManager) {
	if manager == nil {
		return
	}
	_ = manager.RegisterRawExecutor(ToolName, func(ctx context.Context, raw json.RawMessage) (json.RawMessage, error) {
		var input Input
		if err := json.Unmarshal(raw, &input); err != nil {
			return nil, err
		}
		input.Async = false
		result, err := executeInput(ctx, root, input)
		if err != nil {
			return nil, err
		}
		return json.Marshal(result)
	})
}

func flowCapability(root string, manager *verify.TaskManager) capabilityruntime.Capability {
	return capabilityruntime.Capability{
		ProjectID: telemetry.ProjectID(root),
		Spec: capabilityruntime.ToolSpec{
			Name: ToolName, Version: "1", Risk: capabilityruntime.RiskWrite,
			ReadOnly: false, Idempotent: false, OpenWorld: false,
			Timeout: 15 * time.Minute, MaxInputBytes: 256 * 1024, MaxOutputBytes: 16 * 1024,
		},
		Handler: capabilityruntime.RequireWorkTree(root, flowHandler(root, manager)),
	}
}

func flowHandler(root string, manager *verify.TaskManager) capabilityruntime.Handler {
	return func(ctx context.Context, rawInput json.RawMessage) (capabilityruntime.Result, error) {
		var input Input
		if err := json.Unmarshal(rawInput, &input); err != nil {
			return capabilityruntime.Result{}, err
		}
		validation := Validate(input.Flow)
		if validation.Blocked() {
			flow := FlowResult{Status: "blocked", Summary: "Flow validation blocked.", Waves: [][]string{}, Trace: []TraceEvent{}, Findings: validation.Findings}
			return capabilityruntime.Result{Status: "blocked", Summary: flow.Summary, Data: ToolData{Flow: &flow}, Artifacts: []string{}, Warnings: []string{}, NextActions: []string{}}, nil
		}
		if input.Async {
			encoded, err := json.Marshal(input)
			if err != nil {
				return capabilityruntime.Result{}, err
			}
			info, err := manager.StartRaw(ctx, ToolName, input.RunID, encoded)
			if err != nil {
				return capabilityruntime.Result{}, err
			}
			return capabilityruntime.Result{Status: "accepted", Summary: "Flow task accepted.", Data: ToolData{Task: &info}, Artifacts: []string{}, Warnings: []string{}, NextActions: []string{"poll jacu_status with this task_id"}}, nil
		}
		flow, err := executeInput(ctx, root, input)
		if err != nil {
			return capabilityruntime.Result{}, err
		}
		return capabilityruntime.Result{Status: flow.Status, Summary: flow.Summary, Data: ToolData{Flow: &flow}, Artifacts: []string{}, Warnings: []string{}, NextActions: []string{}}, nil
	}
}
