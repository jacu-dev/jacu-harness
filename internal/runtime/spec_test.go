package runtime

import (
	"testing"
	"time"
)

func TestToolSpecValidateRejectsInvalid(t *testing.T) {
	cases := []struct {
		name string
		spec ToolSpec
	}{
		{"empty name", ToolSpec{Risk: RiskSafe, Timeout: time.Second}},
		{"bad name chars", ToolSpec{Name: "Jacu-Inspect!", Risk: RiskSafe, Timeout: time.Second}},
		{"no jacu_ prefix", ToolSpec{Name: "project_inspect", Risk: RiskSafe, Timeout: time.Second}},
		{"zero timeout", ToolSpec{Name: "jacu_x", Risk: RiskSafe}},
		{"unknown risk", ToolSpec{Name: "jacu_x", Risk: "scary", Timeout: time.Second}},
		{"safe but not readonly", ToolSpec{Name: "jacu_x", Risk: RiskSafe, ReadOnly: false, Timeout: time.Second}},
	}
	for _, c := range cases {
		if err := c.spec.Validate(); err == nil {
			t.Errorf("%s: Validate() aceitou spec inválida", c.name)
		}
	}
	ok := ToolSpec{Name: "jacu_project_inspect", Version: "1", Risk: RiskSafe,
		ReadOnly: true, Idempotent: true, Timeout: 10 * time.Second,
		MaxInputBytes: 262144, MaxOutputBytes: 16384}
	if err := ok.Validate(); err != nil {
		t.Fatalf("spec válida rejeitada: %v", err)
	}
}
