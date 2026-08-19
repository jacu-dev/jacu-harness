package main

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
)

var supportedHostPacks = []string{"claude-code", "claude-desktop", "codex", "cursor", "generic", "opencode"}

type mcpServerConfig struct {
	Command string   `json:"command"`
	Args    []string `json:"args"`
}

type mcpHostConfig struct {
	MCPServers map[string]mcpServerConfig `json:"mcpServers"`
}

type openCodeServerConfig struct {
	Type    string   `json:"type"`
	Command []string `json:"command"`
	Enabled bool     `json:"enabled"`
}

type openCodeConfig struct {
	Schema string                          `json:"$schema"`
	MCP    map[string]openCodeServerConfig `json:"mcp"`
}

func renderHostPack(host, repo string) (string, error) {
	server := mcpServerConfig{Command: "jacu", Args: []string{"serve"}}
	var value any
	switch host {
	case "claude-code", "cursor", "generic":
		value = mcpHostConfig{MCPServers: map[string]mcpServerConfig{"jacu": server}}
	case "claude-desktop":
		if strings.TrimSpace(repo) == "" {
			return "", fmt.Errorf("claude-desktop requires --repo <path> so the server cwd is anchored")
		}
		abs, err := filepath.Abs(repo)
		if err != nil {
			return "", fmt.Errorf("claude-desktop repo: %w", err)
		}
		value = mcpHostConfig{MCPServers: map[string]mcpServerConfig{"jacu": {
			Command: "sh",
			Args:    []string{"-c", "cd '" + escapeSingleQuotes(abs) + "' && exec jacu serve"},
		}}}
	case "codex":
		return "[mcp_servers.jacu]\ncommand = \"jacu\"\nargs = [\"serve\"]\n", nil
	case "opencode":
		value = openCodeConfig{
			Schema: "https://opencode.ai/config.json",
			MCP:    map[string]openCodeServerConfig{"jacu": {Type: "local", Command: []string{"jacu", "serve"}, Enabled: true}},
		}
	default:
		return "", fmt.Errorf("unknown host %q; supported hosts: %s", host, strings.Join(supportedHostPacks, ", "))
	}
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return "", err
	}
	return string(encoded) + "\n", nil
}

func escapeSingleQuotes(path string) string {
	return strings.ReplaceAll(path, "'", `'\''`)
}

func hostConfigHint(host, home string) string {
	switch host {
	case "claude-code":
		return filepath.Join(home, ".claude.json")
	case "claude-desktop":
		return filepath.Join(home, "Library", "Application Support", "Claude", "claude_desktop_config.json")
	case "codex":
		return filepath.Join(home, ".codex", "config.toml")
	case "cursor":
		return filepath.Join(home, ".cursor", "mcp.json")
	case "opencode":
		return filepath.Join(home, ".config", "opencode", "opencode.json")
	case "generic":
		return "the host's MCP config file (any stdio-capable client)"
	default:
		return ""
	}
}

func defaultSkillsDir(host, home string) string {
	switch host {
	case "claude-code", "claude-desktop":
		return filepath.Join(home, ".claude", "skills")
	case "codex":
		return filepath.Join(home, ".codex", "skills")
	case "cursor":
		return filepath.Join(home, ".cursor", "skills")
	case "opencode":
		return filepath.Join(home, ".config", "opencode", "skills")
	default:
		return ""
	}
}
