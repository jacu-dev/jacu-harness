package reportgen

import (
	"strings"
	"testing"

	"github.com/jacu-dev/jacu-harness/internal/report"
)

func TestMarkdownProjectsPreparedAudit(t *testing.T) {
	input := report.Report{
		SchemaVersion: report.SchemaVersion,
		Kind:          report.KindAudit,
		Title:         "Workspace audit",
		Summary:       "one active run",
		Blocks: report.ReportBlocks{
			Summary:  []string{"one active run"},
			Steps:    []report.ReportStep{{ID: "run_a", Label: "Fix parser", Status: "open"}},
			Decision: []report.DecisionPoint{},
			Risks:    []string{},
			Flow: report.FlowBlock{
				Nodes: []report.FlowNode{{ID: "run_a", Label: "Fix parser", Kind: "run"}},
				Edges: []report.FlowEdge{},
			},
			Chart:   []report.ChartPoint{{Label: "open", Value: 1}},
			Table:   report.TableBlock{Columns: []string{"run_id", "status"}, Rows: [][]string{{"run_a", "open"}}},
			Metrics: []report.Metric{{Name: "runs", Value: 1, Available: true}},
		},
	}
	markdown, err := Markdown(input)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(markdown, "```mermaid") || !strings.Contains(markdown, "| run_id | status |") {
		t.Fatalf("projection omitted blocks: %q", markdown)
	}
}
