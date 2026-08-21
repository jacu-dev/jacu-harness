package main

import (
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/jacu-dev/jacu-harness/internal/mcpadapter"
)

func TestDoctorReportsVersions(t *testing.T) {
	gitOutput, err := exec.Command("git", "--version").Output()
	if err != nil {
		t.Fatalf("git --version: %v", err)
	}
	cmd := exec.Command("go", "run", "-buildvcs=false", ".", "doctor")
	cmd.Env = isolatedUserStateEnv(t)
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("doctor: %v", err)
	}

	for _, want := range []string{
		"jacu dev",
		runtime.Version(),
		strings.TrimSpace(string(gitOutput)),
		"2026-07-28",
		"2025-11-25",
	} {
		if !strings.Contains(string(output), want) {
			t.Fatalf("doctor output = %q; missing %q", output, want)
		}
	}
}

func TestDoctorEmitClaudeDesktopWithoutRepoFails(t *testing.T) {
	cmd := exec.Command("go", "run", "-buildvcs=false", ".", "doctor", "--emit", "claude-desktop")
	cmd.Env = isolatedUserStateEnv(t)
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("doctor --emit claude-desktop without --repo exited 0:\n%s", output)
	}
	if !strings.Contains(string(output), "--repo") {
		t.Fatalf("refusal must name --repo:\n%s", output)
	}
}

func TestDoctorEmitClaudeDesktopAnchorsCwd(t *testing.T) {
	repo := filepath.Join(t.TempDir(), "example-repo")
	cmd := exec.Command("go", "run", "-buildvcs=false", ".", "doctor", "--emit", "claude-desktop", "--repo", repo) // #nosec G204 -- go run with a TempDir --repo path
	cmd.Env = isolatedUserStateEnv(t)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("doctor --emit claude-desktop --repo: %v\n%s", err, output)
	}
	abs, err := filepath.Abs(repo)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(output), "cd '"+abs+"'") || !strings.Contains(string(output), "exec jacu serve") {
		t.Fatalf("pack does not anchor cwd:\n%s", output)
	}
}

func TestSupportedProtocolVersions(t *testing.T) {
	if len(mcpadapter.SupportedProtocolVersions) == 0 {
		t.Fatal("SupportedProtocolVersions must not be empty")
	}
	if !slices.Contains(mcpadapter.SupportedProtocolVersions, "2026-07-28") {
		t.Fatal("SupportedProtocolVersions must contain 2026-07-28")
	}
}
