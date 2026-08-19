package orchestration

import (
	"sort"
	"strings"
)

// Opinion is structured untrusted review data. It never creates an apply
// receipt; it is only rendered into the flow trace.
type Opinion struct {
	SessionID string `json:"session_id"`
	Model     string `json:"model"`
	Verdict   string `json:"verdict"`
	Reason    string `json:"reason,omitempty"`
}

func RenderPanelMarkdown(opinions []Opinion) string {
	unique := make(map[string]Opinion, len(opinions))
	order := make([]string, 0, len(opinions))
	for _, opinion := range opinions {
		if strings.TrimSpace(opinion.SessionID) == "" {
			continue
		}
		if _, seen := unique[opinion.SessionID]; seen {
			continue
		}
		unique[opinion.SessionID] = opinion
		order = append(order, opinion.SessionID)
	}
	sort.Strings(order)
	var builder strings.Builder
	builder.WriteString("| session | model | verdict | reason |\n|---|---|---|---|\n")
	for _, sessionID := range order {
		opinion := unique[sessionID]
		builder.WriteString("| ")
		builder.WriteString(markdownCell(sessionID))
		builder.WriteString(" | ")
		builder.WriteString(markdownCell(opinion.Model))
		builder.WriteString(" | ")
		builder.WriteString(markdownCell(opinion.Verdict))
		builder.WriteString(" | ")
		builder.WriteString(markdownCell(opinion.Reason))
		builder.WriteString(" |\n")
	}
	return builder.String()
}

func markdownCell(value string) string {
	value = strings.ReplaceAll(value, "|", "\\|")
	value = strings.ReplaceAll(value, "\r", " ")
	return strings.ReplaceAll(value, "\n", " ")
}
