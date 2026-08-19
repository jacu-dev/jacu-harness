package workspace

import (
	"context"
	"testing"

	"github.com/jacu-dev/jacu-harness/internal/runner"
)

func TestExecuteProgramContinuesIndependentMissionAfterEscalation(t *testing.T) {
	missions := []ProgramMission{
		{Index: 0},
		{Index: 1, After: []int{0}},
		{Index: 2},
	}
	called := []int{}
	states, err := ExecuteProgram(context.Background(), missions, func(index int) (MissionOutcome, error) {
		called = append(called, index)
		if index == 1 {
			return MissionOutcome{Status: MissionEscalated, Verdict: "blocked", Warnings: []string{"conflict"}}, nil
		}
		return MissionOutcome{Status: MissionApplied, Verdict: "pass", DiffDigest: "sha256:diff", EvidenceDigest: "sha256:evidence"}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(called) != 3 || called[2] != 2 {
		t.Fatalf("called = %v; want independent mission 2 after escalation", called)
	}
	if states[1].Status != MissionEscalated || states[2].Status != MissionApplied || states[2].Audit.EvidenceDigest == "" {
		t.Fatalf("states = %#v", states)
	}
}

func TestRemediationCycleFlakyRerunsOnceAndEscalatesTwice(t *testing.T) {
	if got := ClassifyCheckFailure("test", "exit 1"); got != CheckTest {
		t.Fatalf("classify test = %q", got)
	}
	if decision := NextRemediation("lint", CheckFlaky, 0); decision.Action != RemediationRerun {
		t.Fatalf("flaky first decision = %#v", decision)
	}
	if decision := NextRemediation("lint", CheckFlaky, 1); decision.Action != RemediationMission {
		t.Fatalf("flaky second decision = %#v", decision)
	}
	if decision := NextRemediation("lint", CheckLint, 2); decision.Action != RemediationEscalate {
		t.Fatalf("same check second failure = %#v", decision)
	}
}

func TestBuildRemediationPlanDerivesScopedMissionAndOwnBudget(t *testing.T) {
	plan := BuildRemediationPlan(runner.CheckFailureEvidence{
		Check:          runner.CheckRun{Name: "lint", State: "FAILURE", Workflow: "CI"},
		EvidenceDigest: "sha256:evidence",
		Annotations:    []runner.CheckAnnotation{{Path: "internal/runner/ci.go"}},
	}, 0)
	if plan.Decision.Action != RemediationMission || plan.Mission == nil {
		t.Fatalf("plan = %#v; want remediation mission", plan)
	}
	if plan.Mission.Objective != "make check lint green" || plan.Mission.BudgetIterations != 1 ||
		plan.Mission.EvidenceDigest != "sha256:evidence" || len(plan.Mission.AllowedPaths) != 1 {
		t.Fatalf("mission = %#v", plan.Mission)
	}
}

func TestBuildRemediationPlanFailsClosedOnUnsafeScopeAndRepeatedFailure(t *testing.T) {
	unsafe := BuildRemediationPlan(runner.CheckFailureEvidence{
		Check:       runner.CheckRun{Name: "test", State: "FAILURE"},
		Annotations: []runner.CheckAnnotation{{Path: "../outside"}, {Path: "/tmp/outside"}},
	}, 0)
	if unsafe.Decision.Action != RemediationEscalate || unsafe.Mission != nil {
		t.Fatalf("unsafe plan = %#v; want escalation", unsafe)
	}
	repeated := BuildRemediationPlan(runner.CheckFailureEvidence{
		Check:       runner.CheckRun{Name: "lint", State: "FAILURE"},
		Annotations: []runner.CheckAnnotation{{Path: "internal/runner/ci.go"}},
	}, 2)
	if repeated.Decision.Action != RemediationEscalate || repeated.Mission != nil {
		t.Fatalf("repeated plan = %#v; want escalation", repeated)
	}
}
