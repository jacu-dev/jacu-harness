package memory

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/jacu-dev/jacu-harness/internal/project"
)

func TestFuzzSnapshotDetectsSiblingWrite(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "root")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatalf("mkdir root: %v", err)
	}
	before, err := snapshotDirectory(parent)
	if err != nil {
		t.Fatalf("snapshot before: %v", err)
	}
	if err := os.WriteFile(filepath.Join(parent, "sibling.txt"), []byte("outside"), 0o600); err != nil {
		t.Fatalf("sibling write: %v", err)
	}
	after, err := snapshotDirectory(parent)
	if err != nil {
		t.Fatalf("snapshot after: %v", err)
	}
	if err := assertNoOutsideWrites(before, after, parent, root); err == nil {
		t.Fatal("sibling write was not detected")
	}
}

func FuzzMemorySaveInput(f *testing.F) {
	f.Add([]byte(`{"project_id":"","kind":"preference","title":"Keep files local","body":"Use the local store.","evidence":["docs/adr/ADR-001.md"],"source":"human"}`))
	f.Add([]byte(`{"project_id":"__FUZZ_PROJECT_ROOT__","kind":"decision","title":"Keep files local","body":"Use the local store.","evidence":["docs/adr/ADR-001.md"],"source":"human"}`))
	f.Add([]byte("not json"))
	f.Fuzz(func(t *testing.T, data []byte) {
		runFuzzMemorySaveInput(t, data)
	})
}

func FuzzMemoryRecallQuery(f *testing.F) {
	f.Add("local store")
	f.Add("\x00\xff hostile query")
	f.Fuzz(func(t *testing.T, query string) {
		runFuzzMemoryRecallQuery(t, query)
	})
}

func runFuzzMemorySaveInput(t *testing.T, data []byte) {
	t.Helper()
	// Keep fuzz cases bounded before JSON decoding can allocate from hostile input.
	if len(data) > 64*1024 {
		return
	}
	var input Input
	if err := json.Unmarshal(data, &input); err != nil {
		return
	}
	parent := t.TempDir()
	root := filepath.Join(parent, "root")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatalf("mkdir root: %v", err)
	}
	before, err := snapshotDirectory(parent)
	if err != nil {
		t.Fatalf("snapshot before: %v", err)
	}
	normalized := normalize(input)
	projectSeed := normalized.ProjectID == "__FUZZ_PROJECT_ROOT__"
	globalSeed := normalized.ProjectID == "" && normalized.Kind == "preference"
	if projectSeed {
		projectID, err := project.ID(root)
		if err != nil {
			t.Fatalf("project ID: %v", err)
		}
		normalized.ProjectID = projectID
	}
	lints := lint(root, normalized)
	if hasBlock(lints) {
		after, err := snapshotDirectory(parent)
		if err != nil {
			t.Fatalf("snapshot after BLOCK: %v", err)
		}
		if diff := snapshotDiff(before, after); diff != "" {
			t.Fatalf("BLOCK lint persisted outside/root state: %s", diff)
		}
		return
	}
	record := Record{
		MemoryID: memoryID(normalized), ProjectID: normalized.ProjectID,
		Kind: normalized.Kind, Title: normalized.Title, Body: normalized.Body,
		Evidence: normalized.Evidence, Source: normalized.Source,
		Status: "active", CreatedAt: "2026-01-01T00:00:00Z", UpdatedAt: "2026-01-01T00:00:00Z",
	}
	store := NewFileStore(root)
	saveErr := store.Save(record, normalized.Supersedes)
	if (projectSeed || globalSeed) && saveErr != nil {
		t.Fatalf("valid fuzz seed Save: %v", saveErr)
	}
	if projectSeed || globalSeed {
		if _, ok := store.Get(record.MemoryID); !ok {
			t.Fatal("valid fuzz seed Save did not persist a record")
		}
	}
	after, err := snapshotDirectory(parent)
	if err != nil {
		t.Fatalf("snapshot after Save: %v", err)
	}
	if err := assertNoOutsideWrites(before, after, parent, root); err != nil {
		t.Fatal(err)
	}
}

func runFuzzMemoryRecallQuery(t *testing.T, query string) {
	t.Helper()
	if len(query) > 16*1024 {
		query = query[:16*1024]
	}
	parent := t.TempDir()
	root := filepath.Join(parent, "root")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatalf("mkdir root: %v", err)
	}
	before, err := snapshotDirectory(parent)
	if err != nil {
		t.Fatalf("snapshot before: %v", err)
	}
	projectID, err := project.ID(root)
	if err != nil {
		t.Fatalf("project ID: %v", err)
	}
	store := NewFileStore(root)
	seed := []Record{
		{MemoryID: "mem_0000000000000001", ProjectID: projectID, Kind: "decision", Title: "Local memory", Body: "Keep records local", Evidence: []string{"docs/design.md"}, Source: "human", Status: "active", CreatedAt: "2026-01-01T00:00:00Z", UpdatedAt: "2026-01-01T00:00:00Z"},
		{MemoryID: "mem_0000000000000002", ProjectID: projectID, Kind: "convention", Title: "Stable ranking", Body: "Sort ties by id", Evidence: []string{"docs/design.md"}, Source: "human", Status: "active", CreatedAt: "2026-01-01T00:00:00Z", UpdatedAt: "2026-01-01T00:00:00Z"},
		{MemoryID: "mem_0000000000000003", ProjectID: projectID, Kind: "gotcha", Title: "No hidden writes", Body: "Recall is read only", Evidence: []string{"docs/design.md"}, Source: "human", Status: "active", CreatedAt: "2026-01-01T00:00:00Z", UpdatedAt: "2026-01-01T00:00:00Z"},
	}
	for _, record := range seed {
		if err := store.Save(record, ""); err != nil {
			t.Fatalf("seed Save: %v", err)
		}
	}
	q := SearchQuery{ProjectID: projectID, Query: query, K: 3}
	first, second := store.Search(q), store.Search(q)
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("identical searches differ: %#v != %#v", first, second)
	}
	after, err := snapshotDirectory(parent)
	if err != nil {
		t.Fatalf("snapshot after recall: %v", err)
	}
	if err := assertNoOutsideWrites(before, after, parent, root); err != nil {
		t.Fatal(err)
	}
}

func hasBlock(lints []Lint) bool {
	for _, item := range lints {
		if item.Level == "BLOCK" {
			return true
		}
	}
	return false
}

func snapshotDirectory(parent string) (map[string]string, error) {
	snapshot := make(map[string]string)
	err := filepath.WalkDir(parent, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(parent, path)
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		value := fmt.Sprintf("%s:%d", info.Mode().String(), info.Size())
		if info.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			value += ":" + target
		} else if info.Mode().IsRegular() {
			content, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			hash := sha256.Sum256(content)
			value += fmt.Sprintf(":%x", hash[:])
		}
		snapshot[rel] = value
		return nil
	})
	return snapshot, err
}

func assertNoOutsideWrites(before, after map[string]string, parent, root string) error {
	diff := snapshotDiffOutside(before, after, parent, root)
	if diff != "" {
		return fmt.Errorf("write escaped configured root: %s", diff)
	}
	return nil
}

func snapshotDiff(before, after map[string]string) string {
	keys := make(map[string]struct{}, len(before)+len(after))
	for key := range before {
		keys[key] = struct{}{}
	}
	for key := range after {
		keys[key] = struct{}{}
	}
	return snapshotDiffKeys(before, after, keys)
}

func snapshotDiffOutside(before, after map[string]string, parent, root string) string {
	rootRel, err := filepath.Rel(parent, root)
	if err != nil {
		return err.Error()
	}
	keys := make(map[string]struct{}, len(before)+len(after))
	for key := range before {
		keys[key] = struct{}{}
	}
	for key := range after {
		keys[key] = struct{}{}
	}
	for key := range keys {
		if key == rootRel || strings.HasPrefix(key, rootRel+string(os.PathSeparator)) {
			delete(keys, key)
		}
	}
	return snapshotDiffKeys(before, after, keys)
}

func snapshotDiffKeys(before, after map[string]string, keys map[string]struct{}) string {
	var changed []string
	for key := range keys {
		if before[key] != after[key] {
			changed = append(changed, key)
		}
	}
	sort.Strings(changed)
	return strings.Join(changed, ", ")
}
