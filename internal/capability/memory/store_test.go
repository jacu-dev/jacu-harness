package memory

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

const testProjectID = "prj_0123456789abcdef"

func TestFileStoreSaveGetAndDeterministicUpsert(t *testing.T) {
	root := t.TempDir()
	var store Store = NewFileStore(root)
	rec := testRecord("mem_0000000000000001", testProjectID, "decision", "First title", "old body")

	if err := store.Save(rec, ""); err != nil {
		t.Fatalf("Save first: %v", err)
	}
	got, ok := store.Get(rec.MemoryID)
	if !ok || !reflect.DeepEqual(got, rec) {
		t.Fatalf("Get first = (%#v, %v); want (%#v, true)", got, ok, rec)
	}

	rec.Title = "Updated title"
	rec.Body = "new body"
	if err := store.Save(rec, ""); err != nil {
		t.Fatalf("Save upsert: %v", err)
	}
	got, ok = store.Get(rec.MemoryID)
	if !ok || !reflect.DeepEqual(got, rec) {
		t.Fatalf("Get upsert = (%#v, %v); want (%#v, true)", got, ok, rec)
	}

	path := filepath.Join(root, "memory", testProjectID, rec.MemoryID+".json")
	first, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile first: %v", err)
	}
	if err := store.Save(rec, ""); err != nil {
		t.Fatalf("Save deterministic repeat: %v", err)
	}
	second, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile second: %v", err)
	}
	if string(first) != string(second) {
		t.Fatalf("identical upsert changed bytes:\nfirst=%s\nsecond=%s", first, second)
	}
}

func TestFileStoreUsesGlobalDirectoryAndPrivatePermissions(t *testing.T) {
	root := t.TempDir()
	store := NewFileStore(root)
	rec := testRecord("mem_0000000000000002", "", "preference", "Global preference", "Use concise output")

	if err := store.Save(rec, ""); err != nil {
		t.Fatalf("Save: %v", err)
	}

	memoryDir := filepath.Join(root, "memory")
	globalDir := filepath.Join(memoryDir, "_global")
	path := filepath.Join(globalDir, rec.MemoryID+".json")
	for _, dir := range []string{memoryDir, globalDir} {
		info, err := os.Stat(dir)
		if err != nil {
			t.Fatalf("Stat %s: %v", dir, err)
		}
		if got := info.Mode().Perm(); got != 0o700 {
			t.Fatalf("mode %s = %04o; want 0700", dir, got)
		}
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat record: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("record mode = %04o; want 0600", got)
	}
	got, ok := store.Get(rec.MemoryID)
	if !ok || !reflect.DeepEqual(got, rec) {
		t.Fatalf("Get global = (%#v, %v); want (%#v, true)", got, ok, rec)
	}
}

func TestFileStoreRejectsInvalidIdentifiersBeforeBuildingPaths(t *testing.T) {
	root := t.TempDir()
	store := NewFileStore(root)
	tests := []struct {
		name string
		rec  Record
	}{
		{name: "memory traversal", rec: testRecord("../mem_0000000000000003", testProjectID, "gotcha", "Traversal", "blocked")},
		{name: "memory uppercase", rec: testRecord("mem_ABCDEFABCDEFABCD", testProjectID, "gotcha", "Uppercase", "blocked")},
		{name: "project traversal", rec: testRecord("mem_0000000000000003", "../../outside", "gotcha", "Traversal", "blocked")},
		{name: "project malformed", rec: testRecord("mem_0000000000000003", "project", "gotcha", "Malformed", "blocked")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := store.Save(tt.rec, ""); err == nil {
				t.Fatal("Save error = nil; want invalid identifier error")
			}
		})
	}
	if _, err := os.Stat(filepath.Join(root, "memory")); !os.IsNotExist(err) {
		t.Fatalf("invalid identifiers created memory tree: %v", err)
	}
	for _, id := range []string{"", "../mem_0000000000000003", "mem_ABCDEFABCDEFABCD"} {
		if got, ok := store.Get(id); ok || !reflect.DeepEqual(got, Record{}) {
			t.Fatalf("Get(%q) = (%#v, %v); want zero, false", id, got, ok)
		}
	}
}

func TestFileStoreSaveRejectsInvalidRecordEnumsAndGlobalScope(t *testing.T) {
	root := t.TempDir()
	store := NewFileStore(root)
	valid := testRecord("mem_0000000000000040", testProjectID, "decision", "Valid", "Valid body")

	tests := []struct {
		name   string
		mutate func(*Record)
	}{
		{name: "invalid status", mutate: func(rec *Record) { rec.Status = "pending" }},
		{name: "invalid kind", mutate: func(rec *Record) { rec.Kind = "note" }},
		{name: "invalid source", mutate: func(rec *Record) { rec.Source = "agent" }},
		{name: "global non-preference", mutate: func(rec *Record) { rec.ProjectID = "" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := valid
			tt.mutate(&rec)
			if err := store.Save(rec, ""); err == nil {
				t.Fatal("Save error = nil; want invalid record error")
			}
		})
	}
	if _, err := os.Stat(filepath.Join(root, "memory")); !os.IsNotExist(err) {
		t.Fatalf("invalid records created memory tree: %v", err)
	}
}

func TestFileStoreSaveRejectsMemoryIDAlreadyUsedInAnotherScope(t *testing.T) {
	root := t.TempDir()
	store := NewFileStore(root)
	id := "mem_0000000000000041"
	global := testRecord(id, "", "preference", "Global", "Global preference")
	if err := store.Save(global, ""); err != nil {
		t.Fatalf("Save global: %v", err)
	}

	project := testRecord(id, testProjectID, "preference", "Project", "Project preference")
	if err := store.Save(project, ""); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("Save duplicate error = %v; want already exists error", err)
	}
	if _, err := os.Stat(filepath.Join(root, "memory", testProjectID, id+".json")); !os.IsNotExist(err) {
		t.Fatalf("duplicate Save created project record: %v", err)
	}
}

func TestFileStoreSerializesSameRootAcrossDistinctStoreInstances(t *testing.T) {
	root := t.TempDir()
	firstReady := make(chan struct{}, 1)
	secondReady := make(chan struct{}, 1)
	release := make(chan struct{})
	first := newFileStoreWithHooks(root, storeHooks{
		afterUniquenessCheck: func() error {
			firstReady <- struct{}{}
			<-release
			return nil
		},
	})
	second := newFileStoreWithHooks(root, storeHooks{
		afterUniquenessCheck: func() error {
			secondReady <- struct{}{}
			<-release
			return nil
		},
	})
	id := "mem_0000000000000045"
	errs := make(chan error, 2)
	go func() {
		errs <- first.Save(testRecord(id, "", "preference", "Global", "First writer"), "")
	}()
	select {
	case <-firstReady:
	case <-time.After(5 * time.Second):
		t.Fatal("first store did not reach uniqueness check")
	}
	go func() {
		errs <- second.Save(testRecord(id, testProjectID, "preference", "Project", "Second writer"), "")
	}()

	secondReachedCheck := false
	select {
	case <-secondReady:
		secondReachedCheck = true
		close(release)
	case <-time.After(250 * time.Millisecond):
		close(release)
	}
	firstErr, secondErr := <-errs, <-errs
	if secondReachedCheck {
		t.Error("second store reached uniqueness check before the first released")
	}
	assertOneDuplicateSave(t, firstErr, secondErr)
}

func TestFileStoreSerializesSameRootAcrossProcesses(t *testing.T) {
	if os.Getenv("JACU_MEMORY_LOCK_WORKER") != "" {
		runFileStoreLockWorker(t)
		return
	}

	root := t.TempDir()
	release := filepath.Join(root, "release")
	id := "mem_0000000000000046"
	first := startFileStoreLockWorker(t, root, "first", "", id)
	waitForFile(t, filepath.Join(root, "first.ready"), 5*time.Second)
	second := startFileStoreLockWorker(t, root, "second", testProjectID, id)

	secondReachedCheck := waitForFileUntil(filepath.Join(root, "second.ready"), 500*time.Millisecond)
	if err := os.WriteFile(release, []byte("go"), 0o600); err != nil {
		t.Fatalf("release workers: %v", err)
	}
	if err := first.Wait(); err != nil {
		t.Fatalf("first worker: %v", err)
	}
	if err := second.Wait(); err != nil {
		t.Fatalf("second worker: %v", err)
	}
	if secondReachedCheck {
		t.Error("second process reached uniqueness check before the first released")
	}
	firstResult, err := os.ReadFile(filepath.Join(root, "first.result"))
	if err != nil {
		t.Fatalf("read first result: %v", err)
	}
	secondResult, err := os.ReadFile(filepath.Join(root, "second.result"))
	if err != nil {
		t.Fatalf("read second result: %v", err)
	}
	assertOneDuplicateSave(t, resultError(firstResult), resultError(secondResult))
	lockInfo, err := os.Stat(filepath.Join(root, ".memory.lock"))
	if err != nil {
		t.Fatalf("persistent process lock: %v", err)
	}
	if got := lockInfo.Mode().Perm(); got != 0o600 {
		t.Fatalf("process lock mode = %04o; want 0600", got)
	}
}

func runFileStoreLockWorker(t *testing.T) {
	root := os.Getenv("JACU_MEMORY_LOCK_ROOT")
	name := os.Getenv("JACU_MEMORY_LOCK_NAME")
	projectID := os.Getenv("JACU_MEMORY_LOCK_PROJECT")
	id := os.Getenv("JACU_MEMORY_LOCK_ID")
	store := newFileStoreWithHooks(root, storeHooks{
		afterUniquenessCheck: func() error {
			if err := os.WriteFile(filepath.Join(root, name+".ready"), []byte("ready"), 0o600); err != nil {
				return err
			}
			waitForFile(t, filepath.Join(root, "release"), 5*time.Second)
			return nil
		},
	})
	err := store.Save(testRecord(id, projectID, "preference", name, "Cross-process writer"), "")
	result := "ok"
	if err != nil {
		result = "error:" + err.Error()
	}
	if writeErr := os.WriteFile(filepath.Join(root, name+".result"), []byte(result), 0o600); writeErr != nil {
		t.Fatalf("write worker result: %v", writeErr)
	}
}

func startFileStoreLockWorker(t *testing.T, root, name, projectID, id string) *exec.Cmd {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^TestFileStoreSerializesSameRootAcrossProcesses$", "-test.count=1")
	cmd.Env = append(os.Environ(),
		"JACU_MEMORY_LOCK_WORKER=1",
		"JACU_MEMORY_LOCK_ROOT="+root,
		"JACU_MEMORY_LOCK_NAME="+name,
		"JACU_MEMORY_LOCK_PROJECT="+projectID,
		"JACU_MEMORY_LOCK_ID="+id,
	)
	if err := cmd.Start(); err != nil {
		t.Fatalf("start %s worker: %v", name, err)
	}
	return cmd
}

func assertOneDuplicateSave(t *testing.T, first, second error) {
	t.Helper()
	errs := []error{first, second}
	successes, duplicates := 0, 0
	for _, err := range errs {
		switch {
		case err == nil:
			successes++
		case strings.Contains(err.Error(), "already exists"):
			duplicates++
		default:
			t.Fatalf("unexpected Save results (%v, %v)", first, second)
		}
	}
	if successes != 1 || duplicates != 1 {
		t.Fatalf("Save results = (%v, %v); want one success and one duplicate", first, second)
	}
}

func resultError(result []byte) error {
	value := string(result)
	if value == "ok" {
		return nil
	}
	return errors.New(strings.TrimPrefix(value, "error:"))
}

func waitForFile(t *testing.T, path string, timeout time.Duration) {
	t.Helper()
	if !waitForFileUntil(path, timeout) {
		t.Fatalf("timed out waiting for %s", path)
	}
}

func waitForFileUntil(path string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return false
}

func TestFileStoreGetSkipsCorruptShadowAndFindsLaterValidRecord(t *testing.T) {
	root := t.TempDir()
	store := NewFileStore(root)
	id := "mem_0000000000000042"
	want := testRecord(id, testProjectID, "gotcha", "Valid record", "Must remain discoverable")
	if err := store.Save(want, ""); err != nil {
		t.Fatalf("Save valid: %v", err)
	}
	globalDir := filepath.Join(root, "memory", "_global")
	if err := os.MkdirAll(globalDir, 0o700); err != nil {
		t.Fatalf("MkdirAll global: %v", err)
	}
	if err := os.WriteFile(filepath.Join(globalDir, id+".json"), []byte("{"), 0o600); err != nil {
		t.Fatalf("Write corrupt shadow: %v", err)
	}

	got, ok := store.Get(id)
	if !ok || !reflect.DeepEqual(got, want) {
		t.Fatalf("Get with corrupt shadow = (%#v, %v); want (%#v, true)", got, ok, want)
	}
}

func TestFileStoreRejectsSupersedesWithoutPersisting(t *testing.T) {
	root := t.TempDir()
	store := NewFileStore(root)
	rec := testRecord("mem_0000000000000004", testProjectID, "decision", "Not task five", "Do not supersede yet")

	err := store.Save(rec, "mem_0000000000000003")
	if err == nil || !strings.Contains(err.Error(), "supersedes") {
		t.Fatalf("Save error = %v; want explicit supersedes rejection", err)
	}
	if got, ok := store.Get(rec.MemoryID); ok || !reflect.DeepEqual(got, Record{}) {
		t.Fatalf("rejected Save persisted record: (%#v, %v)", got, ok)
	}
}

func TestFileStoreSupersedesSuccessorFirstAndHidesTargetByDefault(t *testing.T) {
	root := t.TempDir()
	store := NewFileStore(root)
	target := testRecord("mem_0000000000000070", testProjectID, "decision", "Old decision", "old")
	successor := testRecord("mem_0000000000000071", testProjectID, "decision", "New decision", "new")
	if err := store.Save(target, ""); err != nil {
		t.Fatalf("Save target: %v", err)
	}
	if err := store.Save(successor, target.MemoryID); err != nil {
		t.Fatalf("Save successor: %v", err)
	}
	gotTarget, ok := store.Get(target.MemoryID)
	if !ok || gotTarget.Status != "superseded" || gotTarget.SupersededBy != successor.MemoryID {
		t.Fatalf("target = (%#v, %v); want superseded by successor", gotTarget, ok)
	}
	gotSuccessor, ok := store.Get(successor.MemoryID)
	if !ok || !reflect.DeepEqual(gotSuccessor, successor) {
		t.Fatalf("successor = (%#v, %v); want %#v", gotSuccessor, ok, successor)
	}
	visible := store.Search(SearchQuery{ProjectID: testProjectID, Query: "decision", K: 10})
	for _, result := range visible {
		if result.Record.MemoryID == target.MemoryID {
			t.Fatalf("default search returned superseded target: %#v", visible)
		}
	}
	included := store.Search(SearchQuery{ProjectID: testProjectID, Query: "decision", IncludeSuperseded: true, K: 10})
	if len(included) != 2 {
		t.Fatalf("include superseded search len = %d; want 2", len(included))
	}
}

func TestFileStoreSupersedesMissingTargetReturnsError(t *testing.T) {
	root := t.TempDir()
	store := NewFileStore(root)
	successor := testRecord("mem_0000000000000072", testProjectID, "decision", "New decision", "new")
	err := store.Save(successor, "mem_0000000000000073")
	if err == nil || !strings.Contains(err.Error(), "supersedes target") {
		t.Fatalf("Save missing target error = %v; want supersedes target error", err)
	}
	if _, ok := store.Get(successor.MemoryID); ok {
		t.Fatal("missing-target Save persisted successor")
	}
}

func TestFileStoreSupersedeCrashLeavesBothActiveAndRetryRepairs(t *testing.T) {
	root := t.TempDir()
	target := testRecord("mem_0000000000000074", testProjectID, "decision", "Old decision", "old")
	successor := testRecord("mem_0000000000000075", testProjectID, "decision", "New decision", "new")
	base := NewFileStore(root)
	if err := base.Save(target, ""); err != nil {
		t.Fatalf("Save target: %v", err)
	}
	crash := errors.New("simulated crash between supersede writes")
	store := newFileStoreWithHooks(root, storeHooks{
		afterSupersedeSuccessor: func() error { return crash },
	})
	if err := store.Save(successor, target.MemoryID); !errors.Is(err, crash) {
		t.Fatalf("crashed supersede error = %v; want %v", err, crash)
	}
	gotTarget, ok := store.Get(target.MemoryID)
	if !ok || gotTarget.Status != "active" {
		t.Fatalf("target after crash = (%#v, %v); want active", gotTarget, ok)
	}
	gotSuccessor, ok := store.Get(successor.MemoryID)
	if !ok || gotSuccessor.Status != "active" {
		t.Fatalf("successor after crash = (%#v, %v); want active", gotSuccessor, ok)
	}
	store.hooks.afterSupersedeSuccessor = nil
	if err := store.Save(successor, target.MemoryID); err != nil {
		t.Fatalf("retry supersede: %v", err)
	}
	gotTarget, ok = store.Get(target.MemoryID)
	if !ok || gotTarget.Status != "superseded" || gotTarget.SupersededBy != successor.MemoryID {
		t.Fatalf("target after retry = (%#v, %v); want superseded", gotTarget, ok)
	}
	if err := store.Save(successor, target.MemoryID); err != nil {
		t.Fatalf("idempotent retry supersede: %v", err)
	}
	gotSuccessor, ok = store.Get(successor.MemoryID)
	if !ok || !reflect.DeepEqual(gotSuccessor, successor) {
		t.Fatalf("successor after idempotent retry = (%#v, %v); want %#v", gotSuccessor, ok, successor)
	}
}

func TestFileStoreCleansTemporaryFileWhenRenameFails(t *testing.T) {
	root := t.TempDir()
	renameErr := errors.New("rename failed")
	store := newFileStoreWithHooks(root, storeHooks{
		rename: func(*os.Root, string, string) error { return renameErr },
	})
	rec := testRecord("mem_0000000000000005", testProjectID, "convention", "Atomic write", "Temp then rename")

	err := store.Save(rec, "")
	if !errors.Is(err, renameErr) {
		t.Fatalf("Save error = %v; want %v", err, renameErr)
	}
	dir := filepath.Join(root, "memory", testProjectID)
	temps, globErr := filepath.Glob(filepath.Join(dir, ".*.tmp"))
	if globErr != nil {
		t.Fatalf("Glob temps: %v", globErr)
	}
	if len(temps) != 0 {
		t.Fatalf("temporary files remain: %v", temps)
	}
	if _, statErr := os.Stat(filepath.Join(dir, rec.MemoryID+".json")); !os.IsNotExist(statErr) {
		t.Fatalf("partial record exists: %v", statErr)
	}
}

func TestFileStoreSyncsScopeDirectoryAfterRename(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "memory", testProjectID), 0o700); err != nil {
		t.Fatalf("MkdirAll existing scope: %v", err)
	}
	syncErr := errors.New("directory sync failed")
	syncCalled := false
	store := newFileStoreWithHooks(root, storeHooks{
		syncDir: func(dir *os.Root) error {
			if filepath.Base(dir.Name()) == testProjectID {
				syncCalled = true
				return syncErr
			}
			return nil
		},
	})
	rec := testRecord("mem_0000000000000043", testProjectID, "convention", "Durable rename", "Sync its parent")

	err := store.Save(rec, "")
	if !errors.Is(err, syncErr) {
		t.Fatalf("Save error = %v; want %v", err, syncErr)
	}
	if !syncCalled {
		t.Fatal("Save did not sync the scope directory")
	}
	if _, statErr := os.Stat(filepath.Join(root, "memory", testProjectID, rec.MemoryID+".json")); statErr != nil {
		t.Fatalf("directory sync ran before destination existed: %v", statErr)
	}
}

func TestFileStoreFirstSaveSyncsCreatedParentsBeforePersistingRecord(t *testing.T) {
	root := t.TempDir()
	events := []string{}
	store := newFileStoreWithHooks(root, storeHooks{
		syncDir: func(dir *os.Root) error {
			events = append(events, "sync:"+filepath.Base(dir.Name()))
			return nil
		},
		rename: func(scope *os.Root, oldName, newName string) error {
			events = append(events, "rename")
			return scope.Rename(oldName, newName)
		},
	})
	rec := testRecord("mem_0000000000000047", testProjectID, "decision", "First save", "Durable directories")

	if err := store.Save(rec, ""); err != nil {
		t.Fatalf("Save: %v", err)
	}
	want := []string{
		"sync:" + filepath.Base(filepath.Dir(root)),
		"sync:" + filepath.Base(root),
		"sync:memory",
		"rename",
		"sync:" + testProjectID,
	}
	if !reflect.DeepEqual(events, want) {
		t.Fatalf("events = %v; want %v", events, want)
	}
}

func TestFileStoreFirstSaveSyncsConfiguredRootParentBeforeTreePersistence(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "configured-root")
	events := []string{}
	store := newFileStoreWithHooks(root, storeHooks{
		syncDir: func(dir *os.Root) error {
			events = append(events, "sync:"+filepath.Base(dir.Name()))
			return nil
		},
		rename: func(scope *os.Root, oldName, newName string) error {
			events = append(events, "rename")
			return scope.Rename(oldName, newName)
		},
	})
	rec := testRecord("mem_0000000000000049", testProjectID, "decision", "Configured root", "Sync its parent")

	if err := store.Save(rec, ""); err != nil {
		t.Fatalf("Save: %v", err)
	}
	want := []string{
		"sync:" + filepath.Base(parent),
		"sync:" + filepath.Base(root),
		"sync:memory",
		"rename",
		"sync:" + testProjectID,
	}
	if !reflect.DeepEqual(events, want) {
		t.Fatalf("events = %v; want %v", events, want)
	}
}

func TestFileStoreConvergesConfiguredRootCreationAcrossDistinctStores(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "configured-root")
	firstReady := make(chan struct{}, 1)
	secondReady := make(chan struct{}, 1)
	release := make(chan struct{})
	first := newFileStoreWithHooks(root, storeHooks{
		beforeRootMkdir: func() error {
			firstReady <- struct{}{}
			<-release
			return nil
		},
	})
	second := newFileStoreWithHooks(root, storeHooks{
		beforeRootMkdir: func() error {
			secondReady <- struct{}{}
			<-release
			return nil
		},
	})
	id := "mem_0000000000000050"
	errs := make(chan error, 2)
	go func() {
		errs <- first.Save(testRecord(id, "", "preference", "Global", "First creator"), "")
	}()
	go func() {
		errs <- second.Save(testRecord(id, testProjectID, "preference", "Project", "Second creator"), "")
	}()
	for _, ready := range []<-chan struct{}{firstReady, secondReady} {
		select {
		case <-ready:
		case <-time.After(5 * time.Second):
			t.Fatal("store did not reach configured-root creation")
		}
	}
	close(release)
	assertOneDuplicateSave(t, <-errs, <-errs)
}

func TestFileStoreConvergesConfiguredRootCreationAcrossProcesses(t *testing.T) {
	if os.Getenv("JACU_MEMORY_ROOT_CREATE_WORKER") != "" {
		runFileStoreRootCreateWorker(t)
		return
	}

	control := t.TempDir()
	root := filepath.Join(control, "configured-root")
	id := "mem_0000000000000051"
	first := startFileStoreRootCreateWorker(t, control, root, "first", "", id)
	second := startFileStoreRootCreateWorker(t, control, root, "second", testProjectID, id)
	waitForFile(t, filepath.Join(control, "first.ready"), 5*time.Second)
	waitForFile(t, filepath.Join(control, "second.ready"), 5*time.Second)
	if err := os.WriteFile(filepath.Join(control, "release-create"), []byte("go"), 0o600); err != nil {
		t.Fatalf("release workers: %v", err)
	}
	if err := first.Wait(); err != nil {
		t.Fatalf("first worker: %v", err)
	}
	if err := second.Wait(); err != nil {
		t.Fatalf("second worker: %v", err)
	}
	firstResult, err := os.ReadFile(filepath.Join(control, "first.result"))
	if err != nil {
		t.Fatalf("read first result: %v", err)
	}
	secondResult, err := os.ReadFile(filepath.Join(control, "second.result"))
	if err != nil {
		t.Fatalf("read second result: %v", err)
	}
	assertOneDuplicateSave(t, resultError(firstResult), resultError(secondResult))
}

func TestFileStoreRetryResyncsEveryDirectoryAfterCreationSyncFailure(t *testing.T) {
	for failAt := 1; failAt <= 3; failAt++ {
		t.Run(fmt.Sprintf("sync_%d", failAt), func(t *testing.T) {
			parent := t.TempDir()
			root := filepath.Join(parent, "configured-root")
			syncCalls := 0
			retrying := false
			events := []string{}
			syncErr := errors.New("injected directory sync failure")
			store := newFileStoreWithHooks(root, storeHooks{
				syncDir: func(dir *os.Root) error {
					syncCalls++
					if !retrying && syncCalls == failAt {
						return syncErr
					}
					if retrying {
						events = append(events, "sync:"+filepath.Base(dir.Name()))
					}
					return nil
				},
				rename: func(scope *os.Root, oldName, newName string) error {
					if retrying {
						events = append(events, "rename")
					}
					return scope.Rename(oldName, newName)
				},
			})
			rec := testRecord(fmt.Sprintf("mem_%016x", 0x52+failAt), testProjectID, "decision", "Retry durability", "Resync every parent")

			if err := store.Save(rec, ""); !errors.Is(err, syncErr) {
				t.Fatalf("first Save error = %v; want %v", err, syncErr)
			}
			retrying = true
			if err := store.Save(rec, ""); err != nil {
				t.Fatalf("retry Save: %v", err)
			}
			want := []string{
				"sync:" + filepath.Base(parent),
				"sync:" + filepath.Base(root),
				"sync:memory",
				"rename",
				"sync:" + testProjectID,
			}
			if !reflect.DeepEqual(events, want) {
				t.Fatalf("retry events = %v; want %v", events, want)
			}
		})
	}
}

func runFileStoreRootCreateWorker(t *testing.T) {
	control := os.Getenv("JACU_MEMORY_ROOT_CREATE_CONTROL")
	root := os.Getenv("JACU_MEMORY_ROOT_CREATE_ROOT")
	name := os.Getenv("JACU_MEMORY_ROOT_CREATE_NAME")
	projectID := os.Getenv("JACU_MEMORY_ROOT_CREATE_PROJECT")
	id := os.Getenv("JACU_MEMORY_ROOT_CREATE_ID")
	store := newFileStoreWithHooks(root, storeHooks{
		beforeRootMkdir: func() error {
			if err := os.WriteFile(filepath.Join(control, name+".ready"), []byte("ready"), 0o600); err != nil {
				return err
			}
			waitForFile(t, filepath.Join(control, "release-create"), 5*time.Second)
			return nil
		},
	})
	err := store.Save(testRecord(id, projectID, "preference", name, "Concurrent root creator"), "")
	result := "ok"
	if err != nil {
		result = "error:" + err.Error()
	}
	if writeErr := os.WriteFile(filepath.Join(control, name+".result"), []byte(result), 0o600); writeErr != nil {
		t.Fatalf("write worker result: %v", writeErr)
	}
}

func startFileStoreRootCreateWorker(t *testing.T, control, root, name, projectID, id string) *exec.Cmd {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^TestFileStoreConvergesConfiguredRootCreationAcrossProcesses$", "-test.count=1")
	cmd.Env = append(os.Environ(),
		"JACU_MEMORY_ROOT_CREATE_WORKER=1",
		"JACU_MEMORY_ROOT_CREATE_CONTROL="+control,
		"JACU_MEMORY_ROOT_CREATE_ROOT="+root,
		"JACU_MEMORY_ROOT_CREATE_NAME="+name,
		"JACU_MEMORY_ROOT_CREATE_PROJECT="+projectID,
		"JACU_MEMORY_ROOT_CREATE_ID="+id,
	)
	if err := cmd.Start(); err != nil {
		t.Fatalf("start %s worker: %v", name, err)
	}
	return cmd
}

func TestFileStoreSaveCannotEscapeThroughScopeSymlinkSwap(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	memoryDir := filepath.Join(root, "memory")
	scopeDir := filepath.Join(memoryDir, testProjectID)
	heldDir := filepath.Join(memoryDir, testProjectID+".held")
	if err := os.MkdirAll(scopeDir, 0o700); err != nil {
		t.Fatalf("MkdirAll scope: %v", err)
	}

	store := newFileStoreWithHooks(root, storeHooks{
		afterScopeOpen: func() error {
			if err := os.Rename(scopeDir, heldDir); err != nil {
				return err
			}
			return os.Symlink(outside, scopeDir)
		},
	})
	rec := testRecord("mem_0000000000000044", testProjectID, "gotcha", "Confinement", "Do not escape")

	if err := store.Save(rec, ""); err != nil {
		t.Fatalf("Save during symlink swap: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outside, rec.MemoryID+".json")); !os.IsNotExist(err) {
		t.Fatalf("Save escaped through swapped scope symlink: %v", err)
	}
	if _, err := os.Stat(filepath.Join(heldDir, rec.MemoryID+".json")); err != nil {
		t.Fatalf("Save did not remain on opened scope directory: %v", err)
	}
}

func TestFileStoreSaveCannotEscapeThroughConfiguredRootSymlinkSwap(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "configured-root")
	heldRoot := filepath.Join(parent, "configured-root.held")
	outside := t.TempDir()
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatalf("Mkdir root: %v", err)
	}

	store := newFileStoreWithHooks(root, storeHooks{
		afterRootLstat: func() error {
			if err := os.Rename(root, heldRoot); err != nil {
				return err
			}
			return os.Symlink(outside, root)
		},
	})
	rec := testRecord("mem_0000000000000048", testProjectID, "gotcha", "Root confinement", "Do not escape")

	saveErr := store.Save(rec, "")
	if _, err := os.Stat(filepath.Join(outside, "memory", testProjectID, rec.MemoryID+".json")); !os.IsNotExist(err) {
		t.Fatalf("Save escaped through swapped configured root: %v", err)
	}
	if saveErr == nil {
		if _, err := os.Stat(filepath.Join(heldRoot, "memory", testProjectID, rec.MemoryID+".json")); err != nil {
			t.Fatalf("Save did not remain on opened configured root: %v", err)
		}
		return
	}
	if _, err := os.Stat(filepath.Join(heldRoot, "memory", testProjectID, rec.MemoryID+".json")); !os.IsNotExist(err) {
		t.Fatalf("Save did not remain on opened configured root: %v", err)
	}
}

func TestFileStoreSaveRejectsInternalMemoryDirectorySwapBeforeOpen(t *testing.T) {
	root := t.TempDir()
	memoryDir := filepath.Join(root, "memory")
	heldDir := filepath.Join(root, "memory.held")
	redirectDir := filepath.Join(root, "redirect-memory")
	if err := os.Mkdir(memoryDir, 0o700); err != nil {
		t.Fatalf("Mkdir memory: %v", err)
	}
	if err := os.Mkdir(redirectDir, 0o700); err != nil {
		t.Fatalf("Mkdir redirect: %v", err)
	}
	store := newFileStoreWithHooks(root, storeHooks{
		beforeMemoryOpen: func() error {
			return swapDirectoryForInternalSymlink(memoryDir, heldDir, redirectDir)
		},
	})
	rec := testRecord("mem_0000000000000056", testProjectID, "gotcha", "Memory swap", "Reject redirect")

	if err := store.Save(rec, ""); err == nil {
		t.Fatal("Save error = nil; want identity-change rejection")
	}
	if _, err := os.Stat(filepath.Join(redirectDir, testProjectID, rec.MemoryID+".json")); !os.IsNotExist(err) {
		t.Fatalf("Save redirected into internal memory directory: %v", err)
	}
}

func TestFileStoreSaveRejectsInternalScopeSwapBeforeOpen(t *testing.T) {
	root := t.TempDir()
	memoryDir := filepath.Join(root, "memory")
	scopeDir := filepath.Join(memoryDir, testProjectID)
	heldDir := filepath.Join(memoryDir, testProjectID+".held")
	redirectDir := filepath.Join(memoryDir, "redirect-scope")
	if err := os.MkdirAll(scopeDir, 0o700); err != nil {
		t.Fatalf("Mkdir scope: %v", err)
	}
	if err := os.Mkdir(redirectDir, 0o700); err != nil {
		t.Fatalf("Mkdir redirect: %v", err)
	}
	store := newFileStoreWithHooks(root, storeHooks{
		beforeScopeOpen: func(scope string) error {
			if scope != testProjectID {
				return nil
			}
			return swapDirectoryForInternalSymlink(scopeDir, heldDir, redirectDir)
		},
	})
	rec := testRecord("mem_0000000000000057", testProjectID, "gotcha", "Scope swap", "Reject redirect")

	if err := store.Save(rec, ""); err == nil {
		t.Fatal("Save error = nil; want identity-change rejection")
	}
	if _, err := os.Stat(filepath.Join(redirectDir, rec.MemoryID+".json")); !os.IsNotExist(err) {
		t.Fatalf("Save redirected into internal scope directory: %v", err)
	}
}

func TestFileStoreGetRejectsInternalMemoryDirectorySwapBeforeOpen(t *testing.T) {
	root := t.TempDir()
	id := "mem_0000000000000058"
	rec := testRecord(id, testProjectID, "gotcha", "Read redirect", "Must fail closed")
	if err := NewFileStore(root).Save(rec, ""); err != nil {
		t.Fatalf("Save fixture: %v", err)
	}
	memoryDir := filepath.Join(root, "memory")
	heldDir := filepath.Join(root, "memory.held")
	redirectDir := filepath.Join(root, "redirect-memory")
	copyRecordFixture(t, filepath.Join(memoryDir, testProjectID, id+".json"), filepath.Join(redirectDir, testProjectID, id+".json"))
	store := newFileStoreWithHooks(root, storeHooks{
		beforeMemoryOpen: func() error {
			return swapDirectoryForInternalSymlink(memoryDir, heldDir, redirectDir)
		},
	})

	if got, ok := store.Get(id); ok || !reflect.DeepEqual(got, Record{}) {
		t.Fatalf("Get followed internal memory redirect: (%#v, %v)", got, ok)
	}
}

func TestFileStoreGetRejectsInternalScopeSwapBeforeOpen(t *testing.T) {
	root := t.TempDir()
	id := "mem_0000000000000059"
	rec := testRecord(id, testProjectID, "gotcha", "Get scope redirect", "Must fail closed")
	if err := NewFileStore(root).Save(rec, ""); err != nil {
		t.Fatalf("Save fixture: %v", err)
	}
	memoryDir := filepath.Join(root, "memory")
	scopeDir := filepath.Join(memoryDir, testProjectID)
	heldDir := filepath.Join(memoryDir, testProjectID+".held")
	redirectDir := filepath.Join(memoryDir, "redirect-scope")
	copyRecordFixture(t, filepath.Join(scopeDir, id+".json"), filepath.Join(redirectDir, id+".json"))
	store := newFileStoreWithHooks(root, storeHooks{
		beforeScopeOpen: func(scope string) error {
			if scope != testProjectID {
				return nil
			}
			return swapDirectoryForInternalSymlink(scopeDir, heldDir, redirectDir)
		},
	})

	if got, ok := store.Get(id); ok || !reflect.DeepEqual(got, Record{}) {
		t.Fatalf("Get followed internal scope redirect: (%#v, %v)", got, ok)
	}
}

func TestFileStoreSearchRejectsInternalScopeSwapBeforeOpen(t *testing.T) {
	root := t.TempDir()
	id := "mem_0000000000000060"
	rec := testRecord(id, testProjectID, "gotcha", "Search scope redirect", "Must fail closed")
	if err := NewFileStore(root).Save(rec, ""); err != nil {
		t.Fatalf("Save fixture: %v", err)
	}
	memoryDir := filepath.Join(root, "memory")
	scopeDir := filepath.Join(memoryDir, testProjectID)
	heldDir := filepath.Join(memoryDir, testProjectID+".held")
	redirectDir := filepath.Join(memoryDir, "redirect-scope")
	copyRecordFixture(t, filepath.Join(scopeDir, id+".json"), filepath.Join(redirectDir, id+".json"))
	store := newFileStoreWithHooks(root, storeHooks{
		beforeScopeOpen: func(scope string) error {
			if scope != testProjectID {
				return nil
			}
			return swapDirectoryForInternalSymlink(scopeDir, heldDir, redirectDir)
		},
	})

	if got := store.Search(SearchQuery{ProjectID: testProjectID, Query: "Search scope redirect", K: 3}); len(got) != 0 {
		t.Fatalf("Search followed internal scope redirect: %#v", got)
	}
}

func TestFileStoreSaveFailsClosedWhenOtherScopeSwapsBeforeDuplicateCheckOpen(t *testing.T) {
	root := t.TempDir()
	id := "mem_0000000000000061"
	global := testRecord(id, "", "preference", "Existing duplicate", "Must not be missed")
	if err := NewFileStore(root).Save(global, ""); err != nil {
		t.Fatalf("Save global fixture: %v", err)
	}
	memoryDir := filepath.Join(root, "memory")
	globalDir := filepath.Join(memoryDir, "_global")
	heldDir := filepath.Join(memoryDir, "_global.held")
	redirectDir := filepath.Join(memoryDir, "redirect-scope")
	if err := os.Mkdir(redirectDir, 0o700); err != nil {
		t.Fatalf("Mkdir redirect: %v", err)
	}
	store := newFileStoreWithHooks(root, storeHooks{
		beforeOtherScopeOpen: func(scope string) error {
			if scope != "_global" {
				return nil
			}
			return swapDirectoryForInternalSymlink(globalDir, heldDir, redirectDir)
		},
	})
	project := testRecord(id, testProjectID, "preference", "New duplicate", "Must fail closed")

	if err := store.Save(project, ""); err == nil {
		t.Fatal("Save error = nil; duplicate check followed internal scope redirect")
	}
	if _, err := os.Stat(filepath.Join(root, "memory", testProjectID, id+".json")); !os.IsNotExist(err) {
		t.Fatalf("failed-closed Save persisted duplicate: %v", err)
	}
}

func TestFileStoreGetRejectsRegularFileSwapBeforeOpen(t *testing.T) {
	root := t.TempDir()
	id := "mem_0000000000000062"
	rec := testRecord(id, testProjectID, "gotcha", "Get file swap", "Must fail closed")
	if err := NewFileStore(root).Save(rec, ""); err != nil {
		t.Fatalf("Save fixture: %v", err)
	}
	scopeDir := filepath.Join(root, "memory", testProjectID)
	recordPath := filepath.Join(scopeDir, id+".json")
	heldPath := filepath.Join(scopeDir, id+".held")
	alternatePath := filepath.Join(scopeDir, "alternate.json")
	copyRecordFixture(t, recordPath, alternatePath)
	store := newFileStoreWithHooks(root, storeHooks{
		beforeRecordOpen: func(name string) error {
			if name != id+".json" {
				return nil
			}
			return swapRegularFileForInternalSymlink(recordPath, heldPath, alternatePath)
		},
	})

	if got, ok := store.Get(id); ok || !reflect.DeepEqual(got, Record{}) {
		t.Fatalf("Get followed internal regular-file redirect: (%#v, %v)", got, ok)
	}
}

func TestFileStoreSearchRejectsRegularFileSwapBeforeOpen(t *testing.T) {
	root := t.TempDir()
	id := "mem_0000000000000063"
	rec := testRecord(id, testProjectID, "gotcha", "Search file swap", "Must fail closed")
	if err := NewFileStore(root).Save(rec, ""); err != nil {
		t.Fatalf("Save fixture: %v", err)
	}
	scopeDir := filepath.Join(root, "memory", testProjectID)
	recordPath := filepath.Join(scopeDir, id+".json")
	heldPath := filepath.Join(scopeDir, id+".held")
	alternatePath := filepath.Join(scopeDir, "alternate.json")
	copyRecordFixture(t, recordPath, alternatePath)
	store := newFileStoreWithHooks(root, storeHooks{
		beforeRecordOpen: func(name string) error {
			if name != id+".json" {
				return nil
			}
			return swapRegularFileForInternalSymlink(recordPath, heldPath, alternatePath)
		},
	})

	if got := store.Search(SearchQuery{ProjectID: testProjectID, Query: "Search file swap", K: 3}); len(got) != 0 {
		t.Fatalf("Search followed internal regular-file redirect: %#v", got)
	}
}

func TestFileStoreReadsDoNotSyncDirectories(t *testing.T) {
	root := t.TempDir()
	id := "mem_0000000000000064"
	rec := testRecord(id, testProjectID, "gotcha", "Pure read", "Reads must not sync")
	if err := NewFileStore(root).Save(rec, ""); err != nil {
		t.Fatalf("Save fixture: %v", err)
	}

	t.Run("get", func(t *testing.T) {
		calls := 0
		store := newFileStoreWithHooks(root, storeHooks{
			syncDir: func(*os.Root) error {
				calls++
				return errors.New("read path attempted sync")
			},
		})
		got, ok := store.Get(id)
		if !ok || !reflect.DeepEqual(got, rec) {
			t.Fatalf("Get = (%#v, %v); want fixture", got, ok)
		}
		if calls != 0 {
			t.Fatalf("Get sync calls = %d; want 0", calls)
		}
	})

	t.Run("search", func(t *testing.T) {
		calls := 0
		store := newFileStoreWithHooks(root, storeHooks{
			syncDir: func(*os.Root) error {
				calls++
				return errors.New("read path attempted sync")
			},
		})
		got := store.Search(SearchQuery{ProjectID: testProjectID, Query: "Pure read", K: 3})
		if len(got) != 1 || !reflect.DeepEqual(got[0].Record, rec) {
			t.Fatalf("Search = %#v; want fixture", got)
		}
		if calls != 0 {
			t.Fatalf("Search sync calls = %d; want 0", calls)
		}
	})
}

func swapDirectoryForInternalSymlink(path, held, target string) error {
	if err := os.Rename(path, held); err != nil {
		return err
	}
	return os.Symlink(filepath.Base(target), path)
}

func swapRegularFileForInternalSymlink(path, held, target string) error {
	if err := os.Rename(path, held); err != nil {
		return err
	}
	return os.Symlink(filepath.Base(target), path)
}

func copyRecordFixture(t *testing.T, source, destination string) {
	t.Helper()
	content, err := os.ReadFile(source)
	if err != nil {
		t.Fatalf("Read fixture: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
		t.Fatalf("Mkdir fixture destination: %v", err)
	}
	if err := os.WriteFile(destination, content, 0o600); err != nil {
		t.Fatalf("Write fixture: %v", err)
	}
}

func writePersistedRecord(t *testing.T, root string, rec Record) {
	t.Helper()
	content, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		t.Fatalf("Marshal record: %v", err)
	}
	dir := filepath.Join(root, "memory", scopeDirectory(rec.ProjectID))
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("Mkdir persisted scope: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, rec.MemoryID+".json"), append(content, '\n'), 0o600); err != nil {
		t.Fatalf("Write persisted record: %v", err)
	}
}

func TestFileStoreGetFailsClosedForCorruptOrMismatchedRecord(t *testing.T) {
	root := t.TempDir()
	store := NewFileStore(root)
	dir := filepath.Join(root, "memory", testProjectID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	corruptID := "mem_0000000000000006"
	if err := os.WriteFile(filepath.Join(dir, corruptID+".json"), []byte("{"), 0o600); err != nil {
		t.Fatalf("Write corrupt: %v", err)
	}
	mismatchID := "mem_0000000000000007"
	mismatch := `{"memory_id":"mem_0000000000000008","project_id":"prj_0123456789abcdef","status":"active"}`
	if err := os.WriteFile(filepath.Join(dir, mismatchID+".json"), []byte(mismatch), 0o600); err != nil {
		t.Fatalf("Write mismatch: %v", err)
	}
	for _, id := range []string{corruptID, mismatchID} {
		if got, ok := store.Get(id); ok || !reflect.DeepEqual(got, Record{}) {
			t.Fatalf("Get(%q) = (%#v, %v); want zero, false", id, got, ok)
		}
	}
}

func TestFileStoreGetAndSearchFailClosedForInvalidPersistedRecord(t *testing.T) {
	tests := []struct {
		name   string
		record Record
	}{
		{
			name:   "invalid kind",
			record: testRecord("mem_0000000000000065", testProjectID, "note", "Invalid kind", "Fail closed"),
		},
		{
			name: "invalid source",
			record: func() Record {
				rec := testRecord("mem_0000000000000066", testProjectID, "gotcha", "Invalid source", "Fail closed")
				rec.Source = "agent"
				return rec
			}(),
		},
		{
			name:   "global non-preference",
			record: testRecord("mem_0000000000000067", "", "decision", "Invalid global", "Fail closed"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			writePersistedRecord(t, root, tt.record)
			store := NewFileStore(root)

			if got, ok := store.Get(tt.record.MemoryID); ok || !reflect.DeepEqual(got, Record{}) {
				t.Errorf("Get invalid persisted record = (%#v, %v); want zero, false", got, ok)
			}
			if got := store.Search(SearchQuery{ProjectID: tt.record.ProjectID, Query: "", K: 3}); len(got) != 0 {
				t.Fatalf("Search invalid persisted record = %#v; want empty", got)
			}
		})
	}
}

func TestFileStoreGetFailsClosedForSameIDInTwoValidScopes(t *testing.T) {
	root := t.TempDir()
	id := "mem_0000000000000068"
	global := testRecord(id, "", "preference", "Global duplicate", "First valid match")
	project := testRecord(id, testProjectID, "preference", "Project duplicate", "Second valid match")
	writePersistedRecord(t, root, global)
	writePersistedRecord(t, root, project)

	got, ok := NewFileStore(root).Get(id)
	if ok || !reflect.DeepEqual(got, Record{}) {
		t.Fatalf("Get duplicate persisted records = (%#v, %v); want zero, false", got, ok)
	}
}

func TestFileStoreSerializesConcurrentSaves(t *testing.T) {
	root := t.TempDir()
	store := NewFileStore(root)
	const count = 32
	var wg sync.WaitGroup
	errs := make(chan error, count)
	for i := 0; i < count; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			id := fmt.Sprintf("mem_%016x", i+1)
			errs <- store.Save(testRecord(id, testProjectID, "decision", "Concurrent", fmt.Sprintf("body %d", i)), "")
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent Save: %v", err)
		}
	}
	for i := 0; i < count; i++ {
		id := fmt.Sprintf("mem_%016x", i+1)
		if _, ok := store.Get(id); !ok {
			t.Fatalf("Get(%q) = false after concurrent Save", id)
		}
	}
}

func testRecord(id, projectID, kind, title, body string) Record {
	return Record{
		MemoryID: id, ProjectID: projectID, Kind: kind, Title: title, Body: body,
		Evidence: []string{"docs/evidence.md"}, Source: "human", Status: "active",
		SupersededBy: "", CreatedAt: "2026-07-31T12:00:00Z", UpdatedAt: "2026-07-31T12:00:00Z",
	}
}
