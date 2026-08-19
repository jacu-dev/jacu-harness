package preflight

import "github.com/jacu-dev/jacu-harness/internal/runstate"

func questionFindings(mission runstate.MissionSnapshot) []Finding {
	if mission.Program == nil || len(mission.Program.OpenQuestions) == 0 {
		return []Finding{}
	}
	return []Finding{{Class: ClassOpenQuestions, Target: "program.open_questions", Detail: "question requires a human answer"}}
}
