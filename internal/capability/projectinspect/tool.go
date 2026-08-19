package projectinspect

import (
	"context"
	"encoding/json"
	"time"

	capabilityruntime "github.com/jacu-dev/jacu-harness/internal/runtime"
	"github.com/jacu-dev/jacu-harness/internal/telemetry"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const ToolName = "jacu_project_inspect"

type envelope[T any] struct {
	Status      string   `json:"status"`
	Summary     string   `json:"summary"`
	Data        T        `json:"data,omitempty"`
	Artifacts   []string `json:"artifacts"`
	Warnings    []string `json:"warnings"`
	NextActions []string `json:"next_actions"`
	TraceID     string   `json:"trace_id"`
}

func RegisterTool(server *mcp.Server, root string) {
	destructive := false
	openWorld := false
	capability := capabilityruntime.Capability{
		ProjectID: telemetry.ProjectID(root),
		Spec: capabilityruntime.ToolSpec{
			Name:           ToolName,
			Version:        "1",
			Risk:           capabilityruntime.RiskSafe,
			ReadOnly:       true,
			Idempotent:     true,
			OpenWorld:      false,
			Timeout:        10 * time.Second,
			MaxInputBytes:  256 * 1024,
			MaxOutputBytes: 16 * 1024,
		},
		Handler: inspectHandler(root),
	}

	mcp.AddTool(server, &mcp.Tool{
		Name:        ToolName,
		Description: "Inspect.",
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:    true,
			DestructiveHint: &destructive,
			IdempotentHint:  true,
			OpenWorldHint:   &openWorld,
		},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input Input) (*mcp.CallToolResult, envelope[Summary], error) {
		rawInput, err := json.Marshal(input)
		if err != nil {
			return nil, envelope[Summary]{}, err
		}
		result := capabilityruntime.Execute(ctx, capability, rawInput)
		data, _ := result.Data.(Summary)
		return nil, envelope[Summary]{
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

func inspectHandler(root string) capabilityruntime.Handler {
	return capabilityruntime.RequireWorkTree(root, func(ctx context.Context, rawInput json.RawMessage) (capabilityruntime.Result, error) {
		var input Input
		if unmarshalErr := json.Unmarshal(rawInput, &input); unmarshalErr != nil {
			return capabilityruntime.Result{}, unmarshalErr
		}
		summary, warnings, err := Scan(ctx, root, input)
		if err != nil {
			return capabilityruntime.Result{}, err
		}
		status := "ok"
		if summary.Truncated {
			status = "partial"
		}
		return capabilityruntime.Result{
			Status:      status,
			Summary:     "Project inspection completed.",
			Data:        summary,
			Artifacts:   []string{},
			Warnings:    warnings,
			NextActions: []string{},
		}, nil
	})
}
