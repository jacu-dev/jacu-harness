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
