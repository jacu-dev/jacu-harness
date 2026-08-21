package report

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/jacu-dev/jacu-harness/internal/capability/missioncompile"
	"github.com/jacu-dev/jacu-harness/internal/runstate"
	"github.com/jacu-dev/jacu-harness/internal/telemetry"
)

func TestReportValidationDigestAndMarkdownAreDeterministic(t *testing.T) {
	report := Report{
		SchemaVersion: "1",
		Kind:          KindAudit,
		Title:         "Workspace audit",
		Summary:       "one active run",
		Blocks: ReportBlocks{
			Summary:  []string{"one active run"},
			Steps:    []ReportStep{{ID: "run_a", Label: "Fix parser", Status: "open"}},
			Decision: []DecisionPoint{},
			Risks:    []string{},
			Flow: FlowBlock{
				Nodes: []FlowNode{{ID: "run_a", Label: "Fix parser", Kind: "run"}},
				Edges: []FlowEdge{},
			},
			Chart:   []ChartPoint{{Label: "open", Value: 1}},
			Table:   TableBlock{Columns: []string{"run_id", "status"}, Rows: [][]string{{"run_a", "open"}}},
			Metrics: []Metric{{Name: "runs", Value: 1}},
		},
	}

	if err := Validate(report); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	digest, err := Digest(report)
	if err != nil {
		t.Fatalf("Digest: %v", err)
	}
	if !strings.HasPrefix(digest, "sha256:") {
		t.Fatalf("digest = %q; want sha256 prefix", digest)
	}
	markdown, err := Markdown(report)
	if err != nil {
		t.Fatalf("Markdown: %v", err)
	}
	if !strings.Contains(markdown, "```mermaid") || !strings.Contains(markdown, "| run_id | status |") {
		t.Fatalf("markdown omitted deterministic projections: %q", markdown)
	}
	digestAgain, _ := Digest(report)
	markdownAgain, _ := Markdown(report)
	if digestAgain != digest || markdownAgain != markdown {
		t.Fatal("same report produced different digest or Markdown")
	}
	encoded, err := EncodeJSON(report)
	if err != nil {
		t.Fatalf("EncodeJSON: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(encoded, &payload); err != nil {
		t.Fatalf("EncodeJSON: %v", err)
	}
	if payload["kind"] != KindAudit || payload["schema_version"] != SchemaVersion {
		t.Fatalf("%s identity = %#v", QualityJSONName, payload)
	}
	if _, ok := payload["trace_id"]; ok {
		t.Fatalf("%s must not be the capability envelope: %s", QualityJSONName, encoded)
	}
}

func TestAuditWalkerProjectsRunsMissionsAndProgramsWithoutSensitiveFields(t *testing.T) {
	root := t.TempDir()
	first := runstate.Run{
		RunID:           "run_0000000000000001",
		MissionID:       "msn_0000000000000001",
		Mission:         missioncompile.Mission{MissionID: "msn_0000000000000001", Objective: "Fix parser /Users/secret", Risk: "write"},
		Status:          runstate.StatusOpen,
		ProgramID:       "prg_0000000000000001",
		ProgramCursor:   1,
		ProgramMissions: []runstate.ProgramMissionState{{Index: 0, Status: "running", Iterations: 2}},
	}
	second := first
	second.RunID = "run_0000000000000002"
	second.MissionID = "msn_0000000000000002"
	second.Mission.MissionID = second.MissionID
	second.Status = runstate.StatusReviewed
	if err := runstate.Save(root, first); err != nil {
		t.Fatalf("save first run: %v", err)
	}
	if err := runstate.Save(root, second); err != nil {
		t.Fatalf("save second run: %v", err)
	}

	projected, err := BuildAudit(root)
	if err != nil {
		t.Fatalf("BuildAudit: %v", err)
	}
	if projected.Kind != KindAudit || len(projected.Blocks.Steps) != 2 || len(projected.Blocks.Flow.Nodes) < 3 {
		t.Fatalf("audit projection = %#v; want runs, program, and mission nodes", projected)
	}
	markdown, err := Markdown(projected)
	if err != nil {
		t.Fatalf("Markdown audit: %v", err)
	}
	if strings.Contains(markdown, "/Users/secret") || strings.Contains(markdown, "verification_commands") {
		t.Fatalf("audit leaked sensitive or raw structured fields: %q", markdown)
	}
}

func TestStatuslineIsHonestWhenIdleAndShowsProgram(t *testing.T) {
	root := t.TempDir()
	idle, err := Statusline(root)
	if err != nil {
		t.Fatalf("idle statusline: %v", err)
	}
	if !strings.Contains(idle, "idle") || !strings.Contains(idle, "not measured") {
		t.Fatalf("idle statusline = %q; want honest idle output", idle)
	}
	if saveErr := runstate.Save(root, runstate.Run{
		RunID: "run_0000000000000003", MissionID: "msn_0000000000000003", Status: runstate.StatusOpen,
		ProgramID: "prg_0000000000000002", ProgramCursor: 2,
	}); saveErr != nil {
		t.Fatalf("save run: %v", saveErr)
	}
	active, err := Statusline(root)
	if err != nil {
		t.Fatalf("active statusline: %v", err)
	}
	for _, want := range []string{"run_0000000000000003", "msn_0000000000000003", "prg_0000000000000002", "phase=open", "cost=not measured"} {
		if !strings.Contains(active, want) {
			t.Fatalf("active statusline = %q; missing %q", active, want)
		}
	}
}

func TestAuditReportProjectsTelemetryMetricsWithoutPayloads(t *testing.T) {
	base := t.TempDir()
	t.Setenv("JACU_HOME", base)
	store := telemetry.NewStoreAt(base)
	start := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	mission := telemetry.Event{
		SchemaVersion: telemetry.CurrentSchemaVersion, Timestamp: start, Level: telemetry.LevelUser,
		ProjectID: telemetry.ProjectID(base), TraceID: "tr_0123456789abcdef", MissionID: "msn_0123456789abcdef",
		Module: "mission", Stage: telemetry.EventToolCall, Event: telemetry.EventToolCall,
		Tool: "jacu_mission_compile", Status: "ok",
	}
	verify := mission
	verify.Timestamp = start.Add(time.Second)
	verify.Event = telemetry.EventVerify
	verify.Tool = "jacu_verify"
	verify.Status = "ok"
	verify.Verdict = "pass"
	verify.Iteration = 1
	apply := mission
	apply.Timestamp = start.Add(2 * time.Second)
	apply.Event = telemetry.EventApply
	apply.Tool = "jacu_apply"
	apply.Status = "applied"
	apply.Verdict = "pass"
	for _, event := range []telemetry.Event{mission, verify, apply} {
		if err := store.Emit(event); err != nil {
			t.Fatalf("emit: %v", err)
		}
	}
	projected, err := BuildAudit(base)
	if err != nil {
		t.Fatalf("BuildAudit: %v", err)
	}
	found := false
	for _, metric := range projected.Blocks.Metrics {
		if metric.Name == telemetry.MetricFirstPassVerify {
			found = metric.Available && metric.ValueText == "100.0%"
		}
	}
	if !found {
		t.Fatalf("telemetry metric missing: %+v", projected.Blocks.Metrics)
	}
	markdown, err := Markdown(projected)
	if err != nil {
		t.Fatalf("Markdown: %v", err)
	}
	if !strings.Contains(markdown, telemetry.MetricFirstPassVerify) || strings.Contains(markdown, "trace_id") {
		t.Fatalf("telemetry projection missing or leaked event payload: %q", markdown)
	}
}

func TestAuditReportIncludesUserLevelTelemetryMetrics(t *testing.T) {
	root := t.TempDir()
	t.Setenv("JACU_HOME", t.TempDir())
	projected, err := BuildAudit(root)
	if err != nil {
		t.Fatalf("BuildAudit: %v", err)
	}
	want := []string{"mission_bytes_in", "mission_bytes_out", "mission_interruptions", "clean_exit_result"}
	for _, name := range want {
		found := false
		for _, metric := range projected.Blocks.Metrics {
			if metric.Name == name {
				found = true
				if metric.Available {
					t.Fatalf("empty metric %q is available: %+v", name, metric)
				}
			}
		}
		if !found {
			t.Fatalf("user metric %q missing: %+v", name, projected.Blocks.Metrics)
		}
	}
}
