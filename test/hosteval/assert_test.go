//go:build hosteval

package hosteval

import "testing"

func TestExpectations(t *testing.T) {
	seq := []string{"jacu_mission_compile", "jacu_workspace_open", "jacu_verify", "jacu_diff"}

	cases := []struct {
		name    string
		exp     Expectation
		tools   []string
		wantErr bool
	}{
		{"contains hit", Contains("jacu_verify"), seq, false},
		{"contains miss", Contains("jacu_memory_save"), seq, true},
		{"not contains ok", NotContains("jacu_memory_save"), seq, false},
		{"not contains violated", NotContains("jacu_diff"), seq, true},
		{"before ok", Before("jacu_verify", "jacu_diff"), seq, false},
		{"before violated", Before("jacu_diff", "jacu_verify"), seq, true},
		{"before with absent first", Before("jacu_report", "jacu_diff"), seq, true},
		{"before with absent second", Before("jacu_verify", "jacu_report"), seq, true},
		{"no tools on empty", NoJacuTools(), nil, false},
		{"no tools violated", NoJacuTools(), seq, true},
		{"contains fails on empty", Contains("jacu_verify"), nil, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.exp.Check(tc.tools)
			if (err != nil) != tc.wantErr {
				t.Fatalf("%s: err = %v, wantErr = %v", tc.exp, err, tc.wantErr)
			}
		})
	}
}

// A skipped step must not read as correct. This is the 4.2 failure mode named
// in sr-skills-refactor.md: routing straight to the workspace without compiling
// a mission. Before() must reject it rather than pass vacuously.
func TestBeforeRejectsSkippedStepInsteadOfPassingVacuously(t *testing.T) {
	skippedMission := []string{"jacu_workspace_open", "jacu_diff"}
	if err := Before("jacu_mission_compile", "jacu_workspace_open").Check(skippedMission); err == nil {
		t.Fatal("Before passed while the first tool was never called; a skipped step must fail")
	}
}

func TestOnlyNegativeDistinguishesPositiveClaims(t *testing.T) {
	if !onlyNegative([]Expectation{NoJacuTools()}) {
		t.Fatal("NoJacuTools alone is a negative-only case")
	}
	if !onlyNegative([]Expectation{NotContains("jacu_apply")}) {
		t.Fatal("NotContains alone is a negative-only case")
	}
	if onlyNegative([]Expectation{NotContains("jacu_apply"), Contains("jacu_status")}) {
		t.Fatal("a case with a positive claim is not negative-only")
	}
	if onlyNegative(nil) {
		t.Fatal("a case with no expectations asserts nothing and must not be negative-only")
	}
}
