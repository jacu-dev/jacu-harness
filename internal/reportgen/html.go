package reportgen

import (
	"fmt"
	"html"
	"strconv"
	"strings"

	"github.com/jacu-dev/jacu-harness/internal/report"
)

const factoryCSS = `body{font-family:sans-serif;margin:24px;color:#111;background:#fff}` +
	`header{margin-bottom:24px}main section{margin-bottom:24px}` +
	`table{border-collapse:collapse}td,th{border:1px solid #ccc;padding:4px 8px}` +
	`.meta{color:#555}svg{max-width:100%}`

func HTML(input report.Report) (string, error) {
	if err := refusePresentationMarkup(input); err != nil {
		return "", err
	}
	prepared, err := report.Prepared(input)
	if err != nil {
		return "", err
	}
	var out strings.Builder
	out.WriteString("<!DOCTYPE html>\n<html lang=\"en\">\n<head>\n")
	out.WriteString("<meta charset=\"utf-8\">\n<title>")
	out.WriteString(html.EscapeString(prepared.Title))
	out.WriteString("</title>\n<style>")
	out.WriteString(factoryCSS)
	out.WriteString("</style>\n</head>\n<body>\n<header>\n<h1>")
	out.WriteString(html.EscapeString(prepared.Title))
	out.WriteString("</h1>\n<p class=\"meta\">schema_version ")
	out.WriteString(html.EscapeString(prepared.SchemaVersion))
	out.WriteString(" · kind ")
	out.WriteString(html.EscapeString(prepared.Kind))
	out.WriteString("</p>\n</header>\n<main>\n")
	writeHTMLSection(&out, "Summary", htmlList(prepared.Blocks.Summary, prepared.Summary))
	writeHTMLSection(&out, "Steps", htmlSteps(prepared.Blocks.Steps))
	writeHTMLSection(&out, "Decisions", htmlDecisions(prepared.Blocks.Decision))
	writeHTMLSection(&out, "Risks", htmlList(prepared.Blocks.Risks, ""))
	writeHTMLSection(&out, "Flow", htmlFlow(prepared.Blocks.Flow))
	writeHTMLSection(&out, "Chart", htmlChart(prepared.Blocks.Chart))
	writeHTMLSection(&out, "Table", htmlTable(prepared.Blocks.Table))
	writeHTMLSection(&out, "Metrics", htmlMetrics(prepared.Blocks.Metrics))
	out.WriteString("</main>\n</body>\n</html>\n")
	return out.String(), nil
}

func writeHTMLSection(out *strings.Builder, title, body string) {
	fmt.Fprintf(out, "<section id=\"%s\">\n<h2>%s</h2>\n%s</section>\n", htmlID(title), html.EscapeString(title), body)
}

func htmlList(items []string, fallback string) string {
	if len(items) == 0 {
		if fallback == "" {
			return "<p class=\"empty\">none</p>\n"
		}
		return "<p>" + html.EscapeString(fallback) + "</p>\n"
	}
	var out strings.Builder
	out.WriteString("<ul>\n")
	for _, item := range items {
		out.WriteString("<li>")
		out.WriteString(html.EscapeString(item))
		out.WriteString("</li>\n")
	}
	out.WriteString("</ul>\n")
	return out.String()
}

func htmlSteps(steps []report.ReportStep) string {
	if len(steps) == 0 {
		return "<p class=\"empty\">none</p>\n"
	}
	var out strings.Builder
	out.WriteString("<ol>\n")
	for _, step := range steps {
		fmt.Fprintf(&out, "<li><code>%s</code> %s (<code>%s</code>)</li>\n",
			html.EscapeString(step.ID), html.EscapeString(step.Label), html.EscapeString(step.Status))
	}
	out.WriteString("</ol>\n")
	return out.String()
}

func htmlDecisions(decisions []report.DecisionPoint) string {
	if len(decisions) == 0 {
		return "<p class=\"empty\">none</p>\n"
	}
	var out strings.Builder
	out.WriteString("<ul>\n")
	for _, decision := range decisions {
		fmt.Fprintf(&out, "<li data-decision=\"%s\">%s", html.EscapeString(decision.ID), html.EscapeString(decision.Question))
		if decision.Answer != "" {
			fmt.Fprintf(&out, " — <strong>%s</strong>", html.EscapeString(decision.Answer))
		}
		out.WriteString("</li>\n")
	}
	out.WriteString("</ul>\n")
	return out.String()
}

func htmlFlow(flow report.FlowBlock) string {
	if len(flow.Nodes) == 0 {
		return "<p class=\"empty\">none</p>\n"
	}
	width := 120*len(flow.Nodes) + 40
	if width < 200 {
		width = 200
	}
	indexOf := map[string]int{}
	var out strings.Builder
	fmt.Fprintf(&out, "<svg role=\"img\" width=\"%d\" height=\"160\" viewBox=\"0 0 %d 160\">\n", width, width)
	for index, node := range flow.Nodes {
		indexOf[node.ID] = index
		x := 20 + index*120
		fmt.Fprintf(&out, "<rect x=\"%d\" y=\"40\" width=\"100\" height=\"48\" fill=\"#f4f4f4\" stroke=\"#333\"/>\n", x)
		fmt.Fprintf(&out, "<text x=\"%d\" y=\"68\" font-size=\"12\">%s</text>\n", x+8, html.EscapeString(node.Label))
	}
	for _, edge := range flow.Edges {
		from, okFrom := indexOf[edge.From]
		to, okTo := indexOf[edge.To]
		if !okFrom || !okTo {
			continue
		}
		x1 := 70 + from*120
		x2 := 70 + to*120
		fmt.Fprintf(&out, "<line x1=\"%d\" y1=\"88\" x2=\"%d\" y2=\"120\" stroke=\"#333\"/>\n", x1, x2)
	}
	out.WriteString("</svg>\n")
	return out.String()
}

func htmlChart(points []report.ChartPoint) string {
	if len(points) == 0 {
		return "<p class=\"empty\">none</p>\n"
	}
	max := 1
	for _, point := range points {
		if point.Value > max {
			max = point.Value
		}
	}
	width := 40 + 48*len(points)
	var out strings.Builder
	fmt.Fprintf(&out, "<svg role=\"img\" width=\"%d\" height=\"160\" viewBox=\"0 0 %d 160\">\n", width, width)
	for index, point := range points {
		height := 8 + (point.Value*96)/max
		x := 16 + index*48
		y := 120 - height
		fmt.Fprintf(&out, "<rect x=\"%d\" y=\"%d\" width=\"32\" height=\"%d\" fill=\"#336\"/>\n", x, y, height)
		fmt.Fprintf(&out, "<text x=\"%d\" y=\"140\" font-size=\"10\">%s</text>\n", x, html.EscapeString(point.Label))
		fmt.Fprintf(&out, "<text x=\"%d\" y=\"%d\" font-size=\"10\">%s</text>\n", x, y-4, strconv.Itoa(point.Value))
	}
	out.WriteString("</svg>\n")
	return out.String()
}

func htmlTable(table report.TableBlock) string {
	var out strings.Builder
	out.WriteString("<table>\n<thead><tr>")
	for _, column := range table.Columns {
		out.WriteString("<th>")
		out.WriteString(html.EscapeString(column))
		out.WriteString("</th>")
	}
	out.WriteString("</tr></thead>\n<tbody>\n")
	for _, row := range table.Rows {
		out.WriteString("<tr>")
		for _, cell := range row {
			out.WriteString("<td>")
			out.WriteString(html.EscapeString(cell))
			out.WriteString("</td>")
		}
		out.WriteString("</tr>\n")
	}
	out.WriteString("</tbody>\n</table>\n")
	return out.String()
}

func htmlMetrics(metrics []report.Metric) string {
	if len(metrics) == 0 {
		return "<p class=\"empty\">none</p>\n"
	}
	items := make([]string, 0, len(metrics))
	for _, metric := range metrics {
		value := metric.ValueText
		if !metric.Available {
			value = "no-data"
		} else if value == "" {
			value = strconv.Itoa(metric.Value)
		}
		items = append(items, metric.Name+": "+value)
	}
	return htmlList(items, "")
}

func htmlID(title string) string {
	return strings.ToLower(strings.ReplaceAll(title, " ", "-"))
}
