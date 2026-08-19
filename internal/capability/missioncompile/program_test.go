package missioncompile

import "testing"

func TestCompileProgramValidatesQueue(t *testing.T) {
	tests := []struct {
		name      string
		program   *ProgramInput
		wantState string
		wantLint  string
	}{
		{
			name: "valid ordered missions",
			program: &ProgramInput{OpenQuestions: []string{}, Missions: []ProgramMissionInput{
				{Objective: "fix first", AllowedPaths: []string{"a.txt"}},
				{Objective: "fix second", AllowedPaths: []string{"b.txt"}, After: []int{0}},
			}},
			wantState: "ok",
		},
		{
			name:      "open question",
			program:   &ProgramInput{OpenQuestions: []string{"which API?"}, Missions: []ProgramMissionInput{{Objective: "fix first", AllowedPaths: []string{"a.txt"}}}},
			wantState: "blocked", wantLint: "program_open_questions",
		},
		{
			name:      "invalid nested mission",
			program:   &ProgramInput{OpenQuestions: []string{}, Missions: []ProgramMissionInput{{Objective: "", AllowedPaths: []string{"a.txt"}}}},
			wantState: "blocked", wantLint: "program_mission_invalid",
		},
		{
			name: "cycle",
			program: &ProgramInput{OpenQuestions: []string{}, Missions: []ProgramMissionInput{
				{Objective: "fix first", AllowedPaths: []string{"a.txt"}, After: []int{1}},
				{Objective: "fix second", AllowedPaths: []string{"b.txt"}, After: []int{0}},
			}},
			wantState: "blocked", wantLint: "program_dependency_cycle",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mission, status, _ := compileWithDeclaredPaths(t, Input{
				Objective:    "run program",
				AllowedPaths: []string{"program.txt"},
				Program:      tt.program,
			})
			if status != tt.wantState {
				t.Fatalf("Compile status = %q, want %q; mission = %#v", status, tt.wantState, mission)
			}
			if tt.wantState == "ok" {
				if mission.Program == nil || mission.Program.ProgramID == "" || len(mission.Program.MissionIDs) != 2 {
					t.Fatalf("program metadata = %#v; want deterministic ids for both missions", mission.Program)
				}
				return
			}
			found := false
			for _, lint := range mission.Lint {
				if lint.Rule == tt.wantLint {
					found = true
				}
			}
			if !found {
				t.Fatalf("lints = %#v; want %q", mission.Lint, tt.wantLint)
			}
		})
	}
}
