// Package report contains the structured, headless read-model for JACU state.
// Markdown is an output projection; it is never used as an input source.
package report

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"github.com/jacu-dev/jacu-harness/internal/runstate"
	"github.com/jacu-dev/jacu-harness/internal/telemetry"
)

const (
	SchemaVersion = "1"
	KindAdhoc     = "adhoc"
	KindPlan      = "plan"
	KindAudit     = "audit"
)

type Report struct {
	SchemaVersion string       `json:"schema_version"`
	Kind          string       `json:"kind"`
	Title         string       `json:"title"`
	Summary       string       `json:"summary"`
	Blocks        ReportBlocks `json:"blocks"`
}

type ReportBlocks struct {
	Summary  []string        `json:"summary"`
	Steps    []ReportStep    `json:"steps"`
	Decision []DecisionPoint `json:"decision"`
	Risks    []string        `json:"risks"`
	Flow     FlowBlock       `json:"flow"`
	Chart    []ChartPoint    `json:"chart"`
	Table    TableBlock      `json:"table"`
	Metrics  []Metric        `json:"metrics"`
}

type ReportStep struct {
	ID     string `json:"id"`
	Label  string `json:"label"`
	Status string `json:"status"`
}

type DecisionPoint struct {
	ID       string   `json:"id"`
	Question string   `json:"question"`
	Kind     string   `json:"kind"`
	Options  []string `json:"options,omitempty"`
	Answer   string   `json:"answer,omitempty"`
}

type FlowBlock struct {
	Nodes []FlowNode `json:"nodes"`
	Edges []FlowEdge `json:"edges"`
}

type FlowNode struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Kind  string `json:"kind"`
	Tech  string `json:"tech,omitempty"`
}

type FlowEdge struct {
	From string `json:"from"`
	To   string `json:"to"`
}

type ChartPoint struct {
	Label string `json:"label"`
	Value int    `json:"value"`
}

type TableBlock struct {
	Columns []string   `json:"columns"`
	Rows    [][]string `json:"rows"`
}

type Metric struct {
	Name      string `json:"name"`
	Value     int    `json:"value"`
	ValueText string `json:"value_text,omitempty"`
	Available bool   `json:"available"`
	Heuristic bool   `json:"heuristic,omitempty"`
}

var secretPattern = regexp.MustCompile(`(?i)(sk-|ghp_|bearer |password|oauth|token=|api[_ -]?key|begin private key)`)

func Validate(report Report) error {
	if report.SchemaVersion != SchemaVersion {
		return fmt.Errorf("unsupported report schema_version %q", report.SchemaVersion)
	}
	if report.Kind != KindAdhoc && report.Kind != KindPlan && report.Kind != KindAudit {
		return fmt.Errorf("unsupported report kind %q", report.Kind)
	}
	if strings.TrimSpace(report.Title) == "" {
		return errors.New("report title is required")
	}
	if strings.TrimSpace(report.Summary) == "" {
		return errors.New("report summary is required")
	}
	if len(report.Blocks.Summary) == 0 || report.Blocks.Summary == nil {
		return errors.New("report summary block is required")
	}
	if report.Blocks.Steps == nil || report.Blocks.Decision == nil || report.Blocks.Risks == nil ||
		report.Blocks.Flow.Nodes == nil || report.Blocks.Flow.Edges == nil || report.Blocks.Chart == nil ||
		report.Blocks.Table.Columns == nil || report.Blocks.Table.Rows == nil || report.Blocks.Metrics == nil {
		return errors.New("report blocks must use non-nil arrays")
	}
	if len(report.Blocks.Table.Columns) == 0 {
		return errors.New("report table needs columns")
	}
	seenNodes := make(map[string]struct{}, len(report.Blocks.Flow.Nodes))
	for _, node := range report.Blocks.Flow.Nodes {
		if strings.TrimSpace(node.ID) == "" || strings.TrimSpace(node.Kind) == "" {
			return errors.New("flow nodes need id and kind")
		}
		if _, exists := seenNodes[node.ID]; exists {
			return fmt.Errorf("duplicate flow node %q", node.ID)
		}
		seenNodes[node.ID] = struct{}{}
	}
	for _, edge := range report.Blocks.Flow.Edges {
		if _, ok := seenNodes[edge.From]; !ok {
			return fmt.Errorf("flow edge references missing node %q", edge.From)
		}
		if _, ok := seenNodes[edge.To]; !ok {
			return fmt.Errorf("flow edge references missing node %q", edge.To)
		}
	}
	for _, row := range report.Blocks.Table.Rows {
		if len(row) != len(report.Blocks.Table.Columns) {
			return errors.New("report table row has wrong column count")
		}
	}
	for _, decision := range report.Blocks.Decision {
		if strings.TrimSpace(decision.ID) == "" || strings.TrimSpace(decision.Question) == "" {
			return errors.New("decision points need id and question")
		}
	}
	return nil
}

func Digest(report Report) (string, error) {
	normalized := normalize(report)
	if err := Validate(normalized); err != nil {
		return "", err
	}
	encoded, err := json.Marshal(normalized)
	if err != nil {
		return "", fmt.Errorf("encode report: %w", err)
	}
	hash := sha256.Sum256(encoded)
	return "sha256:" + hex.EncodeToString(hash[:]), nil
}

func Markdown(report Report) (string, error) {
	report = normalize(report)
	if err := Validate(report); err != nil {
		return "", err
	}
	var out strings.Builder
	fmt.Fprintf(&out, "# %s\n\n", markdownHeading(report.Title))
	fmt.Fprintf(&out, "> schema_version: `%s` · kind: `%s`\n\n", report.SchemaVersion, report.Kind)
	fmt.Fprintf(&out, "## Summary\n\n%s\n\n", markdownText(report.Summary))

	out.WriteString("## Steps\n\n")
	if len(report.Blocks.Steps) == 0 {
		out.WriteString("_none_\n\n")
	} else {
		for _, step := range report.Blocks.Steps {
			fmt.Fprintf(&out, "- `%s` — %s (`%s`)\n", markdownCell(step.ID), markdownText(step.Label), markdownCell(step.Status))
		}
		out.WriteByte('\n')
	}

	out.WriteString("## Decisions\n\n")
	if len(report.Blocks.Decision) == 0 {
		out.WriteString("_none_\n\n")
	} else {
		for _, decision := range report.Blocks.Decision {
			fmt.Fprintf(&out, "- `%s` — %s\n", markdownCell(decision.ID), markdownText(decision.Question))
		}
		out.WriteByte('\n')
	}

	out.WriteString("## Risks\n\n")
	if len(report.Blocks.Risks) == 0 {
		out.WriteString("_none_\n\n")
	} else {
		for _, risk := range report.Blocks.Risks {
			fmt.Fprintf(&out, "- %s\n", markdownText(risk))
		}
		out.WriteByte('\n')
	}

	out.WriteString("## Flow\n\n```mermaid\nflowchart TD\n")
	for _, node := range report.Blocks.Flow.Nodes {
		fmt.Fprintf(&out, "  %s[\"%s\"]\n", mermaidID(node.ID), mermaidText(node.Label))
	}
	for _, edge := range report.Blocks.Flow.Edges {
		fmt.Fprintf(&out, "  %s --> %s\n", mermaidID(edge.From), mermaidID(edge.To))
	}
	out.WriteString("```\n\n")

	out.WriteString("## Chart\n\n")
	for _, point := range report.Blocks.Chart {
		fmt.Fprintf(&out, "- %s: %d\n", markdownText(point.Label), point.Value)
	}
	if len(report.Blocks.Chart) == 0 {
		out.WriteString("_none_\n")
	}
	out.WriteByte('\n')

	out.WriteString("## Table\n\n")
	writeTable(&out, report.Blocks.Table)
	out.WriteString("\n## Metrics\n\n")
	for _, metric := range report.Blocks.Metrics {
		value := metric.ValueText
		if !metric.Available {
			value = "no-data"
		} else if value == "" {
			value = fmt.Sprintf("%d", metric.Value)
		}
		fmt.Fprintf(&out, "- %s: %s\n", markdownText(metric.Name), markdownText(value))
	}
	if len(report.Blocks.Metrics) == 0 {
		out.WriteString("_none_\n")
	}
	return out.String(), nil
}

func BuildAudit(root string) (Report, error) {
	runs, err := runstate.List(root)
	if err != nil {
		return Report{}, err
	}
	blocks := ReportBlocks{
		Summary: []string{}, Steps: []ReportStep{}, Decision: []DecisionPoint{}, Risks: []string{},
		Flow: FlowBlock{Nodes: []FlowNode{}, Edges: []FlowEdge{}}, Chart: []ChartPoint{},
		Table: TableBlock{Columns: []string{"run_id", "status", "mission_id", "program_id", "program_cursor"}, Rows: [][]string{}}, Metrics: []Metric{},
	}
	statusCounts := map[string]int{}
	nodes := map[string]struct{}{}
	addNode := func(node FlowNode) {
		if _, exists := nodes[node.ID]; exists {
			return
		}
		nodes[node.ID] = struct{}{}
		blocks.Flow.Nodes = append(blocks.Flow.Nodes, node)
	}
	active := 0
	programs := map[string]struct{}{}
	for _, run := range runs {
		status := string(run.Status)
		statusCounts[status]++
		if run.Status == runstate.StatusOpen || run.Status == runstate.StatusReviewed {
			active++
		}
		label := sanitizeText(run.Mission.Objective)
		if label == "" {
			label = "mission " + run.MissionID
		}
		blocks.Steps = append(blocks.Steps, ReportStep{ID: run.RunID, Label: label, Status: status})
		blocks.Table.Rows = append(blocks.Table.Rows, []string{run.RunID, status, run.MissionID, run.ProgramID, fmt.Sprintf("%d", run.ProgramCursor)})
		runNode := "run:" + run.RunID
		addNode(FlowNode{ID: runNode, Label: run.RunID, Kind: "run"})
		if run.MissionID != "" {
			missionNode := "mission:" + run.MissionID
			addNode(FlowNode{ID: missionNode, Label: sanitizeText(run.MissionID), Kind: "mission"})
			blocks.Flow.Edges = append(blocks.Flow.Edges, FlowEdge{From: missionNode, To: runNode})
		}
		if run.ProgramID != "" {
			programs[run.ProgramID] = struct{}{}
			programNode := "program:" + run.ProgramID
			addNode(FlowNode{ID: programNode, Label: sanitizeText(run.ProgramID), Kind: "program"})
			blocks.Flow.Edges = append(blocks.Flow.Edges, FlowEdge{From: programNode, To: runNode})
			for _, mission := range run.ProgramMissions {
				programMissionNode := fmt.Sprintf("program:%s:mission:%d", run.ProgramID, mission.Index)
				addNode(FlowNode{ID: programMissionNode, Label: fmt.Sprintf("%s mission %d", run.ProgramID, mission.Index), Kind: "program_mission"})
				blocks.Flow.Edges = append(blocks.Flow.Edges, FlowEdge{From: programNode, To: programMissionNode})
			}
		}
	}
	blocks.Summary = append(blocks.Summary, fmt.Sprintf("%d run(s); %d active; %d program(s).", len(runs), active, len(programs)))
	summary := blocks.Summary[0]
	for status, count := range statusCounts {
		blocks.Chart = append(blocks.Chart, ChartPoint{Label: status, Value: count})
		blocks.Metrics = append(blocks.Metrics, Metric{Name: "status." + status, Value: count, Available: true})
	}
	blocks.Metrics = append(blocks.Metrics, buildTelemetryMetrics(root)...)
	report := Report{SchemaVersion: SchemaVersion, Kind: KindAudit, Title: "JACU workspace audit", Summary: summary, Blocks: blocks}
	report = normalize(report)
	if err := Validate(report); err != nil {
		return Report{}, err
	}
	return report, nil
}

func buildTelemetryMetrics(root string) []Metric {
	stats := telemetry.ReportStats(root)
	metric := func(name, value string, available, heuristic bool) Metric {
		return Metric{Name: name, ValueText: value, Available: available, Heuristic: heuristic}
	}
	available := func(name string) bool { return stats.Available[name] }
	percent := func(value float64) string { return fmt.Sprintf("%.1f%%", value) }
	metrics := []Metric{
		metric(telemetry.MetricFirstPassVerify, percent(stats.FirstPassVerifyPct), available(telemetry.MetricFirstPassVerify), false),
		metric(telemetry.MetricRemediation, fmt.Sprintf("%.1f", stats.RemediationIterations), available(telemetry.MetricRemediation), false),
		metric(telemetry.MetricEscalation, percent(stats.EscalationPct), available(telemetry.MetricEscalation), false),
		metric(telemetry.MetricAutoApply, percent(stats.AutoApplyPct), available(telemetry.MetricAutoApply), false),
		metric(telemetry.MetricMissionApplyP50, fmt.Sprintf("%d", stats.MissionApplyP50Ms), available(telemetry.MetricMissionApplyP50), false),
		metric(telemetry.MetricMissionApplyP95, fmt.Sprintf("%d", stats.MissionApplyP95Ms), available(telemetry.MetricMissionApplyP95), false),
		metric(telemetry.MetricMissionsPerDay, fmt.Sprintf("%.1f", stats.MissionsPerDay), available(telemetry.MetricMissionsPerDay), false),
		metric(telemetry.MetricRevertedApplyPct, percent(stats.RevertedApplyPct), available(telemetry.MetricRevertedApplyPct), stats.RevertHeuristic),
		metric(telemetry.MetricMissionBytesIn, fmt.Sprintf("%d", stats.MissionBytesIn), available(telemetry.MetricMissionBytesIn), false),
		metric(telemetry.MetricMissionBytesOut, fmt.Sprintf("%d", stats.MissionBytesOut), available(telemetry.MetricMissionBytesOut), false),
		metric(telemetry.MetricMissionInterruptions, "0", available(telemetry.MetricMissionInterruptions), false),
		metric(telemetry.MetricCleanExitResult, "no-data", available(telemetry.MetricCleanExitResult), false),
	}
	for tool, duration := range stats.ToolP95Ms {
		metrics = append(metrics, metric("tool_p95_ms."+tool, fmt.Sprintf("%d", duration), true, false))
	}
	return metrics
}

func normalize(report Report) Report {
	report.Title = sanitizeText(report.Title)
	report.Summary = sanitizeText(report.Summary)
	report.Blocks.Summary = append([]string{}, report.Blocks.Summary...)
	for index := range report.Blocks.Summary {
		report.Blocks.Summary[index] = sanitizeText(report.Blocks.Summary[index])
	}
	report.Blocks.Steps = append([]ReportStep{}, report.Blocks.Steps...)
	for index := range report.Blocks.Steps {
		report.Blocks.Steps[index].ID = sanitizeText(report.Blocks.Steps[index].ID)
		report.Blocks.Steps[index].Label = sanitizeText(report.Blocks.Steps[index].Label)
		report.Blocks.Steps[index].Status = sanitizeText(report.Blocks.Steps[index].Status)
	}
	sort.Slice(report.Blocks.Steps, func(i, j int) bool { return report.Blocks.Steps[i].ID < report.Blocks.Steps[j].ID })
	report.Blocks.Decision = append([]DecisionPoint{}, report.Blocks.Decision...)
	for index := range report.Blocks.Decision {
		report.Blocks.Decision[index].ID = sanitizeText(report.Blocks.Decision[index].ID)
		report.Blocks.Decision[index].Question = sanitizeText(report.Blocks.Decision[index].Question)
		report.Blocks.Decision[index].Answer = sanitizeText(report.Blocks.Decision[index].Answer)
		report.Blocks.Decision[index].Options = append([]string{}, report.Blocks.Decision[index].Options...)
		for option := range report.Blocks.Decision[index].Options {
			report.Blocks.Decision[index].Options[option] = sanitizeText(report.Blocks.Decision[index].Options[option])
		}
	}
	sort.Slice(report.Blocks.Decision, func(i, j int) bool { return report.Blocks.Decision[i].ID < report.Blocks.Decision[j].ID })
	report.Blocks.Risks = append([]string{}, report.Blocks.Risks...)
	sort.Strings(report.Blocks.Risks)
	report.Blocks.Flow.Nodes = append([]FlowNode{}, report.Blocks.Flow.Nodes...)
	for index := range report.Blocks.Flow.Nodes {
		report.Blocks.Flow.Nodes[index].ID = sanitizeText(report.Blocks.Flow.Nodes[index].ID)
		report.Blocks.Flow.Nodes[index].Label = sanitizeText(report.Blocks.Flow.Nodes[index].Label)
		report.Blocks.Flow.Nodes[index].Kind = sanitizeText(report.Blocks.Flow.Nodes[index].Kind)
		report.Blocks.Flow.Nodes[index].Tech = sanitizeText(report.Blocks.Flow.Nodes[index].Tech)
	}
	sort.Slice(report.Blocks.Flow.Nodes, func(i, j int) bool { return report.Blocks.Flow.Nodes[i].ID < report.Blocks.Flow.Nodes[j].ID })
	report.Blocks.Flow.Edges = append([]FlowEdge{}, report.Blocks.Flow.Edges...)
	sort.Slice(report.Blocks.Flow.Edges, func(i, j int) bool {
		if report.Blocks.Flow.Edges[i].From == report.Blocks.Flow.Edges[j].From {
			return report.Blocks.Flow.Edges[i].To < report.Blocks.Flow.Edges[j].To
		}
		return report.Blocks.Flow.Edges[i].From < report.Blocks.Flow.Edges[j].From
	})
	report.Blocks.Chart = append([]ChartPoint{}, report.Blocks.Chart...)
	for index := range report.Blocks.Chart {
		report.Blocks.Chart[index].Label = sanitizeText(report.Blocks.Chart[index].Label)
	}
	sort.Slice(report.Blocks.Chart, func(i, j int) bool { return report.Blocks.Chart[i].Label < report.Blocks.Chart[j].Label })
	report.Blocks.Table.Columns = append([]string{}, report.Blocks.Table.Columns...)
	for index := range report.Blocks.Table.Columns {
		report.Blocks.Table.Columns[index] = sanitizeText(report.Blocks.Table.Columns[index])
	}
	report.Blocks.Table.Rows = append([][]string{}, report.Blocks.Table.Rows...)
	for index := range report.Blocks.Table.Rows {
		report.Blocks.Table.Rows[index] = append([]string{}, report.Blocks.Table.Rows[index]...)
		for cell := range report.Blocks.Table.Rows[index] {
			report.Blocks.Table.Rows[index][cell] = sanitizeText(report.Blocks.Table.Rows[index][cell])
		}
	}
	sort.Slice(report.Blocks.Table.Rows, func(i, j int) bool {
		return strings.Join(report.Blocks.Table.Rows[i], "\x00") < strings.Join(report.Blocks.Table.Rows[j], "\x00")
	})
	report.Blocks.Metrics = append([]Metric{}, report.Blocks.Metrics...)
	for index := range report.Blocks.Metrics {
		report.Blocks.Metrics[index].Name = sanitizeText(report.Blocks.Metrics[index].Name)
		report.Blocks.Metrics[index].ValueText = sanitizeText(report.Blocks.Metrics[index].ValueText)
	}
	sort.Slice(report.Blocks.Metrics, func(i, j int) bool { return report.Blocks.Metrics[i].Name < report.Blocks.Metrics[j].Name })
	return report
}

func sanitizeText(value string) string {
	value = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, value)
	value = strings.TrimSpace(value)
	if secretPattern.MatchString(value) {
		return "<redacted>"
	}
	if looksAbsolutePath(value) {
		return "<path>"
	}
	if len(value) > 200 {
		value = value[:200] + "..."
	}
	return value
}

func looksAbsolutePath(value string) bool {
	if strings.HasPrefix(value, "/") || strings.HasPrefix(value, `\`) || strings.HasPrefix(strings.ToLower(value), "file://") {
		return true
	}
	for index := 1; index+1 < len(value); index++ {
		if (value[index] == '/' || value[index] == '\\') && (value[index-1] == ' ' || value[index-1] == '=' || value[index-1] == ',' || value[index-1] == ';') {
			return true
		}
	}
	return len(value) >= 3 && ((value[0] >= 'A' && value[0] <= 'Z') || (value[0] >= 'a' && value[0] <= 'z')) && value[1] == ':' && (value[2] == '/' || value[2] == '\\')
}

func markdownHeading(value string) string { return strings.ReplaceAll(sanitizeText(value), "#", "\\#") }
func markdownText(value string) string    { return strings.ReplaceAll(sanitizeText(value), "`", "'") }
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

func writeTable(out *strings.Builder, table TableBlock) {
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
