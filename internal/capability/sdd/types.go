package sdd

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

type normalizedDocument struct {
	FrontMatter  map[string]string `json:"front_matter,omitempty"`
	Sections     []Section         `json:"sections,omitempty"`
	Requirements []Requirement     `json:"requirements,omitempty"`
	AllowedPaths []string          `json:"allowed_paths,omitempty"`
	Forbidden    []string          `json:"forbidden_paths,omitempty"`
}

func normalizeDocument(document Document) normalizedDocument {
	frontMatter := make(map[string]string, len(document.FrontMatter))
	for key, value := range document.FrontMatter {
		frontMatter[strings.TrimSpace(key)] = normalizeText(value)
	}
	seenSections := make(map[string]struct{}, len(document.Sections))
	sections := make([]Section, 0, len(document.Sections))
	for _, section := range document.Sections {
		normalized := Section{Level: section.Level, Name: normalizeText(section.Name), Lines: normalizeStrings(section.Lines)}
		key := sectionKey(normalized)
		if _, exists := seenSections[key]; exists {
			continue
		}
		seenSections[key] = struct{}{}
		sections = append(sections, normalized)
	}
	sort.Slice(sections, func(left, right int) bool {
		return sectionKey(sections[left]) < sectionKey(sections[right])
	})
	requirements := make([]Requirement, 0, len(document.Requirements))
	seenRequirements := make(map[string]struct{})
	for _, requirement := range document.Requirements {
		normalized := Requirement{Name: normalizeText(requirement.Name), Text: normalizeText(requirement.Text), Delta: strings.ToUpper(normalizeText(requirement.Delta))}
		seenScenarios := make(map[string]struct{}, len(requirement.Scenarios))
		for _, scenario := range requirement.Scenarios {
			normalizedScenario := Scenario{Name: normalizeText(scenario.Name), When: normalizeText(scenario.When), Then: normalizeText(scenario.Then)}
			encoded, _ := json.Marshal(normalizedScenario)
			if _, exists := seenScenarios[string(encoded)]; exists {
				continue
			}
			seenScenarios[string(encoded)] = struct{}{}
			normalized.Scenarios = append(normalized.Scenarios, normalizedScenario)
		}
		sort.Slice(normalized.Scenarios, func(left, right int) bool {
			return scenarioKey(normalized.Scenarios[left]) < scenarioKey(normalized.Scenarios[right])
		})
		key := normalizedRequirementKey(normalized)
		if _, exists := seenRequirements[key]; exists {
			continue
		}
		seenRequirements[key] = struct{}{}
		requirements = append(requirements, normalized)
	}
	sort.Slice(requirements, func(left, right int) bool {
		return normalizedRequirementKey(requirements[left]) < normalizedRequirementKey(requirements[right])
	})
	return normalizedDocument{
		FrontMatter:  frontMatter,
		Sections:     sections,
		Requirements: requirements,
		AllowedPaths: normalizeStrings(document.AllowedPaths),
		Forbidden:    normalizeStrings(document.ForbiddenPaths),
	}
}

func sddID(document Document) string {
	encoded, _ := json.Marshal(normalizeDocument(document))
	digest := sha256.Sum256(encoded)
	return "sdd_" + fmt.Sprintf("%x", digest[:8])
}

// SDDID exposes the deterministic identity for callers outside this package.
func SDDID(document Document) string {
	return sddID(document)
}

func normalizeStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	unique := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = normalizeText(value)
		if value != "" {
			unique[value] = struct{}{}
		}
	}
	result := make([]string, 0, len(unique))
	for value := range unique {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func normalizeText(value string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
}

func normalizedRequirementKey(requirement Requirement) string {
	encoded, _ := json.Marshal(requirement)
	return string(encoded)
}

func scenarioKey(scenario Scenario) string {
	encoded, _ := json.Marshal(scenario)
	return string(encoded)
}

func sectionKey(section Section) string {
	encoded, _ := json.Marshal(section)
	return string(encoded)
}
