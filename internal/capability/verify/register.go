package verify

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/jacu-dev/jacu-harness/internal/runstate"
	capabilityruntime "github.com/jacu-dev/jacu-harness/internal/runtime"
	"github.com/jacu-dev/jacu-harness/internal/telemetry"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	ToolName = "jacu_verify"
)

type envelope[T any] struct {
	Status      string   `json:"status"`
	Summary     string   `json:"summary"`
	Data        T        `json:"data"`
	Artifacts   []string `json:"artifacts"`
	Warnings    []string `json:"warnings"`
	NextActions []string `json:"next_actions"`
	TraceID     string   `json:"trace_id"`
}

// Spec limits are runtime policy, not tool parameters. The object being
// governed does not get to choose its own timeout or its own output cap.
const (
	toolTimeout    = 630 * time.Second
	maxOutputBytes = 16 * 1024
)

func RegisterTool(server *mcp.Server, root string) {
	manager, err := NewTaskManager(root)
	if err != nil {
		panic("verify: initialize task manager: " + err.Error())
	}
	RegisterToolWithTaskManager(server, root, manager)
}

func RegisterToolWithTaskManager(server *mcp.Server, root string, manager *TaskManager) {
	if manager == nil {
		panic("verify: task manager is nil")
	}
	registerVerify(server, root, manager)
}

func registerVerify(server *mcp.Server, root string, manager *TaskManager) {
	destructive := false
	openWorld := false
	capability := capabilityruntime.Capability{
		ProjectID: telemetry.ProjectID(root),
		Spec: capabilityruntime.ToolSpec{
			Name:    ToolName,
			Version: "1",
			// It runs processes that can touch the worktree, so it is not
			// read-only and not idempotent — a suite writes caches and coverage.
			Risk:           capabilityruntime.RiskWrite,
			ReadOnly:       false,
			Idempotent:     false,
			OpenWorld:      false,
			Timeout:        toolTimeout,
			MaxInputBytes:  16 * 1024,
			MaxOutputBytes: maxOutputBytes,
		},
		Handler: func(ctx context.Context, rawInput json.RawMessage) (capabilityruntime.Result, error) {
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
		},
	}

	tool := &mcp.Tool{
		Name:         ToolName,
		Description:  "Run checks.",
		OutputSchema: outputSchema(),
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:    false,
			DestructiveHint: &destructive,
			IdempotentHint:  false,
			OpenWorldHint:   &openWorld,
		},
	}
	tool.InputSchema = verifyInputSchema()
	mcp.AddTool(server, tool, func(ctx context.Context, _ *mcp.CallToolRequest, input Input) (*mcp.CallToolResult, envelope[Data], error) {
		rawInput, err := json.Marshal(input)
		if err != nil {
			return nil, envelope[Data]{}, err
		}
		result := capabilityruntime.Execute(ctx, capability, rawInput)
		data, _ := result.Data.(Data)
		return nil, envelope[Data]{
			Status:      result.Status,
			Summary:     result.Summary,
			Data:        data,
			Artifacts:   result.Artifacts,
			Warnings:    result.Warnings,
			NextActions: result.NextActions,
			TraceID:     result.TraceID,
		}, nil
	})
}

func verifyInputSchema() *jsonschema.Schema {
	return &jsonschema.Schema{
		Type: "object",
		Properties: map[string]*jsonschema.Schema{
			"run_id":  {Type: "string"},
			"argv":    {Type: "array", Items: &jsonschema.Schema{Type: "string"}},
			"async":   {Type: "boolean"},
			"task_id": {Type: "string"},
			"cancel":  {Type: "boolean"},
		},
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

// outputSchema announces the verdict enum. A host that has to branch on the
// verdict should learn the five values from the contract, not by observation —
// and "not run" existing at all is the part nobody guesses.
func outputSchema() *jsonschema.Schema {
	schema, err := jsonschema.For[envelope[Data]](nil)
	if err != nil {
		panic("verify: infer output schema: " + err.Error())
	}
	data, ok := schema.Properties["data"]
	if !ok {
		panic("verify: output schema lost data")
	}
	verdict, ok := data.Properties["verdict"]
	if !ok {
		panic("verify: output schema lost verdict")
	}
	verdict.Enum = []any{VerdictPass, VerdictFail, VerdictTimeout, VerdictBlocked, VerdictNotRun}
	// Task metadata is already a versioned sub-contract and is also projected
	// by jacu_status. Keep its nested schema opaque here so the same command
	// evidence is not duplicated in tools/list.
	data.Properties["task"] = &jsonschema.Schema{Types: []string{"null", "object"}}
	return schema
}

// fitOutputCap keeps the answer inside the inline cap by dropping evidence in
// priority order, instead of letting the runtime zero the whole payload on
// overflow. The tails of commands that passed go first — nobody reads the
// output of a test that succeeded — then the tails of the ones that did not.
// argv, status, exit code, duration and the digests are never dropped: they are
// what the verdict and the receipt are made of.
func fitOutputCap(data Data, warnings []string) (Data, []string) {
	if encodedSize(data) <= maxOutputBytes {
		return data, warnings
	}
	dropped := 0
	for index := range data.Commands {
		if data.Commands[index].Status == StatusPassed {
			data.Commands[index].StdoutTail = ""
			data.Commands[index].StderrTail = ""
			data.Commands[index].Truncated = true
			dropped++
		}
	}
	if encodedSize(data) > maxOutputBytes {
		for index := range data.Commands {
			data.Commands[index].StdoutTail = ""
			data.Commands[index].StderrTail = ""
			data.Commands[index].Truncated = true
		}
		dropped = len(data.Commands)
	}
	if dropped > 0 {
		warnings = append(warnings,
			"output tails dropped to fit the inline cap; the evidence digest still covers the full output")
	}
	return data, warnings
}

func encodedSize(data Data) int {
	encoded, err := json.Marshal(data)
	if err != nil {
		return maxOutputBytes + 1
	}
	return len(encoded)
}
