package cleanexit

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRemovePreservesUserFileAndEscalatesUnmergedBranch(t *testing.T) {
	project := t.TempDir()
	userFile := filepath.Join(project, "user-not-jacu.txt")
	if err := os.WriteFile(userFile, []byte("keep\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	report := Report{Verdict: "fail", Findings: []Finding{
		{Class: "untracked", Target: userFile},
		{Class: "branch_local", Target: "jacu/run_0123456789abcdef"},
	}}
	result := Remove(project, report)
	if _, err := os.Stat(userFile); err != nil {
		t.Fatalf("user file was removed: %v", err)
	}
	if result.Verdict != "fail" || len(result.Removed) != 0 {
		t.Fatalf("removal result = %+v; want failed no-op", result)
	}
}
