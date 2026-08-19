//go:build hosteval

package hosteval

import (
	"strings"
	"testing"
)

func TestUnavailableHostIsSkippedNeverPass(t *testing.T) {
	h := Host{Name: "ghost", Bin: "definitely-not-installed-jacu-test", Argv: func(p string) []string { return []string{p} }}
	r := Runner{StreamDir: t.TempDir(), Workdir: t.TempDir(), ProjectID: "prj_a"}
	res := r.Run(t.Context(), h, Case{ID: "4.4-no-tool", Expect: []Expectation{NoJacuTools()}})
	if res.Verdict != Skipped {
		t.Fatalf("verdict = %s, want skipped: an absent host proves nothing", res.Verdict)
	}
	if res.Reason == "" {
		t.Fatal("a skipped result must carry the reason")
	}
}

func TestTruncationWarningDetectsTheCodexFinding(t *testing.T) {
	out := "Skill descriptions were shortened to fit the skills context budget. Codex can still see every skill."
	if !TruncationWarning(out) {
		t.Fatal("must detect the 2026-08-13 Codex truncation warning")
	}
	if TruncationWarning("all good here") {
		t.Fatal("must not report truncation on unrelated output")
	}
}

func TestSRCasesCoverTheFourScenarios(t *testing.T) {
	got := SRCases()
	if len(got) != 4 {
		t.Fatalf("SRCases = %d, want the four scenarios of SR Task 4", len(got))
	}
	for _, c := range got {
		if c.Prompt == "" {
			t.Fatalf("case %s has no prompt", c.ID)
		}
		if len(c.Expect) == 0 {
			t.Fatalf("case %s asserts nothing", c.ID)
		}
	}
}

func TestReportShowsObservedSequenceOnFailure(t *testing.T) {
	out := Report([]Result{{
		Case: "4.1-inspect", Host: "codex", Verdict: Fail,
		Tools:     []string{"jacu_mission_compile"},
		Failures:  []string{"expected jacu_project_inspect to be called"},
		Truncated: true,
	}})
	for _, want := range []string{"4.1-inspect", "codex", "fail", "jacu_mission_compile", "truncamento"} {
		if !strings.Contains(out, want) {
			t.Fatalf("report missing %q:\n%s", want, out)
		}
	}
}
