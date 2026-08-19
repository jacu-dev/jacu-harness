package preflight

import (
	"fmt"
	"sort"

	"github.com/jacu-dev/jacu-harness/internal/runstate"
)

const (
	ClassAllowlist         = "allowlist"
	ClassProgramNotOnPath  = "program_not_on_path"
	ClassPathMissing       = "path_missing"
	ClassPathNotWritable   = "path_not_writable"
	ClassCredentialAbsent  = "credential_absent" //nolint:gosec // class identifier, not a credential
	ClassNetworkUndeclared = "network_undeclared"
	ClassDocMissing        = "doc_missing"
	ClassOpenQuestions     = "open_questions"
)

var classes = map[string]struct{}{
	ClassAllowlist: {}, ClassProgramNotOnPath: {}, ClassPathMissing: {},
	ClassPathNotWritable: {}, ClassCredentialAbsent: {},
	ClassNetworkUndeclared: {}, ClassDocMissing: {}, ClassOpenQuestions: {},
}

// Environment is the resolved, presence-only view used by pre-flight. Path
// contains bare executable names, Credentials contains presence flags, and
// WritablePaths allows callers/tests to provide a deterministic write result.
type Environment struct {
	Root            string
	Path            []string
	Credentials     map[string]bool
	WritablePaths   map[string]bool
	NetworkDeclared bool
	RequiredNetwork bool
}

type Finding struct {
	Class  string `json:"class"`
	Target string `json:"target,omitempty"`
	Detail string `json:"detail,omitempty"`
}

type Report struct {
	Verdict  string    `json:"verdict"`
	Findings []Finding `json:"findings"`
}

func Check(mission runstate.MissionSnapshot, env Environment) Report {
	report := Report{Verdict: "pass", Findings: []Finding{}}
	add := func(class, target, detail string) {
		if _, ok := classes[class]; !ok {
			return
		}
		report.Findings = append(report.Findings, Finding{Class: class, Target: target, Detail: detail})
	}

	for _, argv := range mission.VerificationCommands {
		if !allowlistCheck(env.Root, argv) {
			add(ClassAllowlist, first(argv), "command not authorized or policy unresolved")
			continue
		}
		if len(argv) > 0 && !contains(env.Path, argv[0]) {
			add(ClassProgramNotOnPath, argv[0], "program presence unresolved")
		}
	}
	report.Findings = append(report.Findings, environmentFindings(mission, env)...)
	report.Findings = append(report.Findings, questionFindings(mission)...)

	sort.Slice(report.Findings, func(i, j int) bool {
		if report.Findings[i].Class != report.Findings[j].Class {
			return report.Findings[i].Class < report.Findings[j].Class
		}
		return report.Findings[i].Target < report.Findings[j].Target
	})
	if len(report.Findings) > 0 {
		report.Verdict = "block"
	}
	return report
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func first(argv []string) string {
	if len(argv) == 0 {
		return ""
	}
	return argv[0]
}

func (finding Finding) String() string {
	return fmt.Sprintf("%s:%s", finding.Class, finding.Target)
}
