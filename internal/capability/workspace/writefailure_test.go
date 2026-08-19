package workspace

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// Tests that need a write under the run state directory to fail cannot express
// that failure with permission bits: the suite also runs as root inside
// containers, and root writes straight through a mode that would stop any other
// user. The injection used here removes the directory from the path instead —
// the runs directory is moved aside and a regular file takes its place, so
// every open and mkdir below it fails with ENOTDIR for root and non-root alike.
// The move is reversible, so the run state the test asserts on afterwards comes
// back byte for byte.
const blockedDirSuffix = ".jacu-blocked"

// installOneShotRunsDirBlock blocks the run state directory the first time git
// is invoked with the given subcommand. The wrapper lets that git call proceed
// afterwards, so the block lands between the run state read that precedes the
// subcommand and the next write.
func installOneShotRunsDirBlock(t *testing.T, subcommand, runsDir string) {
	t.Helper()
	realGit, err := exec.LookPath("git")
	if err != nil {
		t.Fatalf("git executable: %v", err)
	}
	wrapperDir := t.TempDir()
	marker := filepath.Join(t.TempDir(), "runs-dir-blocked")
	script := fmt.Sprintf(`#!/bin/sh
if [ "$1" = %q ] && [ ! -f %q ]; then
  : > %q
  mv %q %q || exit 92
  : > %q || exit 93
fi
exec %q "$@"
`, subcommand, marker, marker, runsDir, runsDir+blockedDirSuffix, runsDir, realGit)
	// #nosec G306 -- the test wrapper must be executable by the test process.
	if err := os.WriteFile(filepath.Join(wrapperDir, "git"), []byte(script), 0o700); err != nil {
		t.Fatalf("write git wrapper: %v", err)
	}
	t.Setenv("PATH", wrapperDir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// restoreBlockedRunsDir puts the run state directory back where a wrapper
// blocked it. It is a no-op when the block never fired, so tests can defer it
// as a safety net and still call it inline right after the blocked operation.
func restoreBlockedRunsDir(t *testing.T, runsDir string) error {
	t.Helper()
	blocked := runsDir + blockedDirSuffix
	info, err := os.Lstat(blocked)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("blocked run state backup %q is not a directory", blocked)
	}
	if removeErr := os.Remove(runsDir); removeErr != nil && !os.IsNotExist(removeErr) {
		return removeErr
	}
	return os.Rename(blocked, runsDir)
}
