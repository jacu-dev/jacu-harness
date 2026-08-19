package preflight

import "sort"

type Batch struct {
	Findings []Finding `json:"findings"`
	Dispatch bool      `json:"dispatch"`
}

func AssembleBatch(report Report) Batch {
	findings := append([]Finding{}, report.Findings...)
	sort.Slice(findings, func(i, j int) bool {
		if findings[i].Class != findings[j].Class {
			return findings[i].Class < findings[j].Class
		}
		return findings[i].Target < findings[j].Target
	})
	return Batch{Findings: findings, Dispatch: report.Verdict == "pass" && len(findings) == 0}
}
