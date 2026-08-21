package workspace

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/jacu-dev/jacu-harness/internal/scope"
)

type protectedDocument struct {
	Paths []string `json:"paths"`
}

func LoadProtectedPaths(root string) ([]string, error) {
	path := filepath.Join(root, ".jacu", "protected.json")
	// #nosec G304 -- path is fixed under the project root's .jacu directory.
	content, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read protected.json: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	var document protectedDocument
	if err := decoder.Decode(&document); err != nil {
		return nil, fmt.Errorf("decode protected.json: %w", err)
	}
	return document.Paths, nil
}

func ProtectedPath(path string, protected []string) bool {
	return scope.MatchesAny(path, protected)
}
