package sdd

import "testing"

func TestSDDIDIgnoresWhitespaceListOrderAndDuplicates(t *testing.T) {
	left := Document{
		FrontMatter:  map[string]string{"sdd": "001-example", "status": "draft"},
		AllowedPaths: []string{" docs/b ", "docs/a", "docs/a"},
		Requirements: []Requirement{{Name: "Safe parsing", Text: "The system SHALL parse."}},
	}
	right := Document{
		FrontMatter:  map[string]string{"status": " draft ", "sdd": "001-example"},
		AllowedPaths: []string{"docs/a", "docs/b"},
		Requirements: []Requirement{{Name: " Safe parsing ", Text: " The system SHALL parse. "}},
	}
	if leftID, rightID := sddID(left), sddID(right); leftID != rightID {
		t.Fatalf("sddID differs for normalized documents: %q != %q", leftID, rightID)
	}
}

func TestSDDIDChangesWhenContentChanges(t *testing.T) {
	left := Document{FrontMatter: map[string]string{"sdd": "001-example"}, Requirements: []Requirement{{Name: "Safe parsing", Text: "The system SHALL parse."}}}
	right := Document{FrontMatter: map[string]string{"sdd": "001-example"}, Requirements: []Requirement{{Name: "Safe parsing", Text: "The system SHALL reject."}}}
	if sddID(left) == sddID(right) {
		t.Fatal("sddID did not change after normative content changed")
	}
}

func TestSDDIDIgnoresRequirementOrderAndDuplicateScenarios(t *testing.T) {
	left := Document{Requirements: []Requirement{
		{Name: "B", Text: "The system SHALL b.", Scenarios: []Scenario{{Name: "second", When: "b", Then: "b"}, {Name: "first", When: "a", Then: "a"}}},
		{Name: "A", Text: "The system SHALL a."},
		{Name: "A", Text: "The system SHALL a."},
	}}
	right := Document{Requirements: []Requirement{
		{Name: "A", Text: "The system SHALL a."},
		{Name: "B", Text: "The system SHALL b.", Scenarios: []Scenario{{Name: "first", When: "a", Then: "a"}, {Name: "second", When: "b", Then: "b"}, {Name: "first", When: "a", Then: "a"}}},
	}}
	if leftID, rightID := sddID(left), sddID(right); leftID != rightID {
		t.Fatalf("sddID differs for normalized requirement lists: %q != %q", leftID, rightID)
	}
}

func TestSDDIDIgnoresSectionOrderAndDuplicateLines(t *testing.T) {
	left := Document{Sections: []Section{
		{Level: 2, Name: "Why", Lines: []string{"second", "first", "first"}},
		{Level: 2, Name: "Tasks", Lines: []string{"task"}},
	}}
	right := Document{Sections: []Section{
		{Level: 2, Name: "Tasks", Lines: []string{"task"}},
		{Level: 2, Name: "Why", Lines: []string{"first", "second"}},
	}}
	if leftID, rightID := sddID(left), sddID(right); leftID != rightID {
		t.Fatalf("sddID differs for normalized section lists: %q != %q", leftID, rightID)
	}
}
