// Package provenance classifies authorship traces, host references, and policy text.
package provenance

type Kind string

const (
	KindAIAuthor               Kind = "ai_author"
	KindAITrailer              Kind = "ai_trailer"
	KindAIEmail                Kind = "ai_email"
	KindGeneratedWith          Kind = "generated_with"
	KindRobotEmoji             Kind = "robot_emoji"
	KindNonEnglishSubject      Kind = "non_english_subject"
	KindNonConventionalSubject Kind = "non_conventional_subject"
)

type Class string

const (
	ClassTrace   Class = "trace"
	ClassProduct Class = "product"
	ClassPolicy  Class = "policy"
)

type Finding struct {
	Kind    Kind   `json:"kind"`
	Class   Class  `json:"class"`
	Rule    string `json:"rule"`
	Path    string `json:"path,omitempty"`
	Line    int    `json:"line,omitempty"`
	Commit  string `json:"commit,omitempty"`
	Excerpt string `json:"excerpt,omitempty"`
}

type Gap struct {
	Check  string `json:"check"`
	Reason string `json:"reason"`
}

type Report struct {
	Findings []Finding `json:"findings"`
	Traces   int       `json:"traces"`
	Products int       `json:"products"`
	Policies int       `json:"policies"`
	Gaps     []Gap     `json:"gaps"`
}

func (r Report) Clean() bool {
	return r.Traces == 0 && len(r.Gaps) == 0
}

func (r *Report) addFinding(finding Finding) {
	r.Findings = append(r.Findings, finding)
	switch finding.Class {
	case ClassTrace:
		r.Traces++
	case ClassProduct:
		r.Products++
	case ClassPolicy:
		r.Policies++
	}
}
