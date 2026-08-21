package verify

import (
	"context"
	"strings"
	"testing"
)

func TestRunBlocksOutsideAGitWorkTreeWithBlockedVerdict(t *testing.T) {
	root := t.TempDir()
	result := Run(context.Background(), root, Input{RunID: "run_0000000000000000"})
	if result.Status != "blocked" {
		t.Fatalf("status = %q; want blocked", result.Status)
	}
	data, ok := result.Data.(Data)
	if !ok || data.Verdict != VerdictBlocked {
		t.Fatalf("data = %#v; want verdict %q", result.Data, VerdictBlocked)
	}
	if !strings.Contains(result.Summary, root) || !strings.Contains(strings.ToLower(result.Summary), "git") {
		t.Fatalf("summary %q must name the cwd and instruct git anchoring", result.Summary)
	}
}
