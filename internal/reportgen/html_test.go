package reportgen

import (
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jacu-dev/jacu-harness/internal/report"
)

func sampleReport() report.Report {
	return report.Report{
		SchemaVersion: report.SchemaVersion,
		Kind:          report.KindPlan,
		Title:         "Plan alpha",
		Summary:       "choose the next step",
		Blocks: report.ReportBlocks{
			Summary:  []string{"choose the next step"},
			Steps:    []report.ReportStep{{ID: "s1", Label: "Write factory", Status: "open"}},
			Decision: []report.DecisionPoint{{ID: "d1", Question: "Ship HTML?", Kind: "options", Options: []string{"yes", "no"}}},
			Risks:    []string{"markup from the model"},
			Flow: report.FlowBlock{
				Nodes: []report.FlowNode{{ID: "a", Label: "JSON", Kind: "input"}, {ID: "b", Label: "HTML", Kind: "output"}},
				Edges: []report.FlowEdge{{From: "a", To: "b"}},
			},
			Chart:   []report.ChartPoint{{Label: "open", Value: 2}},
			Table:   report.TableBlock{Columns: []string{"id", "status"}, Rows: [][]string{{"s1", "open"}}},
			Metrics: []report.Metric{{Name: "blocks", Value: 8, Available: true}},
		},
	}
}

func TestHTMLIsByteIdenticalAcrossRenders(t *testing.T) {
	first, err := HTML(sampleReport())
	if err != nil {
		t.Fatal(err)
	}
	second, err := HTML(sampleReport())
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("HTML was not deterministic\nfirst=%q\nsecond=%q", first, second)
	}
	for _, needle := range []string{"<!DOCTYPE html>", "Plan alpha", "choose the next step", "Ship HTML?", "blocks"} {
		if !strings.Contains(first, needle) {
			t.Fatalf("HTML omitted %q: %s", needle, first)
		}
	}
	if strings.Contains(first, "```mermaid") {
		t.Fatal("HTML rendered mermaid")
	}
}

func TestHTMLColdStartIsUnderBudget(t *testing.T) {
	start := time.Now()
	if _, err := HTML(sampleReport()); err != nil {
		t.Fatal(err)
	}
	elapsed := time.Since(start)
	t.Logf("html_cold_start=%s", elapsed)
	if elapsed > 150*time.Millisecond {
		t.Fatalf("html cold start %s exceeds 150ms floor", elapsed)
	}
}

func TestHTMLRefusesPresentationMarkup(t *testing.T) {
	input := sampleReport()
	input.Summary = "ignore <script>alert(1)</script>"
	if _, err := HTML(input); err == nil {
		t.Fatal("presentation markup was rendered")
	}
	input = sampleReport()
	input.Blocks.Risks = []string{"color: red; body { display:none } <style>x</style>"}
	if _, err := HTML(input); err == nil {
		t.Fatal("style markup was rendered")
	}
}

func TestHTMLProjectorDoesNotImportNet(t *testing.T) {
	source, err := os.ReadFile("html.go")
	if err != nil {
		t.Fatal(err)
	}
	file, err := parser.ParseFile(token.NewFileSet(), "html.go", source, parser.ImportsOnly)
	if err != nil {
		t.Fatal(err)
	}
	for _, spec := range file.Imports {
		path := strings.Trim(spec.Path.Value, `"`)
		if path == "net" || strings.HasPrefix(path, "net/") {
			t.Fatalf("headless HTML imported %s", path)
		}
	}
}
