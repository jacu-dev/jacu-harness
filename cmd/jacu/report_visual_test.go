package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReportRenderWritesDeterministicHTML(t *testing.T) {
	root := t.TempDir()
	input := filepath.Join(root, "plan.report.json")
	output := filepath.Join(root, "plan.html")
	if err := os.WriteFile(input, []byte(sampleReportJSON), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := runReport(root, []string{"render", "--input", input, "--output", output, "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("render exit = %d stderr=%q", code, stderr.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("render --json: %v %q", err, stdout.String())
	}
	if payload["bound_port"] != false {
		t.Fatalf("render bound a port: %#v", payload)
	}
	body, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "<!DOCTYPE html>") || !strings.Contains(string(body), "Plan alpha") {
		t.Fatalf("html = %s", body)
	}
	if code := runReport(root, []string{"render", "--input", input, "--output", output, "--json"}, &stdout, &stderr); code != 0 {
		t.Fatal(code)
	}
}

func TestReportRenderRefusesMarkup(t *testing.T) {
	root := t.TempDir()
	input := filepath.Join(root, "bad.report.json")
	bad := strings.ReplaceAll(sampleReportJSON, "choose the next step", "<script>x</script>")
	if err := os.WriteFile(input, []byte(bad), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if code := runReport(root, []string{"render", "--json", "--input", input}, &stdout, &stderr); code == 0 {
		t.Fatalf("markup render succeeded: %q", stdout.String())
	}
}

const sampleReportJSON = `{
  "schema_version": "1",
  "kind": "plan",
  "title": "Plan alpha",
  "summary": "choose the next step",
  "blocks": {
    "summary": ["choose the next step"],
    "steps": [{"id": "s1", "label": "Write factory", "status": "open"}],
    "decision": [{"id": "d1", "question": "Ship HTML?", "kind": "options", "options": ["yes", "no"]}],
    "risks": ["markup from the model"],
    "flow": {
      "nodes": [{"id": "a", "label": "JSON", "kind": "input"}, {"id": "b", "label": "HTML", "kind": "output"}],
      "edges": [{"from": "a", "to": "b"}]
    },
    "chart": [{"label": "open", "value": 2}],
    "table": {"columns": ["id", "status"], "rows": [["s1", "open"]]},
    "metrics": [{"name": "blocks", "value": 8, "available": true}]
  }
}`
