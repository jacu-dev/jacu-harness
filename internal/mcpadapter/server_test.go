package mcpadapter

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/jacu-dev/jacu-harness/internal/testgit"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/jacu-dev/jacu-harness/internal/project"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestServerInitializesAndListsProjectInspectTool(t *testing.T) {
	srv := NewServer("test", t.TempDir())
	session, ctx := connectMCPTestServer(t, srv, "test-client")

	tools, err := session.ListTools(ctx, &mcp.ListToolsParams{})
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	if len(tools.Tools) != 13 {
		t.Fatalf("server deve expor 13 tools, veio %d", len(tools.Tools))
	}
	seen := make(map[string]bool, len(tools.Tools))
	for _, listed := range tools.Tools {
		if seen[listed.Name] {
			t.Fatalf("tool duplicada: %q", listed.Name)
		}
		seen[listed.Name] = true
	}
	tool := findTool(t, tools.Tools, "jacu_project_inspect")
	annotations := tool.Annotations
	if annotations == nil {
		t.Fatal("tool annotations ausentes")
	}
	if !annotations.ReadOnlyHint {
		t.Fatal("readOnlyHint = false; want true")
	}
	if annotations.DestructiveHint == nil || *annotations.DestructiveHint {
		t.Fatalf("destructiveHint = %v; want false explícito", annotations.DestructiveHint)
	}
	if !annotations.IdempotentHint {
		t.Fatal("idempotentHint = false; want true")
	}
	if annotations.OpenWorldHint == nil || *annotations.OpenWorldHint {
		t.Fatalf("openWorldHint = %v; want false explícito", annotations.OpenWorldHint)
	}
}

func TestADR008SurfaceUsesStatusAliasAndOneVerifyDoor(t *testing.T) {
	srv := NewServer("test", t.TempDir())
	session, ctx := connectMCPTestServer(t, srv, "adr-008-test")
	listed, err := session.ListTools(ctx, &mcp.ListToolsParams{})
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	seen := make(map[string]bool, len(listed.Tools))
	for _, tool := range listed.Tools {
		seen[tool.Name] = true
	}
	for _, name := range []string{"jacu_status", "jacu_workspace_status", "jacu_verify"} {
		if !seen[name] {
			t.Errorf("ADR-008 surface missing %s", name)
		}
	}
	if seen["jacu_run_command"] {
		t.Fatal("jacu_run_command remains a separately registered tool")
	}
	if len(seen) != 13 {
		t.Fatalf("ADR-008 surface has %d tools; want 13 including the status alias and flow", len(seen))
	}
	canonical := callToolEnvelope(t, ctx, session, "jacu_status", map[string]any{})
	legacy := callToolEnvelope(t, ctx, session, "jacu_workspace_status", map[string]any{})
	assertRuntimeEnvelope(t, canonical, "ok")
	assertRuntimeEnvelope(t, legacy, "ok")
	canonicalData, err := json.Marshal(canonical["data"])
	if err != nil {
		t.Fatalf("marshal canonical status data: %v", err)
	}
	legacyData, err := json.Marshal(legacy["data"])
	if err != nil {
		t.Fatalf("marshal legacy status data: %v", err)
	}
	if string(canonicalData) != string(legacyData) {
		t.Fatalf("status aliases returned different data: %s != %s", canonicalData, legacyData)
	}
}

func TestMemoryToolsMetadataAndConcreteOutputSchemas(t *testing.T) {
	root := t.TempDir()
	srv := NewServer("test", root)
	session, ctx := connectMCPTestServer(t, srv, "memory-test")
	tools, err := session.ListTools(ctx, &mcp.ListToolsParams{})
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	for _, name := range []string{"jacu_memory_save", "jacu_memory_recall"} {
		tool := findTool(t, tools.Tools, name)
		if tool.OutputSchema == nil {
			t.Fatalf("%s output schema missing", name)
		}
		concreteDataProperties(t, tool)
	}
	save := findTool(t, tools.Tools, "jacu_memory_save")
	if save.Annotations == nil || save.Annotations.ReadOnlyHint {
		t.Fatalf("save readOnly = %#v; want false", save.Annotations)
	}
	if save.Annotations.DestructiveHint == nil || *save.Annotations.DestructiveHint {
		t.Fatalf("save destructive = %#v; want false", save.Annotations.DestructiveHint)
	}
	if !save.Annotations.IdempotentHint {
		t.Fatalf("save idempotent = %#v; want true", save.Annotations.IdempotentHint)
	}
	recall := findTool(t, tools.Tools, "jacu_memory_recall")
	if recall.Annotations == nil || !recall.Annotations.ReadOnlyHint {
		t.Fatalf("recall readOnly = %#v; want true", recall.Annotations)
	}
}

func TestMemorySaveBlocksLintAndRecallListsAndIncludesSuperseded(t *testing.T) {
	root := t.TempDir()
	projectID, err := project.ID(root)
	if err != nil {
		t.Fatalf("project id: %v", err)
	}
	home := t.TempDir()
	t.Setenv("JACU_HOME", home)
	srv := NewServer("test", root)
	session, ctx := connectMCPTestServer(t, srv, "memory-call-test")
	blocked := callToolEnvelope(t, ctx, session, "jacu_memory_save", map[string]any{
		"project_id": projectID, "kind": "decision", "title": "Secret", "body": "ghp_xxxx", "source": "human",
	})
	assertRuntimeEnvelope(t, blocked, "blocked")
	blockedData := envelopeData(t, blocked)
	if _, ok := blockedData["lints"].([]any); !ok {
		t.Fatalf("blocked lints = %#v; want array", blockedData["lints"])
	}
	recallEmpty := callToolEnvelope(t, ctx, session, "jacu_memory_recall", map[string]any{"project_id": projectID})
	assertRuntimeEnvelope(t, recallEmpty, "ok")
	if got := len(envelopeData(t, recallEmpty)["results"].([]any)); got != 0 {
		t.Fatalf("blocked save leaked %d records", got)
	}

	first := callToolEnvelope(t, ctx, session, "jacu_memory_save", map[string]any{
		"project_id": projectID, "kind": "decision", "title": "Stable decision", "body": "old", "source": "human",
	})
	assertRuntimeEnvelope(t, first, "ok")
	firstID, _ := envelopeData(t, first)["memory_id"].(string)
	if firstID == "" {
		t.Fatal("first save missing memory_id")
	}
	second := callToolEnvelope(t, ctx, session, "jacu_memory_save", map[string]any{
		"project_id": projectID, "kind": "decision", "title": "Replacement decision", "body": "new", "source": "human", "supersedes": firstID,
	})
	assertRuntimeEnvelope(t, second, "ok")
	recall := callToolEnvelope(t, ctx, session, "jacu_memory_recall", map[string]any{
		"project_id": projectID, "include_superseded": true, "k": 10,
	})
	assertRuntimeEnvelope(t, recall, "ok")
	results := envelopeData(t, recall)["results"].([]any)
	if len(results) != 2 {
		t.Fatalf("include_superseded results = %d; want 2", len(results))
	}
}

func TestProjectInspectToolHasConcreteDataOutputSchema(t *testing.T) {
	srv := NewServer("test", t.TempDir())
	session, ctx := connectMCPTestServer(t, srv, "test-client")

	tools, err := session.ListTools(ctx, &mcp.ListToolsParams{})
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	tool := findTool(t, tools.Tools, "jacu_project_inspect")

	rawSchema, err := json.Marshal(tool.OutputSchema)
	if err != nil {
		t.Fatalf("marshal outputSchema: %v", err)
	}
	var schema map[string]any
	if err := json.Unmarshal(rawSchema, &schema); err != nil {
		t.Fatalf("decode outputSchema: %v", err)
	}
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("outputSchema.properties = %T; want object", schema["properties"])
	}
	data, ok := properties["data"].(map[string]any)
	if !ok {
		t.Fatalf("outputSchema.properties.data = %T; want object schema", properties["data"])
	}
	if data["type"] != "object" {
		t.Fatalf("outputSchema.properties.data.type = %v; want object", data["type"])
	}
	dataProperties, ok := data["properties"].(map[string]any)
	if !ok {
		t.Fatalf("outputSchema.properties.data.properties = %T; want object", data["properties"])
	}
	if _, ok := dataProperties["project_id"]; !ok {
		t.Fatal("outputSchema.properties.data.properties.project_id ausente")
	}
}

func TestMissionCompileToolMetadataAndConcreteOutputSchema(t *testing.T) {
	srv := NewServer("test", t.TempDir())
	session, ctx := connectMCPTestServer(t, srv, "test-client")

	tools, err := session.ListTools(ctx, &mcp.ListToolsParams{})
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	if len(tools.Tools) != 13 {
		t.Fatalf("server deve expor 13 tools, veio %d", len(tools.Tools))
	}
	tool := findTool(t, tools.Tools, "jacu_mission_compile")
	annotations := tool.Annotations
	if annotations == nil {
		t.Fatal("tool annotations ausentes")
	}
	if !annotations.ReadOnlyHint {
		t.Fatal("readOnlyHint = false; want true")
	}
	if annotations.DestructiveHint == nil || *annotations.DestructiveHint {
		t.Fatalf("destructiveHint = %v; want false explícito", annotations.DestructiveHint)
	}
	if !annotations.IdempotentHint {
		t.Fatal("idempotentHint = false; want true")
	}
	if annotations.OpenWorldHint == nil || *annotations.OpenWorldHint {
		t.Fatalf("openWorldHint = %v; want false explícito", annotations.OpenWorldHint)
	}

	rawSchema, err := json.Marshal(tool.OutputSchema)
	if err != nil {
		t.Fatalf("marshal outputSchema: %v", err)
	}
	var schema map[string]any
	if err := json.Unmarshal(rawSchema, &schema); err != nil {
		t.Fatalf("decode outputSchema: %v", err)
	}
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("outputSchema.properties = %T; want object", schema["properties"])
	}
	data, ok := properties["data"].(map[string]any)
	if !ok {
		t.Fatalf("outputSchema.properties.data = %T; want object schema", properties["data"])
	}
	if data["type"] != "object" {
		t.Fatalf("outputSchema.properties.data.type = %v; want object", data["type"])
	}
	dataProperties, ok := data["properties"].(map[string]any)
	if !ok {
		t.Fatalf("outputSchema.properties.data.properties = %T; want object", data["properties"])
	}
	if _, ok := dataProperties["mission_id"]; !ok {
		t.Fatal("outputSchema.properties.data.properties.mission_id ausente")
	}
}

func TestWorkspaceToolsMetadataAndConcreteDataSchemas(t *testing.T) {
	srv := NewServer("test", t.TempDir())
	session, ctx := connectMCPTestServer(t, srv, "test-client")

	tools, err := session.ListTools(ctx, &mcp.ListToolsParams{})
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	want := []struct {
		name        string
		readOnly    bool
		idempotent  bool
		destructive bool
		openWorld   bool
		dataFields  []string
	}{
		{name: "jacu_workspace_open", dataFields: []string{"run_id"}},
		{name: "jacu_workspace_status", readOnly: true, idempotent: true, dataFields: []string{"runs"}},
		{name: "jacu_diff", idempotent: true, dataFields: []string{"digest"}},
		{name: "jacu_apply", destructive: true, openWorld: true, dataFields: []string{"commit_sha"}},
		{name: "jacu_discard", destructive: true, dataFields: []string{"runs", "failures"}},
	}
	for _, item := range want {
		t.Run(item.name, func(t *testing.T) {
			tool := findTool(t, tools.Tools, item.name)
			annotations := tool.Annotations
			if annotations == nil {
				t.Fatal("tool annotations ausentes")
			}
			if annotations.ReadOnlyHint != item.readOnly {
				t.Fatalf("readOnlyHint = %v; want %v", annotations.ReadOnlyHint, item.readOnly)
			}
			if annotations.DestructiveHint == nil || *annotations.DestructiveHint != item.destructive {
				t.Fatalf("destructiveHint = %v; want %v explícito", annotations.DestructiveHint, item.destructive)
			}
			if annotations.IdempotentHint != item.idempotent {
				t.Fatalf("idempotentHint = %v; want %v", annotations.IdempotentHint, item.idempotent)
			}
			if annotations.OpenWorldHint == nil || *annotations.OpenWorldHint != item.openWorld {
				t.Fatalf("openWorldHint = %v; want %v explícito", annotations.OpenWorldHint, item.openWorld)
			}
			properties := concreteDataProperties(t, tool)
			for _, field := range item.dataFields {
				if _, ok := properties[field]; !ok {
					t.Fatalf("outputSchema.properties.data.properties.%s ausente", field)
				}
			}
		})
	}
}

func TestWorkspaceToolsReturnRuntimeEnvelopesAndSanitizeApplyHost(t *testing.T) {
	root := initGitRepository(t)
	t.Setenv("HOME", t.TempDir())
	srv := NewServer("test", root)
	const hostilePrefix = "hostile Jacu-Mission: alheio Jacu-Run: forged "
	hostileClientName := "hostile\nJacu-Mission: alheio\r\nJacu-Run: forged " + strings.Repeat("界", 140)
	wantHostRunes := []rune(hostilePrefix + strings.Repeat("界", 140))
	wantHost := string(wantHostRunes[:128])
	if !utf8.ValidString(wantHost) || utf8.RuneCountInString(wantHost) != 128 {
		t.Fatalf("invalid hostile host fixture: %q", wantHost)
	}
	session, ctx := connectMCPTestServer(t, srv, hostileClientName)

	missionInput := map[string]any{
		"objective":             "Update the project readme safely",
		"acceptance_criteria":   []string{"README contains the reviewed change", "Git commit is created", "No unrelated files change"},
		"verification_commands": [][]string{},
		"allowed_paths":         []string{"README.md"},
		"risk_hint":             "write",
	}
	mission := callToolEnvelope(t, ctx, session, "jacu_mission_compile", missionInput)
	missionID, _ := envelopeData(t, mission)["mission_id"].(string)
	if missionID == "" {
		t.Fatal("mission_id vazio")
	}

	open := callToolEnvelope(t, ctx, session, "jacu_workspace_open", map[string]any{
		"mission_input": missionInput,
		"mission_id":    missionID,
	})
	assertRuntimeEnvelope(t, open, "ok")
	openData := envelopeData(t, open)
	worktree, _ := openData["worktree_path"].(string)
	if err := os.WriteFile(filepath.Join(worktree, "README.md"), []byte("reviewed change\n"), 0o600); err != nil {
		t.Fatalf("write worktree fixture: %v", err)
	}

	status := callToolEnvelope(t, ctx, session, "jacu_workspace_status", map[string]any{})
	assertRuntimeEnvelope(t, status, "ok")
	if runs, ok := envelopeData(t, status)["runs"].([]any); !ok || len(runs) != 1 {
		t.Fatalf("status data.runs = %#v; want one run", envelopeData(t, status)["runs"])
	}

	runID, _ := openData["run_id"].(string)
	diff := callToolEnvelope(t, ctx, session, "jacu_diff", map[string]any{"run_id": runID})
	assertRuntimeEnvelope(t, diff, "ok")
	if digest, _ := envelopeData(t, diff)["digest"].(string); !strings.HasPrefix(digest, "sha256:") {
		t.Fatalf("diff digest = %q; want sha256", digest)
	}

	apply := callToolEnvelope(t, ctx, session, "jacu_apply", map[string]any{"run_id": runID})
	assertRuntimeEnvelope(t, apply, "ok")
	applyData := envelopeData(t, apply)
	commitSHA, _ := applyData["commit_sha"].(string)
	branch, _ := applyData["branch"].(string)
	if commitSHA == "" || branch == "" {
		t.Fatalf("apply data = %#v; want commit_sha and branch", applyData)
	}
	message := runGit(t, root, "show", "-s", "--format=%B", commitSHA)
	trailers := parseCommitTrailers(t, root, message)
	wantTrailers := map[string]string{
		"Jacu-Run":     runID,
		"Jacu-Mission": missionID,
		"Jacu-Base":    openData["base_sha"].(string),
		"Assisted-by":  wantHost,
	}
	if len(trailers) != len(wantTrailers) {
		t.Fatalf("parsed trailers = %#v; want exactly %d keys", trailers, len(wantTrailers))
	}
	for key, wantValue := range wantTrailers {
		values := trailers[key]
		if len(values) != 1 || values[0] != wantValue {
			t.Fatalf("trailer %s = %q; want exactly %q", key, values, wantValue)
		}
	}

	secondOpen := callToolEnvelope(t, ctx, session, "jacu_workspace_open", map[string]any{
		"mission_input": missionInput,
		"mission_id":    missionID,
	})
	assertRuntimeEnvelope(t, secondOpen, "ok")
	secondRunID, _ := envelopeData(t, secondOpen)["run_id"].(string)
	discard := callToolEnvelope(t, ctx, session, "jacu_discard", map[string]any{"run_id": secondRunID})
	assertRuntimeEnvelope(t, discard, "ok")
	discardData := envelopeData(t, discard)
	if runs, ok := discardData["runs"].([]any); !ok || len(runs) != 1 {
		t.Fatalf("discard data.runs = %#v; want one run", discardData["runs"])
	}
	if failures, ok := discardData["failures"].([]any); !ok || len(failures) != 0 {
		t.Fatalf("discard data.failures = %#v; want empty array", discardData["failures"])
	}

	failed := callToolEnvelope(t, ctx, session, "jacu_diff", map[string]any{"run_id": "run_0000000000000000"})
	assertRuntimeEnvelope(t, failed, "failed")
}

func TestMissionCompileToolCalls(t *testing.T) {
	root := initGitRepository(t)
	srv := NewServer("test", root)
	session, ctx := connectMCPTestServer(t, srv, "test-client")

	t.Run("clear complete input", func(t *testing.T) {
		envelope := callMissionCompile(t, ctx, session, map[string]any{
			"objective":             "Fix the parser output correctly",
			"acceptance_criteria":   []string{"Parser returns output", "Parser tests pass", "No regression remains"},
			"verification_commands": [][]string{{"go", "test", "./..."}},
			"allowed_paths":         []string{"internal/parser"},
			"risk_hint":             "write",
		})
		if envelope["status"] != "ok" {
			t.Fatalf("status = %v; want ok", envelope["status"])
		}
		data := missionData(t, envelope)
		missionID, _ := data["mission_id"].(string)
		if !strings.HasPrefix(missionID, "msn_") {
			t.Fatalf("mission_id = %q; want prefix msn_", missionID)
		}
		if data["ceremony"] != "full" {
			t.Fatalf("ceremony = %v; want full", data["ceremony"])
		}
	})

	t.Run("blocked input", func(t *testing.T) {
		envelope := callMissionCompile(t, ctx, session, map[string]any{
			"objective": "   ",
			"risk_hint": "write",
		})
		if envelope["status"] != "blocked" {
			t.Fatalf("status = %v; want blocked", envelope["status"])
		}
		data := missionData(t, envelope)
		if data["mission_id"] != "" {
			t.Fatalf("mission_id = %v; want empty", data["mission_id"])
		}
		if data["ceremony"] != "" {
			t.Fatalf("ceremony = %v; want empty", data["ceremony"])
		}
		assertLintLevel(t, data, "BLOCK")
	})

	t.Run("vague input", func(t *testing.T) {
		envelope := callMissionCompile(t, ctx, session, map[string]any{
			"objective": "Fix bug",
			"risk_hint": "write",
		})
		if envelope["status"] != "ok" {
			t.Fatalf("status = %v; want ok", envelope["status"])
		}
		assertLintLevel(t, missionData(t, envelope), "WARN")
		if actions, ok := envelope["next_actions"].([]any); !ok || len(actions) == 0 {
			t.Fatalf("next_actions = %#v; want non-empty array", envelope["next_actions"])
		}
	})

	t.Run("direct input", func(t *testing.T) {
		envelope := callMissionCompile(t, ctx, session, map[string]any{
			"objective": "Explain how this project works",
			"risk_hint": "safe",
		})
		if envelope["status"] != "ok" {
			t.Fatalf("status = %v; want ok", envelope["status"])
		}
		data := missionData(t, envelope)
		if data["ceremony"] != "direct" {
			t.Fatalf("ceremony = %v; want direct", data["ceremony"])
		}
		if data["mission_id"] != "" {
			t.Fatalf("mission_id = %v; want empty", data["mission_id"])
		}
		actions, ok := envelope["next_actions"].([]any)
		if !ok || len(actions) == 0 || !strings.Contains(actions[0].(string), "host answer suffices") {
			t.Fatalf("next_actions = %#v; want host answer suffices guidance", envelope["next_actions"])
		}
	})
}

func TestProjectInspectToolReturnsStructuredAndTextEnvelope(t *testing.T) {
	root := initGitRepository(t)
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/project\n\ngo 1.26\n"), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	srv := NewServer("test", root)
	session, ctx := connectMCPTestServer(t, srv, "test-client")

	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "jacu_project_inspect",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatalf("call tool: %v", err)
	}
	if result.IsError {
		t.Fatalf("tool result is error: %+v", result)
	}
	assertInspectEnvelope(t, result.StructuredContent)

	var text string
	for _, content := range result.Content {
		if value, ok := content.(*mcp.TextContent); ok {
			text = value.Text
			break
		}
	}
	if text == "" {
		t.Fatalf("TextContent ausente: %+v", result.Content)
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(text), &decoded); err != nil {
		t.Fatalf("TextContent não é JSON: %v: %q", err, text)
	}
	assertInspectEnvelope(t, decoded)
}

func assertInspectEnvelope(t *testing.T, value any) {
	t.Helper()
	envelope, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("envelope type = %T; want map[string]any", value)
	}
	if envelope["status"] != "ok" {
		t.Fatalf("status = %v; want ok", envelope["status"])
	}
	traceID, _ := envelope["trace_id"].(string)
	if !strings.HasPrefix(traceID, "tr_") {
		t.Fatalf("trace_id = %q; want prefix tr_", traceID)
	}
	data, ok := envelope["data"].(map[string]any)
	if !ok {
		t.Fatalf("data type = %T; want map[string]any", envelope["data"])
	}
	projectID, _ := data["project_id"].(string)
	if !strings.HasPrefix(projectID, "prj_") {
		t.Fatalf("project_id = %q; want prefix prj_", projectID)
	}
}

func TestServerDoesNotAdvertiseLogging(t *testing.T) {
	srv := NewServer("test", t.TempDir())
	session, _ := connectMCPTestServer(t, srv, "test-client")

	if logging := session.InitializeResult().Capabilities.Logging; logging != nil {
		t.Fatalf("logging capability must not be advertised, got %#v", logging)
	}
}

func findTool(t *testing.T, tools []*mcp.Tool, name string) *mcp.Tool {
	t.Helper()
	for _, tool := range tools {
		if tool.Name == name {
			return tool
		}
	}
	t.Fatalf("tool %q ausente", name)
	return nil
}

func callMissionCompile(t *testing.T, ctx context.Context, session *mcp.ClientSession, arguments map[string]any) map[string]any {
	t.Helper()
	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "jacu_mission_compile",
		Arguments: arguments,
	})
	if err != nil {
		t.Fatalf("call tool: %v", err)
	}
	if result.IsError {
		t.Fatalf("tool result is error: %+v", result)
	}
	envelope, ok := result.StructuredContent.(map[string]any)
	if !ok {
		t.Fatalf("envelope type = %T; want map[string]any", result.StructuredContent)
	}
	return envelope
}

func parseCommitTrailers(t *testing.T, root, message string) map[string][]string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "interpret-trailers", "--parse")
	cmd.Dir = root
	cmd.Env = testgit.Env()
	cmd.Stdin = strings.NewReader(message)
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err := cmd.Run(); err != nil {
		t.Fatalf("git interpret-trailers --parse: %v\n%s", err, output.String())
	}
	trailers := map[string][]string{}
	for _, line := range strings.Split(strings.TrimSpace(output.String()), "\n") {
		if line == "" {
			continue
		}
		key, value, ok := strings.Cut(line, ": ")
		if !ok {
			t.Fatalf("unparseable trailer line %q", line)
		}
		trailers[key] = append(trailers[key], value)
	}
	return trailers
}

func missionData(t *testing.T, envelope map[string]any) map[string]any {
	t.Helper()
	data, ok := envelope["data"].(map[string]any)
	if !ok {
		t.Fatalf("data type = %T; want map[string]any", envelope["data"])
	}
	return data
}

func assertLintLevel(t *testing.T, data map[string]any, level string) {
	t.Helper()
	items, ok := data["lint"].([]any)
	if !ok {
		t.Fatalf("lint type = %T; want array", data["lint"])
	}
	for _, item := range items {
		entry, ok := item.(map[string]any)
		if ok && entry["level"] == level {
			return
		}
	}
	t.Fatalf("lint = %#v; want level %s", items, level)
}

func concreteDataProperties(t *testing.T, tool *mcp.Tool) map[string]any {
	t.Helper()
	rawSchema, err := json.Marshal(tool.OutputSchema)
	if err != nil {
		t.Fatalf("marshal outputSchema: %v", err)
	}
	var schema map[string]any
	if err := json.Unmarshal(rawSchema, &schema); err != nil {
		t.Fatalf("decode outputSchema: %v", err)
	}
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("outputSchema.properties = %T; want object", schema["properties"])
	}
	data, ok := properties["data"].(map[string]any)
	if !ok || data["type"] != "object" {
		t.Fatalf("outputSchema.properties.data = %#v; want concrete object schema", properties["data"])
	}
	dataProperties, ok := data["properties"].(map[string]any)
	if !ok || len(dataProperties) == 0 {
		t.Fatalf("outputSchema.properties.data.properties = %#v; want non-empty object", data["properties"])
	}
	return dataProperties
}

func callToolEnvelope(t *testing.T, ctx context.Context, session *mcp.ClientSession, name string, arguments map[string]any) map[string]any {
	t.Helper()
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: arguments})
	if err != nil {
		t.Fatalf("call %s: %v", name, err)
	}
	if result.IsError {
		t.Fatalf("%s returned transport error: %+v", name, result)
	}
	envelope, ok := result.StructuredContent.(map[string]any)
	if !ok {
		t.Fatalf("%s envelope type = %T; want map[string]any", name, result.StructuredContent)
	}
	return envelope
}

func assertRuntimeEnvelope(t *testing.T, envelope map[string]any, wantStatus string) {
	t.Helper()
	if envelope["status"] != wantStatus {
		t.Fatalf("status = %v; want %s; envelope=%#v", envelope["status"], wantStatus, envelope)
	}
	if summary, ok := envelope["summary"].(string); !ok || summary == "" {
		t.Fatalf("summary = %#v; want non-empty string", envelope["summary"])
	}
	if _, ok := envelope["data"].(map[string]any); !ok {
		t.Fatalf("data = %#v; want object", envelope["data"])
	}
	for _, field := range []string{"artifacts", "warnings", "next_actions"} {
		if _, ok := envelope[field].([]any); !ok {
			t.Fatalf("%s = %#v; want array", field, envelope[field])
		}
	}
	traceID, _ := envelope["trace_id"].(string)
	if !strings.HasPrefix(traceID, "tr_") {
		t.Fatalf("trace_id = %q; want runtime trace", traceID)
	}
}

func envelopeData(t *testing.T, envelope map[string]any) map[string]any {
	t.Helper()
	data, ok := envelope["data"].(map[string]any)
	if !ok {
		t.Fatalf("data = %#v; want object", envelope["data"])
	}
	return data
}

func initGitRepository(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	runGit(t, root, "init", "-b", "main")
	runGit(t, root, "config", "user.name", "Jacu Test")
	runGit(t, root, "config", "user.email", "jacu@example.test")
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("initial\n"), 0o600); err != nil {
		t.Fatalf("write repository fixture: %v", err)
	}
	runGit(t, root, "add", "README.md")
	runGit(t, root, "commit", "-m", "initial")
	return root
}

func runGit(t *testing.T, root string, args ...string) string {
	t.Helper()
	command := exec.Command("git", "-C", root)
	command.Env = testgit.Env()
	command.Args = append(command.Args, args...)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, output)
	}
	return string(output)
}

// TestMissionCompileAnnouncesRiskHintEnum pins the first half of the risk_hint
// fix: the host cannot send a value outside the enum without the SDK rejecting
// it, so a malformed hint never reaches the lint in the first place.
func TestMissionCompileAnnouncesRiskHintEnum(t *testing.T) {
	srv := NewServer("test", t.TempDir())
	session, ctx := connectMCPTestServer(t, srv, "test-client")

	tools, err := session.ListTools(ctx, &mcp.ListToolsParams{})
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	tool := findTool(t, tools.Tools, "jacu_mission_compile")
	rawSchema, err := json.Marshal(tool.InputSchema)
	if err != nil {
		t.Fatalf("marshal inputSchema: %v", err)
	}
	var schema map[string]any
	if err := json.Unmarshal(rawSchema, &schema); err != nil {
		t.Fatalf("decode inputSchema: %v", err)
	}
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("inputSchema.properties = %T; want object", schema["properties"])
	}
	riskHint, ok := properties["risk_hint"].(map[string]any)
	if !ok {
		t.Fatalf("inputSchema.properties.risk_hint = %T; want object schema", properties["risk_hint"])
	}
	enum, ok := riskHint["enum"].([]any)
	if !ok {
		t.Fatalf("risk_hint.enum = %T; want the announced enum", riskHint["enum"])
	}
	want := []string{"safe", "write", "destructive"}
	if len(enum) != len(want) {
		t.Fatalf("risk_hint.enum = %v; want %v", enum, want)
	}
	for index, value := range want {
		if enum[index] != value {
			t.Fatalf("risk_hint.enum = %v; want %v", enum, want)
		}
	}
	if _, required := properties["objective"]; !required {
		t.Fatal("inputSchema lost objective; the explicit schema must keep the whole input")
	}
}

func TestRepoScopedToolsBlockOutsideAGitWorkTree(t *testing.T) {
	root := t.TempDir()
	srv := NewServer("test", root)
	session, ctx := connectMCPTestServer(t, srv, "cwd-guard")

	repoScoped := []string{
		"jacu_project_inspect",
		"jacu_mission_compile",
		"jacu_workspace_open",
		"jacu_diff",
		"jacu_apply",
		"jacu_discard",
		"jacu_report",
	}
	for _, name := range repoScoped {
		args := map[string]any{}
		switch name {
		case "jacu_mission_compile":
			args = map[string]any{"objective": "Fix the parser output correctly"}
		case "jacu_flow_run":
			args = map[string]any{"flow": map[string]any{"nodes": []map[string]any{{"id": "n1", "uses": "unknown"}}}}
		case "jacu_workspace_open":
			args = map[string]any{
				"mission_id": "msn_test",
				"mission_input": map[string]any{
					"objective": "Update the project readme safely",
					"risk_hint": "write",
				},
			}
		case "jacu_diff", "jacu_apply", "jacu_discard", "jacu_verify":
			args = map[string]any{"run_id": "run_0000000000000000"}
		}
		envelope := callToolEnvelope(t, ctx, session, name, args)
		if envelope["status"] != "blocked" {
			t.Fatalf("%s status = %v; want blocked outside a git work tree", name, envelope["status"])
		}
		summary, _ := envelope["summary"].(string)
		if !strings.Contains(summary, root) {
			t.Fatalf("%s summary %q does not name the cwd", name, summary)
		}
		if !strings.Contains(strings.ToLower(summary), "git") {
			t.Fatalf("%s summary %q does not instruct anchoring to a git work tree", name, summary)
		}
	}

	status := callToolEnvelope(t, ctx, session, "jacu_status", map[string]any{})
	if status["status"] == "blocked" {
		if summary, _ := status["summary"].(string); strings.Contains(strings.ToLower(summary), "git work tree") {
			t.Fatalf("jacu_status is user-scoped and must not use the work-tree guard: %q", summary)
		}
	}
}
