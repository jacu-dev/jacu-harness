//go:build hosteval

// Package hosteval judges host routing by the delta of jacu's own telemetry
// stream, never by scraping a host transcript. A transcript parser breaks on
// every host release; the event stream is a contract this repository owns.
package hosteval

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Event mirrors the subset of internal/telemetry.Event this harness reads. It
// is deliberately a separate struct: the harness must keep working against a
// stream written by an older binary, so it never shares the producer's type.
type Event struct {
	Timestamp time.Time `json:"ts"`
	ProjectID string    `json:"project_id"`
	TraceID   string    `json:"trace_id"`
	RunID     string    `json:"run_id,omitempty"`
	MissionID string    `json:"mission_id,omitempty"`
	Event     string    `json:"event"`
	Tool      string    `json:"tool,omitempty"`
	Status    string    `json:"status"`
	Verdict   string    `json:"verdict,omitempty"`
}

// StreamDir is the append-only telemetry directory jacu writes to.
func StreamDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("hosteval: resolve home: %w", err)
	}
	return filepath.Join(home, ".jacu-harness", "telemetry"), nil
}

// Offsets records the size of every stream file at snapshot time. Reading a
// delta by byte offset — rather than by timestamp — is what makes the harness
// safe to run twice in the same minute and safe against clock skew.
type Offsets map[string]int64

// Snapshot records current sizes. A missing directory is not an error: it means
// jacu has never written an event on this machine, and the delta is whatever
// the run produces.
func Snapshot(dir string) (Offsets, error) {
	off := Offsets{}
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return off, nil
	}
	if err != nil {
		return nil, fmt.Errorf("hosteval: read %s: %w", dir, err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			return nil, fmt.Errorf("hosteval: stat %s: %w", e.Name(), err)
		}
		off[e.Name()] = info.Size()
	}
	return off, nil
}

// Delta returns the events appended after the snapshot, filtered by projectID
// and ordered by timestamp. A malformed line is skipped, never fatal: the
// stream is append-only and a torn final write is expected, not exceptional.
// Skipping is reported so a caller can refuse to judge on a shredded stream.
func Delta(dir string, since Offsets, projectID string) (events []Event, skipped int, err error) {
	entries, readErr := os.ReadDir(dir)
	if errors.Is(readErr, os.ErrNotExist) {
		return nil, 0, nil
	}
	if readErr != nil {
		return nil, 0, fmt.Errorf("hosteval: read %s: %w", dir, readErr)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		got, n, fileErr := deltaFile(filepath.Join(dir, e.Name()), since[e.Name()], projectID)
		if fileErr != nil {
			return nil, 0, fileErr
		}
		events = append(events, got...)
		skipped += n
	}
	sort.SliceStable(events, func(i, j int) bool {
		return events[i].Timestamp.Before(events[j].Timestamp)
	})
	return events, skipped, nil
}

func deltaFile(path string, from int64, projectID string) ([]Event, int, error) {
	f, err := os.Open(path) //nolint:gosec // path comes from ReadDir of a fixed directory
	if err != nil {
		return nil, 0, fmt.Errorf("hosteval: open %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	if _, err := f.Seek(from, io.SeekStart); err != nil {
		return nil, 0, fmt.Errorf("hosteval: seek %s: %w", path, err)
	}

	var (
		out     []Event
		skipped int
	)
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var ev Event
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			skipped++
			continue
		}
		if projectID != "" && ev.ProjectID != projectID {
			continue
		}
		out = append(out, ev)
	}
	if err := sc.Err(); err != nil {
		return nil, 0, fmt.Errorf("hosteval: scan %s: %w", path, err)
	}
	return out, skipped, nil
}

// Tools returns the tool names in call order. Events without a tool — verify,
// apply, escalation — are not tool calls and are dropped here on purpose.
func Tools(events []Event) []string {
	var out []string
	for _, e := range events {
		if e.Tool != "" {
			out = append(out, e.Tool)
		}
	}
	return out
}
