package verify

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jacu-dev/jacu-harness/internal/runstate"
	"github.com/jacu-dev/jacu-harness/internal/telemetry"
)

func seedRun(t *testing.T, runID string, commands [][]string, status runstate.Status) (string, string) {
	t.Helper()
	root := t.TempDir()
	worktree := t.TempDir()
	// The fixture declares its own policy, which is also how a real project
	// authorizes a tool the curated global list does not ship.
	writeConfig(t, root, `{"allow":[{"program":"printf"},{"program":"false"},{"program":"touch"}]}`)
	input := runstate.MissionInput{
		Objective:            "Fix the parser output",
		VerificationCommands: commands,
		AllowedPaths:         []string{"parser.go"},
		RiskHint:             "write",
	}
	mission := runstate.MissionSnapshot{
		MissionID:            "msn_0123456789abcdef",
		Objective:            input.Objective,
		VerificationCommands: append([][]string{}, commands...),
		AllowedPaths:         append([]string{}, input.AllowedPaths...),
		Risk:                 input.RiskHint,
	}
	run := runstate.Run{
		RunID:        runID,
		MissionID:    mission.MissionID,
		MissionInput: input,
		Mission:      mission,
		Status:       status,
		CreatedAt:    time.Unix(1786000000, 0).UTC(),
		BaseSHA:      "0000000000000000000000000000000000000000",
		Branch:       "jacu/run-" + strings.TrimPrefix(runID, "run_"),
		Worktree:     worktree,
	}
	if err := runstate.Save(root, run); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	return root, worktree
}

func TestVerifyRunsMissionCommandsInOrderAndAggregatesPass(t *testing.T) {
	root, worktree := seedRun(t, "run_1111111111111111", [][]string{
		{"printf", "first"},
		{"printf", "second"},
	}, runstate.StatusOpen)

	result := Verify(context.Background(), root, Input{RunID: "run_1111111111111111"})
	if result.Status != "ok" {
		t.Fatalf("status = %q (%s)", result.Status, result.Summary)
	}
	if result.Data.Verdict != VerdictPass {
		t.Fatalf("verdict = %q; want pass", result.Data.Verdict)
	}
	if len(result.Data.Commands) != 2 {
		t.Fatalf("commands = %#v; want both, in order", result.Data.Commands)
	}
	if result.Data.Commands[0].StdoutTail != "first" || result.Data.Commands[1].StdoutTail != "second" {
		t.Fatalf("commands ran out of order: %#v", result.Data.Commands)
	}
	if !strings.HasPrefix(result.Data.EvidenceDigest, "sha256:") {
		t.Fatalf("evidence digest = %q", result.Data.EvidenceDigest)
	}
	_ = worktree
}

func TestVerifyOptionalArgvUsesTheGovernedDiagnosticPath(t *testing.T) {
	root, worktree := seedRun(t, "run_1234567890abcdef", nil, runstate.StatusOpen)

	result := Verify(context.Background(), root, Input{
		RunID: "run_1234567890abcdef",
		ArgV:  []string{"touch", "diagnostic"},
	})
	if result.Status != "ok" || result.Data.Verdict != VerdictPass {
		t.Fatalf("status = %q verdict = %q (%s); want governed argv pass", result.Status, result.Data.Verdict, result.Summary)
	}
	if len(result.Data.Commands) != 1 || result.Data.Commands[0].ArgV[0] != "touch" {
		t.Fatalf("commands = %#v; want one diagnostic command", result.Data.Commands)
	}
	if _, err := os.Stat(filepath.Join(worktree, "diagnostic")); err != nil {
		t.Fatalf("diagnostic argv did not run in the worktree: %v", err)
	}
}

func TestVerifyAggregatesFailWithoutLosingTheOtherCommands(t *testing.T) {
	root, _ := seedRun(t, "run_2222222222222222", [][]string{
		{"printf", "ok"},
		{"false"},
	}, runstate.StatusOpen)

	result := Verify(context.Background(), root, Input{RunID: "run_2222222222222222"})
	if result.Data.Verdict != VerdictFail {
		t.Fatalf("verdict = %q; want fail", result.Data.Verdict)
	}
	if len(result.Data.Commands) != 2 || result.Data.Commands[0].Status != StatusPassed {
		t.Fatalf("commands = %#v; the passing one must still be reported", result.Data.Commands)
	}
}

// A refused command means nothing runs at all — not "runs until the refusal".
// The proof is a side effect: the first command would create a file.
func TestVerifyRefusesTheWholeBatchBeforeRunningAnything(t *testing.T) {
	root, worktree := seedRun(t, "run_3333333333333333", [][]string{
		{"touch", "ran"},
		{"curl", "https://example.invalid"},
	}, runstate.StatusOpen)
	marker := filepath.Join(worktree, "ran")

	result := Verify(context.Background(), root, Input{RunID: "run_3333333333333333"})
	if result.Status != "blocked" || result.Data.Verdict != VerdictBlocked {
		t.Fatalf("status = %q verdict = %q; want blocked", result.Status, result.Data.Verdict)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("the allowed command ran before the batch was refused (stat err = %v)", err)
	}
	if !strings.Contains(result.Summary, "curl") {
		t.Fatalf("summary = %q; it has to name the refused command", result.Summary)
	}
}

// not_run is not a pass. A mission that declares no verification is not a
// verified mission, and the autonomy policy has to be able to refuse it without
// needing an exception.
func TestVerifyWithoutCommandsIsNotRunNotPass(t *testing.T) {
	root, _ := seedRun(t, "run_4444444444444444", nil, runstate.StatusOpen)

	result := Verify(context.Background(), root, Input{RunID: "run_4444444444444444"})
	if result.Data.Verdict != VerdictNotRun {
		t.Fatalf("verdict = %q; want not_run", result.Data.Verdict)
	}
	if len(result.Warnings) == 0 {
		t.Fatal("no warning; the host has to learn nothing was verified")
	}
}

func TestVerifyBlocksWithoutAUsableRun(t *testing.T) {
	root, _ := seedRun(t, "run_5555555555555555", [][]string{{"printf", "x"}}, runstate.StatusApplied)

	for _, tt := range []struct {
		name  string
		runID string
		want  string
	}{
		{name: "missing run", runID: "run_9999999999999999", want: "run"},
		{name: "invalid run id", runID: "../escape", want: "invalid run_id"},
		{name: "closed run", runID: "run_5555555555555555", want: "not open"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			result := Verify(context.Background(), root, Input{RunID: tt.runID})
			if result.Status != "blocked" {
				t.Fatalf("status = %q (%s); want blocked", result.Status, result.Summary)
			}
			if !strings.Contains(result.Summary, tt.want) {
				t.Fatalf("summary = %q; want it to mention %q", result.Summary, tt.want)
			}
		})
	}
}

func TestVerifyTelemetryRecordsReturnedBlockedResult(t *testing.T) {
	base := t.TempDir()
	t.Setenv("JACU_HOME", base)
	root, _ := seedRun(t, "run_6666666666666666", [][]string{{"printf", "x"}}, runstate.StatusApplied)

	result := Verify(context.Background(), root, Input{RunID: "run_6666666666666666"})
	if result.Status != "blocked" || result.Data.Verdict != VerdictBlocked {
		t.Fatalf("result = %+v; want blocked", result)
	}
	events, err := telemetry.NewStoreAt(base).ReadSince(time.Unix(0, 0).UTC())
	if err != nil {
		t.Fatalf("read telemetry: %v", err)
	}
	var verifyEvent, gateEvent bool
	for _, event := range events {
		switch event.Event {
		case telemetry.EventVerify:
			verifyEvent = event.Status == "blocked" && event.Verdict == VerdictBlocked
		case telemetry.EventGateDecision:
			gateEvent = event.Verdict == "block"
		}
	}
	if !verifyEvent || !gateEvent {
		t.Fatalf("verify telemetry = %+v; want blocked verify and gate events", events)
	}
}
