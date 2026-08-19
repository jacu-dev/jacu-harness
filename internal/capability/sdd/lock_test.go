package sdd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteLockContainsOnlyStructuralDataAndDetectsDrift(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "001-example")
	if err := os.Mkdir(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	document, err := Parse([]byte("---\nsdd: 001-example\n---\n# Example\n## Requirements\n### Requirement: Safe\nThe system SHALL be safe.\n#### Scenario: Works\n- **WHEN** input\n- **THEN** output\nDelta: ADDED\n"))
	if err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if closeErr := root.Close(); closeErr != nil {
			t.Errorf("close lock root: %v", closeErr)
		}
	})
	if writeErr := WriteLockRoot(root, ".", document); writeErr != nil {
		t.Fatalf("WriteLockRoot() error = %v", writeErr)
	}
	lockBytes, err := root.ReadFile("sdd.lock.json")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(lockBytes), "The system SHALL") {
		t.Fatalf("lock contains prose: %s", lockBytes)
	}
	if findings := lintDocumentWithLock(document, directory, nil, lockBytes); findCode(findings, "sdd_stale_lock") != nil {
		t.Fatalf("fresh lock was reported stale: %#v", findings)
	}
	document.Requirements[0].Text = "The system SHALL reject unsafe input."
	if findings := lintDocumentWithLock(document, directory, nil, lockBytes); findCode(findings, "sdd_stale_lock") == nil {
		t.Fatalf("drift was not reported: %#v", findings)
	}
}
