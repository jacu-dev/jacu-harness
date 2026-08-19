package verify

import (
	"context"
)

// RunCommandInput is the one place a command may be named by the caller. It is
// still not a shell: argv is an array, it faces the same allowlist and the same
// limits as verification, and there is no timeout or policy parameter.
// RunCommandInput is retained as an internal source-compatibility alias while
// callers migrate to Input and the single jacu_verify MCP door.
type RunCommandInput = Input

// RunCommand executes a single diagnostic command inside a run worktree.
func RunCommand(ctx context.Context, root string, in RunCommandInput) Envelope {
	return Verify(ctx, root, Input(in))
}
