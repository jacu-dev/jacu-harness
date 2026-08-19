//go:build e2e

package e2e

import (
	"fmt"
	"sort"
	"testing"
	"time"
)

// PLANO.md declares "cold start p95 < 150ms" as a budget, with the note that it
// is a target to benchmark before freezing. Nothing ever measured it.
//
// This measures it on every run and reports the number, but only fails on
// coldStartCeiling — an order of magnitude above the target. A shared CI runner
// is noisy enough that gating on 150ms would buy flakes, not signal; gating on
// a second still catches the regression that matters (a server that suddenly
// scans the repository, resolves the network, or waits on a lock at startup).
const (
	coldStartTarget  = 150 * time.Millisecond
	coldStartCeiling = 1 * time.Second
	coldStartSamples = 20
)

// TestColdStartBudget measures the wall time from process spawn to the first
// usable answer — the latency a host pays every time it starts the server.
func TestColdStartBudget(t *testing.T) {
	project := newProjectRepo(t)

	// Warm up outside the measurement. The first session of a run pays for
	// building the binary and for faulting it into the page cache — costs a
	// host never pays, and which would otherwise land in the sample set as a
	// half-second outlier and dominate p95.
	warmup := startSession(t, project)
	warmup.useSessionlessProtocol("jacu-e2e-budget-warmup")
	warmup.toolNames()
	warmup.close()

	samples := make([]time.Duration, 0, coldStartSamples)
	for i := 0; i < coldStartSamples; i++ {
		start := time.Now()
		s := startSession(t, project)
		s.useSessionlessProtocol("jacu-e2e-budget")
		if names, _ := s.toolNames(); len(names) != len(expectedTools) {
			t.Fatalf("cold start %d listed %d tools; want %d", i, len(names), len(expectedTools))
		}
		samples = append(samples, time.Since(start))
		s.close()
	}

	sort.Slice(samples, func(i, j int) bool { return samples[i] < samples[j] })
	p50 := samples[len(samples)/2]
	p95 := samples[(len(samples)*95)/100]
	report(t, fmt.Sprintf("cold start: p50 %v, p95 %v (target %v, ceiling %v, n=%d)",
		p50.Round(time.Millisecond), p95.Round(time.Millisecond),
		coldStartTarget, coldStartCeiling, len(samples)))

	if p95 > coldStartCeiling {
		t.Fatalf("cold start p95 = %v; ceiling is %v", p95, coldStartCeiling)
	}
	if p95 > coldStartTarget {
		t.Logf("cold start p95 %v is over the %v target in PLANO.md; under the ceiling, so not a failure",
			p95.Round(time.Millisecond), coldStartTarget)
	}
}

// report logs a measured number in the shape scripts/e2e.sh looks for when it
// builds the Actions job summary. The test only writes to its own log: teaching
// it to append to a file named by an environment variable would hand a test a
// filesystem write it has no business doing, and gosec is right to call that
// tainted. Formatting stays here, plumbing stays in the script.
func report(t *testing.T, line string) {
	t.Helper()
	t.Log(line)
}
