package cleanexit

import (
	"encoding/json"
	"github.com/jacu-dev/jacu-harness/internal/testgit"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestReceiptContainsVerdictClassesAndRemovedPathsOnly(t *testing.T) {
	receipt := NewReceipt(RemovalReport{
		Verdict: "pass", Removed: []string{"worktree"},
		Findings: []Finding{{Class: "untracked", Target: "user-file", Detail: "must not enter receipt"}},
	})
	encoded, err := json.Marshal(receipt)
	if err != nil {
		t.Fatalf("marshal receipt: %v", err)
	}
	if strings.Contains(string(encoded), "must not enter receipt") || receipt.Verdict != "pass" || len(receipt.Classes) != 1 {
		t.Fatalf("receipt leaked or lost typed fields: %s / %+v", encoded, receipt)
	}
	if err := receipt.Validate(); err != nil {
		t.Fatalf("validate receipt: %v", err)
	}
}

func TestWriteReceiptPublishesTypedLocalArtifact(t *testing.T) {
	root := t.TempDir()
	// #nosec G204 -- test initializes a test-owned temporary repository.
	command := exec.Command("git", "-C", root, "init", "--quiet")
	command.Env = testgit.Env()
	if err := command.Run(); err != nil {
		t.Fatal(err)
	}
	path, err := WriteReceipt(root, RemovalReport{Verdict: "fail", Findings: []Finding{{Class: "untracked"}}})
	if err != nil {
		t.Fatal(err)
	}
	if filepath.ToSlash(path) != filepath.ToSlash(filepath.Join(root, ".git", "jacu", "clean-exit", "latest.json")) {
		t.Fatalf("unexpected receipt path: %s", path)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("receipt mode = %o, want 600", info.Mode().Perm())
	}
	// #nosec G304 -- path was returned by WriteReceipt inside the test-owned temp repository.
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(content), "untracked") == false {
		t.Fatalf("receipt omitted typed class: %s", content)
	}
}
