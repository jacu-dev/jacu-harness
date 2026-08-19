package workspace

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/jacu-dev/jacu-harness/internal/testgit"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jacu-dev/jacu-harness/internal/project"
	"github.com/jacu-dev/jacu-harness/internal/runstate"
	capabilityruntime "github.com/jacu-dev/jacu-harness/internal/runtime"
)

const (
	fuzzOpenRunID      = "run_0000000000000001"
	fuzzReviewedRunID  = "run_0000000000000002"
	fuzzTraversalRunID = "../../../../HEAD"
)

type applyFuzzGitState struct {
	symbolicRef string
	headSHA     string
	refSHA      string
}

func FuzzApplyInput(f *testing.F) {
	repo, wantGitState, wantRunStates := newApplyFuzzFixture(f)
	capability := workspaceApplyCapability(repo, "fuzz-host")
	seeds := [][]byte{
		[]byte(`{`),
		[]byte(`{}`),
		[]byte(`null`),
		[]byte(`{"run_id":"` + fuzzOpenRunID + `"}`),
		[]byte(`{"run_id":"` + fuzzReviewedRunID + `"}`),
		[]byte(`{"approve_destructive":true}`),
		[]byte(`{"run_id":"` + fuzzOpenRunID + `","approve_destructive":true}`),
		[]byte(`{"run_id":"` + fuzzTraversalRunID + `","approve_destructive":true}`),
		[]byte("{\"run_id\":\"hostile\\u0000修正🔥\"}"),
		[]byte(`{"run_id":"` + fuzzOpenRunID + `","unknown":{"nested":[null,true,{},[1,2,3]]}}`),
		applyFuzzBoundarySeed(f, fuzzOpenRunID, capability.Spec.MaxInputBytes-1),
		applyFuzzBoundarySeed(f, fuzzOpenRunID, capability.Spec.MaxInputBytes),
		applyFuzzBoundarySeed(f, fuzzOpenRunID, capability.Spec.MaxInputBytes+1),
	}
	for _, seed := range seeds {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, raw []byte) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if int64(len(raw)) > capability.Spec.MaxInputBytes {
			result := capabilityruntime.Execute(ctx, capability, json.RawMessage(raw))
			assertFuzzApplyRefused(t, "oversized runtime", result, nil)
			if result.Status != "blocked" || result.Summary != "input exceeds tool limit" {
				t.Fatalf("oversized runtime result = %#v; want exact input-limit refusal", result)
			}
		} else {
			handlerResult, handlerErr := capability.Handler(ctx, json.RawMessage(raw))
			assertFuzzApplyRefused(t, "direct handler", handlerResult, handlerErr)

			runtimeResult := capabilityruntime.Execute(ctx, capability, json.RawMessage(raw))
			assertFuzzApplyRefused(t, "runtime", runtimeResult, nil)
			if runtimeResult.Status == "blocked" && runtimeResult.Summary == "input exceeds tool limit" {
				t.Fatalf("runtime refused %d-byte input at or below %d-byte cap", len(raw), capability.Spec.MaxInputBytes)
			}
			if wantSummary, knownRun := fuzzExpectedFixtureRefusal(raw); knownRun {
				assertFuzzExactRefusal(t, "direct handler", handlerResult, handlerErr, wantSummary)
				assertFuzzExactRefusal(t, "runtime", runtimeResult, nil, wantSummary)
			}
		}
		if ctxErr := ctx.Err(); ctxErr != nil {
			t.Fatalf("apply fuzz iteration exhausted 10s budget: %v", ctxErr)
		}

		if _, knownRun := fuzzExpectedFixtureRefusal(raw); knownRun {
			assertApplyFuzzGitStateUnchanged(t, repo, wantGitState)
		}
		for runID, wantStatus := range map[string]runstate.Status{
			fuzzOpenRunID:     runstate.StatusOpen,
			fuzzReviewedRunID: runstate.StatusReviewed,
		} {
			run, loadErr := runstate.Load(repo, runID)
			if loadErr != nil {
				t.Fatalf("load run %q after apply: %v", runID, loadErr)
			}
			if run.Status != wantStatus || run.AppliedCommit != "" {
				t.Fatalf("run %q changed: status=%q commit=%q; want status=%q and no commit", runID, run.Status, run.AppliedCommit, wantStatus)
			}
			assertFuzzGitFileUnchanged(t, fuzzRunStatePath(repo, runID), wantRunStates[runID])
		}
	})
}

func newApplyFuzzFixture(f *testing.F) (repo string, gitState applyFuzzGitState, runStates map[string][]byte) {
	f.Helper()
	repo = f.TempDir()
	home := f.TempDir()
	f.Setenv("HOME", home)
	runApplyFuzzGit(f, repo, "-c", "init.defaultRefFormat=reftable", "init")
	runApplyFuzzGit(f, repo, "config", "user.name", "Jacu Fuzz")
	runApplyFuzzGit(f, repo, "config", "user.email", "jacu-fuzz@example.invalid")
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("fixture\n"), 0o600); err != nil {
		f.Fatalf("write README fixture: %v", err)
	}
	runApplyFuzzGit(f, repo, "add", "README.md")
	runApplyFuzzGit(f, repo, "commit", "-m", "initial commit")

	gitState.symbolicRef = runApplyFuzzGit(f, repo, "symbolic-ref", "HEAD")
	gitState.headSHA = runApplyFuzzGit(f, repo, "rev-parse", "HEAD")
	gitState.refSHA = runApplyFuzzGit(f, repo, "rev-parse", gitState.symbolicRef)
	if gitState.headSHA != gitState.refSHA {
		f.Fatalf("fixture HEAD SHA %q differs from ref %q SHA %q", gitState.headSHA, gitState.symbolicRef, gitState.refSHA)
	}
	if !strings.HasPrefix(gitState.symbolicRef, "refs/heads/") {
		f.Fatalf("fixture HEAD is not a branch ref: %q", gitState.symbolicRef)
	}
	projectID, err := project.ID(repo)
	if err != nil {
		f.Fatalf("derive project ID for apply fixture: %v", err)
	}
	createdAt := time.Unix(1_700_000_000, 0).UTC()
	for _, fixture := range []struct {
		runID          string
		status         runstate.Status
		reviewedDigest string
	}{
		{runID: fuzzOpenRunID, status: runstate.StatusOpen},
		{runID: fuzzReviewedRunID, status: runstate.StatusReviewed, reviewedDigest: "sha256:deliberate-mismatch"},
	} {
		suffix := strings.TrimPrefix(fixture.runID, "run_")
		branch := "jacu/run-" + suffix
		worktree := filepath.Join(home, ".jacu-harness", "worktrees", projectID, fixture.runID)
		runApplyFuzzGit(f, repo, "worktree", "add", "-b", branch, worktree, gitState.headSHA)
		run := runstate.Run{
			RunID:          fixture.runID,
			Status:         fixture.status,
			CreatedAt:      createdAt,
			BaseSHA:        gitState.headSHA,
			Branch:         branch,
			Worktree:       worktree,
			ReviewedDigest: fixture.reviewedDigest,
		}
		if err := runstate.Save(repo, run); err != nil {
			f.Fatalf("save run %q fixture: %v", run.RunID, err)
		}
	}
	plantApplyTraversalTarget(f, repo)
	runStates = make(map[string][]byte, 2)
	for _, runID := range []string{fuzzOpenRunID, fuzzReviewedRunID} {
		state, err := os.ReadFile(fuzzRunStatePath(repo, runID))
		if err != nil {
			f.Fatalf("read run %q fixture: %v", runID, err)
		}
		runStates[runID] = state
	}
	return repo, gitState, runStates
}

func plantApplyTraversalTarget(f *testing.F, repo string) {
	f.Helper()
	target := fuzzRunStatePath(repo, fuzzTraversalRunID)
	state := []byte(`{"run_id":"../../../../HEAD"}`)
	if err := os.WriteFile(target, state, 0o600); err != nil {
		f.Fatalf("plant traversal target %q: %v", target, err)
	}
	if _, err := os.Stat(target); err != nil {
		f.Fatalf("stat traversal target %q: %v", target, err)
	}
}

func assertApplyFuzzGitStateUnchanged(t *testing.T, repo string, want applyFuzzGitState) {
	t.Helper()
	gotSymbolicRef := runApplyFuzzGit(t, repo, "symbolic-ref", "HEAD")
	if gotSymbolicRef != want.symbolicRef {
		t.Fatalf("Git HEAD symbolic ref changed: got %q; want %q", gotSymbolicRef, want.symbolicRef)
	}
	gotHeadSHA := runApplyFuzzGit(t, repo, "rev-parse", "HEAD")
	if gotHeadSHA != want.headSHA {
		t.Fatalf("Git HEAD SHA changed: got %q; want %q", gotHeadSHA, want.headSHA)
	}
	gotRefSHA := runApplyFuzzGit(t, repo, "rev-parse", want.symbolicRef)
	if gotRefSHA != want.refSHA {
		t.Fatalf("Git ref %q changed: got %q; want %q", want.symbolicRef, gotRefSHA, want.refSHA)
	}
}

func fuzzRunStatePath(repo, runID string) string {
	return filepath.Join(repo, ".git", "jacu", "runs", runID+".json")
}

func runApplyFuzzGit(tb testing.TB, repo string, args ...string) string {
	tb.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	// #nosec G204 -- the test helper invokes a fixed binary with test-controlled argv.
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = repo
	cmd.Env = testgit.Env()
	if output, err := cmd.CombinedOutput(); err != nil {
		tb.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, output)
	} else {
		return strings.TrimSpace(string(output))
	}
	return ""
}

func applyFuzzBoundarySeed(f *testing.F, runID string, size int64) []byte {
	f.Helper()
	seed := []byte(`{"run_id":"` + runID + `"}`)
	if size < int64(len(seed)) || size > int64(^uint(0)>>1) {
		f.Fatalf("invalid apply fuzz boundary size %d", size)
	}
	return append(seed, bytes.Repeat([]byte{' '}, int(size)-len(seed))...)
}

func fuzzExpectedFixtureRefusal(raw []byte) (string, bool) {
	var input ApplyInput
	if err := json.Unmarshal(raw, &input); err != nil {
		return "", false
	}
	switch input.RunID {
	case fuzzOpenRunID:
		return "diff not reviewed; call jacu_diff first", true
	case fuzzReviewedRunID:
		return "worktree changed after review; review the diff again", true
	default:
		return "", false
	}
}

func assertFuzzApplyRefused(t *testing.T, path string, result capabilityruntime.Result, err error) {
	t.Helper()
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		t.Fatalf("%s timed out or was canceled: %v", path, err)
	}
	if result.Status == "ok" {
		t.Fatalf("%s returned ok: %#v; error=%v", path, result, err)
	}
	if data, ok := result.Data.(ApplyData); ok && data.CommitSHA != "" {
		t.Fatalf("%s returned commit %q", path, data.CommitSHA)
	}
}

func assertFuzzExactRefusal(t *testing.T, path string, result capabilityruntime.Result, err error, wantSummary string) {
	t.Helper()
	if err != nil {
		t.Fatalf("%s returned error for known run: %v", path, err)
	}
	if result.Status != "blocked" || result.Summary != wantSummary {
		t.Fatalf("%s result = %#v; want blocked %q", path, result, wantSummary)
	}
}

func assertFuzzGitFileUnchanged(t *testing.T, path string, want []byte) {
	t.Helper()
	// #nosec G304 -- path is a runstate fixture created under the fuzz test repository.
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read Git state %q: %v", path, err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("Git state %q changed: got %q; want %q", path, got, want)
	}
}
