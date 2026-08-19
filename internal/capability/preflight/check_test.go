package preflight

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jacu-dev/jacu-harness/internal/runstate"
)

func TestCheckClassifiesEveryPredictableInterruption(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".jacu"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".jacu", "verify-allowlist.json"), []byte(`{"allow":[{"program":"go","required_arg_prefix":["test"]}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "readonly.txt"), []byte("fixture"), 0o400); err != nil {
		t.Fatal(err)
	}
	env := Environment{
		Root:            root,
		Path:            []string{},
		Credentials:     map[string]bool{"present": true},
		WritablePaths:   map[string]bool{},
		NetworkDeclared: false,
		RequiredNetwork: true,
	}
	cases := []struct {
		name    string
		mission runstate.MissionSnapshot
		want    string
	}{
		{name: "allowlist", mission: runstate.MissionSnapshot{VerificationCommands: [][]string{{"not-allowlisted"}}}, want: "allowlist"},
		{name: "program_not_on_path", mission: runstate.MissionSnapshot{VerificationCommands: [][]string{{"go", "test"}}}, want: "program_not_on_path"},
		{name: "path_missing", mission: runstate.MissionSnapshot{Objective: "path:missing.txt"}, want: "path_missing"},
		{name: "path_not_writable", mission: runstate.MissionSnapshot{AllowedPaths: []string{"readonly.txt"}}, want: "path_not_writable"},
		{name: "credential_absent", mission: runstate.MissionSnapshot{Objective: "credential:missing"}, want: "credential_absent"},
		{name: "network_undeclared", mission: runstate.MissionSnapshot{}, want: "network_undeclared"},
		{name: "doc_missing", mission: runstate.MissionSnapshot{Objective: "doc:docs/missing.md"}, want: "doc_missing"},
		{name: "open_questions", mission: runstate.MissionSnapshot{Program: &runstate.Program{OpenQuestions: []string{"which credential?"}}}, want: "open_questions"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			report := Check(tc.mission, env)
			if report.Verdict != "block" {
				t.Fatalf("verdict = %q, want block: %+v", report.Verdict, report)
			}
			if !hasClass(report, tc.want) {
				t.Fatalf("missing class %q in %+v", tc.want, report)
			}
		})
	}
}

func TestCheckFailsClosedWhenEnvironmentCannotResolve(t *testing.T) {
	mission := runstate.MissionSnapshot{VerificationCommands: [][]string{{"go", "test"}}}
	report := Check(mission, Environment{})
	if report.Verdict != "block" || !hasClass(report, "program_not_on_path") {
		t.Fatalf("unresolved environment passed: %+v", report)
	}
}

func hasClass(report Report, want string) bool {
	for _, finding := range report.Findings {
		if finding.Class == want {
			return true
		}
	}
	return false
}
