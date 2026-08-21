package export

import (
	"strings"
	"testing"
)

func TestPlanSubjectsAreConventionalEnglish(t *testing.T) {
	t.Parallel()
	if err := Validate(); err != nil {
		t.Fatal(err)
	}
	plan := Plan()
	if len(plan) == 0 {
		t.Fatal("curated import plan is empty")
	}
	seen := map[string]struct{}{}
	for _, commit := range plan {
		if strings.TrimSpace(commit.Subject) == "" || strings.TrimSpace(commit.Area) == "" {
			t.Fatalf("empty subject or area: %+v", commit)
		}
		if _, ok := seen[commit.Subject]; ok {
			t.Fatalf("duplicate subject %q", commit.Subject)
		}
		seen[commit.Subject] = struct{}{}
	}
	if !strings.Contains(Author, "ecouto123@gmail.com") {
		t.Fatalf("author %q lost the provenance identity", Author)
	}
}
