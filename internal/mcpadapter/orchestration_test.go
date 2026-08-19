package mcpadapter

import (
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestFlowToolAdvertisesOneCompactContractAndBlocksBeforeExecution(t *testing.T) {
	srv := NewServer("test", t.TempDir())
	session, ctx := connectMCPTestServer(t, srv, "flow-test")
	tools, err := session.ListTools(ctx, &mcp.ListToolsParams{})
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	flow := findTool(t, tools.Tools, "jacu_flow_run")
	if flow.InputSchema == nil || flow.OutputSchema == nil || flow.Annotations == nil {
		t.Fatal("flow tool must expose schema and annotations")
	}
	if flow.Annotations.ReadOnlyHint || flow.Annotations.IdempotentHint {
		t.Fatalf("flow annotations = %#v; want mutable and non-idempotent", flow.Annotations)
	}
	properties := concreteDataProperties(t, flow)
	for _, field := range []string{"task", "flow"} {
		if _, ok := properties[field]; !ok {
			t.Fatalf("flow output data missing %q: %#v", field, properties)
		}
	}
	result := callToolEnvelope(t, ctx, session, "jacu_flow_run", map[string]any{
		"flow": map[string]any{"nodes": []map[string]any{{"id": "bad", "uses": "unknown"}}},
	})
	assertRuntimeEnvelope(t, result, "blocked")
}
