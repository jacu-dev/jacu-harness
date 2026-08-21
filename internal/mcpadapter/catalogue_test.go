package mcpadapter

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/jacu-dev/jacu-harness/test/hosteval"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const maxToolsListBytes = 20 * 1024
const minToolsListHeadroom = 2 * 1024

func TestCatalogueCompactsEnvelopeWithDefs(t *testing.T) {
	srv := NewServer("test", t.TempDir())
	session, ctx := connectMCPTestServer(t, srv, "catalogue-test")
	listed, err := session.ListTools(ctx, &mcp.ListToolsParams{})
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	if len(listed.Tools) != 13 {
		t.Fatalf("tools = %d; want 13", len(listed.Tools))
	}
	encoded, err := json.Marshal(listed)
	if err != nil {
		t.Fatal(err)
	}
	headroom := maxToolsListBytes - len(encoded)
	if headroom < minToolsListHeadroom {
		t.Fatalf("tools/list = %d bytes; headroom %d; want at least %d under cap %d", len(encoded), headroom, minToolsListHeadroom, maxToolsListBytes)
	}
	raw := string(encoded)
	if !strings.Contains(raw, `"$defs"`) || !strings.Contains(raw, `"$ref"`) {
		t.Fatalf("compacted catalogue missing $defs/$ref")
	}
}

func TestAdvertisedCatalogueRoundTripRejectsTruncation(t *testing.T) {
	srv := NewServer("test", t.TempDir())
	session, ctx := connectMCPTestServer(t, srv, "catalogue-roundtrip")
	listed, err := session.ListTools(ctx, &mcp.ListToolsParams{})
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	advertised := make([]hosteval.ToolDesc, 0, len(listed.Tools))
	for _, tool := range listed.Tools {
		advertised = append(advertised, hosteval.ToolDesc{Name: tool.Name, Description: tool.Description})
	}
	if err = hosteval.CompareToolCatalogue(advertised, advertised); err != nil {
		t.Fatal(err)
	}
	if len(advertised) == 0 || advertised[0].Description == "" {
		t.Fatal("advertised catalogue is empty")
	}
	truncated := append([]hosteval.ToolDesc(nil), advertised...)
	want := truncated[0].Description
	truncated[0].Description = want[:len(want)/2]
	err = hosteval.CompareToolCatalogue(advertised, truncated)
	if err == nil {
		t.Fatal("truncated advertised catalogue must fail")
	}
	if !strings.Contains(err.Error(), advertised[0].Name) || !strings.Contains(err.Error(), "truncated") {
		t.Fatalf("error must name the tool and truncation: %v", err)
	}
}
