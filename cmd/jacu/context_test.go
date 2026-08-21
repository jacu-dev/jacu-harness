package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	headlessreport "github.com/jacu-dev/jacu-harness/internal/report"
)

func TestReportJSONIsQualityAuditObject(t *testing.T) {
	root := initCLIGitRepo(t)
	var stdout, stderr bytes.Buffer
	code := runReport(root, []string{"--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("report --json exit = %d stderr=%q", code, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("diagnostics leaked onto stderr: %q", stderr.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("quality.json stdout is not JSON: %v; %q", err, stdout.String())
	}
	if payload["schema_version"] != "1" || payload["kind"] != "audit" {
		t.Fatalf("quality.json identity = schema %#v kind %#v", payload["schema_version"], payload["kind"])
	}
	blocks, _ := payload["blocks"].(map[string]any)
	for _, name := range []string{"summary", "steps", "decision", "risks", "flow", "chart", "table", "metrics"} {
		if _, ok := blocks[name]; !ok {
			t.Fatalf("quality.json missing block %s: %q", name, stdout.String())
		}
	}
	if _, ok := payload["status"]; ok {
		t.Fatalf("quality.json must not be the capability envelope: %q", stdout.String())
	}
	if !strings.Contains(string(stdout.Bytes()), `"schema_version"`) {
		t.Fatal("quality.json omitted schema_version")
	}
	_ = headlessreport.QualityJSONName
}

func TestReportDefaultIsMarkdownProjection(t *testing.T) {
	root := initCLIGitRepo(t)
	var stdout, stderr bytes.Buffer
	code := runReport(root, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("report exit = %d stderr=%q", code, stderr.String())
	}
	text := stdout.String()
	if !strings.Contains(text, "# JACU workspace audit") || strings.HasPrefix(strings.TrimSpace(text), "{") {
		t.Fatalf("default report is not Markdown: %q", text)
	}
}

func TestContextSDDAdmitsActiveLivingDocument(t *testing.T) {
	root := initCLIGitRepo(t)
	writeLivingSDD(t, root, "011-workspace-contract", "doing")
	var stdout, stderr bytes.Buffer
	code := runContext(root, []string{"--sdd"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("context --sdd exit = %d stderr=%q", code, stderr.String())
	}
	text := stdout.String()
	if !strings.Contains(text, "docs/sdd/011-workspace-contract/sdd.md") {
		t.Fatalf("stdout did not name the living path: %q", text)
	}
	if !strings.Contains(text, "# 011-workspace-contract — Workspace contract") {
		t.Fatalf("stdout omitted the document: %q", text)
	}
}

func TestContextSDDJSONAndRefusal(t *testing.T) {
	root := initCLIGitRepo(t)
	writeLivingSDD(t, root, "011-workspace-contract", "doing")
	var stdout, stderr bytes.Buffer
	code := runContext(root, []string{"--sdd", "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("context --sdd --json exit = %d stderr=%q", code, stderr.String())
	}
	var payload admittedSDD
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("context JSON: %v; %q", err, stdout.String())
	}
	if payload.Path != "docs/sdd/011-workspace-contract/sdd.md" {
		t.Fatalf("path = %q", payload.Path)
	}
	if !strings.Contains(payload.Document, "Workspace contract") {
		t.Fatalf("document omitted body: %q", payload.Document)
	}

	empty := initCLIGitRepo(t)
	var emptyOut, emptyErr bytes.Buffer
	refuse := runContext(empty, []string{"--sdd"}, &emptyOut, &emptyErr)
	if refuse != 1 {
		t.Fatalf("no active SDD exit = %d; want 1", refuse)
	}
	if !strings.Contains(emptyErr.String(), contextNoActiveCode) {
		t.Fatalf("stderr = %q; want typed %s", emptyErr.String(), contextNoActiveCode)
	}
	if emptyOut.Len() != 0 {
		t.Fatalf("refusal leaked onto stdout: %q", emptyOut.String())
	}

	usageErr := bytes.Buffer{}
	if code := runContext(root, nil, &bytes.Buffer{}, &usageErr); code != 2 {
		t.Fatalf("missing --sdd exit = %d; want 2", code)
	}
	if !strings.Contains(usageErr.String(), "usage") {
		t.Fatalf("usage stderr = %q", usageErr.String())
	}
}

func TestContextSDDUsesProgramNextWhenNoDoingTask(t *testing.T) {
	root := initCLIGitRepo(t)
	writeLivingSDD(t, root, "016-open-source-export", "todo")
	program := "# Program\n\n| # | SDD | Delivers | State |\n|---|---|---|---|\n| **016** | **`open-source-export`** | public repo | **next** · queued |\n"
	if err := os.WriteFile(filepath.Join(root, "docs", "sdd", "PROGRAM.md"), []byte(program), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := runContext(root, []string{"--sdd", "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("program-next exit = %d stderr=%q stdout=%q", code, stderr.String(), stdout.String())
	}
	var payload admittedSDD
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Path != "docs/sdd/016-open-source-export/sdd.md" {
		t.Fatalf("program-next path = %q", payload.Path)
	}
}

func writeLivingSDD(t *testing.T, root, directory, taskStatus string) {
	t.Helper()
	dir := filepath.Join(root, "docs", "sdd", directory)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	body := "# " + directory + " — Workspace contract\n\nThe living SDD body.\n"
	if err := os.WriteFile(filepath.Join(dir, "sdd.md"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	lock := `{"sdd_id":"sdd_test","content_sha256":"abc","requirements":[],"tasks":[{"number":"T1","verify":"go test","status":"` + taskStatus + `"}]}`
	if err := os.WriteFile(filepath.Join(dir, "sdd.lock.json"), []byte(lock), 0o600); err != nil {
		t.Fatal(err)
	}
}
