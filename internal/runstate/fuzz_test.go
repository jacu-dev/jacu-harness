package runstate

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func FuzzRunStateDecode(f *testing.F) {
	const runID = "run_0000000000000003"
	repo := f.TempDir()
	runs := filepath.Join(repo, ".git", "jacu", "runs")
	if err := os.MkdirAll(runs, 0o700); err != nil {
		f.Fatalf("create runs directory: %v", err)
	}
	statePath := filepath.Join(runs, runID+".json")
	valid, err := json.Marshal(Run{
		RunID:     runID,
		MissionID: "mis_fuzz",
		Status:    StatusOpen,
		CreatedAt: time.Unix(1_700_000_000, 0).UTC(),
		BaseSHA:   "0123456789012345678901234567890123456789",
		Branch:    "jacu/run-fuzz",
		Worktree:  "/tmp/jacu-run-fuzz",
	})
	if err != nil {
		f.Fatalf("marshal valid run seed: %v", err)
	}
	seeds := [][]byte{
		valid,
		[]byte(`{}`),
		[]byte(`null`),
		[]byte(`{"run_id":"run_000000000000000a"`),
		{0xff, 0xfe, 0xfd},
		[]byte(`[]`),
		[]byte(`{"nested":{"array":[null,true,{},[1,2,3]]}}`),
		bytes.Repeat([]byte{0x00, 0xff, '{', '}'}, 256),
	}
	for _, seed := range seeds {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, raw []byte) {
		if err := os.WriteFile(statePath, raw, 0o600); err != nil {
			t.Fatalf("write fuzz run: %v", err)
		}

		run, err := Load(repo, runID)
		if err != nil {
			return
		}
		encoded, err := json.Marshal(run)
		if err != nil {
			t.Fatalf("marshal loaded run: %v", err)
		}
		var decoded Run
		if err := json.Unmarshal(encoded, &decoded); err != nil {
			t.Fatalf("decode loaded run serialization: %v", err)
		}
		if !reflect.DeepEqual(decoded, run) {
			t.Fatalf("serialization changed loaded run: got %#v; want %#v", decoded, run)
		}
	})
}
