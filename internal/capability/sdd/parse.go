package sdd

import (
	"bytes"
	"strings"
)

// Document is the structural view of one hand-authored sdd.md. It deliberately
// keeps prose as text; the linter, not the parser, decides whether prose is
// complete or normative.
type Document struct {
	FrontMatter    map[string]string
	Sections       []Section
	Requirements   []Requirement
	AllowedPaths   []string
	ForbiddenPaths []string
	Raw            []byte
}

type Section struct {
	Level int
	Name  string
	Lines []string
}

type Requirement struct {
	Name      string
	Text      string
	Delta     string
	Scenarios []Scenario
}

type Scenario struct {
	Name string
	When string
	Then string
}

// Parse extracts markdown structure without interpreting arbitrary prose as
// syntax. Malformed front matter or markdown is retained as best-effort text.
func Parse(content []byte) (Document, error) {
	document := Document{
		FrontMatter: make(map[string]string),
		Raw:         append([]byte(nil), content...),
	}
	lines := splitLines(content)
	bodyStart := parseFrontMatter(lines, document.FrontMatter)

	var currentSection *Section
	var currentRequirement *Requirement
	var currentScenario *Scenario
	codeFence := false
	for _, rawLine := range lines[bodyStart:] {
		line := strings.TrimRight(rawLine, "\r")
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			codeFence = !codeFence
			continue
		}
		if codeFence {
			appendSectionLine(currentSection, line)
			continue
		}

		if level, name, ok := markdownHeading(trimmed); ok {
			section := Section{Level: level, Name: name}
			document.Sections = append(document.Sections, section)
			currentSection = &document.Sections[len(document.Sections)-1]
			currentRequirement = nil
			currentScenario = nil
			if level == 3 && strings.HasPrefix(name, "Requirement:") {
				requirement := Requirement{Name: strings.TrimSpace(strings.TrimPrefix(name, "Requirement:"))}
				document.Requirements = append(document.Requirements, requirement)
				currentRequirement = &document.Requirements[len(document.Requirements)-1]
			} else if level == 4 && strings.HasPrefix(name, "Scenario:") && len(document.Requirements) > 0 {
				currentRequirement = &document.Requirements[len(document.Requirements)-1]
				currentRequirement.Scenarios = append(currentRequirement.Scenarios, Scenario{Name: strings.TrimSpace(strings.TrimPrefix(name, "Scenario:"))})
				currentScenario = &currentRequirement.Scenarios[len(currentRequirement.Scenarios)-1]
			}
			continue
		}

		appendSectionLine(currentSection, line)
		if currentRequirement == nil {
			continue
		}
		if currentScenario != nil {
			if value, ok := prefixedValue(trimmed, "- **WHEN**"); ok {
				currentScenario.When = value
				continue
			}
			if value, ok := prefixedValue(trimmed, "- **THEN**"); ok {
				currentScenario.Then = value
				continue
			}
		}
		if delta := strings.TrimSpace(trimmed); delta == "Delta: ADDED" || delta == "Delta: MODIFIED" || delta == "Delta: REMOVED" {
			currentRequirement.Delta = strings.TrimSpace(strings.TrimPrefix(delta, "Delta:"))
			continue
		}
		if trimmed != "" && !strings.HasPrefix(trimmed, "####") {
			currentRequirement.Text = strings.TrimSpace(strings.Join([]string{currentRequirement.Text, trimmed}, " "))
		}
	}
	return document, nil
}

func splitLines(content []byte) []string {
	rawLines := bytes.Split(content, []byte{'\n'})
	lines := make([]string, len(rawLines))
	for index, rawLine := range rawLines {
		lines[index] = string(rawLine)
	}
	return lines
}

func parseFrontMatter(lines []string, target map[string]string) int {
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return 0
	}
	for index := 1; index < len(lines); index++ {
		line := strings.TrimSpace(lines[index])
		if line == "---" {
			return index + 1
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok || strings.TrimSpace(key) == "" {
			continue
		}
		target[strings.TrimSpace(key)] = strings.Trim(strings.TrimSpace(value), "\"'")
	}
	return 1
}

func markdownHeading(line string) (int, string, bool) {
	level := 0
	for level < len(line) && line[level] == '#' {
		level++
	}
	if level == 0 || level > 6 || level == len(line) || line[level] != ' ' {
		return 0, "", false
	}
	return level, strings.TrimSpace(line[level+1:]), true
}

func prefixedValue(line, prefix string) (string, bool) {
	if !strings.HasPrefix(line, prefix) {
		return "", false
	}
	return strings.TrimSpace(strings.TrimPrefix(line, prefix)), true
}

func appendSectionLine(section *Section, line string) {
	if section != nil {
		section.Lines = append(section.Lines, line)
	}
}
