package scope

import "testing"

func TestScopesConflictFailsClosed(t *testing.T) {
	allowed := []string{"internal/capability/sdd/**"}
	forbidden := []string{"internal/capability/sdd/private/**"}
	cases := []struct {
		path string
		want bool
	}{
		{path: "internal/capability/sdd/parse.go", want: false},
		{path: "internal/capability/sdd/private/key.go", want: true},
		{path: "README.md", want: true},
	}
	for _, testCase := range cases {
		if got := ScopesConflict(testCase.path, allowed, forbidden); got != testCase.want {
			t.Fatalf("ScopesConflict(%q) = %v; want %v", testCase.path, got, testCase.want)
		}
	}
}

func TestListsConflictMatchesWaveContract(t *testing.T) {
	if !ListsConflict(nil, []string{"src"}) {
		t.Fatal("empty left must conflict")
	}
	if ListsConflict([]string{"src/a"}, []string{"src/b"}) {
		t.Fatal("sibling prefixes must not conflict")
	}
	if !ListsConflict([]string{"src"}, []string{"src/a"}) {
		t.Fatal("directory prefix must conflict")
	}
}
