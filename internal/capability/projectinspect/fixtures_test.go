package projectinspect

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func writeFixture(t testing.TB, root, name, content string) {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir fixture: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
}

func writeFiles(t testing.TB, root string, count int) {
	t.Helper()
	for i := 0; i < count; i++ {
		writeFixture(t, root, fmt.Sprintf("file-%05d.txt", i), "x")
	}
}
