package runstate

import (
	"fmt"
	"os"
	"path/filepath"
)

// WithLock serializes runstate mutations for one repository across processes.
// The lock file is deliberately outside the individual run JSON files so the
// critical section also covers worktree and branch operations owned by a
// workspace capability.
func WithLock(repo string, fn func() error) error {
	if fn == nil {
		return fmt.Errorf("runstate lock callback is nil")
	}
	dir := filepath.Join(repo, ".git", "jacu")
	// #nosec G703 -- repo is the absolute project root resolved by the server;
	// the lock path is fixed below its .git/jacu state directory.
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("prepare runstate lock directory: %w", err)
	}
	// #nosec G304,G703 -- the filename is fixed and dir is derived from the resolved
	// project root, never from a run_id or command argument.
	file, err := os.OpenFile(filepath.Join(dir, "runs.lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return fmt.Errorf("open runstate lock: %w", err)
	}
	if err := lockRunstateFile(file); err != nil {
		_ = file.Close()
		return fmt.Errorf("lock runstate: %w", err)
	}
	callbackErr := fn()
	unlockErr := unlockRunstateFile(file)
	closeErr := file.Close()
	if callbackErr != nil {
		return callbackErr
	}
	if unlockErr != nil {
		return fmt.Errorf("unlock runstate: %w", unlockErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close runstate lock: %w", closeErr)
	}
	return nil
}
