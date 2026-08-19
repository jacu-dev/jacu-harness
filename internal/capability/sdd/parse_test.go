package sdd

import (
	"bytes"
	"testing"
)

func TestParseSectionsRequirementsAndScenarios(t *testing.T) {
	document, err := Parse([]byte(`---
sdd: 001-example
branch: 001-example
---
# Example
## Requirements
### Requirement: Safe parsing
The system SHALL parse a document without panicking.
#### Scenario: Valid markdown
- **WHEN** markdown has a requirement
- **THEN** the parser returns its scenario
Delta: ADDED
`))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if document.FrontMatter["sdd"] != "001-example" {
		t.Fatalf("sdd = %q; want 001-example", document.FrontMatter["sdd"])
	}
	if len(document.Requirements) != 1 {
		t.Fatalf("requirements = %d; want 1", len(document.Requirements))
	}
	requirement := document.Requirements[0]
	if requirement.Name != "Safe parsing" || len(requirement.Scenarios) != 1 {
		t.Fatalf("requirement = %#v", requirement)
	}
	scenario := requirement.Scenarios[0]
	if scenario.When != "markdown has a requirement" || scenario.Then != "the parser returns its scenario" {
		t.Fatalf("scenario = %#v", scenario)
	}
}

func TestParseMalformedMarkdownReturnsDocumentOrErrorButNeverPanics(t *testing.T) {
	inputs := [][]byte{
		nil,
		[]byte("---\n: not yaml\n---\n### Requirement:\n#### Scenario:\n- **WHEN**\n"),
		[]byte("# unclosed [link\n```\n### Requirement: inside code\n"),
	}
	for _, input := range inputs {
		func() {
			defer func() {
				if recovered := recover(); recovered != nil {
					t.Fatalf("Parse() panicked for %q: %v", input, recovered)
				}
			}()
			_, _ = Parse(input)
		}()
	}
}

func TestParseHandlesOneMiBSingleLine(t *testing.T) {
	content := append([]byte("# "), bytes.Repeat([]byte{'x'}, (1<<20)-2)...)
	document, err := Parse(content)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(document.Sections) != 1 {
		t.Fatalf("sections = %d; want one section for a 1 MiB line", len(document.Sections))
	}
}
