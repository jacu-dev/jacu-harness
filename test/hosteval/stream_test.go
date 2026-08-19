//go:build hosteval

package hosteval

import (
	"os"
	"path/filepath"
	"testing"
)

func writeStream(t *testing.T, dir, name string, lines ...string) {
	t.Helper()
	body := ""
	for _, l := range lines {
		body += l + "\n"
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

func appendStream(t *testing.T, dir, name string, lines ...string) {
	t.Helper()
	// #nosec G304 -- test-only path is rooted in t.TempDir and uses a fixture filename.
	f, err := os.OpenFile(filepath.Join(dir, name), os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatalf("append %s: %v", name, err)
	}
	defer func() { _ = f.Close() }()
	for _, l := range lines {
		if _, err := f.WriteString(l + "\n"); err != nil {
			t.Fatalf("append line: %v", err)
		}
	}
}

func ev(project, tool string) string {
	return `{"ts":"2026-08-14T10:00:0` + string(rune('0'+len(tool)%10)) +
		`Z","project_id":"` + project + `","trace_id":"tr_1","event":"tool_call","tool":"` +
		tool + `","status":"ok"}`
}

func TestDeltaOnlyReturnsEventsAppendedAfterSnapshot(t *testing.T) {
	dir := t.TempDir()
	writeStream(t, dir, "events-2026-08.jsonl", ev("prj_a", "jacu_status"))

	before, err := Snapshot(dir)
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	appendStream(t, dir, "events-2026-08.jsonl", ev("prj_a", "jacu_project_inspect"))

	got, skipped, err := Delta(dir, before, "prj_a")
	if err != nil {
		t.Fatalf("delta: %v", err)
	}
	if skipped != 0 {
		t.Fatalf("skipped = %d, want 0", skipped)
	}
	tools := Tools(got)
	if len(tools) != 1 || tools[0] != "jacu_project_inspect" {
		t.Fatalf("tools = %v, want only the appended event", tools)
	}
}

func TestDeltaFiltersByProjectID(t *testing.T) {
	dir := t.TempDir()
	writeStream(t, dir, "events-2026-08.jsonl")
	before, _ := Snapshot(dir)
	appendStream(t, dir, "events-2026-08.jsonl",
		ev("prj_other", "jacu_apply"),
		ev("prj_mine", "jacu_project_inspect"),
	)

	got, _, err := Delta(dir, before, "prj_mine")
	if err != nil {
		t.Fatalf("delta: %v", err)
	}
	if tools := Tools(got); len(tools) != 1 || tools[0] != "jacu_project_inspect" {
		t.Fatalf("tools = %v, want only the throwaway project's event", tools)
	}
}

func TestDeltaCountsMalformedLinesInsteadOfFailing(t *testing.T) {
	dir := t.TempDir()
	writeStream(t, dir, "events-2026-08.jsonl")
	before, _ := Snapshot(dir)
	appendStream(t, dir, "events-2026-08.jsonl",
		`{"this is not`,
		ev("prj_a", "jacu_status"),
		``,
	)

	got, skipped, err := Delta(dir, before, "prj_a")
	if err != nil {
		t.Fatalf("delta must tolerate a torn line, got %v", err)
	}
	if skipped != 1 {
		t.Fatalf("skipped = %d, want 1", skipped)
	}
	if tools := Tools(got); len(tools) != 1 {
		t.Fatalf("tools = %v, want the one readable event", tools)
	}
}

func TestMissingStreamDirIsNotAnError(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "never-written")
	before, err := Snapshot(dir)
	if err != nil {
		t.Fatalf("snapshot of missing dir: %v", err)
	}
	if _, _, err := Delta(dir, before, "prj_a"); err != nil {
		t.Fatalf("delta of missing dir: %v", err)
	}
}
