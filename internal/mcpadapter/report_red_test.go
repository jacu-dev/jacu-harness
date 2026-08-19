package mcpadapter

import (
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestReportToolIsRegisteredWithConcreteReadOnlyMetadata(t *testing.T) {
	srv := NewServer("test", initGitRepository(t))
	session, ctx := connectMCPTestServer(t, srv, "report-test")
	tools, err := session.ListTools(ctx, &mcp.ListToolsParams{})
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	if len(tools.Tools) != 13 {
		t.Fatalf("server exposes %d tools; want 13", len(tools.Tools))
	}
	report := findTool(t, tools.Tools, "jacu_report")
	if report.OutputSchema == nil || report.Annotations == nil {
		t.Fatal("jacu_report must expose schema and annotations")
	}
	if !report.Annotations.ReadOnlyHint || !report.Annotations.IdempotentHint {
		t.Fatalf("jacu_report annotations = %#v; want read-only and idempotent", report.Annotations)
	}
	if report.Annotations.DestructiveHint == nil || *report.Annotations.DestructiveHint {
		t.Fatalf("jacu_report destructive hint = %#v; want explicit false", report.Annotations.DestructiveHint)
	}
	if report.Annotations.OpenWorldHint == nil || *report.Annotations.OpenWorldHint {
		t.Fatalf("jacu_report open-world hint = %#v; want explicit false", report.Annotations.OpenWorldHint)
	}
	concreteDataProperties(t, report)
	result := callToolEnvelope(t, ctx, session, "jacu_report", map[string]any{})
	assertRuntimeEnvelope(t, result, "ok")
	data := envelopeData(t, result)
	for _, field := range []string{"report", "markdown", "digest"} {
		if _, ok := data[field]; !ok {
			t.Fatalf("jacu_report data = %#v; missing %q", data, field)
		}
	}
}
