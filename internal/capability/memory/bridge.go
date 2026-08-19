package memory

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

const (
	BridgeStart          = "<!-- JACU MEMORY START -->"
	BridgeEnd            = "<!-- JACU MEMORY END -->"
	bridgeSourcePrefix   = "<!-- JACU MEMORY SOURCE: "
	bridgeChecksumPrefix = "<!-- JACU MEMORY CHECKSUM: "
	bridgeChecksumSuffix = " -->"
	maxBridgeRecords     = 10000
)

var (
	ErrBridgeChecksumMismatch = errors.New("JACU memory bridge checksum mismatch")
	ErrBridgeChecksumMissing  = errors.New("JACU memory bridge checksum missing")
	ErrBridgeMalformed        = errors.New("JACU memory bridge region is malformed")
	ErrBridgeSecretContent    = errors.New("JACU memory bridge contains secret content")
)

type BridgeStatus string

const (
	BridgeCreated   BridgeStatus = "created"
	BridgeAppended  BridgeStatus = "appended"
	BridgeUpdated   BridgeStatus = "updated"
	BridgeUnchanged BridgeStatus = "unchanged"
)

type BridgeResult struct {
	Status      BridgeStatus `json:"status"`
	SourceHash  string       `json:"source_hash"`
	RecordCount int          `json:"record_count"`
	Changed     bool         `json:"changed"`
}

type bridgeSourceRecord struct {
	MemoryID  string   `json:"memory_id"`
	ProjectID string   `json:"project_id"`
	Title     string   `json:"title"`
	Body      string   `json:"body"`
	Evidence  []string `json:"evidence"`
}

type bridgeRegion struct {
	start int
	end   int
}

var bridgePathLocks sync.Map

// RenderAgentsBridge returns the complete managed region and its source hash.
// The source hash covers normalized active conventions, not timestamps.
func RenderAgentsBridge(records []Record) ([]byte, string, error) {
	conventions, err := activeConventions(records)
	if err != nil {
		return nil, "", err
	}
	sourceHash, err := bridgeSourceHash(conventions)
	if err != nil {
		return nil, "", err
	}

	var body strings.Builder
	fmt.Fprintf(&body, "%ssha256:%s%s\n", bridgeSourcePrefix, sourceHash, bridgeChecksumSuffix)
	body.WriteString("## JACU active conventions\n\n")
	if len(conventions) == 0 {
		body.WriteString("_No active conventions._\n")
	}
	for _, record := range conventions {
		fmt.Fprintf(&body, "- `%s` **%s** — %s\n", record.MemoryID, bridgeText(record.Title), bridgeText(record.Body))
	}
	bodyBytes := []byte(body.String())
	checksum := digestHex(bodyBytes)
	region := fmt.Sprintf("%s\n%s%s%s\n%s\n", BridgeStart, bodyBytes, bridgeChecksumPrefix, "sha256:"+checksum+bridgeChecksumSuffix, BridgeEnd)
	return []byte(region), "sha256:" + sourceHash, nil
}

// SyncAgentsFile appends or replaces only the managed memory region. A region
// whose checksum cannot be verified is never rewritten.
func SyncAgentsFile(path string, records []Record) (BridgeResult, error) {
	region, sourceHash, err := RenderAgentsBridge(records)
	result := BridgeResult{SourceHash: sourceHash, RecordCount: len(activeConventionIDs(records))}
	if err != nil {
		return result, err
	}
	if strings.TrimSpace(path) == "" {
		return result, errors.New("AGENTS.md path is required")
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return result, err
	}
	lock, _ := bridgePathLocks.LoadOrStore(absPath, &sync.Mutex{})
	lock.(*sync.Mutex).Lock()
	defer lock.(*sync.Mutex).Unlock()

	current, mode, exists, err := readAgentsFile(absPath)
	if err != nil {
		return result, err
	}
	var next []byte
	if !exists {
		next = appendManagedRegion(nil, region)
		result.Status = BridgeCreated
	} else if managed, found, inspectErr := inspectBridge(current); inspectErr != nil {
		return result, inspectErr
	} else if !found {
		next = appendManagedRegion(current, region)
		result.Status = BridgeAppended
	} else {
		if err := verifyBridgeChecksum(current, managed); err != nil {
			return result, err
		}
		next = make([]byte, 0, len(current)+len(region))
		next = append(next, current[:managed.start]...)
		next = append(next, region...)
		tailStart := managed.end + len(BridgeEnd)
		if strings.HasPrefix(string(current[tailStart:]), "\r\n") {
			tailStart += 2
		} else if tailStart < len(current) && current[tailStart] == '\n' {
			tailStart++
		}
		next = append(next, current[tailStart:]...)
		result.Status = BridgeUpdated
	}
	if string(next) == string(current) {
		result.Status = BridgeUnchanged
		return result, nil
	}
	if !exists {
		mode = 0o644
	}
	if err := atomicReplace(absPath, next, mode); err != nil {
		return result, err
	}
	result.Changed = true
	return result, nil
}

// SyncProjectAgents renders all current project conventions through the same
// normal memory flow used by the save tool.
func SyncProjectAgents(store Store, projectID, agentsPath string) (BridgeResult, error) {
	if store == nil {
		return BridgeResult{}, errors.New("memory store is required")
	}
	if !validProjectID(projectID) || projectID == "" {
		return BridgeResult{}, errors.New("project_id is required for AGENTS bridge")
	}
	records := store.Search(SearchQuery{ProjectID: projectID, Kinds: []string{"convention"}, K: maxBridgeRecords})
	return SyncAgentsFile(agentsPath, recordsToRecords(records))
}

func activeConventions(records []Record) ([]Record, error) {
	result := make([]Record, 0, len(records))
	seen := make(map[string]struct{}, len(records))
	for _, record := range records {
		if record.Kind != "convention" || record.Status != "active" {
			continue
		}
		if !memoryIDPattern.MatchString(record.MemoryID) || !validProjectID(record.ProjectID) || record.ProjectID == "" || !validStoredSource(record.Source) {
			return nil, fmt.Errorf("invalid convention identity %q", record.MemoryID)
		}
		if strings.TrimSpace(record.Title) == "" || containsSecret(record.Title) || containsSecret(record.Body) {
			return nil, ErrBridgeSecretContent
		}
		if _, ok := seen[record.MemoryID]; ok {
			return nil, fmt.Errorf("duplicate convention memory_id %q", record.MemoryID)
		}
		seen[record.MemoryID] = struct{}{}
		record.Title = strings.TrimSpace(record.Title)
		record.Body = strings.TrimSpace(record.Body)
		record.Evidence = normalizeEvidence(record.Evidence)
		result = append(result, record)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].MemoryID < result[j].MemoryID })
	return result, nil
}

func bridgeSourceHash(records []Record) (string, error) {
	projection := make([]bridgeSourceRecord, len(records))
	for index, record := range records {
		projection[index] = bridgeSourceRecord{MemoryID: record.MemoryID, ProjectID: record.ProjectID, Title: record.Title, Body: record.Body, Evidence: record.Evidence}
	}
	encoded, err := json.Marshal(projection)
	if err != nil {
		return "", err
	}
	return digestHex(encoded), nil
}

func bridgeText(value string) string {
	value = strings.NewReplacer("<!--", "&lt;!--", "-->", "--&gt;").Replace(value)
	return strings.Join(strings.Fields(value), " ")
}

func activeConventionIDs(records []Record) []string {
	result := make([]string, 0)
	for _, record := range records {
		if record.Kind == "convention" && record.Status == "active" {
			result = append(result, record.MemoryID)
		}
	}
	return result
}

func recordsToRecords(scored []Scored) []Record {
	result := make([]Record, 0, len(scored))
	for _, item := range scored {
		result = append(result, item.Record)
	}
	return result
}

func inspectBridge(content []byte) (bridgeRegion, bool, error) {
	text := string(content)
	startCount := strings.Count(text, BridgeStart)
	endCount := strings.Count(text, BridgeEnd)
	if startCount == 0 && endCount == 0 {
		return bridgeRegion{}, false, nil
	}
	if startCount != 1 || endCount != 1 {
		return bridgeRegion{}, false, ErrBridgeMalformed
	}
	start := strings.Index(text, BridgeStart)
	end := strings.Index(text, BridgeEnd)
	if end < start || !lineMarker(text, start, len(BridgeStart)) || !lineMarker(text, end, len(BridgeEnd)) {
		return bridgeRegion{}, false, ErrBridgeMalformed
	}
	return bridgeRegion{start: start, end: end}, true, nil
}

func lineMarker(content string, index, length int) bool {
	if index > 0 && content[index-1] != '\n' {
		return false
	}
	end := index + length
	return end == len(content) || content[end] == '\n' || content[end] == '\r'
}

func verifyBridgeChecksum(content []byte, region bridgeRegion) error {
	text := string(content[region.start+len(BridgeStart) : region.end])
	text = strings.TrimPrefix(text, "\n")
	text = strings.TrimPrefix(text, "\r\n")
	marker := strings.Index(text, bridgeChecksumPrefix)
	if marker < 0 {
		return ErrBridgeChecksumMissing
	}
	if marker > 0 && text[marker-1] != '\n' {
		return ErrBridgeMalformed
	}
	body := text[:marker]
	lineEnd := strings.IndexByte(text[marker:], '\n')
	line := text[marker:]
	trailing := ""
	if lineEnd >= 0 {
		line = text[marker : marker+lineEnd]
		trailing = text[marker+lineEnd+1:]
	}
	if strings.TrimSpace(trailing) != "" || !strings.HasPrefix(line, bridgeChecksumPrefix) || !strings.HasSuffix(line, bridgeChecksumSuffix) {
		return ErrBridgeMalformed
	}
	want := strings.TrimSuffix(strings.TrimPrefix(line, bridgeChecksumPrefix), bridgeChecksumSuffix)
	if want != "sha256:"+digestHex([]byte(body)) {
		return ErrBridgeChecksumMismatch
	}
	return nil
}

func appendManagedRegion(current, region []byte) []byte {
	if len(current) == 0 {
		return append([]byte{}, region...)
	}
	next := append([]byte{}, current...)
	if next[len(next)-1] != '\n' {
		next = append(next, '\n')
	}
	if len(next) < 2 || next[len(next)-2] != '\n' {
		next = append(next, '\n')
	}
	return append(next, region...)
}

func readAgentsFile(path string) ([]byte, os.FileMode, bool, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return []byte{}, 0, false, nil
	}
	if err != nil {
		return nil, 0, false, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, 0, false, fmt.Errorf("AGENTS.md is not a regular file")
	}
	content, err := os.ReadFile(path) // #nosec G304 -- path is the absolute AGENTS target after symlink rejection.
	return content, info.Mode().Perm(), true, err
}

func atomicReplace(path string, content []byte, mode os.FileMode) error {
	directory := filepath.Dir(path)
	temp, err := os.CreateTemp(directory, ".jacu-agents-*")
	if err != nil {
		return err
	}
	tempName := temp.Name()
	remove := true
	defer func() {
		_ = temp.Close()
		if remove {
			_ = os.Remove(tempName)
		}
	}()
	if chmodErr := temp.Chmod(mode.Perm()); chmodErr != nil {
		return chmodErr
	}
	if _, writeErr := temp.Write(content); writeErr != nil {
		return writeErr
	}
	if syncErr := temp.Sync(); syncErr != nil {
		return syncErr
	}
	if closeErr := temp.Close(); closeErr != nil {
		return closeErr
	}
	if renameErr := os.Rename(tempName, path); renameErr != nil {
		return renameErr
	}
	remove = false
	dir, err := os.Open(directory) // #nosec G304 -- directory is the parent of the absolute AGENTS target.
	if err != nil {
		return err
	}
	err = dir.Sync()
	closeErr := dir.Close()
	if err != nil {
		return err
	}
	return closeErr
}

func digestHex(content []byte) string {
	digest := sha256.Sum256(content)
	return hex.EncodeToString(digest[:])
}
