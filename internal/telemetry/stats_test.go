package telemetry

import (
	"github.com/jacu-dev/jacu-harness/internal/testgit"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestComputeStatsCalculatesV1Metrics(t *testing.T) {
	base := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	events := []Event{
		statsEvent(base, EventToolCall, "jacu_mission_compile", "ok", "run_0123456789abcdef", "msn_0123456789abcdef", 100),
		statsEvent(base.Add(10*time.Second), EventVerify, "jacu_verify", "ok", "run_0123456789abcdef", "msn_0123456789abcdef", 1000),
		statsEvent(base.Add(20*time.Second), EventApply, "jacu_apply", "applied", "run_0123456789abcdef", "msn_0123456789abcdef", 100),
		statsEvent(base.Add(30*time.Second), EventToolCall, "jacu_report", "ok", "", "", 200),
		statsEvent(base.Add(40*time.Second), EventRemediation, "", "ok", "run_0123456789abcdef", "msn_0123456789abcdef", 5),
		statsEvent(base.Add(50*time.Second), EventEscalation, "", "escalated", "run_ffffffffffffffff", "msn_ffffffffffffffff", 0),
		statsEvent(base.Add(60*time.Second), EventApply, "autonomy", "applied", "run_ffffffffffffffff", "msn_ffffffffffffffff", 0),
	}
	events[1].Verdict = "pass"
	events[1].Iteration = 1
	events[4].Iteration = 5
	events[5].Iteration = 1
	stats, err := ComputeStats(events, base.Add(-time.Hour), base.Add(24*time.Hour), nil)
	if err != nil {
		t.Fatalf("ComputeStats: %v", err)
	}
	if !stats.Available[MetricFirstPassVerify] || stats.FirstPassVerifyPct != 100 {
		t.Fatalf("first pass stats = %+v", stats)
	}
	if stats.RemediationIterations != 5 || stats.EscalationPct != 50 || stats.AutoApplyPct != 50 {
		t.Fatalf("outcome stats = %+v", stats)
	}
	if stats.MissionApplyP50Ms != 20000 || stats.MissionApplyP95Ms != 20000 {
		t.Fatalf("mission apply stats = %+v", stats)
	}
	if stats.ToolP95Ms["jacu_report"] != 200 || stats.MissionsPerDay != 2 {
		t.Fatalf("throughput stats = %+v", stats)
	}
}

func TestComputeStatsUsesNoDataInsteadOfInventedZero(t *testing.T) {
	stats, err := ComputeStats(nil, time.Now().Add(-24*time.Hour), time.Now(), nil)
	if err != nil {
		t.Fatalf("ComputeStats: %v", err)
	}
	for name, available := range stats.Available {
		if available {
			t.Fatalf("metric %q is available with no events", name)
		}
	}
	if got := FormatStats(stats); !strings.Contains(got, "no-data") {
		t.Fatalf("no-data stats output = %q", got)
	}
}

func TestComputeStatsUsesHighestRemediationIterationPerMission(t *testing.T) {
	base := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	events := []Event{
		statsEvent(base, EventRemediation, "autonomy", "ok", "run_0123456789abcdef", "msn_0123456789abcdef", 0),
		statsEvent(base.Add(time.Second), EventRemediation, "autonomy", "ok", "run_0123456789abcdef", "msn_0123456789abcdef", 0),
	}
	events[0].Iteration = 1
	events[1].Iteration = 2
	stats, err := ComputeStats(events, base.Add(-time.Hour), base.Add(time.Hour), nil)
	if err != nil {
		t.Fatalf("ComputeStats: %v", err)
	}
	if stats.RemediationIterations != 2 {
		t.Fatalf("remediation iterations = %v; want highest ordinal 2", stats.RemediationIterations)
	}
}

func TestFilterProjectKeepsOnlyCurrentProject(t *testing.T) {
	events := []Event{
		statsEvent(time.Now().UTC(), EventVerify, "jacu_verify", "ok", "run_0123456789abcdef", "msn_0123456789abcdef", 0),
		statsEvent(time.Now().UTC(), EventVerify, "jacu_verify", "ok", "run_ffffffffffffffff", "msn_ffffffffffffffff", 0),
	}
	events[1].ProjectID = "prj_ffffffffffffffff"
	filtered := FilterProject(events, "prj_0123456789abcdef")
	if len(filtered) != 1 || filtered[0].ProjectID != "prj_0123456789abcdef" {
		t.Fatalf("filtered project events = %+v", filtered)
	}
}

func TestComputeStatsCountsHeuristicJacuRunRevert(t *testing.T) {
	repo := t.TempDir()
	gitInit(t, repo)
	if err := os.WriteFile(filepath.Join(repo, "note"), []byte("one\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	gitRun(t, repo, "add", "note")
	gitRun(t, repo, "commit", "-m", "apply\n\nJacu-Run: run_0123456789abcdef")
	if err := os.WriteFile(filepath.Join(repo, "note"), []byte("reverted\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	gitRun(t, repo, "add", "note")
	gitRun(t, repo, "commit", "-m", "Revert apply\n\nJacu-Run: run_0123456789abcdef")
	event := statsEvent(time.Now().UTC(), EventApply, "jacu_apply", "applied", "run_0123456789abcdef", "msn_0123456789abcdef", 1)
	stats, err := ComputeStats([]Event{event}, time.Now().Add(-time.Hour), time.Now().Add(time.Hour), &GitHistory{Repo: repo})
	if err != nil {
		t.Fatalf("ComputeStats: %v", err)
	}
	if !stats.Available[MetricRevertedApplyPct] || stats.RevertedApplyPct != 100 || !stats.RevertHeuristic {
		t.Fatalf("revert stats = %+v", stats)
	}
}

func statsEvent(ts time.Time, kind, tool, status, runID, missionID string, duration int64) Event {
	return Event{Timestamp: ts, ProjectID: "prj_0123456789abcdef", TraceID: NewTraceID(), Event: kind, Tool: tool, Status: status, RunID: runID, MissionID: missionID, DurationMs: duration}
}

func gitInit(t *testing.T, repo string) {
	t.Helper()
	gitRun(t, repo, "init", "-q")
	gitRun(t, repo, "config", "user.email", "telemetry@example.invalid")
	gitRun(t, repo, "config", "user.name", "Telemetry Test")
	gitRun(t, repo, "config", "commit.gpgsign", "false")
}

func gitRun(t *testing.T, repo string, args ...string) {
	t.Helper()
	// #nosec G204 -- test invokes fixed git with a test-owned repository and controlled fixture arguments.
	command := exec.Command("git", append([]string{"-C", repo}, args...)...)
	command.Env = testgit.Env()
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", args, err, output)
	}
}
