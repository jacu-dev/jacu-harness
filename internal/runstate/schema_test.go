package runstate

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
)

func TestLegacyRunstateGetsSchemaVersionOnSave(t *testing.T) {
	repo := newStateRepo(t)
	run := fixtureRun("run_0000000000000010", time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC))
	content := legacyRunstateJSON(t, run)
	if mkdirErr := os.MkdirAll(runsDir(repo), 0o700); mkdirErr != nil {
		t.Fatal(mkdirErr)
	}
	if writeErr := os.WriteFile(runPath(repo, run.RunID), content, 0o600); writeErr != nil {
		t.Fatal(writeErr)
	}
	loaded, loadErr := Load(repo, run.RunID)
	if loadErr != nil || loaded.SchemaVersion != CurrentSchemaVersion {
		t.Fatalf("legacy Load = %#v err %v; want schema %q", loaded, loadErr, CurrentSchemaVersion)
	}
	if saveErr := Save(repo, loaded); saveErr != nil {
		t.Fatal(saveErr)
	}
	// #nosec G304 -- runPath is the canonical path in this test-owned repository.
	encoded, readErr := os.ReadFile(runPath(repo, run.RunID))
	if readErr != nil {
		t.Fatal(readErr)
	}
	var persisted Run
	if decodeErr := json.Unmarshal(encoded, &persisted); decodeErr != nil {
		t.Fatal(decodeErr)
	}
	if persisted.SchemaVersion != CurrentSchemaVersion {
		t.Fatalf("saved legacy schema = %q; want %q", persisted.SchemaVersion, CurrentSchemaVersion)
	}
}

func TestUnknownRunstateSchemaFailsClosed(t *testing.T) {
	repo := newStateRepo(t)
	run := fixtureRun("run_0000000000000011", time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC))
	run.SchemaVersion = "99"
	content, marshalErr := json.Marshal(run)
	if marshalErr != nil {
		t.Fatal(marshalErr)
	}
	if mkdirErr := os.MkdirAll(runsDir(repo), 0o700); mkdirErr != nil {
		t.Fatal(mkdirErr)
	}
	if writeErr := os.WriteFile(runPath(repo, run.RunID), content, 0o600); writeErr != nil {
		t.Fatal(writeErr)
	}
	_, loadErr := Load(repo, run.RunID)
	if loadErr == nil || !strings.Contains(loadErr.Error(), "unsupported run schema version") {
		t.Fatalf("Load unknown schema error = %v", loadErr)
	}
}

func legacyRunstateJSON(t *testing.T, run Run) []byte {
	t.Helper()
	encoded, marshalErr := json.Marshal(run)
	if marshalErr != nil {
		t.Fatal(marshalErr)
	}
	var payload map[string]json.RawMessage
	if decodeErr := json.Unmarshal(encoded, &payload); decodeErr != nil {
		t.Fatal(decodeErr)
	}
	delete(payload, "schema_version")
	legacy, remarshalErr := json.Marshal(payload)
	if remarshalErr != nil {
		t.Fatal(remarshalErr)
	}
	return legacy
}
