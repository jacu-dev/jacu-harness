package telemetry

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jacu-dev/jacu-harness/internal/userstate"
)

const (
	retainedMonths  = 12
	maxSegmentBytes = int64(8 << 20)
	maxTotalBytes   = int64(128 << 20)
)

type Store struct {
	base            string
	maxSegmentBytes int64
	maxTotalBytes   int64
	retainedMonths  int
	remove          func(string) error
	beforeOpen      func()
	beforeRemove    func()
}

func NewStore() Store {
	if configured := os.Getenv(userstate.HomeEnv); configured != "" {
		return NewStoreAt(configured)
	}
	return NewStoreAt(userstate.DirOrLocal())
}

func NewStoreAt(base string) Store {
	return NewStoreAtWithLimits(base, maxSegmentBytes, maxTotalBytes, retainedMonths)
}

// NewStoreAtWithLimits is intended for deterministic package tests. Production
// constructors retain the fixed 8MiB/128MiB/12-month policy.
func NewStoreAtWithLimits(base string, segmentBytes, totalBytes int64, months int) Store {
	if segmentBytes <= 0 {
		segmentBytes = maxSegmentBytes
	}
	if totalBytes <= 0 {
		totalBytes = maxTotalBytes
	}
	if months <= 0 {
		months = retainedMonths
	}
	return Store{base: base, maxSegmentBytes: segmentBytes, maxTotalBytes: totalBytes, retainedMonths: months}
}

func (store Store) Directory() string { return filepath.Join(store.base, "telemetry") }

func (store Store) Emit(event Event) error {
	if !TelemetryEnabled() {
		return nil
	}
	line, err := encodeEvent(event)
	if err != nil {
		return err
	}
	if mkdirErr := os.MkdirAll(store.Directory(), 0o700); mkdirErr != nil {
		return fmt.Errorf("create telemetry directory: %w", mkdirErr)
	}
	if chmodErr := os.Chmod(store.Directory(), 0o700); chmodErr != nil { // #nosec G302 -- directory is intentionally protected more strictly than files.
		return fmt.Errorf("protect telemetry directory: %w", chmodErr)
	}
	lock, err := os.OpenFile(filepath.Join(store.Directory(), "events.lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return fmt.Errorf("open telemetry lock: %w", err)
	}
	defer func() { _ = lock.Close() }()
	if lockErr := lockStoreFile(lock); lockErr != nil {
		return fmt.Errorf("lock telemetry stream: %w", lockErr)
	}
	defer func() { _ = unlockStoreFile(lock) }()
	month := event.Timestamp.UTC().Format("2006-01")
	path, pathErr := store.nextSegmentPath(month, int64(len(line)))
	if pathErr != nil {
		return pathErr
	}
	// #nosec G304 -- path is generated exclusively from the fixed telemetry directory and YYYY-MM filename.
	file, err := openRegularAppend(path, store.beforeOpen)
	if err != nil {
		return fmt.Errorf("open telemetry stream: %w", err)
	}
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		return fmt.Errorf("protect telemetry stream: %w", err)
	}
	written, writeErr := file.Write(line)
	if writeErr == nil && written != len(line) {
		writeErr = io.ErrShortWrite
	}
	if writeErr == nil {
		writeErr = file.Sync()
	}
	closeErr := file.Close()
	if writeErr != nil {
		return fmt.Errorf("append telemetry event: %w", writeErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close telemetry stream: %w", closeErr)
	}
	if err := store.gcLocked(path); err != nil {
		return err
	}
	return nil
}

func TelemetryEnabled() bool {
	return strings.ToLower(strings.TrimSpace(os.Getenv("JACU_TELEMETRY"))) != "off"
}

func (store Store) gcLocked(protectedPath string) error {
	segments, err := store.segments()
	if err != nil {
		return err
	}
	months := map[string]struct{}{}
	for _, segment := range segments {
		months[segment.month] = struct{}{}
	}
	monthNames := make([]string, 0, len(months))
	for month := range months {
		monthNames = append(monthNames, month)
	}
	sort.Strings(monthNames)
	if len(monthNames) > store.retainedMonths {
		cut := monthNames[:len(monthNames)-store.retainedMonths]
		for _, segment := range segments {
			if containsString(cut, segment.month) && segment.path != protectedPath {
				if removeErr := store.removeRegularSegment(segment.path); removeErr != nil {
					return fmt.Errorf("remove expired telemetry stream: %w", removeErr)
				}
			}
		}
		segments, err = store.segments()
		if err != nil {
			return err
		}
	}
	var total int64
	for _, segment := range segments {
		info, statErr := regularFileInfo(segment.path)
		if statErr != nil {
			return fmt.Errorf("stat telemetry stream: %w", statErr)
		}
		total += info.Size()
	}
	for _, segment := range segments {
		if total <= store.maxTotalBytes {
			break
		}
		if segment.path == protectedPath {
			continue
		}
		info, err := regularFileInfo(segment.path)
		if err != nil {
			return fmt.Errorf("stat telemetry stream: %w", err)
		}
		if err := store.removeRegularSegment(segment.path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove old telemetry stream: %w", err)
		}
		total -= info.Size()
	}
	return nil
}

func (store Store) ReadSince(since time.Time) ([]Event, error) {
	segments, err := store.segments()
	if err != nil {
		return nil, err
	}
	events := make([]Event, 0)
	for _, segment := range segments {
		if segment.month < since.UTC().Format("2006-01") {
			continue
		}
		path := segment.path
		// #nosec G304 -- path comes from the fixed telemetry directory glob.
		file, err := openRegularRead(path, store.beforeOpen)
		if err != nil {
			return nil, fmt.Errorf("open telemetry stream: %w", err)
		}
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			event, decodeErr := DecodeEvent(scanner.Bytes())
			if decodeErr != nil {
				continue
			}
			if !event.Timestamp.Before(since) {
				events = append(events, event)
			}
		}
		scanErr := scanner.Err()
		closeErr := file.Close()
		if scanErr != nil {
			return nil, fmt.Errorf("read telemetry stream: %w", scanErr)
		}
		if closeErr != nil {
			return nil, fmt.Errorf("close telemetry stream: %w", closeErr)
		}
	}
	sort.SliceStable(events, func(i, j int) bool {
		if events[i].Timestamp.Equal(events[j].Timestamp) {
			return events[i].TraceID < events[j].TraceID
		}
		return events[i].Timestamp.Before(events[j].Timestamp)
	})
	return events, nil
}

type segment struct {
	path, month string
	sequence    int
}

func (store Store) segments() ([]segment, error) {
	entries, err := os.ReadDir(store.Directory())
	if err != nil {
		if os.IsNotExist(err) {
			return []segment{}, nil
		}
		return nil, fmt.Errorf("read telemetry directory: %w", err)
	}
	segments := make([]segment, 0)
	for _, entry := range entries {
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			continue
		}
		name := entry.Name()
		if !strings.HasPrefix(name, "events-") || !strings.HasSuffix(name, ".jsonl") {
			continue
		}
		stem := strings.TrimSuffix(strings.TrimPrefix(name, "events-"), ".jsonl")
		month, sequence := stem, 0
		if index := strings.LastIndex(stem, "-"); index >= 7 {
			month = stem[:index]
			sequence, err = strconv.Atoi(stem[index+1:])
			if err != nil {
				continue
			}
		}
		if len(month) != 7 || month[4] != '-' {
			continue
		}
		if _, err := time.Parse("2006-01", month); err != nil {
			continue
		}
		info, infoErr := entry.Info()
		if infoErr != nil {
			return nil, fmt.Errorf("inspect telemetry entry %q: %w", name, infoErr)
		}
		if !info.Mode().IsRegular() {
			continue
		}
		segments = append(segments, segment{filepath.Join(store.Directory(), name), month, sequence})
	}
	sort.Slice(segments, func(i, j int) bool {
		if segments[i].month != segments[j].month {
			return segments[i].month < segments[j].month
		}
		return segments[i].sequence < segments[j].sequence
	})
	return segments, nil
}

func (store Store) nextSegmentPath(month string, incoming int64) (string, error) {
	segments, err := store.segments()
	if err != nil {
		return "", err
	}
	newest := -1
	path := filepath.Join(store.Directory(), "events-"+month+".jsonl")
	for _, candidate := range segments {
		if candidate.month == month && candidate.sequence > newest {
			newest, path = candidate.sequence, candidate.path
		}
	}
	if info, err := regularFileInfo(path); err == nil && info.Size() > 0 && info.Size()+incoming > store.maxSegmentBytes {
		path = filepath.Join(store.Directory(), fmt.Sprintf("events-%s-%04d.jsonl", month, newest+1))
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("inspect telemetry stream: %w", err)
	}
	return path, nil
}

func regularFileInfo(path string) (os.FileInfo, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("telemetry stream is not a regular file")
	}
	return info, nil
}

func openRegularAppend(path string, beforeOpen func()) (*os.File, error) {
	root, name, err := segmentRoot(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = root.Close() }()
	before, err := root.Lstat(name)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	if err == nil && !before.Mode().IsRegular() {
		return nil, errors.New("telemetry stream is not a regular file")
	}
	if beforeOpen != nil {
		beforeOpen()
	}
	file, err := root.OpenFile(name, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, err
	}
	after, statErr := file.Stat()
	current, currentErr := root.Lstat(name)
	if statErr != nil || currentErr != nil || !after.Mode().IsRegular() || !current.Mode().IsRegular() || !os.SameFile(current, after) || (before != nil && !os.SameFile(before, after)) {
		_ = file.Close()
		if statErr != nil {
			return nil, statErr
		}
		if currentErr != nil {
			return nil, currentErr
		}
		return nil, errors.New("telemetry stream changed while opening")
	}
	return file, nil
}

func openRegularRead(path string, beforeOpen func()) (*os.File, error) {
	root, name, err := segmentRoot(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = root.Close() }()
	before, err := root.Lstat(name)
	if err != nil {
		return nil, err
	}
	if !before.Mode().IsRegular() {
		return nil, errors.New("telemetry stream is not a regular file")
	}
	if beforeOpen != nil {
		beforeOpen()
	}
	file, err := root.Open(name)
	if err != nil {
		return nil, err
	}
	after, statErr := file.Stat()
	current, currentErr := root.Lstat(name)
	if statErr != nil || currentErr != nil || !after.Mode().IsRegular() || !current.Mode().IsRegular() || !os.SameFile(before, after) || !os.SameFile(current, after) {
		_ = file.Close()
		if statErr != nil {
			return nil, statErr
		}
		if currentErr != nil {
			return nil, currentErr
		}
		return nil, errors.New("telemetry stream changed while opening")
	}
	return file, nil
}

func (store Store) removeRegularSegment(path string) error {
	root, name, err := segmentRoot(path)
	if err != nil {
		return err
	}
	defer func() { _ = root.Close() }()
	before, err := root.Lstat(name)
	if err != nil || !before.Mode().IsRegular() {
		if err != nil {
			return err
		}
		return errors.New("telemetry stream is not a regular file")
	}
	if store.beforeRemove != nil {
		store.beforeRemove()
	}
	current, err := root.Lstat(name)
	if err != nil || !current.Mode().IsRegular() || !os.SameFile(before, current) {
		if err != nil {
			return err
		}
		return errors.New("telemetry stream changed before removal")
	}
	remove := store.remove
	if remove == nil {
		return root.Remove(name)
	}
	return remove(path)
}

func segmentRoot(path string) (*os.Root, string, error) {
	root, err := os.OpenRoot(filepath.Dir(path))
	if err != nil {
		return nil, "", err
	}
	name := filepath.Base(path)
	if name == "." || name == string(filepath.Separator) || strings.Contains(name, string(filepath.Separator)) {
		_ = root.Close()
		return nil, "", errors.New("invalid telemetry segment name")
	}
	return root, name, nil
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func EmitBestEffort(event Event) {
	if !TelemetryEnabled() {
		return
	}
	if err := NewStore().Emit(event); err != nil {
		// Keep the warning deliberately bounded and free of paths or payloads.
		slog.Warn("telemetry write failed")
	}
}

func EmitBestEffortInput(input EventInput) {
	EmitBestEffortInputLive(nil, input)
}

// EmitBestEffortInputLive writes the same v2 envelope to the local store and,
// when w is set, as one NDJSON line to w. Serve must not pass stdout.
func EmitBestEffortInputLive(w io.Writer, input EventInput) {
	if input.ProjectID == "" {
		return
	}
	event, err := NewEvent(input)
	if err != nil {
		slog.Warn("telemetry event rejected")
		return
	}
	EmitBestEffort(event)
	WriteLive(w, event)
}

// WriteLive encodes a validated event as NDJSON. It does not persist.
func WriteLive(w io.Writer, event Event) {
	if w == nil {
		return
	}
	encoded, err := encodeEvent(event)
	if err != nil {
		slog.Warn("telemetry live encode failed")
		return
	}
	if _, err := w.Write(encoded); err != nil {
		slog.Warn("telemetry live write failed")
	}
}

// WriteLiveInput encodes EventInput as NDJSON without persisting. Used for
// progress pulses (verify running) that must appear before Execute finishes.
func WriteLiveInput(w io.Writer, input EventInput) {
	if w == nil || input.ProjectID == "" {
		return
	}
	event, err := NewEvent(input)
	if err != nil {
		slog.Warn("telemetry live event rejected")
		return
	}
	WriteLive(w, event)
}
