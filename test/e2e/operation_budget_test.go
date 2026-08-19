//go:build e2e

package e2e

import (
	"fmt"
	"sort"
	"testing"
	"time"
)

const operationBudgetCeiling = time.Second

// TestOperationBudget records real request latency on the built binary. These
// are the inexpensive operations available before task runtime exists, so the
// numbers form the baseline that the phase 08 entry report can compare with.
func TestOperationBudget(t *testing.T) {
	project := newProjectRepo(t)
	s := startSession(t, project)
	s.useSessionlessProtocol("jacu-e2e-operation-budget")

	mission := map[string]any{
		"objective":             "Measure the operation baseline",
		"acceptance_criteria":   []string{"the baseline is recorded"},
		"verification_commands": []any{},
		"allowed_paths":         []string{"README.md"},
		"risk_hint":             "write",
	}
	for _, operation := range []struct {
		name string
		args map[string]any
	}{
		{name: "project_inspect", args: map[string]any{}},
		{name: "mission_compile", args: mission},
		{name: "status", args: map[string]any{}},
		{name: "report", args: map[string]any{}},
	} {
		samples := make([]time.Duration, 0, 20)
		for i := 0; i < 20; i++ {
			start := time.Now()
			result := s.callTool("jacu_"+operation.name, operation.args)
			if result.Status != "ok" {
				t.Fatalf("jacu_%s status = %q (%s)", operation.name, result.Status, result.Summary)
			}
			samples = append(samples, time.Since(start))
		}
		sort.Slice(samples, func(i, j int) bool { return samples[i] < samples[j] })
		p50 := samples[len(samples)/2]
		p95 := samples[(len(samples)*95)/100]
		report(t, fmt.Sprintf("operation %s: p50 %v, p95 %v (ceiling %v, n=%d)",
			operation.name, p50.Round(time.Millisecond), p95.Round(time.Millisecond), operationBudgetCeiling, len(samples)))
		if p95 > operationBudgetCeiling {
			t.Fatalf("jacu_%s p95 = %v; ceiling is %v", operation.name, p95, operationBudgetCeiling)
		}
	}
}
