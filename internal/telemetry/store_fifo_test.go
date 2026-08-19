//go:build !windows

package telemetry

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestStoreRefusesFIFOSegment(t *testing.T) {
	store := NewStoreAt(t.TempDir())
	if err := os.MkdirAll(store.Directory(), 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(store.Directory(), "events-2026-08.jsonl")
	if err := syscall.Mkfifo(path, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := store.Emit(mustEvent(validEventInput())); err == nil {
		t.Fatal("Emit accepted FIFO segment")
	}
}
