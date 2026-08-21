package projectinspect

import (
	"context"

	capabilityruntime "github.com/jacu-dev/jacu-harness/internal/runtime"
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
		return nil, toEnvelope[Summary](Run(ctx, root, input)), nil
	})
}

func toEnvelope[T any](result capabilityruntime.Result) envelope[T] {
	data, _ := result.Data.(T)
	return envelope[T]{
		Status:      result.Status,
		Summary:     result.Summary,
		Data:        data,
		Artifacts:   result.Artifacts,
		Warnings:    result.Warnings,
		NextActions: result.NextActions,
		TraceID:     result.TraceID,
	}
}
