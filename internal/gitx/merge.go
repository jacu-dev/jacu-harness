package gitx

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// Push lives in cmd/jacu/deliver.go via Git.Exec, not in this file.

var (
	ErrNotFastForward = errors.New("not a fast-forward")
	ErrMergeConflict  = errors.New("merge conflict")
	ErrDetachedHEAD   = errors.New("HEAD is detached")
)

func (g *Git) CurrentBranch(ctx context.Context, repo string) (string, error) {
	name, err := g.Output(ctx, repo, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "", err
	}
	if name == "" || name == "HEAD" {
		return "", ErrDetachedHEAD
	}
	return name, nil
}

func (g *Git) IsClean(ctx context.Context, repo string) (bool, error) {
	status, err := g.StatusPorcelainZ(ctx, repo)
	if err != nil {
		return false, err
	}
	return status == "", nil
}

// HasTrackedChanges ignores untracked files, which a merge neither consumes nor
// overwrites. The checkout that runs autonomy always carries untracked JACU
// state — `.jacu/autonomy-policy.json` among them — and IsClean would refuse
// every local merge because of it.
func (g *Git) HasTrackedChanges(ctx context.Context, repo string) (bool, error) {
	status, err := g.OutputRaw(ctx, repo, "status", "--porcelain=v1", "-z", "--untracked-files=no")
	if err != nil {
		return false, err
	}
	return strings.TrimRight(status, "\x00") != "", nil
}

func (g *Git) MergeFFOnly(ctx context.Context, repo, branch string) error {
	_, stderr, err := g.run(ctx, repo, "merge", "--ff-only", branch)
	if err != nil {
		if isNotFastForward(stderr, err) {
			return fmt.Errorf("%w: %s", ErrNotFastForward, strings.TrimSpace(stderr))
		}
		return err
	}
	return nil
}

func (g *Git) MergeNoFF(ctx context.Context, repo, branch string) error {
	_, stderr, err := g.run(ctx, repo, "merge", "--no-ff", "--no-edit", branch)
	if err != nil {
		if isMergeConflict(stderr, err) {
			return fmt.Errorf("%w: %s", ErrMergeConflict, strings.TrimSpace(stderr))
		}
		return err
	}
	return nil
}

func (g *Git) MergeAbort(ctx context.Context, repo string) error {
	return g.Exec(ctx, repo, "merge", "--abort")
}

func isNotFastForward(stderr string, err error) bool {
	if errors.Is(err, ErrNotFastForward) {
		return true
	}
	combined := strings.ToLower(stderr + " " + err.Error())
	return strings.Contains(combined, "not possible to fast-forward") || strings.Contains(combined, "not a fast-forward")
}

func isMergeConflict(stderr string, err error) bool {
	if errors.Is(err, ErrMergeConflict) {
		return true
	}
	combined := strings.ToLower(stderr + " " + err.Error())
	return strings.Contains(combined, "conflict") || strings.Contains(combined, "automatic merge failed")
}
