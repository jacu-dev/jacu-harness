package projectinspect

import (
	"context"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jacu-dev/jacu-harness/internal/project"
)

type Input struct {
	Include  []string `json:"include,omitempty"`
	MaxFiles int      `json:"max_files,omitempty"`
}

type Summary struct {
	ProjectID    string   `json:"project_id"`
	Name         string   `json:"name"`
	Languages    []string `json:"languages"`
	Manifests    []string `json:"manifests"`
	TestCommands []string `json:"test_commands"`
	FileCount    int      `json:"file_count"`
	Truncated    bool     `json:"truncated"`
}

func Scan(ctx context.Context, root string, in Input) (Summary, []string, error) {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return Summary{}, nil, err
	}
	rootAbs, err = filepath.EvalSymlinks(rootAbs)
	if err != nil {
		return Summary{}, nil, err
	}

	projectID, err := project.ID(rootAbs)
	if err != nil {
		return Summary{}, nil, err
	}
	summary := Summary{
		ProjectID:    projectID,
		Name:         filepath.Base(rootAbs),
		Languages:    []string{},
		Manifests:    []string{},
		TestCommands: []string{},
	}
	languages := make(map[string]bool)
	manifests := make(map[string]bool)
	testCommands := make(map[string]bool)
	warnings := []string{}
	sensitiveFiles := []string{}
	maxFiles := in.MaxFiles
	if maxFiles <= 0 {
		maxFiles = 2000
	}
	if maxFiles > 10000 {
		maxFiles = 10000
	}

	err = filepath.WalkDir(rootAbs, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if entry.IsDir() && entry.Name() == ".git" && path != rootAbs {
			warnings = append(warnings, "git directory present (not scanned)")
			return fs.SkipDir
		}
		if entry.IsDir() {
			return nil
		}
		if summary.FileCount >= maxFiles {
			summary.Truncated = true
			warnings = append(warnings, "file limit reached")
			return fs.SkipAll
		}
		summary.FileCount++
		if entry.Type()&os.ModeSymlink != 0 {
			return nil
		}
		if isSensitiveName(entry.Name()) {
			relative, err := filepath.Rel(rootAbs, path)
			if err != nil {
				return err
			}
			sensitiveFiles = append(sensitiveFiles, filepath.ToSlash(relative))
			return nil
		}
		if filepath.Ext(entry.Name()) == ".go" {
			languages["go"] = true
		}
		if ext := filepath.Ext(entry.Name()); ext == ".ts" || ext == ".tsx" {
			languages["typescript"] = true
		}
		if entry.Name() == "go.mod" {
			languages["go"] = true
			manifests["go.mod"] = true
			testCommands["go test ./..."] = true
		}
		if entry.Name() == "package.json" {
			manifests["package.json"] = true
			info, err := entry.Info()
			if err != nil {
				return err
			}
			if info.Size() > 256*1024 {
				warnings = append(warnings, "package.json exceeds 256KB (not read)")
				return nil
			}
			content, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			var manifest struct {
				Scripts map[string]string `json:"scripts"`
			}
			if err := json.Unmarshal(content, &manifest); err != nil {
				warnings = append(warnings, "malformed package.json")
			} else if _, ok := manifest.Scripts["test"]; ok {
				testCommands["npm test"] = true
			}
		}
		return nil
	})
	if err != nil {
		return Summary{}, nil, err
	}
	if len(sensitiveFiles) > 0 {
		sort.Strings(sensitiveFiles)
		warnings = append(warnings, "sensitive files present (not read): "+strings.Join(sensitiveFiles, ", "))
	}

	summary.Languages = mapKeys(languages)
	summary.Manifests = mapKeys(manifests)
	summary.TestCommands = mapKeys(testCommands)
	return summary, warnings, nil
}

func isSensitiveName(name string) bool {
	return strings.HasPrefix(name, ".env") ||
		strings.HasPrefix(name, "id_rsa") ||
		strings.HasSuffix(name, ".pem")
}

func mapKeys(values map[string]bool) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
