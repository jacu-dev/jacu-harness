package hosteval

import (
	"strings"
	"testing"
)

func TestCatalogueRoundTripAcceptsAnIdenticalHostList(t *testing.T) {
	t.Parallel()
	tools := []ToolDesc{
		{Name: "jacu_project_inspect", Description: "Inspect."},
		{Name: "jacu_mission_compile", Description: "Compile."},
	}
	if err := CompareToolCatalogue(tools, tools); err != nil {
		t.Fatal(err)
	}
}

func TestTruncatingHostFailsTheMatrix(t *testing.T) {
	t.Parallel()
	err := CompareToolCatalogue(
		[]ToolDesc{{Name: "jacu_project_inspect", Description: "Inspect the repository."}},
		[]ToolDesc{{Name: "jacu_project_inspect", Description: "Inspect"}},
	)
	if err == nil {
		t.Fatal("truncated host catalogue must fail")
	}
	if !strings.Contains(err.Error(), "jacu_project_inspect") {
		t.Fatalf("error must name the tool: %v", err)
	}
	if !strings.Contains(err.Error(), "observed 7") {
		t.Fatalf("error must name the observed length: %v", err)
	}
}

func TestEmptyHostDescriptionFailsTheMatrix(t *testing.T) {
	t.Parallel()
	err := CompareToolCatalogue(
		[]ToolDesc{{Name: "jacu_status", Description: "Status."}},
		[]ToolDesc{{Name: "jacu_status", Description: ""}},
	)
	if err == nil {
		t.Fatal("empty host description must fail")
	}
	if !strings.Contains(err.Error(), "jacu_status") || !strings.Contains(err.Error(), "observed length 0") {
		t.Fatalf("error must name the tool and empty length: %v", err)
	}
}

func TestCatalogueFailsWhenHostOmitsATool(t *testing.T) {
	t.Parallel()
	err := CompareToolCatalogue(
		[]ToolDesc{
			{Name: "jacu_project_inspect", Description: "Inspect."},
			{Name: "jacu_verify", Description: "Run checks."},
		},
		[]ToolDesc{{Name: "jacu_project_inspect", Description: "Inspect."}},
	)
	if err == nil {
		t.Fatal("omitted tool must fail")
	}
	if !strings.Contains(err.Error(), "jacu_verify") {
		t.Fatalf("error must name the omitted tool: %v", err)
	}
}
