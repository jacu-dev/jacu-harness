package clarity

import "testing"

func TestVarianceFailsWhenRunsDisagreeEvenIfEachMatchesSpec(t *testing.T) {
	document := parseFixture(t)
	allowed := Expected(document).WriteScope
	if len(allowed) == 0 {
		t.Fatal("fixture has no write scope")
	}
	base := Expected(document)
	one := base
	one.WriteScope = []string{"internal/capability/context/pack.go"}
	two := base
	two.WriteScope = []string{"internal/capability/context/digest.go"}
	three := base
	three.WriteScope = []string{"internal/capability/context/anchor.go"}
	for i, readback := range []Readback{one, two, three} {
		if div := Diverge(document, readback); len(div) != 0 {
			t.Fatalf("run %d diverged against spec: %#v", i+1, div)
		}
	}
	variance := CompareRuns([]Readback{one, two, three})
	if !variance.Disagree || variance.Field != FieldWriteScope || variance.Runs != 3 {
		t.Fatalf("variance = %#v; want disagree on write_scope across 3 runs", variance)
	}
	report := Evaluate(document, int64(len(fixtureSDD)), int64(len(fixtureSDD)), 1, []Readback{one, two, three})
	if report.Verdict != "fail" || report.VarianceRuns != 3 || report.DivergenceField != FieldWriteScope {
		t.Fatalf("verdict = %#v", report)
	}
}
