package missioncompile

import (
	"context"
	"encoding/json"
	"time"

	capabilityruntime "github.com/jacu-dev/jacu-harness/internal/runtime"
	"github.com/jacu-dev/jacu-harness/internal/telemetry"
)

func Run(ctx context.Context, root string, in Input) capabilityruntime.Result {
	return capabilityruntime.ExecuteInput(ctx, compileCapability(root), in)
}

func compileCapability(root string) capabilityruntime.Capability {
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
		Handler: capabilityruntime.RequireWorkTree(root, compileHandler(root)),
	}
}

func compileHandler(root string) capabilityruntime.Handler {
	return func(_ context.Context, rawInput json.RawMessage) (capabilityruntime.Result, error) {
		var input Input
		if err := json.Unmarshal(rawInput, &input); err != nil {
			return capabilityruntime.Result{}, err
		}
		mission, status, nextActions := Compile(root, input)
		emitPreflightTelemetry(root, input, status, mission)
		summary := "Mission compiled."
		if status == "blocked" {
			summary = "Mission compilation blocked."
		} else if mission.Ceremony == "direct" {
			summary = "No mission required; host answer suffices."
		}
		return capabilityruntime.Result{
			Status:      status,
			Summary:     summary,
			Data:        mission,
			Artifacts:   []string{},
			Warnings:    []string{},
			NextActions: nextActions,
		}, nil
	}
}
