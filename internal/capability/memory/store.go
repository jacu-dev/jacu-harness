package memory

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
)

var (
	projectIDPattern = regexp.MustCompile(`^prj_[a-f0-9]{16}$`)
)

type SearchQuery struct {
	ProjectID         string
	Query             string
	Kinds             []string
	IncludeSuperseded bool
	K                 int
}

type Scored struct {
	Record Record
	Score  int
}

type Store interface {
	Save(rec Record, supersedes string) error
	Get(id string) (Record, bool)
	Search(q SearchQuery) []Scored
}

type storeHooks struct {
	rename                  func(*os.Root, string, string) error
	syncDir                 func(*os.Root) error
	beforeRootMkdir         func() error
	afterRootLstat          func() error
	beforeMemoryOpen        func() error
	beforeScopeOpen         func(string) error
	beforeOtherScopeOpen    func(string) error
	beforeRecordOpen        func(string) error
	afterScopeOpen          func() error
	afterUniquenessCheck    func() error
	afterSupersedeSuccessor func() error
}

type fileStore struct {
	root  string
	hooks storeHooks
	mu    sync.Mutex
}

var storeRootLocks sync.Map

func NewFileStore(root string) Store {
	return newFileStoreWithHooks(root, storeHooks{})
}

func newFileStoreWithHooks(root string, hooks storeHooks) *fileStore {
	if hooks.rename == nil {
		hooks.rename = func(scope *os.Root, oldName, newName string) error {
			return scope.Rename(oldName, newName)
		}
	}
	if hooks.syncDir == nil {
		hooks.syncDir = syncRootDirectory
	}
	return &fileStore{root: root, hooks: hooks}
}

func (s *fileStore) Save(rec Record, supersedes string) error {
	if !memoryIDPattern.MatchString(rec.MemoryID) {
		return fmt.Errorf("invalid memory_id %q", rec.MemoryID)
	}
	if !validProjectID(rec.ProjectID) {
		return fmt.Errorf("invalid project_id %q", rec.ProjectID)
	}
	if !validStoredStatus(rec.Status) {
		return fmt.Errorf("invalid status %q", rec.Status)
	}
	if !validKind(rec.Kind) {
		return fmt.Errorf("invalid kind %q", rec.Kind)
	}
	if !validStoredSource(rec.Source) {
		return fmt.Errorf("invalid source %q", rec.Source)
	}
	if rec.ProjectID == "" && rec.Kind != "preference" {
		return errors.New("global scope is restricted to preference records")
	}
	if supersedes != "" {
		if !memoryIDPattern.MatchString(supersedes) {
			return fmt.Errorf("invalid supersedes memory_id %q", supersedes)
		}
		if supersedes == rec.MemoryID {
			return errors.New("supersedes target must differ from successor")
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	configuredRoot, canonicalRoot, err := s.openConfiguredRoot(true)
	if err != nil {
		return err
	}
	defer configuredRoot.Close()
	rootLock := mutexForStoreRoot(canonicalRoot)
	rootLock.Lock()
	defer rootLock.Unlock()
	processLock, err := acquireStoreProcessLock(configuredRoot)
	if err != nil {
		return err
	}
	defer releaseStoreProcessLock(processLock)

	memoryRoot, scopeRoot, err := s.openScopeForSave(configuredRoot, rec.ProjectID)
	if err != nil {
		return err
	}
	defer memoryRoot.Close()
	defer scopeRoot.Close()
	if s.hooks.afterScopeOpen != nil {
		if err := s.hooks.afterScopeOpen(); err != nil {
			return err
		}
	}
	if used, err := memoryIDUsedByOtherScope(memoryRoot, scopeDirectory(rec.ProjectID), rec.MemoryID, s.hooks.beforeOtherScopeOpen); err != nil {
		return err
	} else if used {
		return fmt.Errorf("memory_id %q already exists in another scope", rec.MemoryID)
	}
	if s.hooks.afterUniquenessCheck != nil {
		if err := s.hooks.afterUniquenessCheck(); err != nil {
			return err
		}
	}
	if supersedes != "" {
		target, targetScope, found, err := findRecordLocation(memoryRoot, supersedes, s.hooks.beforeScopeOpen, s.hooks.beforeRecordOpen)
		if err != nil {
			return err
		}
		if !found {
			return fmt.Errorf("supersedes target %q not found", supersedes)
		}
		if target.ProjectID != rec.ProjectID {
			return fmt.Errorf("supersedes target %q belongs to a different project", supersedes)
		}
		if target.Status == "superseded" && target.SupersededBy != rec.MemoryID {
			return fmt.Errorf("supersedes target %q already superseded", supersedes)
		}
		targetScopeRoot, _, err := openRootChild(memoryRoot, targetScope, false, false, nil, scopeOpenHook(s.hooks.beforeScopeOpen, targetScope))
		if err != nil {
			return err
		}
		defer targetScopeRoot.Close()
		if err := writeRecordRoot(scopeRoot, rec, s.hooks); err != nil {
			return err
		}
		if s.hooks.afterSupersedeSuccessor != nil {
			if err := s.hooks.afterSupersedeSuccessor(); err != nil {
				return err
			}
		}
		target.Status = "superseded"
		target.SupersededBy = rec.MemoryID
		return writeRecordRoot(targetScopeRoot, target, s.hooks)
	}
	content, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return err
	}
	content = append(content, '\n')

	tempName, temp, err := createRootTemp(scopeRoot, "."+rec.MemoryID+"-")
	if err != nil {
		return err
	}
	removeTemp := true
	defer func() {
		if removeTemp {
			_ = scopeRoot.Remove(tempName)
		}
	}()
	if err := temp.Chmod(0o600); err != nil {
		_ = temp.Close()
		return err
	}
	if _, err := temp.Write(content); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	destination := rec.MemoryID + ".json"
	if err := s.hooks.rename(scopeRoot, tempName, destination); err != nil {
		return err
	}
	removeTemp = false
	return s.hooks.syncDir(scopeRoot)
}

func writeRecordRoot(scopeRoot *os.Root, rec Record, hooks storeHooks) error {
	content, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return err
	}
	content = append(content, '\n')
	tempName, temp, err := createRootTemp(scopeRoot, "."+rec.MemoryID+"-")
	if err != nil {
		return err
	}
	removeTemp := true
	defer func() {
		if removeTemp {
			_ = scopeRoot.Remove(tempName)
		}
	}()
	if err := temp.Chmod(0o600); err != nil {
		_ = temp.Close()
		return err
	}
	if _, err := temp.Write(content); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := hooks.rename(scopeRoot, tempName, rec.MemoryID+".json"); err != nil {
		return err
	}
	removeTemp = false
	return hooks.syncDir(scopeRoot)
}

func findRecordLocation(memoryRoot *os.Root, id string, beforeScope func(string) error, beforeRecord func(string) error) (Record, string, bool, error) {
	entries, err := readRootDir(memoryRoot)
	if err != nil {
		return Record{}, "", false, err
	}
	var found Record
	foundScope := ""
	for _, entry := range entries {
		projectID, ok := projectIDForScope(entry.Name())
		if !ok || entry.Type()&os.ModeSymlink != 0 || !entry.IsDir() {
			continue
		}
		scopeRoot, _, err := openRootChild(memoryRoot, entry.Name(), false, false, nil, scopeOpenHook(beforeScope, entry.Name()))
		if err != nil {
			return Record{}, "", false, err
		}
		rec, exists := readRecordRoot(scopeRoot, id+".json", id, projectID, beforeRecord)
		_ = scopeRoot.Close()
		if !exists {
			continue
		}
		if foundScope != "" {
			return Record{}, "", false, fmt.Errorf("memory_id %q exists in multiple scopes", id)
		}
		found, foundScope = rec, entry.Name()
	}
	return found, foundScope, foundScope != "", nil
}

func (s *fileStore) Get(id string) (Record, bool) {
	if !memoryIDPattern.MatchString(id) {
		return Record{}, false
	}
	memoryRoot, err := s.openMemoryForRead()
	if err != nil {
		return Record{}, false
	}
	defer memoryRoot.Close()
	entries, err := readRootDir(memoryRoot)
	if err != nil {
		return Record{}, false
	}
	var match Record
	foundMatch := false
	for _, entry := range entries {
		projectID, ok := projectIDForScope(entry.Name())
		if !ok || entry.Type()&os.ModeSymlink != 0 || !entry.IsDir() {
			continue
		}
		scopeRoot, _, err := openRootChild(memoryRoot, entry.Name(), false, false, nil, scopeOpenHook(s.hooks.beforeScopeOpen, entry.Name()))
		if err != nil {
			continue
		}
		rec, found := readRecordRoot(scopeRoot, id+".json", id, projectID, s.hooks.beforeRecordOpen)
		_ = scopeRoot.Close()
		if found {
			if foundMatch {
				return Record{}, false
			}
			match = rec
			foundMatch = true
		}
	}
	return match, foundMatch
}

func (s *fileStore) openScopeForSave(root *os.Root, projectID string) (*os.Root, *os.Root, error) {
	memoryRoot, _, err := openRootChild(root, "memory", true, true, nil, s.hooks.beforeMemoryOpen)
	if err != nil {
		return nil, nil, err
	}
	if err := s.hooks.syncDir(root); err != nil {
		memoryRoot.Close()
		return nil, nil, err
	}
	scope := scopeDirectory(projectID)
	scopeRoot, _, err := openRootChild(memoryRoot, scope, true, true, nil, scopeOpenHook(s.hooks.beforeScopeOpen, scope))
	if err != nil {
		memoryRoot.Close()
		return nil, nil, err
	}
	if err := s.hooks.syncDir(memoryRoot); err != nil {
		scopeRoot.Close()
		memoryRoot.Close()
		return nil, nil, err
	}
	return memoryRoot, scopeRoot, nil
}

func (s *fileStore) openMemoryForRead() (*os.Root, error) {
	root, _, err := s.openConfiguredRoot(false)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	memoryRoot, _, err := openRootChild(root, "memory", false, false, nil, s.hooks.beforeMemoryOpen)
	return memoryRoot, err
}

func (s *fileStore) openConfiguredRoot(create bool) (*os.Root, string, error) {
	absRoot, err := filepath.Abs(s.root)
	if err != nil {
		return nil, "", err
	}
	parentPath, base := filepath.Dir(absRoot), filepath.Base(absRoot)
	canonicalParent, err := filepath.EvalSymlinks(parentPath)
	if err != nil {
		return nil, "", err
	}
	parentRoot, err := openAbsoluteRoot(canonicalParent)
	if err != nil {
		return nil, "", err
	}
	defer parentRoot.Close()
	child, _, err := openRootChild(parentRoot, base, create, create, s.hooks.beforeRootMkdir, s.hooks.afterRootLstat)
	if err != nil {
		return nil, "", err
	}
	if create {
		if err := s.hooks.syncDir(parentRoot); err != nil {
			child.Close()
			return nil, "", err
		}
	}
	return child, filepath.Join(canonicalParent, base), nil
}

func openAbsoluteRoot(path string) (*os.Root, error) {
	clean := filepath.Clean(path)
	if !filepath.IsAbs(clean) {
		return nil, fmt.Errorf("trusted root path is not absolute: %s", path)
	}
	root, err := os.OpenRoot(string(filepath.Separator))
	if err != nil {
		return nil, err
	}
	if clean == string(filepath.Separator) {
		return root, nil
	}
	for _, component := range strings.Split(strings.TrimPrefix(clean, string(filepath.Separator)), string(filepath.Separator)) {
		child, _, err := openRootChild(root, component, false, false, nil, nil)
		if err != nil {
			root.Close()
			return nil, err
		}
		root.Close()
		root = child
	}
	return root, nil
}

func openRootChild(parent *os.Root, name string, create, private bool, beforeMkdir, afterLstat func() error) (*os.Root, bool, error) {
	before, err := parent.Lstat(name)
	created := false
	if errors.Is(err, os.ErrNotExist) && create {
		if beforeMkdir != nil {
			if err := beforeMkdir(); err != nil {
				return nil, false, err
			}
		}
		if err := parent.Mkdir(name, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
			return nil, false, err
		}
		// The directory was created during this operation, either here or by a
		// concurrent creator. Both paths must sync the parent before continuing.
		created = true
		before, err = parent.Lstat(name)
	}
	if err != nil {
		return nil, false, err
	}
	if before.Mode()&os.ModeSymlink != 0 || !before.IsDir() {
		return nil, false, fmt.Errorf("memory path is not a directory: %s", name)
	}
	if afterLstat != nil {
		if err := afterLstat(); err != nil {
			return nil, false, err
		}
	}
	child, err := parent.OpenRoot(name)
	if err != nil {
		return nil, false, err
	}
	dir, err := child.Open(".")
	if err != nil {
		child.Close()
		return nil, false, err
	}
	opened, statErr := dir.Stat()
	if statErr == nil && private {
		statErr = dir.Chmod(0o700)
	}
	closeErr := dir.Close()
	if statErr != nil {
		child.Close()
		return nil, false, statErr
	}
	if closeErr != nil {
		child.Close()
		return nil, false, closeErr
	}
	after, err := parent.Lstat(name)
	if err != nil || !os.SameFile(before, opened) || !os.SameFile(after, opened) {
		child.Close()
		return nil, false, fmt.Errorf("memory path identity changed while opening: %s", name)
	}
	return child, created, nil
}

func mutexForStoreRoot(root string) *sync.Mutex {
	lock, _ := storeRootLocks.LoadOrStore(root, &sync.Mutex{})
	return lock.(*sync.Mutex)
}

func scopeOpenHook(hook func(string) error, scope string) func() error {
	if hook == nil {
		return nil
	}
	return func() error { return hook(scope) }
}

func acquireStoreProcessLock(root *os.Root) (*os.File, error) {
	file, err := root.OpenFile(".memory.lock", os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o600)
	if errors.Is(err, os.ErrExist) {
		file, err = root.OpenFile(".memory.lock", os.O_RDWR, 0)
	}
	if err != nil {
		return nil, err
	}
	if err := file.Chmod(0o600); err != nil {
		file.Close()
		return nil, err
	}
	if err := lockStoreFile(file); err != nil {
		file.Close()
		return nil, err
	}
	return file, nil
}

func releaseStoreProcessLock(file *os.File) {
	_ = unlockStoreFile(file)
	_ = file.Close()
}

func readRecordRoot(root *os.Root, name, memoryID, projectID string, beforeOpen func(string) error) (Record, bool) {
	before, err := root.Lstat(name)
	if err != nil || before.Mode()&os.ModeSymlink != 0 || !before.Mode().IsRegular() {
		return Record{}, false
	}
	if beforeOpen != nil {
		if err := beforeOpen(name); err != nil {
			return Record{}, false
		}
	}
	file, err := root.Open(name)
	if err != nil {
		return Record{}, false
	}
	opened, statErr := file.Stat()
	after, lstatErr := root.Lstat(name)
	if statErr != nil || lstatErr != nil || !opened.Mode().IsRegular() || !os.SameFile(before, opened) || !os.SameFile(after, opened) {
		_ = file.Close()
		return Record{}, false
	}
	content, err := io.ReadAll(file)
	closeErr := file.Close()
	if err != nil || closeErr != nil {
		return Record{}, false
	}
	var rec Record
	if err := json.Unmarshal(content, &rec); err != nil {
		return Record{}, false
	}
	if rec.MemoryID != memoryID || rec.ProjectID != projectID ||
		!validStoredStatus(rec.Status) || !validKind(rec.Kind) || !validStoredSource(rec.Source) ||
		rec.ProjectID == "" && rec.Kind != "preference" {
		return Record{}, false
	}
	return rec, true
}

func memoryIDUsedByOtherScope(memoryRoot *os.Root, targetScope, memoryID string, beforeOpen func(string) error) (bool, error) {
	entries, err := readRootDir(memoryRoot)
	if err != nil {
		return false, err
	}
	for _, entry := range entries {
		if entry.Name() == targetScope {
			continue
		}
		if _, ok := projectIDForScope(entry.Name()); !ok {
			continue
		}
		if entry.Type()&os.ModeSymlink != 0 || !entry.IsDir() {
			return false, fmt.Errorf("memory scope is not a directory: %s", entry.Name())
		}
		scopeRoot, _, err := openRootChild(memoryRoot, entry.Name(), false, false, nil, scopeOpenHook(beforeOpen, entry.Name()))
		if err != nil {
			return false, err
		}
		_, statErr := scopeRoot.Lstat(memoryID + ".json")
		_ = scopeRoot.Close()
		if statErr == nil {
			return true, nil
		}
		if !errors.Is(statErr, os.ErrNotExist) {
			return false, statErr
		}
	}
	return false, nil
}

func readRootDir(root *os.Root) ([]os.DirEntry, error) {
	dir, err := root.Open(".")
	if err != nil {
		return nil, err
	}
	entries, readErr := dir.ReadDir(-1)
	closeErr := dir.Close()
	if readErr != nil {
		return nil, readErr
	}
	if closeErr != nil {
		return nil, closeErr
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	return entries, nil
}

func createRootTemp(root *os.Root, prefix string) (string, *os.File, error) {
	for range 100 {
		var random [8]byte
		if _, err := rand.Read(random[:]); err != nil {
			return "", nil, err
		}
		name := fmt.Sprintf("%s%x.tmp", prefix, random)
		file, err := root.OpenFile(name, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o600)
		if errors.Is(err, os.ErrExist) {
			continue
		}
		return name, file, err
	}
	return "", nil, errors.New("could not allocate memory temp file")
}

func syncRootDirectory(root *os.Root) error {
	dir, err := root.Open(".")
	if err != nil {
		return err
	}
	if err := dir.Sync(); err != nil {
		_ = dir.Close()
		return err
	}
	return dir.Close()
}

func validProjectID(projectID string) bool {
	return projectID == "" || projectIDPattern.MatchString(projectID)
}

func validStoredStatus(status string) bool {
	return status == "active" || status == "superseded"
}

func validStoredSource(source string) bool {
	return source == "human" || source == "derived"
}

func scopeDirectory(projectID string) string {
	if projectID == "" {
		return "_global"
	}
	return projectID
}

func projectIDForScope(scope string) (string, bool) {
	if scope == "_global" {
		return "", true
	}
	if !projectIDPattern.MatchString(scope) || strings.ContainsAny(scope, `/\\`) {
		return "", false
	}
	return scope, true
}
