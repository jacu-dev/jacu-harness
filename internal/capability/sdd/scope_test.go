package sdd

import (
	"testing"

	"github.com/jacu-dev/jacu-harness/internal/scope"
)

func TestLintAndScopeUseTheSameVerdict(t *testing.T) {
	allowed := []string{"internal/capability/sdd/**"}
	forbidden := []string{"internal/capability/sdd/private/**"}
	for _, testCase := range []struct {
		path string
		want bool
	}{
		{path: "internal/capability/sdd/parse.go", want: false},
		{path: "internal/capability/sdd/private/key.go", want: true},
		{path: "README.md", want: true},
	} {
		document := Document{Sections: []Section{{Name: "Write scope", Lines: []string{"**Allowed**", allowed[0], "**Forbidden**", forbidden[0]}}}}
		lintVerdict := findCode(lintScope(document, []string{testCase.path}), "sdd_out_of_scope_touched") != nil
		scopeVerdict := scope.ScopesConflict(testCase.path, allowed, forbidden)
		if lintVerdict != testCase.want || scopeVerdict != testCase.want || lintVerdict != scopeVerdict {
			t.Fatalf("path %q: lint=%v scope=%v want=%v", testCase.path, lintVerdict, scopeVerdict, testCase.want)
		}
	}
}
