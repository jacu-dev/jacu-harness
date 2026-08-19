package main

import (
	"strings"
	"testing"
)

func TestDoctorHostPacksAreDeterministicForAllSupportedHosts(t *testing.T) {
	want := map[string]string{
		"claude-code": "{\n  \"mcpServers\": {\n    \"jacu\": {\n      \"command\": \"jacu\",\n      \"args\": [\n        \"serve\"\n      ]\n    }\n  }\n}\n",
		"generic":     "{\n  \"mcpServers\": {\n    \"jacu\": {\n      \"command\": \"jacu\",\n      \"args\": [\n        \"serve\"\n      ]\n    }\n  }\n}\n",
		"codex":       "[mcp_servers.jacu]\ncommand = \"jacu\"\nargs = [\"serve\"]\n",
		"cursor":      "{\n  \"mcpServers\": {\n    \"jacu\": {\n      \"command\": \"jacu\",\n      \"args\": [\n        \"serve\"\n      ]\n    }\n  }\n}\n",
		"opencode":    "{\n  \"$schema\": \"https://opencode.ai/config.json\",\n  \"mcp\": {\n    \"jacu\": {\n      \"type\": \"local\",\n      \"command\": [\n        \"jacu\",\n        \"serve\"\n      ],\n      \"enabled\": true\n    }\n  }\n}\n",
	}
	for host, expected := range want {
		t.Run(host, func(t *testing.T) {
			first, err := renderHostPack(host, "")
			if err != nil {
				t.Fatal(err)
			}
			second, err := renderHostPack(host, "")
			if err != nil {
				t.Fatal(err)
			}
			if first != second || first != expected || strings.TrimSpace(first) == "" {
				t.Fatalf("host pack is not deterministic/non-empty: %q / %q", first, second)
			}
		})
	}
}

func TestDoctorHostPackRejectsUnknownHostWithSupportedList(t *testing.T) {
	if _, err := renderHostPack("unknown", ""); err == nil || !strings.Contains(err.Error(), "claude-code") || !strings.Contains(err.Error(), "generic") {
		t.Fatalf("unknown host error = %v; want supported list", err)
	}
}

func TestClaudeDesktopHostPackRequiresRepoAndAnchorsCwd(t *testing.T) {
	if _, err := renderHostPack("claude-desktop", ""); err == nil || !strings.Contains(err.Error(), "repo") {
		t.Fatalf("missing repo error = %v; want a repo requirement", err)
	}
	pack, err := renderHostPack("claude-desktop", "/tmp/example-repo")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(pack, "cd '/tmp/example-repo'") || !strings.Contains(pack, "exec jacu serve") {
		t.Fatalf("pack does not anchor cwd: %s", pack)
	}
}
