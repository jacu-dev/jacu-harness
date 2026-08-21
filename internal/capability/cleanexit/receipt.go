package cleanexit

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/jacu-dev/jacu-harness/internal/gitx"
)

type Receipt struct {
	Verdict string   `json:"verdict"`
	Classes []string `json:"classes"`
	Removed []string `json:"removed"`
}

func NewReceipt(result RemovalReport) Receipt {
	classes := make([]string, 0, len(result.Findings))
	seen := map[string]struct{}{}
	for _, finding := range result.Findings {
		if _, ok := seen[finding.Class]; !ok {
			seen[finding.Class] = struct{}{}
			classes = append(classes, finding.Class)
		}
	}
	sort.Strings(classes)
	removed := append([]string{}, result.Removed...)
	sort.Strings(removed)
	return Receipt{Verdict: result.Verdict, Classes: classes, Removed: removed}
}

func (receipt Receipt) Validate() error {
	if receipt.Verdict != "pass" && receipt.Verdict != "fail" {
		return fmt.Errorf("cleanexit receipt verdict is invalid: %q", receipt.Verdict)
	}
	if receipt.Classes == nil || receipt.Removed == nil {
		return fmt.Errorf("cleanexit receipt arrays must be non-nil")
	}
	for _, class := range receipt.Classes {
		if _, ok := failureClasses[class]; !ok {
			return fmt.Errorf("cleanexit receipt class is invalid: %q", class)
		}
	}
	for _, path := range receipt.Removed {
		if path == "" || len(path) > 4096 {
			return fmt.Errorf("cleanexit receipt removed path is invalid")
		}
	}
	return nil
}

// WriteReceipt persists the typed close result as a local, non-secret audit
// artifact. The file is kept below .git so it cannot become a project change.
func WriteReceipt(root string, result RemovalReport) (string, error) {
	receipt := NewReceipt(result)
	if err := receipt.Validate(); err != nil {
		return "", err
	}
	gitDir, err := resolveGitDir(root)
	if err != nil {
		return "", err
	}
	directory := filepath.Join(gitDir, "jacu", "clean-exit")
	if mkdirErr := os.MkdirAll(directory, 0o700); mkdirErr != nil {
		return "", fmt.Errorf("create cleanexit receipt directory: %w", mkdirErr)
	}
	content, err := json.MarshalIndent(receipt, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal cleanexit receipt: %w", err)
	}
	content = append(content, '\n')
	path := filepath.Join(directory, "latest.json")
	temporary, err := os.CreateTemp(directory, ".latest-*.json")
	if err != nil {
		return "", fmt.Errorf("create cleanexit receipt: %w", err)
	}
	temporaryName := temporary.Name()
	defer func() { _ = os.Remove(temporaryName) }()
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return "", fmt.Errorf("protect cleanexit receipt: %w", err)
	}
	if _, err := temporary.Write(content); err != nil {
		_ = temporary.Close()
		return "", fmt.Errorf("write cleanexit receipt: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return "", fmt.Errorf("close cleanexit receipt: %w", err)
	}
	if err := os.Rename(temporaryName, path); err != nil {
		return "", fmt.Errorf("publish cleanexit receipt: %w", err)
	}
	return path, nil
}

func resolveGitDir(root string) (string, error) {
	git, err := gitx.New()
	if err != nil {
		return "", fmt.Errorf("cleanexit git directory unavailable")
	}
	gitDir, err := git.RevParseGitDir(context.Background(), root)
	if err != nil || gitDir == "" {
		return "", fmt.Errorf("cleanexit git directory unavailable")
	}
	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(root, gitDir)
	}
	info, err := os.Stat(gitDir)
	if err != nil || !info.IsDir() {
		return "", fmt.Errorf("cleanexit git directory unavailable")
	}
	return filepath.Clean(gitDir), nil
}
