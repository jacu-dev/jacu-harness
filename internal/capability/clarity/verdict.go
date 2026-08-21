package clarity

import "github.com/jacu-dev/jacu-harness/internal/capability/sdd"

type Report struct {
	Verdict         string   `json:"verdict"`
	Round           int      `json:"round,omitempty"`
	Divergences     int      `json:"divergences"`
	DivergenceField string   `json:"divergence_field,omitempty"`
	VarianceRuns    int      `json:"variance_runs,omitempty"`
	SpecBytes       int64    `json:"spec_bytes,omitempty"`
	SpecBytesDelta  int64    `json:"spec_bytes_delta,omitempty"`
	Findings        []Error  `json:"findings,omitempty"`
	Readback        Readback `json:"readback,omitempty"`
}

func Evaluate(document sdd.Document, specBytes, previousSpecBytes int64, round int, readbacks []Readback) Report {
	report := Report{Verdict: "pass", Round: round, SpecBytes: specBytes}
	if delta, err := SpecBytesDelta(previousSpecBytes, specBytes); err != nil {
		report.Verdict = "fail"
		report.SpecBytesDelta = delta
		report.Findings = []Error{{Code: CodeSpecGrew}}
		return report
	} else {
		report.SpecBytesDelta = delta
	}
	if len(readbacks) == 0 {
		report.Verdict = "fail"
		report.Findings = []Error{{Code: CodeMalformed}}
		return report
	}
	if len(readbacks) == 1 {
		report.Readback = Normalize(readbacks[0])
		div := Diverge(document, readbacks[0])
		report.Divergences = len(div)
		if len(div) > 0 {
			report.Verdict = "fail"
			report.DivergenceField = div[0].Field
			report.Findings = divergencesToErrors(div)
		}
		return report
	}
	variance := CompareRuns(readbacks)
	report.VarianceRuns = variance.Runs
	if variance.Disagree {
		report.Verdict = "fail"
		report.DivergenceField = variance.Field
		report.Findings = []Error{{Code: CodeVariance, Field: variance.Field}}
		return report
	}
	div := Diverge(document, readbacks[0])
	report.Divergences = len(div)
	if len(div) > 0 {
		report.Verdict = "fail"
		report.DivergenceField = div[0].Field
		report.Findings = divergencesToErrors(div)
	}
	return report
}

func divergencesToErrors(div []Divergence) []Error {
	out := make([]Error, 0, len(div))
	for _, item := range div {
		out = append(out, Error{Code: CodeDivergence, Field: item.Field, Path: item.Path})
	}
	return out
}
