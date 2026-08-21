package context

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPackIsByteIdenticalAcrossRunsAndWalkOrder(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{"b.txt": "beta", "a.txt": "alpha", "nested/c.txt": "gamma"})
	spec := Spec{Objective: "pack files", AllowedPaths: []string{"."}, BudgetBytes: DefaultBudget}
	first, err := PackRoot(root, spec)
	if err != nil {
		t.Fatal(err)
	}
	second, err := PackRoot(root, spec)
	if err != nil {
		t.Fatal(err)
	}
	if first.Digest != second.Digest {
		t.Fatalf("digest %s vs %s", first.Digest, second.Digest)
	}
	if string(Canonical(first)) != string(Canonical(second)) {
		t.Fatalf("canonical pack differed")
	}
	if len(first.Items) < 3 {
		t.Fatalf("items = %d; want files plus objective", len(first.Items))
	}
}

func writeTree(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for path, content := range files {
		full := filepath.Join(root, path)
		if err := os.MkdirAll(filepath.Dir(full), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}
