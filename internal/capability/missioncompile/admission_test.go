package missioncompile

import (
	"os"
	"path/filepath"
	"testing"

	ctxpack "github.com/jacu-dev/jacu-harness/internal/capability/context"
)

func TestCompileBlocksWhenRequiredContextOverflowsBudget(t *testing.T) {
	root := t.TempDir()
	huge := make([]byte, ctxpack.DefaultBudget+1)
	if err := os.WriteFile(filepath.Join(root, "blob.bin"), huge, 0o600); err != nil {
		t.Fatal(err)
	}
	mission, status, next := Compile(root, Input{
		Objective:    "use the blob",
		AllowedPaths: []string{"blob.bin"},
	})
	if status != "blocked" {
		t.Fatalf("status = %s mission=%+v next=%v", status, mission, next)
	}
}
