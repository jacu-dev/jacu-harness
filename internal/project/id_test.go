package project

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestIDUsesResolvedAbsoluteRoot(t *testing.T) {
	root := t.TempDir()
	link := filepath.Join(t.TempDir(), "project-link")
	if err := os.Symlink(root, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	resolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatalf("resolve root: %v", err)
	}
	absolute, err := filepath.Abs(resolved)
	if err != nil {
		t.Fatalf("absolute root: %v", err)
	}
	digest := sha256.Sum256([]byte(absolute))
	want := "prj_" + fmt.Sprintf("%x", digest[:8])

	got, err := ID(link)
	if err != nil {
		t.Fatalf("ID: %v", err)
	}
	if got != want {
		t.Fatalf("ID = %q; want %q", got, want)
	}
}

func TestIDRejectsMissingRoot(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing")

	if _, err := ID(missing); err == nil {
		t.Fatal("ID missing root returned nil error")
	}
}
