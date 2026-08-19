//go:build hosteval

package hosteval

import (
	"context"
	"errors"
	"time"
)

// Case is one prompt plus what its tool sequence must look like.
type Case struct {
	ID     string
	Prompt string
	Expect []Expectation
	// Timeout is per host invocation. Zero means DefaultTimeout.
	Timeout time.Duration
}

// DefaultTimeout bounds a single host invocation. A host that has not answered
// in five minutes is not thinking, it is stuck waiting on something
// interactive, and the harness must not hang the run.
const DefaultTimeout = 5 * time.Minute

// Verdict is the outcome of one case on one host.
type Verdict string

const (
	Pass    Verdict = "pass"
	Fail    Verdict = "fail"
	Skipped Verdict = "skipped"
)

// Result carries everything the report needs, including the observed sequence
// on failure. A failing case that does not print what it saw costs another
// full run to diagnose.
type Result struct {
	Case         string
	Host         string
	Verdict      Verdict
	Tools        []string
	Failures     []string
	Reason       string
	Truncated    bool
	SkippedLines int
}

// Runner executes cases against a host, judging by the telemetry delta.
type Runner struct {
	StreamDir string
	Workdir   string
	ProjectID string
}

// Run executes one case and returns its verdict.
func (r Runner) Run(ctx context.Context, h Host, c Case) Result {
	res := Result{Case: c.ID, Host: h.Name}

	if _, err := h.Probe(); err != nil {
		res.Verdict = Skipped
		res.Reason = err.Error()
		return res
	}

	before, err := Snapshot(r.StreamDir)
	if err != nil {
		res.Verdict = Skipped
		res.Reason = "cannot snapshot telemetry: " + err.Error()
		return res
	}

	timeout := c.Timeout
	if timeout == 0 {
		timeout = DefaultTimeout
	}

	out, runErr := h.Run(ctx, r.Workdir, c.Prompt, timeout)
	res.Truncated = TruncationWarning(out)

	if runErr != nil && errors.Is(runErr, ErrHostUnavailable) {
		res.Verdict = Skipped
		res.Reason = runErr.Error()
		return res
	}

	events, skipped, err := Delta(r.StreamDir, before, r.ProjectID)
	if err != nil {
		res.Verdict = Skipped
		res.Reason = "cannot read telemetry delta: " + err.Error()
		return res
	}
	res.SkippedLines = skipped
	res.Tools = Tools(events)

	// A host error is only fatal when the case expected jacu to do something.
	// "quanto é 2+2" answered by a CLI that then exits non-zero for an
	// unrelated reason still proves the routing claim.
	if runErr != nil && !onlyNegative(c.Expect) {
		res.Verdict = Fail
		res.Failures = append(res.Failures, runErr.Error())
		return res
	}

	for _, e := range c.Expect {
		if err := e.Check(res.Tools); err != nil {
			res.Failures = append(res.Failures, err.Error())
		}
	}
	if len(res.Failures) > 0 {
		res.Verdict = Fail
		return res
	}
	res.Verdict = Pass
	return res
}

func onlyNegative(exps []Expectation) bool {
	for _, e := range exps {
		switch e.(type) {
		case notContainsExp, emptyExp:
		default:
			return false
		}
	}
	return len(exps) > 0
}

// SRCases are Tasks 4.1-4.4 of the skills refactor, transcribed from
// docs/plano/sr-skills-refactor.md Task 4 and the routing evidence recorded in
// scripts/host-smoke/README.md on 2026-08-13.
//
// These assert on tools, not on which skill file the host loaded. That is a
// proxy, and it is stated here rather than buried: each route owns a disjoint
// tool set, so the tool sequence identifies the route — but a host that reached
// the right tool by reading files instead of loading the skill would pass. The
// manual matrix recorded the same evidence.
func SRCases() []Case {
	return []Case{
		{
			ID:     "4.1-inspect",
			Prompt: "o que faz esse projeto?",
			Expect: []Expectation{
				Contains("jacu_project_inspect"),
				NotContains("jacu_mission_compile"),
				NotContains("jacu_workspace_open"),
			},
		},
		{
			ID:     "4.2-mission-workspace",
			Prompt: "corrige o bug na função Soma e roda os testes",
			Expect: []Expectation{
				Contains("jacu_mission_compile"),
				Contains("jacu_workspace_open"),
				Before("jacu_mission_compile", "jacu_workspace_open"),
				Before("jacu_verify", "jacu_diff"),
			},
			Timeout: 10 * time.Minute,
		},
		{
			ID:     "4.3-memory",
			Prompt: "lembra que usamos go test puro nesse projeto, sem testify",
			Expect: []Expectation{
				Contains("jacu_memory_save"),
				NotContains("jacu_workspace_open"),
			},
		},
		{
			ID:     "4.4-no-tool",
			Prompt: "quanto é 2+2",
			Expect: []Expectation{NoJacuTools()},
		},
	}
}
