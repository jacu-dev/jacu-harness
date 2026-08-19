package preflight

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jacu-dev/jacu-harness/internal/runstate"
)

func TestEnvironmentChecksArePresenceOnlyAndFailClosed(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "readonly.txt"), []byte("secret-looking fixture"), 0o400); err != nil {
		t.Fatal(err)
	}
	mission := runstate.MissionSnapshot{
		AllowedPaths: []string{"readonly.txt"},
		Objective:    "path:missing.txt credential:secret doc:docs/missing.md",
	}
	report := environmentFindings(mission, Environment{
		Root:            root,
		Credentials:     map[string]bool{},
		WritablePaths:   map[string]bool{"readonly.txt": false},
		RequiredNetwork: true,
	})
	for _, want := range []string{ClassPathNotWritable, ClassPathMissing, ClassCredentialAbsent, ClassNetworkUndeclared, ClassDocMissing} {
		if !hasClass(Report{Findings: report}, want) {
			t.Fatalf("missing environment class %q in %+v", want, report)
		}
	}
	for _, finding := range report {
		if finding.Detail == "secret-looking fixture" || finding.Target == "secret" {
			t.Fatalf("environment check leaked content: %+v", finding)
		}
	}
}

func TestNetworkDeclarationTruthTable(t *testing.T) {
	for _, testCase := range []struct {
		name, want         string
		required, declared bool
	}{
		{name: "not required not declared", required: false, declared: false},
		{name: "not required declared", required: false, declared: true},
		{name: "required not declared", required: true, declared: false, want: ClassNetworkUndeclared},
		{name: "required declared", required: true, declared: true},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			report := Check(runstate.MissionSnapshot{}, Environment{Root: t.TempDir(), RequiredNetwork: testCase.required, NetworkDeclared: testCase.declared})
			found := hasClass(report, ClassNetworkUndeclared)
			if (testCase.want != "") != found || len(report.Findings) != map[bool]int{true: 1, false: 0}[found] {
				t.Fatalf("network report = %+v, want class %q exactly once", report, testCase.want)
			}
		})
	}
}

func TestResolveEnvironmentUsesDeclaredPolicyPathDirs(t *testing.T) {
	root := t.TempDir()
	binDir := filepath.Join(root, "bin")
	if err := os.MkdirAll(filepath.Join(root, ".jacu", "bin"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(binDir, 0o700); err != nil {
		t.Fatal(err)
	}
	program := filepath.Join(binDir, "probe")
	if err := os.WriteFile(program, []byte("#!/bin/sh\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(program, 0o700); err != nil { //nolint:gosec // fixture must be executable for PATH resolution
		t.Fatal(err)
	}
	policy := []byte(`{"allow":[{"program":"probe"}],"path_dirs":["` + binDir + `"]}`)
	if err := os.WriteFile(filepath.Join(root, ".jacu", "verify-allowlist.json"), policy, 0o600); err != nil {
		t.Fatal(err)
	}
	env := ResolveEnvironment(root, runstate.MissionSnapshot{VerificationCommands: [][]string{{"probe"}}})
	if len(env.Path) != 1 || env.Path[0] != "probe" {
		t.Fatalf("resolved path = %#v; want declared policy path dir", env.Path)
	}
}
