package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jacu-dev/jacu-harness/internal/capability/clarity"
	"github.com/jacu-dev/jacu-harness/internal/capability/sdd"
)

const clarityFixture = `---
sdd: 006-context-admission
---
# 006
## Write scope
**Allowed**
` + "```" + `
internal/capability/context/**
` + "```" + `
**Forbidden**
` + "```" + `
internal/mcpadapter/**
` + "```" + `
## Out of scope
- Summarising content
## Requirements
### Requirement: The pack is deterministic
Delta: ADDED
## Tasks
| # | Task | Files | Verify | Status | Evidence |
|---|---|---|---|---|---|
| T1 | Write ADR | docs/adr/ADR-024.md | wc | todo | |
`

func TestClarityProbeIngestVerdictJSON(t *testing.T) {
	root := t.TempDir()
	sddPath := filepath.Join(root, "sdd.md")
	if err := os.WriteFile(sddPath, []byte(clarityFixture), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if code := runClarity(root, []string{"probe", "--json", "--sdd", sddPath}, &stdout, &stderr, nil); code != 0 {
		t.Fatalf("probe exit=%d stderr=%q", code, stderr.String())
	}
	var probe map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &probe); err != nil {
		t.Fatalf("probe json: %v %q", err, stdout.String())
	}
	if probe["prompt"] == nil || probe["schema"] == nil {
		t.Fatalf("probe payload = %#v", probe)
	}

	document, err := sdd.Parse([]byte(clarityFixture))
	if err != nil {
		t.Fatal(err)
	}
	agreeing, err := json.Marshal(clarity.Expected(document))
	if err != nil {
		t.Fatal(err)
	}
	readbackPath := filepath.Join(root, "r1.json")
	if err := os.WriteFile(readbackPath, agreeing, 0o600); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	stderr.Reset()
	code := runClarity(root, []string{"ingest", "--json", "--sdd", sddPath, "--readback", readbackPath}, &stdout, &stderr, nil)
	if code != 0 {
		t.Fatalf("ingest exit=%d stderr=%q stdout=%q", code, stderr.String(), stdout.String())
	}

	extra := map[string]any{
		"write_scope":     []string{"internal/mcpadapter/server.go"},
		"forbidden_paths": []string{"internal/mcpadapter/**"},
		"requirements":    []string{"The pack is deterministic"},
		"out_of_scope":    []string{"Summarising content"},
		"tasks":           []string{"T1"},
	}
	extraRaw, _ := json.Marshal(extra)
	r2 := filepath.Join(root, "r2.json")
	if err := os.WriteFile(r2, extraRaw, 0o600); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	stderr.Reset()
	if code := runClarity(root, []string{"ingest", "--json", "--sdd", sddPath, "--readback", r2}, &stdout, &stderr, nil); code != 1 {
		t.Fatalf("divergent ingest exit=%d stdout=%q", code, stdout.String())
	}
	if !strings.Contains(stdout.String(), `"divergence_field":"write_scope"`) {
		t.Fatalf("ingest json missing write_scope: %s", stdout.String())
	}

	r3 := filepath.Join(root, "r3.json")
	if err := os.WriteFile(r3, agreeing, 0o600); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	stderr.Reset()
	if code := runClarity(root, []string{"verdict", "--json", "--sdd", sddPath, "--readback", readbackPath, "--readback", r2, "--readback", r3}, &stdout, &stderr, nil); code != 1 {
		t.Fatalf("verdict exit=%d stdout=%q", code, stdout.String())
	}

	stdout.Reset()
	stderr.Reset()
	if code := runClarity(root, []string{"ingest", "--sdd", sddPath, "--previous-spec-bytes", "1", "--readback", readbackPath}, &stdout, &stderr, nil); code != 1 {
		t.Fatalf("growing spec exit=%d stdout=%q", code, stdout.String())
	}
}

func TestClarityUsageExitTwo(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := runClarity(t.TempDir(), nil, &stdout, &stderr, nil); code != 2 {
		t.Fatalf("usage exit=%d", code)
	}
	if stdout.Len() != 0 || !strings.Contains(stderr.String(), "usage is clarity") {
		t.Fatalf("stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestClarityRejectsProseOnStdin(t *testing.T) {
	root := t.TempDir()
	sddPath := filepath.Join(root, "sdd.md")
	if err := os.WriteFile(sddPath, []byte(clarityFixture), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := runClarity(root, []string{"ingest", "--json", "--sdd", sddPath}, &stdout, &stderr, strings.NewReader("the spec is about packing context"))
	if code != 1 {
		t.Fatalf("prose ingest exit=%d", code)
	}
	if !strings.Contains(stderr.String(), clarity.CodeProse) {
		t.Fatalf("stderr=%q", stderr.String())
	}
}
