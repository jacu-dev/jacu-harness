package missioncompile

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/jacu-dev/jacu-harness/internal/runstate"
)

type Input = runstate.MissionInput
type ProgramInput = runstate.ProgramInput
type ProgramMissionInput = runstate.ProgramMissionInput
type Program = runstate.Program
type Context = runstate.Context
type Lint = runstate.Lint
type Mission = runstate.MissionSnapshot

func normalize(in Input) Input {
	in.Objective = strings.TrimSpace(in.Objective)
	in.AcceptanceCriteria = normalizeStrings(in.AcceptanceCriteria)
	in.AllowedPaths = normalizeStrings(in.AllowedPaths)
	in.ForbiddenPaths = normalizeStrings(in.ForbiddenPaths)
	in.Program = normalizeProgram(in.Program)
	return in
}

func missionID(in Input) string {
	encoded, _ := json.Marshal(normalize(in))
	digest := sha256.Sum256(encoded)
	return "msn_" + fmt.Sprintf("%x", digest[:8])
}

func normalizeStrings(values []string) []string {
	if values == nil {
		return nil
	}
	unique := make(map[string]struct{}, len(values))
	for _, value := range values {
		unique[strings.TrimSpace(value)] = struct{}{}
	}
	result := make([]string, 0, len(unique))
	for value := range unique {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}
