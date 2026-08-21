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

func TestRunProvenanceCommitPlanJSON(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := runProvenance(t.TempDir(), []string{"--commit-plan", "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("commit-plan exit = %d stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "chore: import the sanitized public tree") {
		t.Fatalf("stdout missing commit plan: %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "ecouto123@gmail.com") {
		t.Fatalf("stdout missing author: %s", stdout.String())
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
