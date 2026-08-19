package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/jacu-dev/jacu-harness/internal/provenance"
)

func TestRunProvenanceUnknownOption(t *testing.T) {
	var stderr bytes.Buffer
	code := runProvenance(t.TempDir(), []string{"--nope"}, &bytes.Buffer{}, &stderr)
	if code != 2 {
		t.Fatalf("unknown option exit = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "unknown option") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestMergeReportsAddsCounts(t *testing.T) {
	left := provenance.Report{Traces: 1, Products: 2, Policies: 3, Gaps: []provenance.Gap{{Check: "a"}}}
	right := provenance.Report{Traces: 4, Products: 5, Policies: 6, Gaps: []provenance.Gap{{Check: "b"}}}
	got := mergeReports(left, right)
	if got.Traces != 5 || got.Products != 7 || got.Policies != 9 || len(got.Gaps) != 2 {
		t.Fatalf("mergeReports = %+v", got)
	}
}
