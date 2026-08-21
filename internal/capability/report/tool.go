package report

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const ToolName = "jacu_report"

type Input struct{}

type Data struct {
	Report   ReportInfo `json:"report"`
	Markdown string     `json:"markdown"`
	Digest   string     `json:"digest"`
}

// ReportInfo keeps the MCP catalogue compact. The complete typed report is
// represented by deterministic Markdown plus its digest; these fields let a
// host inspect the projection without duplicating all eight block schemas in
// tools/list.
type ReportInfo struct {
	SchemaVersion string `json:"schema_version"`
	Kind          string `json:"kind"`
	RunCount      int    `json:"run_count"`
	ActiveRuns    int    `json:"active_runs"`
	ProgramCount  int    `json:"program_count"`
}

type envelope struct {
	Status      string   `json:"status"`
	Summary     string   `json:"summary"`
	Data        Data     `json:"data"`
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
		Description: "Show report.",
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:    true,
			DestructiveHint: &destructive,
			IdempotentHint:  true,
			OpenWorldHint:   &openWorld,
		},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input Input) (*mcp.CallToolResult, envelope, error) {
		result := Run(ctx, root, input)
		data, _ := result.Data.(Data)
		return nil, envelope{
			Status: result.Status, Summary: result.Summary, Data: data,
			Artifacts: nonNilStrings(result.Artifacts), Warnings: nonNilStrings(result.Warnings),
			NextActions: nonNilStrings(result.NextActions), TraceID: result.TraceID,
		}, nil
	})
}

func nonNilStrings(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}
