package sdd

import (
	"testing"

	"github.com/jacu-dev/jacu-harness/internal/capability/workspace"
)

func TestLintAndWorkspaceUseTheSameScopeVerdict(t *testing.T) {
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
		workspaceVerdict := workspace.ScopesConflict(testCase.path, allowed, forbidden)
		if lintVerdict != testCase.want || workspaceVerdict != testCase.want || lintVerdict != workspaceVerdict {
			t.Fatalf("path %q: lint=%v workspace=%v want=%v", testCase.path, lintVerdict, workspaceVerdict, testCase.want)
		}
	}
}
