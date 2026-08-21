package context

import (
	"os"
	"path/filepath"
	"testing"
)

func FuzzPackNeverPanics(f *testing.F) {
	f.Add([]byte("objective"), []byte("content"))
	f.Fuzz(func(t *testing.T, objective, content []byte) {
		defer func() {
			if recovered := recover(); recovered != nil {
				t.Fatalf("pack panicked: %v", recovered)
			}
		}()
		root := t.TempDir()
		path := filepath.Join(root, "input.txt")
		if err := os.WriteFile(path, content, 0o600); err != nil {
			t.Fatal(err)
		}
		pack, err := PackRoot(root, Spec{Objective: string(objective), AllowedPaths: []string{"input.txt"}})
		if err != nil {
			if _, ok := err.(Finding); !ok {
				t.Fatalf("untyped error %T", err)
			}
			return
		}
		_ = Digest(pack)
		_ = CheckAnchors(pack)
	})
}
