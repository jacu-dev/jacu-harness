package workspace

import (
	"fmt"
	"strings"

	"github.com/jacu-dev/jacu-harness/internal/runstate"
)

func buildCommitMessage(run runstate.Run, hostName, verifiedSummary string) string {
	if hostName == "" {
		hostName = "unknown host"
	}
	var message strings.Builder
	message.WriteString(run.Mission.Objective)
	message.WriteString("\n\nAcceptance criteria:\n")
	for _, criterion := range run.Mission.AcceptanceCriteria {
		message.WriteString("- ")
		message.WriteString(criterion)
		message.WriteByte('\n')
	}
	message.WriteByte('\n')
	message.WriteString(verifiedSummary)
	message.WriteString("\n\n")
	fmt.Fprintf(&message, "Jacu-Run: %s\n", run.RunID)
	fmt.Fprintf(&message, "Jacu-Mission: %s\n", run.Mission.MissionID)
	fmt.Fprintf(&message, "Jacu-Base: %s\n", run.BaseSHA)
	fmt.Fprintf(&message, "Assisted-by: %s\n", hostName)
	return message.String()
}
