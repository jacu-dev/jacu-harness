package workspace

import (
	"context"

	"github.com/jacu-dev/jacu-harness/internal/capability/verify"
	"github.com/jacu-dev/jacu-harness/internal/gitx"
	capabilityruntime "github.com/jacu-dev/jacu-harness/internal/runtime"
)

const CLIHostName = "jacu-cli"

func RunOpen(ctx context.Context, root string, in OpenInput) capabilityruntime.Result {
	return capabilityruntime.ExecuteInput(ctx, workspaceOpenCapability(root), in)
}

func RunStatus(ctx context.Context, root string, in StatusInput) capabilityruntime.Result {
	root = gitx.OwningRepository(root)
	manager, err := verify.NewTaskManager(root)
	if err != nil {
		return capabilityruntime.ExecuteInput(ctx, workspaceStatusCapabilityWithTaskManager(root, nil), in)
	}
	return RunStatusWithManager(ctx, root, in, manager)
}

func RunStatusWithManager(ctx context.Context, root string, in StatusInput, manager *verify.TaskManager) capabilityruntime.Result {
	return capabilityruntime.ExecuteInput(ctx, workspaceStatusCapabilityWithTaskManager(root, manager), in)
}

func RunDiff(ctx context.Context, root string, in DiffInput) capabilityruntime.Result {
	return capabilityruntime.ExecuteInput(ctx, workspaceDiffCapability(root), in)
}

func RunApply(ctx context.Context, root string, in ApplyInput, hostName string) capabilityruntime.Result {
	if hostName == "" {
		hostName = defaultHostName
	}
	return capabilityruntime.ExecuteInput(ctx, workspaceApplyCapability(root, hostName), in)
}

func RunDiscard(ctx context.Context, root string, in DiscardInput) capabilityruntime.Result {
	return capabilityruntime.ExecuteInput(ctx, workspaceDiscardCapability(root), in)
}
