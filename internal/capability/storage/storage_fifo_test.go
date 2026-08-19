//go:build !windows

package storage

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestToolchainFIFOIsReportOnly(t *testing.T) {
	root, options, now := storageFixture(t)
	if err := os.MkdirAll(options.ToolchainDir, 0o700); err != nil {
		t.Fatal(err)
	}
	fifo := filepath.Join(options.ToolchainDir, "unknown.fifo")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Fatal(err)
	}
	old := now.Add(-31 * 24 * time.Hour)
	if err := os.Chtimes(options.ToolchainDir, old, old); err != nil {
		t.Fatal(err)
	}

	report := PruneWithOptions(root, options, true)
	for _, action := range report.Actions {
		if action.Owner == "toolchain" {
			t.Fatalf("FIFO-containing cache became removable: %+v", report)
		}
	}
	info, err := os.Lstat(fifo)
	if err != nil {
		t.Fatalf("FIFO was removed: %v", err)
	}
	if info.Mode()&os.ModeNamedPipe == 0 {
		t.Fatalf("FIFO mode changed: %v", info.Mode())
	}
}
