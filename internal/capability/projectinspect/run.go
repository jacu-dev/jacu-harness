package projectinspect

import (
	"context"
	"encoding/json"
	"time"

	capabilityruntime "github.com/jacu-dev/jacu-harness/internal/runtime"
	"github.com/jacu-dev/jacu-harness/internal/telemetry"
)

func Run(ctx context.Context, root string, in Input) capabilityruntime.Result {
	return capabilityruntime.ExecuteInput(ctx, inspectCapability(root), in)
}

func inspectCapability(root string) capabilityruntime.Capability {
	return capabilityruntime.Capability{
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
