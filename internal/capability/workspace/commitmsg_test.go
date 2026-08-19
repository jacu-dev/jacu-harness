package workspace

import (
	"bytes"
	"context"
	"github.com/jacu-dev/jacu-harness/internal/testgit"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/jacu-dev/jacu-harness/internal/capability/missioncompile"
	"github.com/jacu-dev/jacu-harness/internal/runstate"
)

func TestBuildCommitMessageIsDeterministicAndHasParseableTrailers(t *testing.T) {
	run := runstate.Run{
		RunID:   "run_000000000000000d",
		BaseSHA: "abc123",
		Mission: missioncompile.Mission{
			MissionID:          "msn_456",
			Objective:          "Update README fixture",
			AcceptanceCriteria: []string{"README fixture updated", "Tests pass"},
		},
	}
	want := "Update README fixture\n\n" +
		"Acceptance criteria:\n" +
		"- README fixture updated\n" +
		"- Tests pass\n\n" +
		"Verified: go test ./... (exit 0)\n\n" +
		"Jacu-Run: run_000000000000000d\n" +
		"Jacu-Mission: msn_456\n" +
		"Jacu-Base: abc123\n" +
		"Assisted-by: Claude Code\n"

	got := buildCommitMessage(run, "Claude Code", "Verified: go test ./... (exit 0)")
	if got != want {
		t.Fatalf("message differs byte for byte:\n--- got ---\n%s--- want ---\n%s", got, want)
	}

	trailers := interpretTrailers(t, got)
	wantTrailers := "Jacu-Run: run_000000000000000d\n" +
		"Jacu-Mission: msn_456\n" +
		"Jacu-Base: abc123\n" +
		"Assisted-by: Claude Code\n"
	if trailers != wantTrailers {
		t.Fatalf("trailers = %q; want %q", trailers, wantTrailers)
	}
}

func TestBuildCommitMessageUsesUnknownHostFallback(t *testing.T) {
	run := runstate.Run{
		RunID:   "run_000000000000000d",
		BaseSHA: "abc123",
		Mission: missioncompile.Mission{MissionID: "msn_456", Objective: "Update README fixture"},
	}

	message := buildCommitMessage(run, "", "Verified: go test ./... (exit 0)")
	if !strings.Contains(message, "Assisted-by: unknown host\n") {
		t.Fatalf("message = %q", message)
	}
}

func interpretTrailers(t *testing.T, message string) string {
	t.Helper()
	repo := newEmptyTestRepo(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "interpret-trailers", "--parse")
	cmd.Dir = repo
	cmd.Env = testgit.Env()
	cmd.Stdin = strings.NewReader(message)
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err := cmd.Run(); err != nil {
		t.Fatalf("git interpret-trailers --parse: %v\n%s", err, output.String())
	}
	return output.String()
}
