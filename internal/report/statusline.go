package report

import (
	"fmt"

	"github.com/jacu-dev/jacu-harness/internal/runstate"
)

func Statusline(root string) (string, error) {
	runs, err := runstate.List(root)
	if err != nil {
		return "", err
	}
	for _, run := range runs {
		if run.Status != runstate.StatusOpen && run.Status != runstate.StatusReviewed {
			continue
		}
		runID := sanitizeText(run.RunID)
		program := sanitizeText(run.ProgramID)
		if program == "" {
			program = "none"
		}
		mission := sanitizeText(run.MissionID)
		if mission == "" {
			mission = "none"
		}
		return fmt.Sprintf("jacu statusline: active=1 run=%s mission=%s phase=%s program=%s cursor=%d model=not measured cost=not measured", runID, mission, run.Status, program, run.ProgramCursor), nil
	}
	return "jacu statusline: idle mission=none phase=idle program=none model=not measured cost=not measured", nil
}
