// Package reportgen projects a validated report onto deterministic Markdown.
// It contains no MCP types.
package reportgen

import (
	"fmt"
	"strings"

	"github.com/jacu-dev/jacu-harness/internal/report"
)

func Markdown(input report.Report) (string, error) {
	prepared, err := report.Prepared(input)
	if err != nil {
		return "", err
	}
	var out strings.Builder
	fmt.Fprintf(&out, "# %s\n\n", markdownHeading(prepared.Title))
	fmt.Fprintf(&out, "> schema_version: `%s` · kind: `%s`\n\n", prepared.SchemaVersion, prepared.Kind)
	fmt.Fprintf(&out, "## Summary\n\n%s\n\n", markdownText(prepared.Summary))

	out.WriteString("## Steps\n\n")
	if len(prepared.Blocks.Steps) == 0 {
		out.WriteString("_none_\n\n")
	} else {
		for _, step := range prepared.Blocks.Steps {
			fmt.Fprintf(&out, "- `%s` — %s (`%s`)\n", markdownCell(step.ID), markdownText(step.Label), markdownCell(step.Status))
		}
		out.WriteByte('\n')
	}

	out.WriteString("## Decisions\n\n")
	if len(prepared.Blocks.Decision) == 0 {
		out.WriteString("_none_\n\n")
	} else {
		for _, decision := range prepared.Blocks.Decision {
			fmt.Fprintf(&out, "- `%s` — %s\n", markdownCell(decision.ID), markdownText(decision.Question))
		}
		out.WriteByte('\n')
	}

	out.WriteString("## Risks\n\n")
	if len(prepared.Blocks.Risks) == 0 {
		out.WriteString("_none_\n\n")
	} else {
		for _, risk := range prepared.Blocks.Risks {
			fmt.Fprintf(&out, "- %s\n", markdownText(risk))
		}
		out.WriteByte('\n')
	}

	out.WriteString("## Flow\n\n```mermaid\nflowchart TD\n")
	for _, node := range prepared.Blocks.Flow.Nodes {
		fmt.Fprintf(&out, "  %s[\"%s\"]\n", mermaidID(node.ID), mermaidText(node.Label))
	}
	for _, edge := range prepared.Blocks.Flow.Edges {
		fmt.Fprintf(&out, "  %s --> %s\n", mermaidID(edge.From), mermaidID(edge.To))
	}
	out.WriteString("```\n\n")

	out.WriteString("## Chart\n\n")
	for _, point := range prepared.Blocks.Chart {
		fmt.Fprintf(&out, "- %s: %d\n", markdownText(point.Label), point.Value)
	}
	if len(prepared.Blocks.Chart) == 0 {
		out.WriteString("_none_\n")
	}
	out.WriteByte('\n')

	out.WriteString("## Table\n\n")
	writeTable(&out, prepared.Blocks.Table)
	out.WriteString("\n## Metrics\n\n")
	for _, metric := range prepared.Blocks.Metrics {
		value := metric.ValueText
		if !metric.Available {
			value = "no-data"
		} else if value == "" {
			value = fmt.Sprintf("%d", metric.Value)
		}
		fmt.Fprintf(&out, "- %s: %s\n", markdownText(metric.Name), markdownText(value))
	}
	if len(prepared.Blocks.Metrics) == 0 {
		out.WriteString("_none_\n")
	}
	return out.String(), nil
}

func markdownHeading(value string) string { return strings.ReplaceAll(value, "#", "\\#") }
func markdownText(value string) string    { return strings.ReplaceAll(value, "`", "'") }
func markdownCell(value string) string {
	return strings.ReplaceAll(strings.ReplaceAll(markdownText(value), "|", "\\|"), "\n", " ")
}
func mermaidText(value string) string { return strings.ReplaceAll(markdownText(value), `"`, "'") }
func mermaidID(value string) string {
	var out strings.Builder
	for _, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') || char == '_' {
			out.WriteRune(char)
		} else {
			out.WriteByte('_')
		}
	}
	return out.String()
}

func writeTable(out *strings.Builder, table report.TableBlock) {
	out.WriteString("| ")
	for index, column := range table.Columns {
		if index > 0 {
			out.WriteString(" | ")
		}
		out.WriteString(markdownCell(column))
	}
	out.WriteString(" |\n| ")
	for index := range table.Columns {
		if index > 0 {
			out.WriteString(" | ")
		}
		out.WriteString("---")
	}
	out.WriteString(" |\n")
	for _, row := range table.Rows {
		out.WriteString("| ")
		for index, cell := range row {
			if index > 0 {
				out.WriteString(" | ")
			}
			out.WriteString(markdownCell(cell))
		}
		out.WriteString(" |\n")
	}
}
