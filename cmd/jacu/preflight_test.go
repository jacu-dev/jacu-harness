package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPreflightCLIExitCodesAndJSON(t *testing.T) {
	root := t.TempDir()

	passCode, passOut, passErr := runPreflightCapture(t, root, "--json")
	if passCode != 0 {
		t.Fatalf("pass exit code = %d; want 0; stderr=%q", passCode, passErr)
	}
	if passErr != "" {
		t.Fatalf("pass wrote stderr: %q", passErr)
	}
	var pass map[string]any
	if err := json.Unmarshal([]byte(passOut), &pass); err != nil {
		t.Fatalf("pass stdout is not JSON: %v; output=%q", err, passOut)
	}
	if pass["verdict"] != "pass" {
		t.Fatalf("pass verdict = %#v; want pass", pass["verdict"])
	}

	blockCode, blockOut, blockErr := runPreflightCapture(t, root, "--json", "--command", "not-allowlisted")
	if blockCode != 1 {
		t.Fatalf("block exit code = %d; want 1; stderr=%q", blockCode, blockErr)
	}
	if blockErr != "" {
		t.Fatalf("block wrote stderr: %q", blockErr)
	}
	var blocked map[string]any
	if err := json.Unmarshal([]byte(blockOut), &blocked); err != nil {
		t.Fatalf("block stdout is not JSON: %v; output=%q", err, blockOut)
	}
	if blocked["verdict"] != "block" {
		t.Fatalf("block verdict = %#v; want block", blocked["verdict"])
	}

	usageCode, usageOut, usageErr := runPreflightCapture(t, root, "--unknown")
	if usageCode != 2 {
		t.Fatalf("invalid usage exit code = %d; want 2; stdout=%q stderr=%q", usageCode, usageOut, usageErr)
	}
	if !strings.Contains(usageErr, "usage") {
		t.Fatalf("invalid usage did not report usage: %q", usageErr)
	}
}

func TestPreflightCLIResolvesAuthorizedCommandEnvironment(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".jacu"), 0o700); err != nil {
		t.Fatal(err)
	}
	policy := []byte(`{"allow":[{"program":"go"}]}`)
	if err := os.WriteFile(filepath.Join(root, ".jacu", "verify-allowlist.json"), policy, 0o600); err != nil {
		t.Fatal(err)
	}

	code, output, stderr := runPreflightCapture(t, root, "--json", "--command", "go")
	if code != 0 || stderr != "" {
		t.Fatalf("authorized command exit = %d, stderr=%q, output=%q; want pass", code, stderr, output)
	}
	var report map[string]any
	if err := json.Unmarshal([]byte(output), &report); err != nil {
		t.Fatal(err)
	}
	if report["verdict"] != "pass" {
		t.Fatalf("authorized command verdict = %#v; want pass", report["verdict"])
	}
}

func TestPreflightCLIAcceptsStructuredArgvAndNetworkDeclaration(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".jacu"), 0o700); err != nil {
		t.Fatal(err)
	}
	policy := []byte(`{"allow":[{"program":"go","required_arg_prefix":["test"]}]}`)
	if err := os.WriteFile(filepath.Join(root, ".jacu", "verify-allowlist.json"), policy, 0o600); err != nil {
		t.Fatal(err)
	}

	code, output, stderr := runPreflightCapture(t, root, "--json", "--command-argv", `["go","test","./..."]`, "--network-required", "--network-declared")
	if code != 0 || stderr != "" {
		t.Fatalf("structured argv exit = %d, stderr=%q, output=%q; want pass", code, stderr, output)
	}
}

func TestPreflightCLINetworkDeclarationTruthTable(t *testing.T) {
	root := t.TempDir()
	for _, testCase := range []struct {
		name                string
		args                []string
		wantCode, wantCount int
	}{
		{name: "not required not declared", args: []string{"--json"}, wantCode: 0},
		{name: "not required declared", args: []string{"--json", "--network-declared"}, wantCode: 0},
		{name: "required not declared", args: []string{"--json", "--network-required"}, wantCode: 1, wantCount: 1},
		{name: "required declared", args: []string{"--json", "--network-required", "--network-declared"}, wantCode: 0},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			code, output, stderr := runPreflightCapture(t, root, testCase.args...)
			if code != testCase.wantCode || stderr != "" {
				t.Fatalf("exit=%d stderr=%q output=%q; want exit=%d", code, stderr, output, testCase.wantCode)
			}
			var report struct {
				Findings []struct {
					Class string `json:"class"`
				} `json:"findings"`
			}
			if err := json.Unmarshal([]byte(output), &report); err != nil {
				t.Fatal(err)
			}
			count := 0
			for _, finding := range report.Findings {
				if finding.Class == "network_undeclared" {
					count++
				}
			}
			if count != testCase.wantCount {
				t.Fatalf("network_undeclared count=%d, want %d: %s", count, testCase.wantCount, output)
			}
		})
	}
}

func runPreflightCapture(t *testing.T, root string, args ...string) (int, string, string) {
	t.Helper()
	stdout, err := os.CreateTemp(t.TempDir(), "stdout")
	if err != nil {
		t.Fatal(err)
	}
	stderr, err := os.CreateTemp(t.TempDir(), "stderr")
	if err != nil {
		t.Fatal(err)
	}
	if code := runPreflight(root, args, stdout, stderr); true {
		return code, readTemp(t, stdout), readTemp(t, stderr)
	}
	return 2, "", ""
}

func readTemp(t *testing.T, file *os.File) string {
	t.Helper()
	if _, err := file.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	var buffer bytes.Buffer
	if _, err := buffer.ReadFrom(file); err != nil {
		t.Fatal(err)
	}
	return buffer.String()
}
