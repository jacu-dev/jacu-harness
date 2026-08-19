package preflight

import "testing"

func TestAssembleBatchAsksOnceAndNeverPartiallyDispatches(t *testing.T) {
	report := Report{
		Verdict: "block",
		Findings: []Finding{
			{Class: ClassAllowlist, Target: "tool"},
			{Class: ClassPathMissing, Target: "input"},
			{Class: ClassCredentialAbsent, Target: "credential"},
		},
	}
	batch := AssembleBatch(report)
	if len(batch.Findings) != 3 || batch.Dispatch {
		t.Fatalf("batch lost gaps or dispatched partially: %+v", batch)
	}
	clean := AssembleBatch(Report{Verdict: "pass", Findings: []Finding{}})
	if len(clean.Findings) != 0 || !clean.Dispatch {
		t.Fatalf("clean report did not dispatch: %+v", clean)
	}
}
