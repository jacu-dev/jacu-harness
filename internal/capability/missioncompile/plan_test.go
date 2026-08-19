package missioncompile

import "testing"

func TestCompilePlanBlocksOpenDecisions(t *testing.T) {
	mission, status, _ := Compile(t.TempDir(), Input{
		Objective: "Implement the provider runner safely",
		Program: &ProgramInput{
			OpenQuestions: []string{"provider"},
			Missions:      []ProgramMissionInput{{Objective: "Implement the provider runner safely"}},
		},
	})
	if status != "blocked" {
		t.Fatalf("status = %q; want blocked", status)
	}
	if mission.MissionID != "" {
		t.Fatalf("blocked mission id = %q; want empty", mission.MissionID)
	}
	if mission.Program != nil {
		t.Fatalf("blocked mission program = %#v; want no executable program", mission.Program)
	}
	if !hasLintRule(mission, "program_open_questions") {
		t.Fatalf("lint = %#v; want program_open_questions", mission.Lint)
	}
}

func TestCompilePlanReadyWhenAllDecisionsAreClosed(t *testing.T) {
	mission, status, _ := Compile(t.TempDir(), Input{
		Objective: "Implement the provider runner safely",
		Program: &ProgramInput{
			OpenQuestions: []string{},
			Missions:      []ProgramMissionInput{{Objective: "Implement the provider runner safely"}},
		},
	})
	if status != "ok" || mission.MissionID == "" {
		t.Fatalf("mission = %#v status = %q; want executable plan", mission, status)
	}
	if mission.Program == nil || !PlanReady(mission.Program) {
		t.Fatalf("program = %#v; want ready with no open decisions", mission.Program)
	}
}

func TestCompilePlanKeepsOpenQuestionGateFailClosed(t *testing.T) {
	mission, status, _ := Compile(t.TempDir(), Input{
		Objective: "Implement the provider runner safely",
		Program: &ProgramInput{
			OpenQuestions: []string{"", "provider"},
			Missions:      []ProgramMissionInput{{Objective: "Implement the provider runner safely"}},
		},
	})
	if status != "blocked" || mission.MissionID != "" {
		t.Fatalf("mission = %#v status = %q; want blocked without identity", mission, status)
	}
	if PlanReady(nil) == false || PlanReady(&Program{OpenQuestions: []string{"provider"}}) {
		t.Fatal("plan gate does not fail closed for an open decision")
	}
	if !hasLintRule(mission, "program_open_questions") {
		t.Fatalf("lint = %#v; want program_open_questions", mission.Lint)
	}
}

func hasLintRule(mission Mission, rule string) bool {
	for _, item := range mission.Lint {
		if item.Rule == rule && item.Level == "BLOCK" {
			return true
		}
	}
	return false
}
