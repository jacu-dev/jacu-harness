package runtime

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jacu-dev/jacu-harness/internal/gitx"
)

// WorkTreeBlock returns a blocked envelope when root is not inside a git work
// tree. Repo-scoped tools use this instead of inspecting the wrong directory.
func WorkTreeBlock(ctx context.Context, root string) (Result, bool) {
	git, err := gitx.New()
	if err != nil || !git.InsideWorkTree(ctx, root) {
		return Result{
			Status:  "blocked",
			Summary: fmt.Sprintf("cwd %q is not inside a git work tree; start jacu serve from a repository (or emit an anchored host pack: jacu doctor --emit claude-desktop --repo <repo>)", root),
			NextActions: []string{
				"cd into the repository and restart the MCP server",
				"or register a host pack that anchors cwd to the repository",
			},
		}, true
	}
	return Result{}, false
}

// RequireWorkTree wraps a handler so it refuses to run outside a git work tree.
func RequireWorkTree(root string, next Handler) Handler {
	return func(ctx context.Context, input json.RawMessage) (Result, error) {
		if blocked, ok := WorkTreeBlock(ctx, root); ok {
			return blocked, nil
		}
		if next == nil {
			return Result{Status: "failed", Summary: "capability handler is missing"}, nil
		}
		return next(ctx, input)
	}
}
