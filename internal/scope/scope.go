// Package scope is the single write-scope verdict for SDD lint, workspace
// apply, and orchestration wave scheduling.
package scope

import (
	"path/filepath"
	"strings"
)

// ScopesConflict fails closed when a path is not explicitly allowed or is
// explicitly forbidden.
func ScopesConflict(path string, allowed, forbidden []string) bool {
	return !MatchesAny(path, allowed) || MatchesAny(path, forbidden)
}

// MatchesAny reports whether path matches any glob-style scope entry.
func MatchesAny(path string, scopes []string) bool {
	path = filepath.ToSlash(strings.TrimPrefix(filepath.Clean(path), "./"))
	for _, scope := range scopes {
		scope = filepath.ToSlash(strings.TrimSuffix(strings.TrimSpace(scope), "/"))
		if scope == "." || scope == "**" || path == scope || strings.HasPrefix(path, scope+"/") {
			return true
		}
		if strings.HasSuffix(scope, "**") && strings.HasPrefix(path, strings.TrimSuffix(scope, "**")) {
			return true
		}
	}
	return false
}

// ListsConflict reports whether two allowed-path lists cannot share a wave.
// Empty scope, a bare glob, or an unknown empty normalized path conflicts with
// every other scope; equal paths and directory prefixes conflict; siblings do
// not.
func ListsConflict(left, right []string) bool {
	if len(left) == 0 || len(right) == 0 {
		return true
	}
	for _, raw := range left {
		if normalizeScope(raw) == "" {
			return true
		}
	}
	for _, raw := range right {
		if normalizeScope(raw) == "" {
			return true
		}
	}
	for _, leftPath := range left {
		for _, rightPath := range right {
			leftPath, rightPath = normalizeScope(leftPath), normalizeScope(rightPath)
			if leftPath == rightPath || dirPrefix(leftPath, rightPath) || dirPrefix(rightPath, leftPath) {
				return true
			}
		}
	}
	return false
}

func normalizeScope(path string) string {
	path = strings.TrimSpace(path)
	if index := strings.IndexAny(path, "*?"); index >= 0 {
		path = path[:index]
	}
	return strings.TrimSuffix(path, "/")
}

func dirPrefix(prefix, path string) bool {
	return prefix != "" && len(path) > len(prefix) && strings.HasPrefix(path, prefix) && path[len(prefix)] == '/'
}
