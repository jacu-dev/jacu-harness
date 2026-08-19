package preflight

import (
	"testing"

	"github.com/jacu-dev/jacu-harness/internal/runstate"
)

func TestOpenQuestionsBlockBeforeDispatch(t *testing.T) {
	mission := runstate.MissionSnapshot{Program: &runstate.Program{OpenQuestions: []string{"which credential?"}}}
	findings := questionFindings(mission)
	if len(findings) != 1 || findings[0].Class != ClassOpenQuestions {
		t.Fatalf("open question did not produce one typed finding: %+v", findings)
	}
	if got := questionFindings(runstate.MissionSnapshot{}); len(got) != 0 {
		t.Fatalf("empty questions blocked: %+v", got)
	}
}
