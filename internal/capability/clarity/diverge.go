package clarity

import (
	"github.com/jacu-dev/jacu-harness/internal/capability/sdd"
	"github.com/jacu-dev/jacu-harness/internal/scope"
)

type Divergence struct {
	Field string
	Path  string
}

func Diverge(document sdd.Document, readback Readback) []Divergence {
	expected := Expected(document)
	readback = Normalize(readback)
	findings := make([]Divergence, 0)
	findings = append(findings, listDivergences(FieldWriteScope, expected.WriteScope, readback.WriteScope, true)...)
	findings = append(findings, listDivergences(FieldForbidden, expected.ForbiddenPaths, readback.ForbiddenPaths, false)...)
	findings = append(findings, listDivergences(FieldRequirements, expected.Requirements, readback.Requirements, false)...)
	findings = append(findings, listDivergences(FieldOutOfScope, expected.OutOfScope, readback.OutOfScope, false)...)
	findings = append(findings, listDivergences(FieldTasks, expected.Tasks, readback.Tasks, false)...)
	return findings
}

func listDivergences(field string, expected, actual []string, paths bool) []Divergence {
	findings := make([]Divergence, 0)
	expectedSet := toSet(expected)
	actualSet := toSet(actual)
	for _, value := range actual {
		if expectedSet[value] {
			continue
		}
		if paths && !scope.ScopesConflict(value, expected, nil) {
			continue
		}
		findings = append(findings, Divergence{Field: field, Path: value})
	}
	for _, value := range expected {
		if actualSet[value] {
			continue
		}
		if paths {
			covered := false
			for _, got := range actual {
				if !scope.ScopesConflict(got, []string{value}, nil) {
					covered = true
					break
				}
			}
			if covered {
				continue
			}
		}
		findings = append(findings, Divergence{Field: field, Path: value})
	}
	return findings
}

func toSet(values []string) map[string]bool {
	out := make(map[string]bool, len(values))
	for _, value := range values {
		out[value] = true
	}
	return out
}
