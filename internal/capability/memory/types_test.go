package memory

import (
	"reflect"
	"testing"
)

func TestInputJSONContract(t *testing.T) {
	want := map[string]string{
		"ProjectID":  "project_id",
		"Kind":       "kind",
		"Title":      "title",
		"Body":       "body",
		"Evidence":   "evidence,omitempty",
		"Source":     "source",
		"Supersedes": "supersedes,omitempty",
	}
	assertJSONTags(t, reflect.TypeOf(Input{}), want)
}

func TestRecordJSONContract(t *testing.T) {
	want := map[string]string{
		"MemoryID":     "memory_id",
		"ProjectID":    "project_id",
		"Kind":         "kind",
		"Title":        "title",
		"Body":         "body",
		"Evidence":     "evidence",
		"Source":       "source",
		"Status":       "status",
		"SupersededBy": "superseded_by",
		"CreatedAt":    "created_at",
		"UpdatedAt":    "updated_at",
	}
	assertJSONTags(t, reflect.TypeOf(Record{}), want)
}

func TestNormalizeTrimsFieldsAndSortsUniqueEvidence(t *testing.T) {
	in := Input{
		ProjectID:  "  prj_demo  ",
		Kind:       "  decision  ",
		Title:      "  Use semantic commits  ",
		Body:       "  Explain the decision.  ",
		Evidence:   []string{" docs/z.md ", "docs/a.md", "docs/z.md"},
		Source:     "  derived  ",
		Supersedes: "  mem_0123456789abcdef  ",
	}
	want := Input{
		ProjectID:  "prj_demo",
		Kind:       "decision",
		Title:      "Use semantic commits",
		Body:       "Explain the decision.",
		Evidence:   []string{"docs/a.md", "docs/z.md"},
		Source:     "derived",
		Supersedes: "mem_0123456789abcdef",
	}

	if got := normalize(in); !reflect.DeepEqual(got, want) {
		t.Fatalf("normalize() = %#v; want %#v", got, want)
	}
}

func TestNormalizePreservesNilEvidence(t *testing.T) {
	got := normalize(Input{Evidence: nil})
	if got.Evidence != nil {
		t.Fatalf("normalize().Evidence = %#v; want nil", got.Evidence)
	}
}

func TestMemoryIDCanonicalVector(t *testing.T) {
	in := Input{ProjectID: "prj_demo", Kind: "decision", Title: "Use semantic commits"}
	if got, want := memoryID(in), "mem_a245ee0d189701f2"; got != want {
		t.Fatalf("memoryID() = %q; want %q", got, want)
	}
}

func TestMemoryIDNormalizesUnicodeWhitespaceAndCase(t *testing.T) {
	left := Input{ProjectID: "prj_demo", Kind: "decision", Title: "  USE\tSemantic\u2003Commits\n"}
	right := Input{ProjectID: "prj_demo", Kind: "decision", Title: "use semantic commits"}

	if leftID, rightID := memoryID(left), memoryID(right); leftID != rightID {
		t.Fatalf("memoryID() differs after title normalization: left=%q right=%q", leftID, rightID)
	}
}

func TestMemoryIDIgnoresBodyEvidenceAndSource(t *testing.T) {
	left := Input{
		ProjectID: "prj_demo", Kind: "decision", Title: "Use semantic commits",
		Body: "old body", Evidence: []string{"old.md"}, Source: "human",
	}
	right := Input{
		ProjectID: "prj_demo", Kind: "decision", Title: "Use semantic commits",
		Body: "new body", Evidence: []string{"new.md"}, Source: "derived",
	}

	if leftID, rightID := memoryID(left), memoryID(right); leftID != rightID {
		t.Fatalf("memoryID() includes mutable fields: left=%q right=%q", leftID, rightID)
	}
}

func TestMemoryIDChangesWithIdentityFields(t *testing.T) {
	base := Input{ProjectID: "prj_demo", Kind: "decision", Title: "Use semantic commits"}
	tests := []struct {
		name string
		in   Input
	}{
		{name: "project", in: Input{ProjectID: "prj_other", Kind: base.Kind, Title: base.Title}},
		{name: "kind", in: Input{ProjectID: base.ProjectID, Kind: "convention", Title: base.Title}},
		{name: "title", in: Input{ProjectID: base.ProjectID, Kind: base.Kind, Title: "Use signed commits"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got, original := memoryID(tt.in), memoryID(base); got == original {
				t.Fatalf("memoryID() = %q for changed %s; want different from %q", got, tt.name, original)
			}
		})
	}
}

func assertJSONTags(t *testing.T, typ reflect.Type, want map[string]string) {
	t.Helper()
	if typ.NumField() != len(want) {
		t.Fatalf("%s has %d fields; want %d", typ.Name(), typ.NumField(), len(want))
	}
	for fieldName, wantTag := range want {
		field, ok := typ.FieldByName(fieldName)
		if !ok {
			t.Errorf("%s is missing field %s", typ.Name(), fieldName)
			continue
		}
		if got := field.Tag.Get("json"); got != wantTag {
			t.Errorf("%s.%s json tag = %q; want %q", typ.Name(), fieldName, got, wantTag)
		}
	}
}
