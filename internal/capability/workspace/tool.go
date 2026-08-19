package workspace

import (
	"context"
	"encoding/json"
	"strings"
	"time"
	"unicode"

	"github.com/jacu-dev/jacu-harness/internal/capability/verify"
	capabilityruntime "github.com/jacu-dev/jacu-harness/internal/runtime"
	"github.com/jacu-dev/jacu-harness/internal/telemetry"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	WorkspaceOpenToolName   = "jacu_workspace_open"
	WorkspaceStatusToolName = "jacu_status"
	WorkspaceStatusAlias    = "jacu_workspace_status"
	DiffToolName            = "jacu_diff"
	ApplyToolName           = "jacu_apply"
	DiscardToolName         = "jacu_discard"

	defaultHostName  = "unknown-mcp-client"
	maxHostNameRunes = 128
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

func RegisterTool(server *mcp.Server, root string) {
	registerWorkspaceToolsWithTaskManager(server, root, newWorkspaceOperationGate(), nil)
}

func RegisterToolWithTaskManager(server *mcp.Server, root string, manager *verify.TaskManager) {
	registerWorkspaceToolsWithTaskManager(server, root, newWorkspaceOperationGate(), manager)
}

type workspaceOperationGate interface {
	acquire(context.Context) error
	release()
}

type contextOperationGate struct {
	token chan struct{}
}

func newWorkspaceOperationGate() workspaceOperationGate {
	gate := &contextOperationGate{token: make(chan struct{}, 1)}
	gate.token <- struct{}{}
	return gate
}

func (gate *contextOperationGate) acquire(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-gate.token:
	}
	select {
	case <-ctx.Done():
		gate.release()
		return ctx.Err()
	default:
		return nil
	}
}

func (gate *contextOperationGate) release() {
	gate.token <- struct{}{}
}

func registerWorkspaceTools(server *mcp.Server, root string, gate workspaceOperationGate) {
	registerWorkspaceToolsWithTaskManager(server, root, gate, nil)
}

func registerWorkspaceToolsWithTaskManager(server *mcp.Server, root string, gate workspaceOperationGate, manager *verify.TaskManager) {
	registerOpenTool(server, root, gate)
	registerStatusToolWithTaskManager(server, root, gate, WorkspaceStatusToolName, manager)
	registerStatusToolWithTaskManager(server, root, gate, WorkspaceStatusAlias, manager)
	registerDiffTool(server, root, gate)
	registerApplyTool(server, root, gate)
	registerDiscardTool(server, root, gate)
}

func registerOpenTool(server *mcp.Server, root string, gate workspaceOperationGate) {
	capability := workspaceOpenCapability(root)
	mcp.AddTool(server, workspaceTool(
		WorkspaceOpenToolName,
		"Open.",
		false,
		false,
		false,
		false,
	), func(ctx context.Context, _ *mcp.CallToolRequest, input OpenInput) (*mcp.CallToolResult, envelope[OpenData], error) {
		return executeTyped[OpenInput, OpenData](ctx, gate, capability, input)
	})
}

func registerStatusTool(server *mcp.Server, root string, gate workspaceOperationGate, name string) {
	registerStatusToolWithTaskManager(server, root, gate, name, nil)
}

func registerStatusToolWithTaskManager(server *mcp.Server, root string, gate workspaceOperationGate, name string, manager *verify.TaskManager) {
	capability := workspaceStatusCapabilityWithTaskManager(root, manager)
	tool := workspaceTool(
		name,
		"Status.",
		true,
		true,
		false,
		false,
	)
	tool.InputSchema = statusInputSchema()
	tool.OutputSchema = statusOutputSchema()
	mcp.AddTool(server, tool, func(ctx context.Context, _ *mcp.CallToolRequest, input StatusInput) (*mcp.CallToolResult, envelope[StatusData], error) {
		return executeTyped[StatusInput, StatusData](ctx, gate, capability, input)
	})
}

func registerDiffTool(server *mcp.Server, root string, gate workspaceOperationGate) {
	capability := workspaceDiffCapability(root)
	mcp.AddTool(server, workspaceTool(
		DiffToolName,
		"Diff.",
		false,
		true,
		false,
		false,
	), func(ctx context.Context, _ *mcp.CallToolRequest, input DiffInput) (*mcp.CallToolResult, envelope[DiffData], error) {
		return executeTyped[DiffInput, DiffData](ctx, gate, capability, input)
	})
}

func registerApplyTool(server *mcp.Server, root string, gate workspaceOperationGate) {
	// Apply is open-world in v1: verification_commands may invoke arbitrary
	// mission binaries and network access is not prevented. When a future phase
	// adds a sandbox/network guard, verification runs closed-world and this hint
	// returns to false.
	mcp.AddTool(server, workspaceTool(
		ApplyToolName,
		"Apply.",
		false,
		false,
		true,
		true,
	), func(ctx context.Context, request *mcp.CallToolRequest, input ApplyInput) (*mcp.CallToolResult, envelope[ApplyData], error) {
		return executeTyped[ApplyInput, ApplyData](ctx, gate, workspaceApplyCapability(root, requestHostName(request)), input)
	})
}

func registerDiscardTool(server *mcp.Server, root string, gate workspaceOperationGate) {
	capability := workspaceDiscardCapability(root)
	mcp.AddTool(server, workspaceTool(
		DiscardToolName,
		"Discard.",
		false,
		false,
		true,
		false,
	), func(ctx context.Context, _ *mcp.CallToolRequest, input DiscardInput) (*mcp.CallToolResult, envelope[DiscardData], error) {
		return executeTyped[DiscardInput, DiscardData](ctx, gate, capability, input)
	})
}

func workspaceTool(name, description string, readOnly, idempotent, destructive, openWorld bool) *mcp.Tool {
	return &mcp.Tool{
		Name:        name,
		Description: description,
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:    readOnly,
			DestructiveHint: &destructive,
			IdempotentHint:  idempotent,
			OpenWorldHint:   &openWorld,
		},
	}
}

func executeTyped[I, D any](ctx context.Context, gate workspaceOperationGate, capability capabilityruntime.Capability, input I) (*mcp.CallToolResult, envelope[D], error) {
	executionCtx, cancel := context.WithTimeout(ctx, capability.Spec.Timeout)
	defer cancel()
	if err := gate.acquire(executionCtx); err != nil {
		return nil, envelope[D]{}, err
	}
	defer gate.release()

	rawInput, err := json.Marshal(input)
	if err != nil {
		return nil, envelope[D]{}, err
	}
	result := capabilityruntime.Execute(executionCtx, capability, rawInput)
	data, _ := result.Data.(D)
	return nil, envelope[D]{
		Status:      result.Status,
		Summary:     result.Summary,
		Data:        data,
		Artifacts:   nonNilStrings(result.Artifacts),
		Warnings:    nonNilStrings(result.Warnings),
		NextActions: nonNilStrings(result.NextActions),
		TraceID:     result.TraceID,
	}, nil
}

func workspaceOpenCapability(root string) capabilityruntime.Capability {
	return capabilityruntime.Capability{
		ProjectID: telemetry.ProjectID(root),
		Spec:      workspaceSpec(WorkspaceOpenToolName, capabilityruntime.RiskWrite, false, false, false, time.Minute, 16*1024),
		Handler: func(ctx context.Context, rawInput json.RawMessage) (capabilityruntime.Result, error) {
			var input OpenInput
			if err := json.Unmarshal(rawInput, &input); err != nil {
				return capabilityruntime.Result{}, err
			}
			result, err := Open(ctx, root, input)
			if err != nil {
				return capabilityruntime.Result{}, err
			}
			return capabilityruntime.Result{
				Status: result.Status, Summary: result.Summary, Data: result.Data,
				Artifacts: []string{}, Warnings: nonNilStrings(result.Warnings), NextActions: []string{},
			}, nil
		},
	}
}

func workspaceStatusCapability(root string) capabilityruntime.Capability {
	return workspaceStatusCapabilityWithTaskManager(root, nil)
}

func workspaceStatusCapabilityWithTaskManager(root string, manager *verify.TaskManager) capabilityruntime.Capability {
	return capabilityruntime.Capability{
		ProjectID: telemetry.ProjectID(root),
		Spec:      workspaceSpec(WorkspaceStatusToolName, capabilityruntime.RiskSafe, true, true, false, 30*time.Second, 16*1024),
		Handler: func(ctx context.Context, rawInput json.RawMessage) (capabilityruntime.Result, error) {
			var input StatusInput
			if err := json.Unmarshal(rawInput, &input); err != nil {
				return capabilityruntime.Result{}, err
			}
			result, err := WorkspaceStatusWithTasks(ctx, root, manager, input)
			if err != nil {
				return capabilityruntime.Result{}, err
			}
			return capabilityruntime.Result{
				Status: result.Status, Summary: result.Summary, Data: result.Data,
				Artifacts: []string{}, Warnings: nonNilStrings(result.Warnings), NextActions: []string{},
			}, nil
		},
	}
}

func workspaceDiffCapability(root string) capabilityruntime.Capability {
	return capabilityruntime.Capability{
		ProjectID: telemetry.ProjectID(root),
		// Repeated calls on the same tree converge to the same digest and reviewed
		// state. Refreshing reviewed_at is intentional freshness, so this remains
		// idempotent despite its real state and index writes.
		Spec: workspaceSpec(DiffToolName, capabilityruntime.RiskWrite, false, true, false, time.Minute, maxDiffOutputBytes),
		Handler: func(ctx context.Context, rawInput json.RawMessage) (capabilityruntime.Result, error) {
			var input DiffInput
			if err := json.Unmarshal(rawInput, &input); err != nil {
				return capabilityruntime.Result{}, err
			}
			result, err := WorkspaceDiff(ctx, root, input)
			if err != nil {
				return capabilityruntime.Result{}, err
			}
			return workspaceDiffRuntimeResult(result), nil
		},
	}
}

func workspaceApplyCapability(root, hostName string) capabilityruntime.Capability {
	return capabilityruntime.Capability{
		ProjectID: telemetry.ProjectID(root),
		// verification_commands can invoke arbitrary mission binaries, and v1 does
		// not prevent network access. A future sandbox/network guard makes these
		// runs closed-world and returns OpenWorld to false.
		Spec: workspaceSpec(ApplyToolName, capabilityruntime.RiskWrite, false, false, true, 10*time.Minute, 16*1024),
		Handler: func(ctx context.Context, rawInput json.RawMessage) (capabilityruntime.Result, error) {
			var input ApplyInput
			if err := json.Unmarshal(rawInput, &input); err != nil {
				return capabilityruntime.Result{}, err
			}
			result, err := Apply(ctx, root, input, hostName)
			if err != nil {
				return capabilityruntime.Result{}, err
			}
			return capabilityruntime.Result{
				Status: result.Status, Summary: result.Summary, Data: result.Data,
				Artifacts: []string{}, Warnings: nonNilStrings(result.Warnings), NextActions: nonNilStrings(result.NextActions),
			}, nil
		},
	}
}

func workspaceDiscardCapability(root string) capabilityruntime.Capability {
	return capabilityruntime.Capability{
		ProjectID: telemetry.ProjectID(root),
		Spec:      workspaceSpec(DiscardToolName, capabilityruntime.RiskWrite, false, false, false, 2*time.Minute, 16*1024),
		Handler: func(ctx context.Context, rawInput json.RawMessage) (capabilityruntime.Result, error) {
			var input DiscardInput
			if err := json.Unmarshal(rawInput, &input); err != nil {
				return capabilityruntime.Result{}, err
			}
			result, err := Discard(ctx, root, input)
			if err != nil {
				return capabilityruntime.Result{}, err
			}
			return capabilityruntime.Result{
				Status: result.Status, Summary: result.Summary, Data: result.Data,
				Artifacts: []string{}, Warnings: nonNilStrings(result.Warnings), NextActions: nonNilStrings(result.NextActions),
			}, nil
		},
	}
}

func workspaceSpec(name string, risk capabilityruntime.RiskLevel, readOnly, idempotent, openWorld bool, timeout time.Duration, maxOutputBytes int64) capabilityruntime.ToolSpec {
	return capabilityruntime.ToolSpec{
		Name: name, Version: "1", Risk: risk, ReadOnly: readOnly, Idempotent: idempotent,
		OpenWorld: openWorld, Timeout: timeout, MaxInputBytes: 256 * 1024, MaxOutputBytes: maxOutputBytes,
	}
}

func requestHostName(request *mcp.CallToolRequest) string {
	if request == nil {
		return defaultHostName
	}
	client := request.ClientInfo()
	if client == nil {
		return defaultHostName
	}
	return canonicalHostName(client.Name)
}

func canonicalHostName(name string) string {
	var canonical strings.Builder
	previousSeparator := false
	for _, character := range name {
		if unicode.IsSpace(character) || unicode.IsControl(character) || unicode.Is(unicode.Cf, character) {
			if !previousSeparator {
				canonical.WriteByte(' ')
			}
			previousSeparator = true
			continue
		}
		canonical.WriteRune(character)
		previousSeparator = false
	}
	value := strings.TrimSpace(canonical.String())
	if value == "" {
		return defaultHostName
	}
	runes := []rune(value)
	if len(runes) > maxHostNameRunes {
		value = strings.TrimSpace(string(runes[:maxHostNameRunes]))
	}
	if value == "" {
		return defaultHostName
	}
	return value
}

func nonNilStrings(items []string) []string {
	if items == nil {
		return []string{}
	}
	return items
}
