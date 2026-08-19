package runstate

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPersistedRunStateMatchesGolden(t *testing.T) {
	repo := newStateRepo(t)
	run := fixtureRun("run_0000000000000010", time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC))
	if err := Save(repo, run); err != nil {
		t.Fatalf("save fixture run: %v", err)
	}

	// #nosec G304 -- runPath is the canonical path in this test-owned repository.
	got, err := os.ReadFile(runPath(repo, run.RunID))
	if err != nil {
		t.Fatalf("read persisted run: %v", err)
	}
	// #nosec G304 -- the path is a repository-owned testdata fixture.
	want, err := os.ReadFile(filepath.Join("testdata", "run-v1.json"))
	if err != nil {
		t.Fatalf("read run-state golden: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("persisted run-state differs from golden\n got: %s\nwant: %s", got, want)
	}
}
