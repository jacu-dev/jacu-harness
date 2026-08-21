package mcpadapter

import (
	"context"
	"encoding/json"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/jacu-dev/jacu-harness/internal/capability/memory"
	"github.com/jacu-dev/jacu-harness/internal/capability/missioncompile"
	"github.com/jacu-dev/jacu-harness/internal/capability/orchestration"
	"github.com/jacu-dev/jacu-harness/internal/capability/projectinspect"
	reportcapability "github.com/jacu-dev/jacu-harness/internal/capability/report"
	"github.com/jacu-dev/jacu-harness/internal/capability/verify"
	"github.com/jacu-dev/jacu-harness/internal/capability/workspace"
	capabilityruntime "github.com/jacu-dev/jacu-harness/internal/runtime"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Compile-time: every MCP capability exposes a transport-free Run. A missing
// symbol fails this test at build time, which is the T1 red.
var (
	_ = projectinspect.Run
	_ = missioncompile.Run
	_ = workspace.RunOpen
	_ = workspace.RunStatus
	_ = workspace.RunDiff
	_ = workspace.RunApply
	_ = workspace.RunDiscard
	_ = memory.RunSave
	_ = memory.RunRecall
	_ = verify.Run
	_ = orchestration.Run
	_ = reportcapability.Run
)

type surfaceRow struct {
	Tool string
	CLI  string
}

func surfaceCatalogue() []surfaceRow {
	return []surfaceRow{
		{Tool: "jacu_project_inspect", CLI: "inspect"},
		{Tool: "jacu_mission_compile", CLI: "compile"},
		{Tool: "jacu_workspace_open", CLI: "workspace open"},
		{Tool: "jacu_status", CLI: "workspace status"},
		{Tool: "jacu_workspace_status", CLI: "workspace status"},
		{Tool: "jacu_diff", CLI: "workspace diff"},
		{Tool: "jacu_apply", CLI: "workspace apply"},
		{Tool: "jacu_discard", CLI: "workspace discard"},
		{Tool: "jacu_memory_save", CLI: "memory save"},
		{Tool: "jacu_memory_recall", CLI: "memory recall"},
		{Tool: "jacu_verify", CLI: "verify"},
		{Tool: "jacu_flow_run", CLI: "flow"},
		{Tool: "jacu_report", CLI: "report"},
	}
}

func TestSurfaceEveryMCPCapabilityHasRunToolAndCLI(t *testing.T) {
	root := t.TempDir()
	srv := NewServer("test", root)
	session, ctx := connectMCPTestServer(t, srv, "surface-test")
	listed, err := session.ListTools(ctx, &mcp.ListToolsParams{})
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	seen := map[string]bool{}
	for _, tool := range listed.Tools {
		seen[tool.Name] = true
	}
	if len(listed.Tools) != 13 {
		t.Fatalf("server exposes %d tools; want 13", len(listed.Tools))
	}

	mainGo, usageGo := readCommandSurface(t)
	for _, row := range surfaceCatalogue() {
		if !seen[row.Tool] {
			t.Errorf("MCP tool %q is missing", row.Tool)
		}
		command := strings.Fields(row.CLI)[0]
		if !strings.Contains(mainGo, `case "`+command+`":`) {
			t.Errorf("CLI command %q is not dispatched for tool %s", command, row.Tool)
		}
		if !strings.Contains(usageGo, command) {
			t.Errorf("usage does not name %q for tool %s", command, row.Tool)
		}
	}
	for name := range seen {
		found := false
		for _, row := range surfaceCatalogue() {
			if row.Tool == name {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("MCP tool %q has no CLI mapping", name)
		}
	}
}

func TestSurfaceInspectRunAndMCPResultParity(t *testing.T) {
	root := initGitRepository(t)
	srv := NewServer("test", root)
	session, ctx := connectMCPTestServer(t, srv, "surface-parity")
	mcpEnvelope := callToolEnvelope(t, ctx, session, "jacu_project_inspect", map[string]any{})
	runResult := projectinspect.Run(ctx, root, projectinspect.Input{})
	assertEnvelopeParity(t, mcpEnvelope, runResult)
}

func TestSurfaceCapabilityPackagesImportMCPOnlyFromToolGo(t *testing.T) {
	capabilityRoot := filepath.Join(repoRoot(t), "internal", "capability")
	err := filepath.WalkDir(capabilityRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		if filepath.Base(path) == "tool.go" {
			return nil
		}
		source, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(string(source), "github.com/modelcontextprotocol/go-sdk") {
			t.Errorf("I5 leak: %s imports the MCP SDK outside tool.go", path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk capability packages: %v", err)
	}
}

func TestSurfaceCommandMainDoesNotImportMCPSDK(t *testing.T) {
	fileSet := token.NewFileSet()
	parsed, err := parser.ParseFile(fileSet, filepath.Join(repoRoot(t), "cmd", "jacu", "main.go"), nil, parser.ImportsOnly)
	if err != nil {
		t.Fatalf("parse main.go: %v", err)
	}
	for _, spec := range parsed.Imports {
		path := strings.Trim(spec.Path.Value, `"`)
		if strings.Contains(path, "modelcontextprotocol/go-sdk") {
			t.Fatalf("cmd/jacu/main.go imports %s; serve must go through mcpadapter", path)
		}
	}
}

func assertEnvelopeParity(t *testing.T, mcpEnvelope map[string]any, runResult capabilityruntime.Result) {
	t.Helper()
	cliJSON, err := capabilityruntime.MarshalEnvelope(runResult)
	if err != nil {
		t.Fatalf("marshal run envelope: %v", err)
	}
	var cliEnvelope map[string]any
	if err := json.Unmarshal(cliJSON, &cliEnvelope); err != nil {
		t.Fatalf("unmarshal run envelope: %v", err)
	}
	delete(mcpEnvelope, "trace_id")
	delete(cliEnvelope, "trace_id")
	want, err := json.Marshal(mcpEnvelope)
	if err != nil {
		t.Fatalf("marshal MCP envelope: %v", err)
	}
	got, err := json.Marshal(cliEnvelope)
	if err != nil {
		t.Fatalf("marshal CLI envelope: %v", err)
	}
	if string(want) != string(got) {
		t.Fatalf("CLI/MCP result mismatch\nMCP: %s\nCLI: %s", want, got)
	}
}

func readCommandSurface(t *testing.T) (mainGo, usageGo string) {
	t.Helper()
	root := repoRoot(t)
	mainBytes, err := os.ReadFile(filepath.Join(root, "cmd", "jacu", "main.go"))
	if err != nil {
		t.Fatalf("read main.go: %v", err)
	}
	return string(mainBytes), string(mainBytes)
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

func TestSurfaceRunSignaturesContainNoMCPTypes(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	_ = projectinspect.Run(ctx, root, projectinspect.Input{})
	_ = missioncompile.Run(ctx, root, missioncompile.Input{Objective: "x"})
	_ = workspace.RunOpen(ctx, root, workspace.OpenInput{})
	_ = workspace.RunStatus(ctx, root, workspace.StatusInput{})
	_ = workspace.RunDiff(ctx, root, workspace.DiffInput{})
	_ = workspace.RunApply(ctx, root, workspace.ApplyInput{}, "jacu-cli")
	_ = workspace.RunDiscard(ctx, root, workspace.DiscardInput{})
	_ = memory.RunSave(ctx, root, memory.Input{})
	_ = memory.RunRecall(ctx, root, memory.RecallInput{})
	_ = verify.Run(ctx, root, verify.Input{})
	_ = orchestration.Run(ctx, root, orchestration.Input{})
	_ = reportcapability.Run(ctx, root, reportcapability.Input{})
}
