package report

import (
	"context"
	"encoding/json"
	"time"

	headlessreport "github.com/jacu-dev/jacu-harness/internal/report"
	"github.com/jacu-dev/jacu-harness/internal/reportgen"
	capabilityruntime "github.com/jacu-dev/jacu-harness/internal/runtime"
	"github.com/jacu-dev/jacu-harness/internal/telemetry"
)

func Run(ctx context.Context, root string, in Input) capabilityruntime.Result {
	return capabilityruntime.ExecuteInput(ctx, reportCapability(root), in)
}

func reportCapability(root string) capabilityruntime.Capability {
	return capabilityruntime.Capability{
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
		Handler: capabilityruntime.RequireWorkTree(root, reportHandler(root)),
	}
}

func reportHandler(root string) capabilityruntime.Handler {
	return func(_ context.Context, _ json.RawMessage) (capabilityruntime.Result, error) {
		report, err := headlessreport.BuildAudit(root)
		if err != nil {
			return capabilityruntime.Result{}, err
		}
		markdown, err := reportgen.Markdown(report)
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
	}
}
