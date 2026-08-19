package orchestration

import (
	"context"
	"fmt"
	"sort"

	"github.com/jacu-dev/jacu-harness/internal/gitx"
)

// FanIn applies patches captured from agent worktrees to an owned target. A
// content collision returns (false, nil) after resetting the target to base;
// that is a governance escalation, not a mechanical failure. The caller owns
// the target and decides whether to preserve the source worktrees.
func FanIn(ctx context.Context, git *gitx.Git, target, baseSHA string, worktrees []string) (bool, error) {
	if git == nil {
		return false, fmt.Errorf("fan-in git client is nil")
	}
	ordered := append([]string{}, worktrees...)
	sort.Strings(ordered)
	patches := make([]string, 0, len(ordered))
	for _, worktree := range ordered {
		patch, err := git.CaptureStagedPatch(ctx, worktree, baseSHA)
		if err != nil {
			return false, err
		}
		if patch != "" {
			patches = append(patches, patch)
		}
	}
	for _, patch := range patches {
		applied, err := git.ApplyPatch3Way(ctx, target, patch)
		if err != nil {
			if resetErr := git.ResetHard(ctx, target, baseSHA); resetErr != nil {
				return false, fmt.Errorf("fan-in mechanical failure: %v; reset to base failed: %w", err, resetErr)
			}
			return false, err
		}
		if !applied {
			if err := git.ResetHard(ctx, target, baseSHA); err != nil {
				return false, fmt.Errorf("fan-in conflict reset failed: %w", err)
			}
			return false, nil
		}
	}
	return true, nil
}
