package memory

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestSearchRanksDeterministicallyAndBreaksTiesByMemoryID(t *testing.T) {
	store := NewFileStore(t.TempDir())
	records := []Record{
		testRecord("mem_0000000000000003", testProjectID, "decision", "Deploy safety", "unrelated"),
		testRecord("mem_0000000000000001", testProjectID, "decision", "Deploy safety", "unrelated"),
		testRecord("mem_0000000000000002", testProjectID, "decision", "Other", "deploy safety"),
	}
	saveRecords(t, store, records...)

	got := store.Search(SearchQuery{ProjectID: testProjectID, Query: "deploy safety", K: 10})
	want := []Scored{{Record: records[1], Score: 6}, {Record: records[0], Score: 6}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Search = %#v; want %#v", got, want)
	}
	gotAgain := store.Search(SearchQuery{ProjectID: testProjectID, Query: "deploy safety", K: 10})
	if !reflect.DeepEqual(gotAgain, want) {
		t.Fatalf("repeated Search = %#v; want %#v", gotAgain, want)
	}
}

func TestSearchUsesWeightedUniqueUnicodeTokensAndMinimumScore(t *testing.T) {
	store := NewFileStore(t.TempDir())
	title := testRecord("mem_0000000000000010", testProjectID, "decision", "REVISÃO revisão", "none")
	bodyEvidenceKind := testRecord("mem_0000000000000011", testProjectID, "gotcha", "Other", "revisão revisão")
	bodyEvidenceKind.Evidence = []string{"revisão", "revisão again"}
	belowMinimum := testRecord("mem_0000000000000012", testProjectID, "decision", "Other", "revisão revisão")
	saveRecords(t, store, title, bodyEvidenceKind, belowMinimum)

	got := store.Search(SearchQuery{ProjectID: testProjectID, Query: "REVISÃO revisão gotcha", K: 10})
	want := []Scored{{Record: bodyEvidenceKind, Score: 4}, {Record: title, Score: 3}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("weighted Search = %#v; want %#v", got, want)
	}

	if got := store.Search(SearchQuery{ProjectID: testProjectID, Query: "gotcha", K: 10}); len(got) != 0 {
		t.Fatalf("kind-only score below minScore returned %#v; want empty", got)
	}
}

func TestSearchEmptyQueryListsWithScopeKindsStatusAndK(t *testing.T) {
	root := t.TempDir()
	store := NewFileStore(root)
	activeDecision := testRecord("mem_0000000000000020", testProjectID, "decision", "Decision", "body")
	activeGotcha := testRecord("mem_0000000000000010", testProjectID, "gotcha", "Gotcha", "body")
	superseded := testRecord("mem_0000000000000005", testProjectID, "decision", "Old", "body")
	superseded.Status = "superseded"
	global := testRecord("mem_0000000000000001", "", "preference", "Global", "body")
	otherProject := testRecord("mem_0000000000000002", "prj_fedcba9876543210", "decision", "Other project", "body")
	saveRecords(t, store, activeDecision, activeGotcha, superseded, global, otherProject)

	got := store.Search(SearchQuery{ProjectID: testProjectID, Kinds: []string{"decision"}, K: 10})
	want := []Scored{{Record: activeDecision, Score: 0}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("filtered list = %#v; want %#v", got, want)
	}

	got = store.Search(SearchQuery{ProjectID: testProjectID, IncludeSuperseded: true, K: 2})
	want = []Scored{{Record: superseded, Score: 0}, {Record: activeGotcha, Score: 0}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("inclusive top-k list = %#v; want %#v", got, want)
	}

	got = store.Search(SearchQuery{ProjectID: "", K: 10})
	want = []Scored{{Record: global, Score: 0}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("global list = %#v; want %#v", got, want)
	}
}

func TestSearchKLessThanOrEqualToZeroReturnsEmpty(t *testing.T) {
	store := NewFileStore(t.TempDir())
	saveRecords(t, store, testRecord("mem_0000000000000030", testProjectID, "decision", "Deploy", "body"))
	for _, k := range []int{0, -1} {
		if got := store.Search(SearchQuery{ProjectID: testProjectID, Query: "deploy", K: k}); len(got) != 0 {
			t.Fatalf("Search(K=%d) = %#v; want empty bounded result", k, got)
		}
	}
}

func TestSearchRejectsInvalidProjectAndIgnoresCorruptRecords(t *testing.T) {
	root := t.TempDir()
	store := NewFileStore(root)
	valid := testRecord("mem_0000000000000040", testProjectID, "decision", "Deploy safely", "body")
	saveRecords(t, store, valid)
	dir := filepath.Join(root, "memory", testProjectID)
	if err := os.WriteFile(filepath.Join(dir, "mem_0000000000000041.json"), []byte("{"), 0o600); err != nil {
		t.Fatalf("WriteFile corrupt: %v", err)
	}

	got := store.Search(SearchQuery{ProjectID: testProjectID, Query: "deploy", K: 10})
	want := []Scored{{Record: valid, Score: 3}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Search with corrupt file = %#v; want %#v", got, want)
	}
	for _, projectID := range []string{"../" + testProjectID, "prj_ABCDEFABCDEFABCD", "project"} {
		if got := store.Search(SearchQuery{ProjectID: projectID, Query: "deploy", K: 10}); len(got) != 0 {
			t.Fatalf("Search invalid project %q = %#v; want empty", projectID, got)
		}
	}
}

func saveRecords(t *testing.T, store Store, records ...Record) {
	t.Helper()
	for _, rec := range records {
		if err := store.Save(rec, ""); err != nil {
			t.Fatalf("Save(%s): %v", rec.MemoryID, err)
		}
	}
}
