package runtime

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jacu-dev/jacu-harness/internal/telemetry"
)

func okSpec() ToolSpec {
	return ToolSpec{Name: "jacu_test", Version: "1", Risk: RiskSafe, ReadOnly: true,
		Idempotent: true, Timeout: 200 * time.Millisecond,
		MaxInputBytes: 1024, MaxOutputBytes: 512}
}

func TestExecuteSetsTraceID(t *testing.T) {
	t.Setenv("JACU_HOME", t.TempDir())
	c := Capability{Spec: okSpec(), Handler: func(ctx context.Context, in json.RawMessage) (Result, error) {
		return Result{Status: "ok", Summary: "x"}, nil
	}}
	r := Execute(context.Background(), c, json.RawMessage(`{}`))
	if !strings.HasPrefix(r.TraceID, "tr_") || len(r.TraceID) < 10 {
		t.Fatalf("trace_id inválido: %q", r.TraceID)
	}
}

func TestExecuteRejectsOversizedInput(t *testing.T) {
	t.Setenv("JACU_HOME", t.TempDir())
	c := Capability{Spec: okSpec(), Handler: func(ctx context.Context, in json.RawMessage) (Result, error) {
		t.Fatal("handler não pode rodar com input acima do cap")
		return Result{}, nil
	}}
	big := json.RawMessage(`{"x":"` + strings.Repeat("a", 2048) + `"}`)
	r := Execute(context.Background(), c, big)
	if r.Status != "blocked" {
		t.Fatalf("status = %q; want blocked", r.Status)
	}
}

func TestExecuteEnforcesTimeout(t *testing.T) {
	t.Setenv("JACU_HOME", t.TempDir())
	c := Capability{Spec: okSpec(), Handler: func(ctx context.Context, in json.RawMessage) (Result, error) {
		<-ctx.Done()
		return Result{}, ctx.Err()
	}}
	start := time.Now()
	r := Execute(context.Background(), c, json.RawMessage(`{}`))
	if time.Since(start) > time.Second {
		t.Fatal("timeout do spec não foi aplicado")
	}
	if r.Status != "failed" {
		t.Fatalf("status = %q; want failed", r.Status)
	}
}

func TestExecuteCapsOutputAsPartial(t *testing.T) {
	t.Setenv("JACU_HOME", t.TempDir())
	c := Capability{Spec: okSpec(), Handler: func(ctx context.Context, in json.RawMessage) (Result, error) {
		return Result{Status: "ok", Summary: "big", Data: strings.Repeat("z", 4096)}, nil
	}}
	r := Execute(context.Background(), c, json.RawMessage(`{}`))
	if r.Status != "partial" || !strings.Contains(strings.Join(r.Warnings, "\n"), "output exceeded inline limit; data reset to empty") {
		t.Fatalf("output acima do cap deve virar partial+warning; veio %+v", r)
	}
	b, _ := json.Marshal(r)
	if int64(len(b)) > 2*okSpec().MaxOutputBytes {
		t.Fatalf("resultado truncado ainda excede o cap: %d bytes", len(b))
	}
}

func TestExecuteRecoversPanic(t *testing.T) {
	t.Setenv("JACU_HOME", t.TempDir())
	c := Capability{Spec: okSpec(), Handler: func(ctx context.Context, in json.RawMessage) (Result, error) {
		panic("boom")
	}}
	r := Execute(context.Background(), c, json.RawMessage(`{}`))
	if r.Status != "failed" {
		t.Fatalf("panic deve virar failed, veio %q", r.Status)
	}
}

func TestExecuteEmitsOneSanitizedTelemetryEvent(t *testing.T) {
	base := t.TempDir()
	t.Setenv("JACU_HOME", base)
	c := Capability{ProjectID: "prj_0123456789abcdef", Spec: okSpec(), Handler: func(ctx context.Context, in json.RawMessage) (Result, error) {
		return Result{Status: "ok", Summary: "x"}, nil
	}}
	result := Execute(context.Background(), c, json.RawMessage(`{"objective":"secret prompt"}`))
	if result.Status != "ok" {
		t.Fatalf("result status = %q; want ok", result.Status)
	}
	events, err := telemetry.NewStoreAt(base).ReadSince(time.Unix(0, 0).UTC())
	if err != nil {
		t.Fatalf("read telemetry: %v", err)
	}
	if len(events) != 1 || events[0].Event != "tool_call" || events[0].Tool != "jacu_test" || events[0].Status != "ok" {
		t.Fatalf("telemetry events = %+v; want one closed tool_call", events)
	}
	if events[0].InputBytes != int64(len(`{"objective":"secret prompt"}`)) || events[0].OutputBytes <= 0 || events[0].Measurement != "exact_bytes" || events[0].Capped || events[0].DegradedPartial {
		t.Fatalf("tool-call measurement = %+v; want exact input/output bytes without degradation", events[0])
	}
	encoded, _ := json.Marshal(events[0])
	if strings.Contains(string(encoded), "secret") || strings.Contains(string(encoded), "objective") {
		t.Fatalf("telemetry leaked input content: %s", encoded)
	}
}

func TestExecuteTelemetryFailureDoesNotChangeOperation(t *testing.T) {
	file, err := os.CreateTemp(t.TempDir(), "not-a-directory")
	if err != nil {
		t.Fatalf("create telemetry failure fixture: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close telemetry failure fixture: %v", err)
	}
	t.Setenv("JACU_HOME", file.Name())
	c := Capability{ProjectID: "prj_0123456789abcdef", Spec: okSpec(), Handler: func(ctx context.Context, in json.RawMessage) (Result, error) {
		return Result{Status: "ok", Summary: "preserved"}, nil
	}}
	result := Execute(context.Background(), c, json.RawMessage(`{}`))
	if result.Status != "ok" || result.Summary != "preserved" {
		t.Fatalf("telemetry failure changed operation result: %+v", result)
	}
}
