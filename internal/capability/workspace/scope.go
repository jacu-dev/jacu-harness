package workspace

import (
	"path/filepath"
	"strings"
)

// ScopesConflict is the single write-scope verdict shared by workspace gates
// and SDD lint. It fails closed when a path is not explicitly allowed or is
// explicitly forbidden.
func ScopesConflict(path string, allowed, forbidden []string) bool {
	return !scopeMatchesAny(path, allowed) || scopeMatchesAny(path, forbidden)
}

func scopeMatchesAny(path string, scopes []string) bool {
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
