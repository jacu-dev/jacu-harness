package context

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jacu-dev/jacu-harness/internal/scope"
)

const DefaultBudget int64 = 256 * 1024

type Spec struct {
	Objective      string
	Acceptance     []string
	AllowedPaths   []string
	ForbiddenPaths []string
	RequiredPaths  []string
	Verification   [][]string
	BudgetBytes    int64
}

type Item struct {
	ID       string `json:"id"`
	Path     string `json:"path"`
	Bytes    int64  `json:"bytes"`
	Required bool   `json:"required"`
	SHA256   string `json:"sha256"`
}

type Pack struct {
	Items   []Item   `json:"items"`
	Digest  string   `json:"digest"`
	Bytes   int64    `json:"bytes"`
	Anchors []string `json:"anchors"`
}

type Finding struct {
	Code string `json:"code"`
}

func (err Finding) Error() string { return err.Code }

func PackRoot(root string, spec Spec) (Pack, error) {
	items := make([]Item, 0)
	items = append(items, synthetic("mission/objective", spec.Objective, true))
	for i, criterion := range spec.Acceptance {
		items = append(items, synthetic("mission/acceptance/"+itoa(i), criterion, true))
	}
	files, err := collectFiles(root, spec)
	if err != nil {
		return Pack{}, Finding{Code: "pack_unreadable"}
	}
	requiredSet := toSet(spec.RequiredPaths)
	requireAll := len(requiredSet) == 0
	for _, file := range files {
		item := file
		item.Required = requireAll || requiredSet[file.Path]
		items = append(items, item)
	}
	for _, required := range spec.RequiredPaths {
		if containsPath(items, required) {
			continue
		}
		items = append(items, Item{ID: "path:" + required, Path: required, Required: true, SHA256: sha256Hex(nil)})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Required != items[j].Required {
			return items[i].Required
		}
		return items[i].Path < items[j].Path
	})
	total := int64(0)
	for _, item := range items {
		total += item.Bytes
	}
	pack := Pack{Items: items, Bytes: total, Anchors: Anchors(spec)}
	pack.Digest = Digest(pack)
	return pack, nil
}

func collectFiles(root string, spec Spec) ([]Item, error) {
	if len(spec.AllowedPaths) == 0 {
		return nil, nil
	}
	collected := make([]Item, 0)
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if rel == "." || strings.HasPrefix(rel, ".git/") {
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		if scope.ScopesConflict(rel, spec.AllowedPaths, spec.ForbiddenPaths) {
			return nil
		}
		raw, err := os.ReadFile(path) // #nosec G304,G122 -- path is walked from the admitted root under the spec write-scope
		if err != nil {
			return err
		}
		collected = append(collected, Item{
			ID:     "file:" + rel,
			Path:   rel,
			Bytes:  int64(len(raw)),
			SHA256: sha256Hex(raw),
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(collected, func(i, j int) bool { return collected[i].Path < collected[j].Path })
	return collected, nil
}

func synthetic(path, content string, required bool) Item {
	raw := []byte(content)
	return Item{ID: "anchor:" + path, Path: path, Bytes: int64(len(raw)), Required: required, SHA256: sha256Hex(raw)}
}

func sha256Hex(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func toSet(values []string) map[string]bool {
	out := make(map[string]bool, len(values))
	for _, value := range values {
		if value != "" {
			out[filepath.ToSlash(value)] = true
		}
	}
	return out
}

func containsPath(items []Item, path string) bool {
	path = filepath.ToSlash(path)
	for _, item := range items {
		if item.Path == path {
			return true
		}
	}
	return false
}

func itoa(value int) string {
	if value == 0 {
		return "0"
	}
	digits := make([]byte, 0, 8)
	for value > 0 {
		digits = append([]byte{byte('0' + value%10)}, digits...)
		value /= 10
	}
	return string(digits)
}

func Canonical(pack Pack) []byte {
	rows := make([]Item, len(pack.Items))
	copy(rows, pack.Items)
	sort.Slice(rows, func(i, j int) bool { return rows[i].Path < rows[j].Path })
	encoded, _ := json.Marshal(rows)
	return encoded
}
