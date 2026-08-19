package memory

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jacu-dev/jacu-harness/internal/project"
)

func TestRenderAgentsBridgeIsDeterministicAndSorted(t *testing.T) {
	records := []Record{
		bridgeRecord("mem_0000000000000002", "Z convention", "keep z"),
		bridgeRecord("mem_0000000000000001", "A convention", "keep a"),
	}
	left, leftHash, err := RenderAgentsBridge(records)
	if err != nil {
		t.Fatal(err)
	}
	reversed := []Record{records[1], records[0]}
	right, rightHash, err := RenderAgentsBridge(reversed)
	if err != nil {
		t.Fatal(err)
	}
	if string(left) != string(right) || leftHash != rightHash {
		t.Fatalf("render is order-dependent:\nleft=%s\nright=%s", left, right)
	}
	if strings.Index(string(left), records[1].MemoryID) > strings.Index(string(left), records[0].MemoryID) {
		t.Fatalf("bridge records are not sorted: %s", left)
	}
	if !strings.Contains(string(left), BridgeStart) || !strings.Contains(string(left), BridgeEnd) || !strings.Contains(string(left), "JACU MEMORY SOURCE") || !strings.Contains(string(left), "JACU MEMORY CHECKSUM") {
		t.Fatalf("bridge markers missing: %s", left)
	}
}

func TestSyncAgentsBridgeAdoptsHumanFileAndPreservesOutsideBytes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "AGENTS.md")
	human := "# Human rules\n\nKeep this paragraph byte-for-byte.\n"
	if err := os.WriteFile(path, []byte(human), 0o640); err != nil { // #nosec G306 -- this test verifies preservation of a human file mode.
		t.Fatal(err)
	}
	result, err := SyncAgentsFile(path, []Record{bridgeRecord("mem_0000000000000001", "Convention", "Use bounded changes")})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Changed || result.Status != BridgeAppended {
		t.Fatalf("first sync = %#v; want appended change", result)
	}
	content, err := os.ReadFile(path) // #nosec G304 -- path is created under this test's temporary directory.
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(content), human) || !strings.Contains(string(content), BridgeStart) {
		t.Fatalf("human bytes were not preserved or bridge missing: %s", content)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o640 {
		t.Fatalf("mode = %04o; want 0640", got)
	}
}

func TestSyncAgentsBridgeNoOpPreservesBytesAndBlocksChecksumDrift(t *testing.T) {
	path := filepath.Join(t.TempDir(), "AGENTS.md")
	records := []Record{bridgeRecord("mem_0000000000000001", "Convention", "Use bounded changes")}
	if _, err := SyncAgentsFile(path, records); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path) // #nosec G304 -- path is created under this test's temporary directory.
	if err != nil {
		t.Fatal(err)
	}
	result, err := SyncAgentsFile(path, records)
	if err != nil {
		t.Fatal(err)
	}
	if result.Changed || result.Status != BridgeUnchanged {
		t.Fatalf("no-op sync = %#v; want unchanged", result)
	}
	after, _ := os.ReadFile(path) // #nosec G304 -- path is created under this test's temporary directory.
	if string(before) != string(after) {
		t.Fatal("no-op sync changed bytes")
	}
	drifted := strings.Replace(string(before), "Use bounded changes", "Human edit", 1)
	if err := os.WriteFile(path, []byte(drifted), 0o600); err != nil { // #nosec G703 -- path is created under this test's temporary directory.
		t.Fatal(err)
	}
	if _, err := SyncAgentsFile(path, records); !errors.Is(err, ErrBridgeChecksumMismatch) {
		t.Fatalf("drift sync error = %v; want checksum mismatch", err)
	}
	unchanged, _ := os.ReadFile(path) // #nosec G304 -- path is created under this test's temporary directory.
	if string(unchanged) != drifted {
		t.Fatal("checksum failure modified the file")
	}
}

func TestSyncAgentsBridgeRejectsMissingChecksumAndSecrets(t *testing.T) {
	path := filepath.Join(t.TempDir(), "AGENTS.md")
	if _, err := SyncAgentsFile(path, []Record{bridgeRecord("mem_0000000000000001", "Convention", "Use bounded changes")}); err != nil {
		t.Fatal(err)
	}
	content, _ := os.ReadFile(path) // #nosec G304 -- path is created under this test's temporary directory.
	content = []byte(strings.Replace(string(content), "<!-- JACU MEMORY CHECKSUM:", "<!-- removed checksum:", 1))
	if err := os.WriteFile(path, content, 0o600); err != nil { // #nosec G703 -- path is created under this test's temporary directory.
		t.Fatal(err)
	}
	if _, err := SyncAgentsFile(path, nil); !errors.Is(err, ErrBridgeChecksumMissing) {
		t.Fatalf("missing checksum error = %v; want missing checksum", err)
	}
	secretPath := filepath.Join(t.TempDir(), "AGENTS.md")
	if _, err := SyncAgentsFile(secretPath, []Record{bridgeRecord("mem_0000000000000001", "Secret", "ghp_not-for-storage")}); err == nil {
		t.Fatal("secret convention was rendered")
	}
	if _, err := os.Stat(secretPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("secret bridge created a file: %v", err)
	}
}

func TestConventionSaveRefreshesAgentsBridgeThroughNormalFlow(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/memory-bridge\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	projectID, err := project.ID(root)
	if err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	t.Setenv("JACU_HOME", home)
	raw, err := json.Marshal(Input{ProjectID: projectID, Kind: "convention", Title: "Use bounded changes", Body: "Every change stays inside the approved scope.", Source: "human"})
	if err != nil {
		t.Fatal(err)
	}
	result, err := saveHandler(root)(context.Background(), raw)
	if err != nil || result.Status != "ok" {
		t.Fatalf("save result = %#v, err=%v; want ok", result, err)
	}
	content, err := os.ReadFile(filepath.Join(root, "AGENTS.md")) // #nosec G304 -- root is a test-owned temporary project.
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "Use bounded changes") || !strings.Contains(string(content), BridgeStart) {
		t.Fatalf("normal convention save did not refresh bridge: %s", content)
	}
}

func bridgeRecord(id, title, body string) Record {
	return Record{MemoryID: id, ProjectID: testProjectID, Kind: "convention", Title: title, Body: body, Evidence: []string{"docs/rule.md"}, Source: "human", Status: "active"}
}
