package projectinspect

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jacu-dev/jacu-harness/internal/gitx"
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
	return func(ctx context.Context, rawInput json.RawMessage) (capabilityruntime.Result, error) {
		git, err := gitx.New()
		if err != nil || !git.InsideWorkTree(ctx, root) {
			return capabilityruntime.Result{
				Status:  "blocked",
				Summary: fmt.Sprintf("cwd %q is not inside a git work tree; start jacu serve from a repository (or emit an anchored host pack: jacu doctor --emit claude-desktop --repo <repo>)", root),
				NextActions: []string{
					"cd into the repository and restart the MCP server",
					"or register a host pack that anchors cwd to the repository",
				},
			}, nil
		}
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
	}
}
