package runtime

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"time"

	"github.com/jacu-dev/jacu-harness/internal/telemetry"
)

type liveEventsKey struct{}

// WithLiveEvents tees the telemetry v2 envelope as NDJSON to w during Execute.
// jacu serve must not set this on stdout: stdout is JSON-RPC.
func WithLiveEvents(ctx context.Context, w io.Writer) context.Context {
	if w == nil {
		return ctx
	}
	return context.WithValue(ctx, liveEventsKey{}, w)
}

// LiveEvents returns the writer attached by WithLiveEvents, if any.
func LiveEvents(ctx context.Context) io.Writer {
	w, _ := ctx.Value(liveEventsKey{}).(io.Writer)
	return w
}

type Handler func(ctx context.Context, input json.RawMessage) (Result, error)

type Capability struct {
	ProjectID string
	Spec      ToolSpec
	Handler   Handler
}

func Execute(ctx context.Context, c Capability, input json.RawMessage) (result Result) {
	started := time.Now()
	defer func() {
		if recover() != nil {
			result = Result{Status: "failed", Summary: "capability execution failed"}
		}
		if result.TraceID == "" {
			result.TraceID = newTraceID()
		}
		uncappedOutput, _ := json.Marshal(result)
		inputBytes := int64(len(input))
		outputBytes := int64(len(uncappedOutput))
		capped := c.Spec.MaxOutputBytes > 0 && outputBytes > c.Spec.MaxOutputBytes
		result = capOutput(result, c.Spec.MaxOutputBytes)
		output, _ := json.Marshal(result)
		fields := telemetryFields(result.Data)
		projectID := c.ProjectID
		if projectID == "" {
			projectID = "prj_unknown"
		}
		event := telemetry.EventInput{
			Timestamp: time.Now().UTC(), ProjectID: projectID, TraceID: result.TraceID,
			Event: telemetry.EventToolCall, Tool: c.Spec.Name, Status: result.Status,
			RunID: fields.runID, MissionID: fields.missionID, Ceremony: fields.ceremony,
			Risk: fields.risk, Verdict: fields.verdict, Duration: time.Since(started),
			Measurement: "exact_bytes", InputBytes: inputBytes, OutputBytes: outputBytes,
			Capped: capped, DegradedPartial: capped && result.Status == "partial",
		}
		telemetry.EmitBestEffortInputLive(LiveEvents(ctx), event)
		slog.Info("capability execution",
			"tool", c.Spec.Name,
			"trace_id", result.TraceID,
			"status", result.Status,
			"duration", time.Since(started),
			"bytes_in", len(input),
			"bytes_out", len(output),
		)
	}()

	if err := c.Spec.Validate(); err != nil {
		return Result{Status: "failed", Summary: "invalid tool specification"}
	}
	if int64(len(input)) > c.Spec.MaxInputBytes {
		return Result{Status: "blocked", Summary: "input exceeds tool limit"}
	}

	execCtx, cancel := context.WithTimeout(ctx, c.Spec.Timeout)
	defer cancel()

	result, err := c.Handler(execCtx, input)
	if err != nil {
		return Result{Status: "failed", Summary: "capability execution failed"}
	}
	return result
}

type telemetryFieldSet struct {
	runID, missionID, ceremony, risk, verdict string
}

func telemetryFields(data any) telemetryFieldSet {
	encoded, err := json.Marshal(data)
	if err != nil {
		return telemetryFieldSet{}
	}
	var fields map[string]any
	if err := json.Unmarshal(encoded, &fields); err != nil {
		return telemetryFieldSet{}
	}
	get := func(name string) string {
		value, _ := fields[name].(string)
		return value
	}
	return telemetryFieldSet{
		runID: get("run_id"), missionID: get("mission_id"), ceremony: get("ceremony"),
		risk: get("risk"), verdict: get("verdict"),
	}
}

func newTraceID() string {
	var id [8]byte
	if _, err := rand.Read(id[:]); err != nil {
		return "tr_unavailable"
	}
	return "tr_" + hex.EncodeToString(id[:])
}

func capOutput(result Result, maxBytes int64) Result {
	encoded, err := json.Marshal(result)
	if err != nil {
		return Result{
			Status:  "failed",
			Summary: "capability output is not serializable",
			TraceID: result.TraceID,
		}
	}
	if maxBytes <= 0 || int64(len(encoded)) <= maxBytes {
		return result
	}
	result.Status = "partial"
	result.Data = nil
	result.Warnings = append(result.Warnings, "output exceeded inline limit; data reset to empty")
	return result
}
