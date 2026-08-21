package orchestration

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var allowedOrchestrationFiles = map[string]struct{}{
	"engine.go": {},
	"fanin.go":  {},
	"graph.go":  {},
	"panel.go":  {},
	"run.go":    {},
	"tool.go":   {},
}

var allowedOrchestrationImports = map[string]struct{}{
	"github.com/google/jsonschema-go/jsonschema":                          {},
	"github.com/jacu-dev/jacu-harness/internal/capability/missioncompile": {},
	"github.com/jacu-dev/jacu-harness/internal/capability/verify":         {},
	"github.com/jacu-dev/jacu-harness/internal/capability/workspace":      {},
	"github.com/jacu-dev/jacu-harness/internal/gitx":                      {},
	"github.com/jacu-dev/jacu-harness/internal/runtime":                   {},
	"github.com/jacu-dev/jacu-harness/internal/scope":                     {},
	"github.com/jacu-dev/jacu-harness/internal/telemetry":                 {},
	"github.com/modelcontextprotocol/go-sdk/mcp":                          {},
}

func TestOrchestrationBoundaryRejectsExtraFilesAndImports(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		if _, ok := allowedOrchestrationFiles[name]; !ok {
			t.Errorf("orchestration grew extra file %s; sequencing-only boundary", name)
		}
		imports, parseErr := fileImports(name)
		if parseErr != nil {
			t.Fatalf("parse %s: %v", name, parseErr)
		}
		for _, path := range imports {
			if !strings.Contains(path, ".") {
				continue
			}
			if _, ok := allowedOrchestrationImports[path]; !ok {
				t.Errorf("%s imports %s; orchestration may only sequence existing capabilities", name, path)
			}
		}
	}
}

func TestOrchestrationBoundarySeededExtraFileIsRefused(t *testing.T) {
	seed := filepath.Join(t.TempDir(), "policy.go")
	if err := os.WriteFile(seed, []byte("package orchestration\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, ok := allowedOrchestrationFiles[filepath.Base(seed)]; ok {
		t.Fatal("seeded extra file was treated as allowed")
	}
}

func fileImports(name string) ([]string, error) {
	file, err := parser.ParseFile(token.NewFileSet(), name, nil, parser.ImportsOnly)
	if err != nil {
		return nil, err
	}
	paths := make([]string, 0, len(file.Imports))
	for _, spec := range file.Imports {
		paths = append(paths, strings.Trim(spec.Path.Value, `"`))
	}
	return paths, nil
}
