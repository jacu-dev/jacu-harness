package reportgen

import (
	"errors"
	"strings"

	"github.com/jacu-dev/jacu-harness/internal/report"
)

func refusePresentationMarkup(input report.Report) error {
	fields := []string{input.Title, input.Summary}
	fields = append(fields, input.Blocks.Summary...)
	fields = append(fields, input.Blocks.Risks...)
	for _, step := range input.Blocks.Steps {
		fields = append(fields, step.ID, step.Label, step.Status)
	}
	for _, decision := range input.Blocks.Decision {
		fields = append(fields, decision.ID, decision.Question, decision.Kind, decision.Answer)
		fields = append(fields, decision.Options...)
	}
	for _, node := range input.Blocks.Flow.Nodes {
		fields = append(fields, node.ID, node.Label, node.Kind, node.Tech)
	}
	for _, edge := range input.Blocks.Flow.Edges {
		fields = append(fields, edge.From, edge.To)
	}
	for _, point := range input.Blocks.Chart {
		fields = append(fields, point.Label)
	}
	fields = append(fields, input.Blocks.Table.Columns...)
	for _, row := range input.Blocks.Table.Rows {
		fields = append(fields, row...)
	}
	for _, metric := range input.Blocks.Metrics {
		fields = append(fields, metric.Name, metric.ValueText)
	}
	for _, field := range fields {
		if hasPresentationMarkup(field) {
			return errors.New("presentation markup is refused before render")
		}
	}
	return nil
}

func hasPresentationMarkup(value string) bool {
	lower := strings.ToLower(value)
	needles := []string{
		"<script", "</script", "<style", "</style", "<html", "<body", "<div", "<span",
		"<link", "<iframe", "<img", "javascript:", "onclick=", "onerror=", "onload=",
	}
	for _, needle := range needles {
		if strings.Contains(lower, needle) {
			return true
		}
	}
	return false
}
