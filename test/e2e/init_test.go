//go:build e2e

package e2e

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitConfiguresEachNamedHostThroughTheShippedBinary(t *testing.T) {
	bin := serverBinary(t)
	hosts := []string{"claude-code", "codex", "opencode", "cursor", "claude-desktop", "generic"}
	for _, host := range hosts {
		host := host
		t.Run(host, func(t *testing.T) {
			home := t.TempDir()
			skills := filepath.Join(home, "skills")
			config := filepath.Join(home, "config")
			args := []string{"init", "--host", host, "--skills-dir", skills, "--config", config, "--json"}
			if host == "claude-desktop" {
				args = append(args, "--repo", filepath.Join(home, "repo"))
			}
			cmd := exec.Command(bin, args...)
			cmd.Env = []string{"HOME=" + home, "PATH=" + os.Getenv("PATH"), "JACU_HOME=" + home}
			var stdout, stderr strings.Builder
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr
			if err := cmd.Run(); err != nil {
				t.Fatalf("jacu init --host %s: %v\nstdout=%s\nstderr=%s", host, err, stdout.String(), stderr.String())
			}
			var result map[string]any
			if err := json.Unmarshal([]byte(stdout.String()), &result); err != nil {
				t.Fatalf("init --json for %s is not exclusive JSON: %v\n%s", host, err, stdout.String())
			}
			if result["host"] != host || result["config"] != "written" {
				t.Fatalf("init %s json = %#v", host, result)
			}
			if _, err := os.Stat(filepath.Join(skills, "using-jacu", "SKILL.md")); err != nil {
				t.Fatalf("init %s did not install skills: %v", host, err)
			}
			raw, err := os.ReadFile(config)
			if err != nil {
				t.Fatalf("init %s did not write named config: %v", host, err)
			}
			if host == "claude-desktop" {
				if !strings.Contains(string(raw), "cd '") || !strings.Contains(string(raw), "exec jacu serve") {
					t.Fatalf("claude-desktop pack is not cwd-anchored:\n%s", raw)
				}
			} else if !strings.Contains(string(raw), "jacu") {
				t.Fatalf("init %s pack missing jacu:\n%s", host, raw)
			}
		})
	}
}

func TestInitPrintsSnippetInsteadOfTouchingAnUnnamedHostConfig(t *testing.T) {
	bin := serverBinary(t)
	home := t.TempDir()
	skills := filepath.Join(home, "skills")
	cmd := exec.Command(bin, "init", "--host", "cursor", "--skills-dir", skills)
	cmd.Env = []string{"HOME=" + home, "PATH=" + os.Getenv("PATH"), "JACU_HOME=" + home}
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("jacu init: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), ".cursor/mcp.json") {
		t.Fatalf("stdout missing exact target path:\n%s", out)
	}
	if _, err := os.Stat(filepath.Join(home, ".cursor", "mcp.json")); err == nil {
		t.Fatal("init edited ~/.cursor/mcp.json without --config")
	}
}

func TestInitClaudeDesktopWithoutRepoFailsThroughTheShippedBinary(t *testing.T) {
	bin := serverBinary(t)
	cmd := exec.Command(bin, "init", "--host", "claude-desktop", "--skills-dir", t.TempDir())
	cmd.Env = []string{"HOME=" + t.TempDir(), "PATH=" + os.Getenv("PATH"), "JACU_HOME=" + t.TempDir()}
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("init accepted claude-desktop without --repo:\n%s", out)
	}
	if !strings.Contains(string(out), "--repo") {
		t.Fatalf("refusal must name --repo:\n%s", out)
	}
}
