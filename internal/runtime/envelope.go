package runtime

import (
	"context"
	"encoding/json"
)

// MarshalEnvelope encodes a capability result with the same keys the MCP
// wrappers emit. Empty slices stay arrays; trace_id is left as Execute set it.
func MarshalEnvelope(result Result) ([]byte, error) {
	if result.Artifacts == nil {
		result.Artifacts = []string{}
	}
	if result.Warnings == nil {
		result.Warnings = []string{}
	}
	if result.NextActions == nil {
		result.NextActions = []string{}
	}
	return json.Marshal(struct {
		Status      string   `json:"status"`
		Summary     string   `json:"summary"`
		Data        any      `json:"data"`
		Artifacts   []string `json:"artifacts"`
		Warnings    []string `json:"warnings"`
		NextActions []string `json:"next_actions"`
		TraceID     string   `json:"trace_id"`
	}{
		Status:      result.Status,
		Summary:     result.Summary,
		Data:        result.Data,
		Artifacts:   result.Artifacts,
		Warnings:    result.Warnings,
		NextActions: result.NextActions,
		TraceID:     result.TraceID,
	})
}

// ExitCode is the CLI contract for a capability envelope: 0 for a completed
// surface (ok/accepted/partial), 1 for blocked/failed, 2 is reserved for usage.
func ExitCode(status string) int {
	switch status {
	case "ok", "accepted", "partial":
		return 0
	default:
		return 1
	}
}

// ExecuteInput marshals a typed input and runs Execute.
func ExecuteInput(ctx context.Context, c Capability, input any) Result {
	raw, err := json.Marshal(input)
	if err != nil {
		return Result{
			Status:      "failed",
			Summary:     "capability execution failed",
			Artifacts:   []string{},
			Warnings:    []string{},
			NextActions: []string{},
		}
	}
	return Execute(ctx, c, raw)
}
