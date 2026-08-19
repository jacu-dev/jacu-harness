package report

import (
	"context"
	"encoding/json"
	"time"

	headlessreport "github.com/jacu-dev/jacu-harness/internal/report"
	capabilityruntime "github.com/jacu-dev/jacu-harness/internal/runtime"
	"github.com/jacu-dev/jacu-harness/internal/telemetry"
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
	capability := capabilityruntime.Capability{
		ProjectID: telemetry.ProjectID(root),
		Spec: capabilityruntime.ToolSpec{
			Name:           ToolName,
			Version:        "1",
			Risk:           capabilityruntime.RiskSafe,
			ReadOnly:       true,
			Idempotent:     true,
			OpenWorld:      false,
			Timeout:        30 * time.Second,
			MaxInputBytes:  16 * 1024,
			MaxOutputBytes: 32 * 1024,
		},
		Handler: func(_ context.Context, _ json.RawMessage) (capabilityruntime.Result, error) {
			report, err := headlessreport.BuildAudit(root)
			if err != nil {
				return capabilityruntime.Result{}, err
			}
			markdown, err := headlessreport.Markdown(report)
			if err != nil {
				return capabilityruntime.Result{}, err
			}
			digest, err := headlessreport.Digest(report)
			if err != nil {
				return capabilityruntime.Result{}, err
			}
			activeRuns := 0
			programs := map[string]struct{}{}
			for _, step := range report.Blocks.Steps {
				if step.Status == "open" || step.Status == "reviewed" {
					activeRuns++
				}
			}
			for _, node := range report.Blocks.Flow.Nodes {
				if node.Kind == "program" {
					programs[node.ID] = struct{}{}
				}
			}
			return capabilityruntime.Result{
				Status: "ok", Summary: "Headless audit report projected.",
				Data: Data{
					Report: ReportInfo{
						SchemaVersion: report.SchemaVersion, Kind: report.Kind,
						RunCount: len(report.Blocks.Steps), ActiveRuns: activeRuns,
						ProgramCount: len(programs),
					},
					Markdown: markdown, Digest: digest,
				},
				Artifacts: []string{}, Warnings: []string{}, NextActions: []string{},
			}, nil
		},
	}

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
		rawInput, err := json.Marshal(input)
		if err != nil {
			return nil, envelope{}, err
		}
		result := capabilityruntime.Execute(ctx, capability, rawInput)
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
