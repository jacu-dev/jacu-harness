package missioncompile

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

func normalizeProgram(program *ProgramInput) *ProgramInput {
	if program == nil {
		return nil
	}
	normalized := &ProgramInput{
		OpenQuestions: normalizeStrings(program.OpenQuestions),
		Missions:      make([]ProgramMissionInput, len(program.Missions)),
	}
	for index, mission := range program.Missions {
		normalized.Missions[index] = ProgramMissionInput{
			Objective:            strings.TrimSpace(mission.Objective),
			Context:              mission.Context,
			AcceptanceCriteria:   normalizeStrings(mission.AcceptanceCriteria),
			VerificationCommands: append([][]string{}, mission.VerificationCommands...),
			AllowedPaths:         normalizeStrings(mission.AllowedPaths),
			ForbiddenPaths:       normalizeStrings(mission.ForbiddenPaths),
			RiskHint:             strings.TrimSpace(mission.RiskHint),
			After:                normalizeInts(mission.After),
		}
	}
	return normalized
}

func normalizeInts(values []int) []int {
	if values == nil {
		return nil
	}
	seen := make(map[int]struct{}, len(values))
	for _, value := range values {
		seen[value] = struct{}{}
	}
	result := make([]int, 0, len(seen))
	for value := range seen {
		result = append(result, value)
	}
	sort.Ints(result)
	return result
}

func programID(program *ProgramInput) string {
	encoded, _ := json.Marshal(normalizeProgram(program))
	digest := sha256.Sum256(encoded)
	return "prg_" + fmt.Sprintf("%x", digest[:8])
}

func compileProgram(root string, program *ProgramInput) (*Program, []Lint) {
	normalized := normalizeProgram(program)
	lints := []Lint{}
	if normalized == nil {
		return nil, lints
	}
	if len(normalized.OpenQuestions) > 0 {
		lints = append(lints, Lint{Level: "BLOCK", Rule: "program_open_questions", Message: "program has open questions; resolve them before compile", Field: "program.open_questions"})
	}
	for index, item := range normalized.Missions {
		missionInput := Input{
			Objective:            item.Objective,
			Context:              item.Context,
			AcceptanceCriteria:   item.AcceptanceCriteria,
			VerificationCommands: item.VerificationCommands,
			AllowedPaths:         item.AllowedPaths,
			ForbiddenPaths:       item.ForbiddenPaths,
			RiskHint:             item.RiskHint,
		}
		_, status, _ := Compile(root, missionInput)
		if status == "blocked" {
			lints = append(lints, Lint{Level: "BLOCK", Rule: "program_mission_invalid", Message: fmt.Sprintf("nested mission %d is invalid", index), Field: fmt.Sprintf("program.missions[%d]", index)})
		}
		for _, dependency := range item.After {
			if dependency < 0 || dependency >= len(normalized.Missions) || dependency == index {
				lints = append(lints, Lint{Level: "BLOCK", Rule: "program_dependency_cycle", Message: fmt.Sprintf("mission %d has invalid after dependency %d", index, dependency), Field: fmt.Sprintf("program.missions[%d].after", index)})
			}
		}
	}
	if !hasProgramCycle(normalized.Missions) {
		return &Program{ProgramID: programID(normalized), OpenQuestions: append([]string{}, normalized.OpenQuestions...)}, lints
	}
	lints = append(lints, Lint{Level: "BLOCK", Rule: "program_dependency_cycle", Message: "program mission dependencies contain a cycle", Field: "program.missions"})
	return &Program{ProgramID: programID(normalized), OpenQuestions: append([]string{}, normalized.OpenQuestions...)}, lints
}

func hasProgramCycle(missions []ProgramMissionInput) bool {
	state := make([]uint8, len(missions))
	var visit func(int) bool
	visit = func(index int) bool {
		if state[index] == 1 {
			return true
		}
		if state[index] == 2 {
			return false
		}
		state[index] = 1
		for _, dependency := range missions[index].After {
			if dependency >= 0 && dependency < len(missions) && visit(dependency) {
				return true
			}
		}
		state[index] = 2
		return false
	}
	for index := range missions {
		if visit(index) {
			return true
		}
	}
	return false
}
