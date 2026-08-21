// Package mcpadapter is the MCP boundary: transports, negotiation, codec.
// Domain logic never lives here.
package mcpadapter

import (
	"context"

	"github.com/jacu-dev/jacu-harness/internal/capability/memory"
	"github.com/jacu-dev/jacu-harness/internal/capability/missioncompile"
	"github.com/jacu-dev/jacu-harness/internal/capability/orchestration"
	"github.com/jacu-dev/jacu-harness/internal/capability/projectinspect"
	reportcapability "github.com/jacu-dev/jacu-harness/internal/capability/report"
	"github.com/jacu-dev/jacu-harness/internal/capability/verify"
	"github.com/jacu-dev/jacu-harness/internal/capability/workspace"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var SupportedProtocolVersions = []string{"2026-07-28", "2025-11-25"}

func NewServer(version, root string) *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "jacu",
		Version: version,
	}, &mcp.ServerOptions{
		Capabilities: &mcp.ServerCapabilities{},
	})
	taskManager, err := verify.NewTaskManager(root)
	if err != nil {
		panic("mcpadapter: initialize task manager: " + err.Error())
	}
	projectinspect.RegisterTool(server, root)
	missioncompile.RegisterTool(server, root)
	workspace.RegisterToolWithTaskManager(server, root, taskManager)
	memory.RegisterTool(server, root)
	verify.RegisterToolWithTaskManager(server, root, taskManager)
	orchestration.RegisterTool(server, root, taskManager)
	reportcapability.RegisterTool(server, root)
	server.AddReceivingMiddleware(compactToolsListMiddleware)
	return server
}

// RunStdio serves MCP over stdin/stdout. cmd/jacu must call this instead of
// importing the SDK: stdout is the protocol wire.
func RunStdio(ctx context.Context, version, root string) error {
	return NewServer(version, root).Run(ctx, &mcp.StdioTransport{})
}
