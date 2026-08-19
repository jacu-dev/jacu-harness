package telemetry

import (
	"bufio"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jacu-dev/jacu-harness/internal/userstate"
)

func TestStoreWritesProtectedMonthlyJSONL(t *testing.T) {
	base := t.TempDir()
	store := NewStoreAt(base)
	event, err := NewEvent(validEventInput())
	if err != nil {
		t.Fatalf("NewEvent: %v", err)
	}
	if emitErr := store.Emit(event); emitErr != nil {
		t.Fatalf("Emit: %v", emitErr)
	}

	telemetryDir := filepath.Join(base, "telemetry")
	info, err := os.Stat(telemetryDir)
	if err != nil {
		t.Fatalf("stat telemetry directory: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o700 {
		t.Fatalf("telemetry directory mode = %o; want 700", got)
	}
	path := filepath.Join(telemetryDir, "events-2026-08.jsonl")
	fileInfo, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat event file: %v", err)
	}
	if got := fileInfo.Mode().Perm(); got != 0o600 {
		t.Fatalf("event file mode = %o; want 600", got)
	}
	// #nosec G304 -- path is assembled from the test-owned temporary directory and fixed month.
	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("open event file: %v", err)
	}
	defer func() { _ = file.Close() }()
	scanner := bufio.NewScanner(file)
	var lines int
	for scanner.Scan() {
		lines++
		var decoded map[string]any
		if decodeErr := json.Unmarshal(scanner.Bytes(), &decoded); decodeErr != nil {
			t.Fatalf("event line is not JSON: %v", decodeErr)
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan event file: %v", err)
	}
	if lines != 1 {
		t.Fatalf("event lines = %d; want 1", lines)
	}
}

func TestStoreInjectedLimitsRotateSegments(t *testing.T) {
	store := NewStoreAtWithLimits(t.TempDir(), 120, 1<<20, 12)
	for i := 0; i < 4; i++ {
		if err := store.Emit(mustEvent(validEventInput())); err != nil {
			t.Fatal(err)
		}
	}
	entries, err := os.ReadDir(store.Directory())
	if err != nil {
		t.Fatal(err)
	}
	segments := 0
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "events-2026-08") && strings.HasSuffix(entry.Name(), ".jsonl") {
			segments++
		}
	}
	if segments < 2 {
		names := make([]string, 0, len(entries))
		for _, entry := range entries {
			names = append(names, entry.Name())
		}
		t.Fatalf("segments = %d names=%v; injected limit did not rotate", segments, names)
	}
}

func mustEvent(input EventInput) Event {
	event, err := NewEvent(input)
	if err != nil {
		panic(err)
	}
	return event
}

func TestReaderAcceptsSanitizedRealV1Corpus(t *testing.T) {
	// #nosec G304 -- the fixture path is repository-owned testdata.
	file, err := os.Open(filepath.Join("testdata", "events-v1.jsonl"))
	if err != nil {
		t.Fatalf("open v1 corpus: %v", err)
	}
	defer func() { _ = file.Close() }()

	scanner := bufio.NewScanner(file)
	lines := 0
	for scanner.Scan() {
		lines++
		event, decodeErr := DecodeEvent(scanner.Bytes())
		if decodeErr != nil {
			t.Fatalf("decode v1 corpus line %d: %v", lines, decodeErr)
		}
		if event.SchemaVersion != "1" || event.Level != NoData || event.Module != NoData || event.Stage != NoData || event.Measurement != NoData {
			t.Fatalf("v1 corpus line %d compatibility fields = %+v", lines, event)
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan v1 corpus: %v", err)
	}
	if lines == 0 {
		t.Fatal("v1 corpus is empty")
	}
}

func TestStoreSerializesConcurrentAppends(t *testing.T) {
	base := t.TempDir()
	store := NewStoreAt(base)
	const writers = 32
	var group sync.WaitGroup
	for index := 0; index < writers; index++ {
		group.Add(1)
		go func(index int) {
			defer group.Done()
			input := validEventInput()
			input.TraceID = "tr_" + hexID(index)
			event, err := NewEvent(input)
			if err != nil {
				t.Errorf("NewEvent(%d): %v", index, err)
				return
			}
			if emitErr := store.Emit(event); emitErr != nil {
				t.Errorf("Emit(%d): %v", index, emitErr)
			}
		}(index)
	}
	group.Wait()

	// #nosec G304 -- path is assembled from the test-owned temporary directory and fixed month.
	file, err := os.Open(filepath.Join(base, "telemetry", "events-2026-08.jsonl"))
	if err != nil {
		t.Fatalf("open event file: %v", err)
	}
	defer func() { _ = file.Close() }()
	scanner := bufio.NewScanner(file)
	lines := 0
	for scanner.Scan() {
		lines++
		var event Event
		if decodeErr := json.Unmarshal(scanner.Bytes(), &event); decodeErr != nil {
			t.Fatalf("line %d is not a complete event: %v", lines, decodeErr)
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan: %v", err)
	}
	if lines != writers {
		t.Fatalf("event lines = %d; want %d", lines, writers)
	}
}

func TestStoreRetainsTwelveNewestMonths(t *testing.T) {
	base := t.TempDir()
	store := NewStoreAt(base)
	for month := time.January; month <= time.December; month++ {
		input := validEventInput()
		input.Timestamp = time.Date(2025, month, 1, 0, 0, 0, 0, time.UTC)
		event, err := NewEvent(input)
		if err != nil {
			t.Fatalf("NewEvent(%s): %v", month, err)
		}
		if emitErr := store.Emit(event); emitErr != nil {
			t.Fatalf("Emit(%s): %v", month, emitErr)
		}
	}
	input := validEventInput()
	input.Timestamp = time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	event, err := NewEvent(input)
	if err != nil {
		t.Fatalf("NewEvent(newest): %v", err)
	}
	if emitErr := store.Emit(event); emitErr != nil {
		t.Fatalf("Emit(newest): %v", emitErr)
	}
	files, err := filepath.Glob(filepath.Join(base, "telemetry", "events-*.jsonl"))
	if err != nil {
		t.Fatalf("glob telemetry files: %v", err)
	}
	if len(files) != 12 {
		t.Fatalf("retained files = %d; want 12", len(files))
	}
	if _, err := os.Stat(filepath.Join(base, "telemetry", "events-2025-01.jsonl")); !os.IsNotExist(err) {
		t.Fatalf("oldest month still exists, stat error = %v", err)
	}
}

func TestStoreKeepsReceivingOversizedSegment(t *testing.T) {
	store := NewStoreAtWithLimits(t.TempDir(), 8<<20, 100, 12)
	event := mustEvent(validEventInput())
	if err := store.Emit(event); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(store.Directory(), "events-2026-08.jsonl")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("receiving oversized segment was removed: %v", err)
	}
	events, err := store.ReadSince(event.Timestamp.Add(-time.Hour))
	if err != nil || len(events) != 1 {
		t.Fatalf("ReadSince = %d events, %v; want retained event", len(events), err)
	}
}

func TestStoreKeepsBackdatedReceivingSegmentUnderTotalCap(t *testing.T) {
	store := NewStoreAtWithLimits(t.TempDir(), 8<<20, 400, 12)
	newer := validEventInput()
	newer.Timestamp = time.Date(2026, time.August, 1, 0, 0, 0, 0, time.UTC)
	if err := store.Emit(mustEvent(newer)); err != nil {
		t.Fatal(err)
	}
	older := validEventInput()
	older.Timestamp = time.Date(2026, time.July, 1, 0, 0, 0, 0, time.UTC)
	if err := store.Emit(mustEvent(older)); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(store.Directory(), "events-2026-07.jsonl")); err != nil {
		t.Fatalf("backdated receiving segment was removed: %v", err)
	}
}

func TestStoreSkipsSymlinkedSegmentAndRefusesAppend(t *testing.T) {
	store := NewStoreAt(t.TempDir())
	if err := os.MkdirAll(store.Directory(), 0o700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "target.jsonl")
	if err := os.WriteFile(target, []byte("not telemetry\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(store.Directory(), "events-2026-08.jsonl")
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ReadSince(time.Date(2026, time.August, 1, 0, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("ReadSince should skip symlink: %v", err)
	}
	if err := store.Emit(mustEvent(validEventInput())); err == nil {
		t.Fatal("Emit accepted a symlinked segment")
	}
}

func TestStorePropagatesRetentionRemovalFailure(t *testing.T) {
	store := NewStoreAtWithLimits(t.TempDir(), 8<<20, 1, 12)
	if err := os.MkdirAll(store.Directory(), 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(store.Directory(), "events-2026-08.jsonl")
	if err := os.WriteFile(path, []byte("event\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	store.remove = func(string) error { return errors.New("remove denied") }
	if err := store.gcLocked(""); err == nil {
		t.Fatal("gcLocked ignored segment removal failure")
	}
}

func TestStoreRejectsSegmentSwappedToSymlinkDuringOpen(t *testing.T) {
	store := NewStoreAt(t.TempDir())
	if err := os.MkdirAll(store.Directory(), 0o700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(store.Directory(), "other.jsonl")
	if err := os.WriteFile(target, []byte("safe\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(store.Directory(), "events-2026-08.jsonl")
	store.beforeOpen = func() {
		if err := os.Symlink("other.jsonl", path); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.Emit(mustEvent(validEventInput())); err == nil {
		t.Fatal("Emit accepted a segment swapped to a symlink")
	}
}

func TestStoreRejectsSegmentSwappedToSymlinkDuringRemoval(t *testing.T) {
	store := NewStoreAtWithLimits(t.TempDir(), 8<<20, 1, 12)
	if err := os.MkdirAll(store.Directory(), 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(store.Directory(), "events-2026-08.jsonl")
	if err := os.WriteFile(path, []byte("event\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(store.Directory(), "other.jsonl")
	if err := os.WriteFile(target, []byte("safe\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	store.beforeRemove = func() {
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink("other.jsonl", path); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.gcLocked(""); err == nil {
		t.Fatal("gcLocked accepted a segment swapped to a symlink")
	}
	if content, err := os.ReadFile(target); err != nil || string(content) != "safe\n" { // #nosec G304 -- target is created in this test's TempDir-backed telemetry directory.
		t.Fatalf("target changed after refused removal: %q, %v", content, err)
	}
}

func TestNewStoreWithoutHomeDoesNotCreateHarnessInWorkingDirectory(t *testing.T) {
	t.Setenv(userstate.HomeEnv, "")
	t.Setenv("HOME", "")
	t.Setenv("JACU_TELEMETRY", "on")
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	leak := filepath.Join(cwd, userstate.Name)
	event, err := NewEvent(validEventInput())
	if err != nil {
		t.Fatalf("NewEvent: %v", err)
	}
	if emitErr := NewStore().Emit(event); emitErr != nil {
		t.Fatalf("Emit: %v", emitErr)
	}
	if _, statErr := os.Stat(leak); !os.IsNotExist(statErr) {
		t.Fatalf("HOME-less Emit created %s", leak)
	}
}

func TestStoreOptOutSuppressesWrites(t *testing.T) {
	base := t.TempDir()
	t.Setenv("JACU_TELEMETRY", "off")
	event, err := NewEvent(validEventInput())
	if err != nil {
		t.Fatalf("NewEvent: %v", err)
	}
	if emitErr := NewStoreAt(base).Emit(event); emitErr != nil {
		t.Fatalf("Emit with opt-out: %v", emitErr)
	}
	if _, err := os.Stat(filepath.Join(base, "telemetry")); !os.IsNotExist(err) {
		t.Fatalf("opt-out created telemetry directory, stat error = %v", err)
	}
}

func hexID(value int) string {
	return "000000000000000" + string(rune('0'+value%10))
}
