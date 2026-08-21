package clarity

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/jacu-dev/jacu-harness/internal/telemetry"
)

func TestProbeEventUsesClosedFieldsAndNoModelString(t *testing.T) {
	modelText := "the spec is ambiguous about worktrees and I think we should rewrite it in prose"
	report := Report{
		Verdict:         "fail",
		Round:           2,
		Divergences:     1,
		DivergenceField: FieldWriteScope,
		VarianceRuns:    3,
		SpecBytes:       512,
		SpecBytesDelta:  0,
		Findings:        []Error{{Code: CodeDivergence, Field: FieldWriteScope, Path: "cmd/secret.go"}},
	}
	event, err := ProbeEvent("prj_0123456789abcdef", report)
	if err != nil {
		t.Fatalf("ProbeEvent: %v", err)
	}
	if event.Event != telemetry.EventClarityProbe || event.Module != "clarity" || event.Stage != "probe" {
		t.Fatalf("event identity = %+v", event)
	}
	if event.Round != 2 || event.Divergences != 1 || event.DivergenceField != FieldWriteScope || event.VarianceRuns != 3 || event.SpecBytes != 512 {
		t.Fatalf("counts = %+v", event)
	}
	encoded, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(encoded), modelText) || strings.Contains(string(encoded), "cmd/secret.go") {
		t.Fatalf("model or path leaked into telemetry: %s", encoded)
	}
}

func TestProbeEventRejectsModelStringAsDivergenceField(t *testing.T) {
	report := Report{Verdict: "fail", DivergenceField: "the model said write_scope is confusing"}
	if _, err := ProbeEvent("prj_0123456789abcdef", report); err == nil {
		t.Fatal("model-controlled divergence_field was stored")
	}
}
