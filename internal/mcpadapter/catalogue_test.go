package mcpadapter

import (
	"encoding/json"
	"strings"
	"testing"

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
