package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitCopiesCompleteSkillSetIntoNamedDirectory(t *testing.T) {
	t.Parallel()
	dest := t.TempDir()
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	code := runInit([]string{"--host", "generic", "--skills-dir", dest, "--from", testdataSkills(t)}, stdout, stderr)
	if code != 0 {
		t.Fatalf("init exit %d: %s", code, stderr.String())
	}
	for _, skill := range []string{"using-jacu", "jacu-inspect"} {
		if _, err := os.Stat(filepath.Join(dest, skill, "SKILL.md")); err != nil {
			t.Fatalf("missing %s: %v", skill, err)
		}
	}
}

func TestInitRefusesPartialSkillSource(t *testing.T) {
	t.Parallel()
	from := t.TempDir()
	writeSkill(t, from, "using-jacu", "router only")
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	code := runInit([]string{"--host", "generic", "--skills-dir", t.TempDir(), "--from", from}, stdout, stderr)
	if code == 0 {
		t.Fatal("init accepted a source that only has the router")
	}
	if !strings.Contains(stderr.String(), "complete") {
		t.Fatalf("stderr = %q; want a complete-set refusal", stderr.String())
	}
}

func TestInitMergesHostConfigWithoutClobberingSiblings(t *testing.T) {
	t.Parallel()
	config := filepath.Join(t.TempDir(), "mcp.json")
	if err := os.WriteFile(config, []byte(`{"mcpServers":{"stripe":{"url":"https://mcp.stripe.com"}}}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	code := runInit([]string{"--host", "generic", "--config", config, "--from", testdataSkills(t), "--skills-dir", t.TempDir()}, &bytes.Buffer{}, &bytes.Buffer{})
	if code != 0 {
		t.Fatalf("init exit %d", code)
	}
	raw, err := os.ReadFile(config) // #nosec G304 -- config is under this test's TempDir.
	if err != nil {
		t.Fatal(err)
	}
	var parsed map[string]map[string]map[string]any
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("parse written config: %v\n%s", err, raw)
	}
	if parsed["mcpServers"]["stripe"]["url"] != "https://mcp.stripe.com" {
		t.Fatalf("clobbered sibling: %s", raw)
	}
	if parsed["mcpServers"]["jacu"]["command"] != "jacu" {
		t.Fatalf("missing jacu entry: %s", raw)
	}
	if _, err := os.Stat(config + ".jacu-init.bak"); err != nil {
		t.Fatalf("expected backup beside the write: %v", err)
	}
}

func TestInitReportsConflictingHostEntryAndChangesNothing(t *testing.T) {
	t.Parallel()
	config := filepath.Join(t.TempDir(), "mcp.json")
	original := []byte(`{"mcpServers":{"jacu":{"command":"/other/jacu","args":["serve"]}}}` + "\n")
	if err := os.WriteFile(config, original, 0o600); err != nil {
		t.Fatal(err)
	}
	stderr := &bytes.Buffer{}
	code := runInit([]string{"--host", "generic", "--config", config, "--from", testdataSkills(t), "--skills-dir", t.TempDir()}, &bytes.Buffer{}, stderr)
	if code == 0 {
		t.Fatal("init overwrote a conflicting jacu entry")
	}
	if !strings.Contains(stderr.String(), "conflict") {
		t.Fatalf("stderr = %q; want conflict", stderr.String())
	}
	got, err := os.ReadFile(config) // #nosec G304 -- config is under this test's TempDir.
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(original) {
		t.Fatalf("config changed:\n%s", got)
	}
	if _, err := os.Stat(config + ".jacu-init.bak"); err == nil {
		t.Fatal("wrote a backup even though the file was not modified")
	}
}

func TestInitRepointsRetiredAliasCommandToJacuServe(t *testing.T) {
	t.Parallel()
	config := filepath.Join(t.TempDir(), "mcp.json")
	original := []byte(`{"mcpServers":{"stripe":{"url":"https://mcp.stripe.com"},"jacu":{"command":"jacu-mcp"}}}` + "\n")
	if err := os.WriteFile(config, original, 0o600); err != nil {
		t.Fatal(err)
	}
	stderr := &bytes.Buffer{}
	code := runInit([]string{"--host", "cursor", "--config", config, "--from", testdataSkills(t), "--skills-dir", t.TempDir()}, &bytes.Buffer{}, stderr)
	if code != 0 {
		t.Fatalf("init exit %d: %s", code, stderr.String())
	}
	raw, err := os.ReadFile(config) // #nosec G304 -- config is under this test's TempDir.
	if err != nil {
		t.Fatal(err)
	}
	var parsed map[string]map[string]map[string]any
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("parse written config: %v\n%s", err, raw)
	}
	if parsed["mcpServers"]["stripe"]["url"] != "https://mcp.stripe.com" {
		t.Fatalf("clobbered sibling: %s", raw)
	}
	if parsed["mcpServers"]["jacu"]["command"] != "jacu" {
		t.Fatalf("command = %v; want jacu\n%s", parsed["mcpServers"]["jacu"]["command"], raw)
	}
	args, _ := parsed["mcpServers"]["jacu"]["args"].([]any)
	if len(args) != 1 || args[0] != "serve" {
		t.Fatalf("args = %#v; want [serve]\n%s", parsed["mcpServers"]["jacu"]["args"], raw)
	}
	if _, stillThere := parsed["mcpServers"]["jacu-mcp"]; stillThere {
		t.Fatalf("retired server key survived: %s", raw)
	}
	if _, err := os.Stat(config + ".jacu-init.bak"); err != nil {
		t.Fatalf("expected backup of the retired config: %v", err)
	}
}

func TestInitRepointsRetiredAliasServerKeyToJacuServe(t *testing.T) {
	t.Parallel()
	config := filepath.Join(t.TempDir(), "mcp.json")
	original := []byte(`{"mcpServers":{"jacu-mcp":{"command":"jacu-mcp"}}}` + "\n")
	if err := os.WriteFile(config, original, 0o600); err != nil {
		t.Fatal(err)
	}
	code := runInit([]string{"--host", "cursor", "--config", config, "--from", testdataSkills(t), "--skills-dir", t.TempDir()}, &bytes.Buffer{}, &bytes.Buffer{})
	if code != 0 {
		t.Fatal("init refused a retired server key")
	}
	raw, err := os.ReadFile(config) // #nosec G304 -- config is under this test's TempDir.
	if err != nil {
		t.Fatal(err)
	}
	var parsed map[string]map[string]map[string]any
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("parse written config: %v\n%s", err, raw)
	}
	if _, stillThere := parsed["mcpServers"]["jacu-mcp"]; stillThere {
		t.Fatalf("retired server key survived: %s", raw)
	}
	if parsed["mcpServers"]["jacu"]["command"] != "jacu" {
		t.Fatalf("missing jacu entry: %s", raw)
	}
}

func TestInitLeavesEquivalentJacuServeEntryAlone(t *testing.T) {
	t.Parallel()
	config := filepath.Join(t.TempDir(), "mcp.json")
	original := []byte(`{"mcpServers":{"jacu":{"type":"stdio","command":"jacu","args":["serve"]}}}` + "\n")
	if err := os.WriteFile(config, original, 0o600); err != nil {
		t.Fatal(err)
	}
	code := runInit([]string{"--host", "cursor", "--config", config, "--from", testdataSkills(t), "--skills-dir", t.TempDir()}, &bytes.Buffer{}, &bytes.Buffer{})
	if code != 0 {
		t.Fatal("init treated an equivalent jacu serve entry as a conflict")
	}
	got, err := os.ReadFile(config) // #nosec G304 -- config is under this test's TempDir.
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(original) {
		t.Fatalf("rewrote an already-correct entry:\n%s", got)
	}
	if _, err := os.Stat(config + ".jacu-init.bak"); err == nil {
		t.Fatal("wrote a backup even though the file was not modified")
	}
}

func TestInitRepointsRetiredCodexAlias(t *testing.T) {
	t.Parallel()
	config := filepath.Join(t.TempDir(), "config.toml")
	original := []byte("[mcp_servers.other]\ncommand = \"keep\"\n\n[mcp_servers.jacu]\ncommand = \"jacu-mcp\"\n")
	if err := os.WriteFile(config, original, 0o600); err != nil {
		t.Fatal(err)
	}
	stderr := &bytes.Buffer{}
	code := runInit([]string{"--host", "codex", "--config", config, "--from", testdataSkills(t), "--skills-dir", t.TempDir()}, &bytes.Buffer{}, stderr)
	if code != 0 {
		t.Fatalf("init exit %d: %s", code, stderr.String())
	}
	got, err := os.ReadFile(config) // #nosec G304 -- config is under this test's TempDir.
	if err != nil {
		t.Fatal(err)
	}
	text := string(got)
	if !strings.Contains(text, `command = "keep"`) {
		t.Fatalf("clobbered sibling:\n%s", text)
	}
	if strings.Contains(text, "jacu-mcp") {
		t.Fatalf("retired command survived:\n%s", text)
	}
	if !strings.Contains(text, `command = "jacu"`) || !strings.Contains(text, `args = ["serve"]`) {
		t.Fatalf("missing jacu serve:\n%s", text)
	}
}

func TestInitRepointsRetiredOpenCodeAlias(t *testing.T) {
	t.Parallel()
	config := filepath.Join(t.TempDir(), "opencode.json")
	original := []byte(`{"mcp":{"jacu-mcp":{"type":"local","command":["jacu-mcp"],"enabled":true}}}` + "\n")
	if err := os.WriteFile(config, original, 0o600); err != nil {
		t.Fatal(err)
	}
	code := runInit([]string{"--host", "opencode", "--config", config, "--from", testdataSkills(t), "--skills-dir", t.TempDir()}, &bytes.Buffer{}, &bytes.Buffer{})
	if code != 0 {
		t.Fatal("init refused a retired OpenCode entry")
	}
	raw, err := os.ReadFile(config) // #nosec G304 -- config is under this test's TempDir.
	if err != nil {
		t.Fatal(err)
	}
	var parsed map[string]map[string]map[string]any
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("parse written config: %v\n%s", err, raw)
	}
	if _, stillThere := parsed["mcp"]["jacu-mcp"]; stillThere {
		t.Fatalf("retired server key survived: %s", raw)
	}
	entry := parsed["mcp"]["jacu"]
	if entry["type"] != "local" {
		t.Fatalf("type = %v; want local\n%s", entry["type"], raw)
	}
	cmd, _ := entry["command"].([]any)
	if len(cmd) != 2 || cmd[0] != "jacu" || cmd[1] != "serve" {
		t.Fatalf("command = %#v; want [jacu serve]\n%s", entry["command"], raw)
	}
}

func TestInitPrintsSnippetWhenConfigIsUnnamed(t *testing.T) {
	t.Parallel()
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	code := runInit([]string{"--host", "cursor", "--from", testdataSkills(t), "--skills-dir", t.TempDir()}, stdout, stderr)
	if code != 0 {
		t.Fatalf("init exit %d: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"command"`) || !strings.Contains(stdout.String(), "jacu") {
		t.Fatalf("stdout missing host pack: %q", stdout.String())
	}
	if !strings.Contains(stdout.String(), ".cursor/mcp.json") {
		t.Fatalf("stdout missing exact target path: %q", stdout.String())
	}
}

func TestInitJSONIsOnlyMachineReadableOutput(t *testing.T) {
	t.Parallel()
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	code := runInit([]string{"--host", "cursor", "--from", testdataSkills(t), "--skills-dir", t.TempDir(), "--json"}, stdout, stderr)
	if code != 0 {
		t.Fatalf("init exit %d: %s", code, stderr.String())
	}
	var result map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("stdout is not exclusive JSON: %v\n%s", err, stdout.String())
	}
	if result["host"] != "cursor" || result["config"] != "printed" {
		t.Fatalf("json = %#v", result)
	}
	pack, _ := result["pack"].(string)
	if !strings.Contains(pack, `"command"`) || !strings.Contains(pack, "jacu") {
		t.Fatalf("pack missing host snippet: %#v", result["pack"])
	}
	path, _ := result["config_path"].(string)
	if !strings.Contains(path, ".cursor/mcp.json") {
		t.Fatalf("config_path = %q", path)
	}
}

func TestInitDryRunDoesNotWriteSkillsOrConfig(t *testing.T) {
	t.Parallel()
	skills := filepath.Join(t.TempDir(), "skills")
	config := filepath.Join(t.TempDir(), "mcp.json")
	code := runInit([]string{"--host", "generic", "--from", testdataSkills(t), "--skills-dir", skills, "--config", config, "--dry-run"}, &bytes.Buffer{}, &bytes.Buffer{})
	if code != 0 {
		t.Fatal("dry-run init failed")
	}
	if _, err := os.Stat(filepath.Join(skills, "using-jacu", "SKILL.md")); err == nil {
		t.Fatal("dry-run wrote skills")
	}
	if _, err := os.Stat(config); err == nil {
		t.Fatal("dry-run wrote config")
	}
}

func testdataSkills(t *testing.T) string {
	t.Helper()
	return filepath.Join("testdata", "skills")
}

func writeSkill(t *testing.T, root, name, body string) {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(body+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}
