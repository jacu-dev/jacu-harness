package workspace

import (
	"go/parser"
	"go/token"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
)

func workspaceDirectInternalImports() ([]string, error) {
	_, source, _, _ := runtime.Caller(0)
	directory := filepath.Dir(source)
	entries, err := filepath.Glob(filepath.Join(directory, "*.go"))
	if err != nil {
		return nil, err
	}
	imports := map[string]struct{}{}
	for _, file := range entries {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		tree, parseErr := parser.ParseFile(token.NewFileSet(), file, nil, 0)
		if parseErr != nil {
			return nil, parseErr
		}
		for _, spec := range tree.Imports {
			path, unquoteErr := strconv.Unquote(spec.Path.Value)
			if unquoteErr == nil && strings.HasPrefix(path, "github.com/jacu-dev/jacu-harness/internal/") {
				imports[path] = struct{}{}
			}
		}
	}
	result := make([]string, 0, len(imports))
	for path := range imports {
		result = append(result, path)
	}
	sort.Strings(result)
	return result, nil
}
