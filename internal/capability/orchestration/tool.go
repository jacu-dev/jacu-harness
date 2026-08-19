package orchestration

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/jacu-dev/jacu-harness/internal/capability/missioncompile"
	"github.com/jacu-dev/jacu-harness/internal/capability/verify"
	"github.com/jacu-dev/jacu-harness/internal/capability/workspace"
	capabilityruntime "github.com/jacu-dev/jacu-harness/internal/runtime"
	"github.com/jacu-dev/jacu-harness/internal/telemetry"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const ToolName = "jacu_flow_run"

type envelope[T any] struct {
	Status      string   `json:"status"`
	Summary     string   `json:"summary"`
	Data        T        `json:"data"`
	Artifacts   []string `json:"artifacts"`
	Warnings    []string `json:"warnings"`
	NextActions []string `json:"next_actions"`
	TraceID     string   `json:"trace_id"`
}

type ToolData struct {
	Task *verify.TaskInfo `json:"task,omitempty"`
	Flow *FlowResult      `json:"flow,omitempty"`
}

func RegisterTool(server *mcp.Server, root string, manager *verify.TaskManager) {
	if manager == nil {
		panic("orchestration: task manager is nil")
	}
	if err := manager.RegisterRawExecutor(ToolName, func(ctx context.Context, raw json.RawMessage) (json.RawMessage, error) {
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
	}); err != nil {
		panic("orchestration: register task executor: " + err.Error())
	}

	destructive := false
	openWorld := false
	capability := capabilityruntime.Capability{
		ProjectID: telemetry.ProjectID(root),
		Spec: capabilityruntime.ToolSpec{
			Name: ToolName, Version: "1", Risk: capabilityruntime.RiskWrite,
			ReadOnly: false, Idempotent: false, OpenWorld: false,
			Timeout: 15 * time.Minute, MaxInputBytes: 256 * 1024, MaxOutputBytes: 16 * 1024,
		},
		Handler: func(ctx context.Context, rawInput json.RawMessage) (capabilityruntime.Result, error) {
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
			status := flow.Status
			if status == "ok" {
				status = "ok"
			}
			return capabilityruntime.Result{Status: status, Summary: flow.Summary, Data: ToolData{Flow: &flow}, Artifacts: []string{}, Warnings: []string{}, NextActions: []string{}}, nil
		},
	}

	mcp.AddTool(server, &mcp.Tool{
		Name:         ToolName,
		Description:  "Flow.",
		InputSchema:  inputSchema(),
		OutputSchema: outputSchema(),
		Annotations:  &mcp.ToolAnnotations{ReadOnlyHint: false, DestructiveHint: &destructive, IdempotentHint: false, OpenWorldHint: &openWorld},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input Input) (*mcp.CallToolResult, envelope[ToolData], error) {
		rawInput, err := json.Marshal(input)
		if err != nil {
			return nil, envelope[ToolData]{}, err
		}
		result := capabilityruntime.Execute(ctx, capability, rawInput)
		data, _ := result.Data.(ToolData)
		return nil, envelope[ToolData]{Status: result.Status, Summary: result.Summary, Data: data, Artifacts: result.Artifacts, Warnings: result.Warnings, NextActions: result.NextActions, TraceID: result.TraceID}, nil
	})
}

func executeInput(ctx context.Context, root string, input Input) (FlowResult, error) {
	initial := make(map[string]any, len(input.Context)+1)
	for key, value := range input.Context {
		initial[key] = value
	}
	if input.RunID != "" {
		initial["run_id"] = input.RunID
	}
	return Execute(ctx, input.Flow, initial, func(ctx context.Context, node Node, results map[string]NodeResult) (NodeResult, error) {
		return executeNode(ctx, root, input, node, results)
	})
}

func executeNode(ctx context.Context, root string, input Input, node Node, _ map[string]NodeResult) (result NodeResult, resultErr error) {
	started := time.Now()
	defer func() {
		status := result.Status
		if status == "" {
			status = "failed"
		}
		telemetry.EmitBestEffortInput(telemetry.EventInput{
			Timestamp: time.Now().UTC(), ProjectID: telemetry.ProjectID(root), TraceID: telemetry.NewTraceID(),
			RunID: input.RunID, Event: telemetry.EventFlowNode, Tool: node.Uses, Status: status,
			Duration: time.Since(started), ExitReason: mapFlowExitReason(status),
		})
	}()
	with := node.With
	switch node.Uses {
	case UseMission:
		var missionInput missioncompile.Input
		if err := decodeWith(with, &missionInput); err != nil {
			return NodeResult{}, err
		}
		mission, status, _ := missioncompile.Compile(root, missionInput)
		output, err := mapData(mission)
		if err != nil {
			return NodeResult{}, err
		}
		return NodeResult{Status: status, Policy: policyForStatus(status), Output: output}, nil
	case UseWorkspace:
		var openInput workspace.OpenInput
		if err := decodeWith(with, &openInput); err != nil {
			return NodeResult{}, err
		}
		if openInput.MissionID == "" {
			return NodeResult{Status: "blocked", Policy: "blocked"}, fmt.Errorf("workspace node %s requires mission_id", node.ID)
		}
		result, err := workspace.Open(ctx, root, openInput)
		if err != nil {
			return NodeResult{}, err
		}
		output, err := mapData(result.Data)
		if err != nil {
			return NodeResult{}, err
		}
		if result.Status != "ok" {
			return NodeResult{Status: result.Status, Policy: policyForStatus(result.Status), Output: output}, nil
		}
		return NodeResult{Status: result.Status, Policy: "pass", Output: output}, nil
	case UseVerify:
		var verifyInput verify.Input
		if err := decodeWith(with, &verifyInput); err != nil {
			return NodeResult{}, err
		}
		if verifyInput.RunID == "" {
			verifyInput.RunID = input.RunID
		}
		result := verify.Verify(ctx, root, verifyInput)
		output, err := mapData(result.Data)
		if err != nil {
			return NodeResult{}, err
		}
		return NodeResult{Status: result.Status, Verdict: result.Data.Verdict, Policy: policyForVerify(result), Output: output}, nil
	case UseReview:
		var diffInput workspace.DiffInput
		if err := decodeWith(with, &diffInput); err != nil {
			return NodeResult{}, err
		}
		if diffInput.RunID == "" {
			diffInput.RunID = input.RunID
		}
		result, err := workspace.WorkspaceDiff(ctx, root, diffInput)
		if err != nil {
			return NodeResult{}, err
		}
		output, err := mapData(result.Data)
		if err != nil {
			return NodeResult{}, err
		}
		opinions := opinionsFrom(with["opinions"])
		policy := "pass"
		if len(opinions) > 0 {
			policy = "require_approval"
		}
		return NodeResult{Status: result.Status, Policy: policyForStatusOr(result.Status, policy), Output: output, Opinions: opinions}, nil
	case UseApply:
		var applyInput workspace.ApplyInput
		if err := decodeWith(with, &applyInput); err != nil {
			return NodeResult{}, err
		}
		if applyInput.RunID == "" {
			applyInput.RunID = input.RunID
		}
		result, err := workspace.Apply(ctx, root, applyInput, "flow")
		if err != nil {
			return NodeResult{}, err
		}
		output, err := mapData(result.Data)
		if err != nil {
			return NodeResult{}, err
		}
		return NodeResult{Status: result.Status, Policy: policyForStatus(result.Status), Output: output}, nil
	default:
		return NodeResult{}, fmt.Errorf("unsupported flow capability %q", node.Uses)
	}
}

func mapFlowExitReason(status string) string {
	switch status {
	case "ok", "pass", "completed":
		return "completed"
	case "blocked":
		return "blocked"
	case "escalated":
		return "escalated"
	default:
		return "failed"
	}
}

func decodeWith(with map[string]any, destination any) error {
	if with == nil {
		with = map[string]any{}
	}
	encoded, err := json.Marshal(with)
	if err != nil {
		return err
	}
	return json.Unmarshal(encoded, destination)
}

func opinionsFrom(value any) []Opinion {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	var opinions []Opinion
	if json.Unmarshal(encoded, &opinions) != nil {
		return nil
	}
	return opinions
}

func policyForStatus(status string) string {
	if status == "ok" {
		return "pass"
	}
	return "blocked"
}

func policyForVerify(result verify.Envelope) string {
	if result.Status != "ok" {
		return "blocked"
	}
	if result.Data.Verdict == verify.VerdictPass {
		return "pass"
	}
	return "require_approval"
}

func policyForStatusOr(status, fallback string) string {
	if status != "ok" {
		return "blocked"
	}
	return fallback
}

func inputSchema() *jsonschema.Schema {
	return &jsonschema.Schema{Type: "object", Properties: map[string]*jsonschema.Schema{
		// The nested flow contract is deliberately opaque in tools/list. Its
		// complete, bounded contract lives in the OpenSpec and the skill; repeating
		// every node field here would evict an existing capability from the MCP
		// catalogue budget. Runtime validation remains authoritative.
		"flow":    {Type: "object"},
		"context": {Type: "object"}, "run_id": {Type: "string"}, "async": {Type: "boolean"},
	}}
}

func outputSchema() *jsonschema.Schema {
	return &jsonschema.Schema{Type: "object", Properties: map[string]*jsonschema.Schema{
		"status": {Type: "string"}, "summary": {Type: "string"},
		"data": {Type: "object", Properties: map[string]*jsonschema.Schema{
			"task": {Types: []string{"null", "object"}}, "flow": {Types: []string{"null", "object"}},
		}},
	}}
}
