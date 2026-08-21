package verify

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jacu-dev/jacu-harness/internal/project"
	"github.com/jacu-dev/jacu-harness/internal/runstate"
	"github.com/jacu-dev/jacu-harness/internal/telemetry"
	"github.com/jacu-dev/jacu-harness/internal/userstate"
)

// Aggregate verdicts. not_run is in the enum on purpose, against the four-value
// draft of the phase plan: a verify that ran nothing is not an approval, and a
// cancelled batch reported as failures becomes N phantom remediations
// downstream. The autonomy policy requires verdict == pass, literally.
const (
	VerdictPass    = "pass"
	VerdictFail    = "fail"
	VerdictTimeout = "timeout"
	VerdictBlocked = "blocked"
	VerdictNotRun  = "not_run"
)

// Input is the whole surface: no command, no timeout, no allowlist. The argv
// comes from the compiled mission, and limits are runtime policy, never a
// parameter of the object being governed.
type Input struct {
	RunID  string   `json:"run_id,omitempty"`
	ArgV   []string `json:"argv,omitempty"`
	Async  bool     `json:"async,omitempty"`
	TaskID string   `json:"task_id,omitempty"`
	Cancel bool     `json:"cancel,omitempty"`
}

// Data is the stable result contract that phases 07 and 09 consume.
type Data struct {
	Verdict         string    `json:"verdict"`
	Commands        []Result  `json:"commands"`
	EvidenceDigest  string    `json:"evidence_digest"`
	TotalDurationMs int64     `json:"total_duration_ms"`
	Task            *TaskInfo `json:"task,omitempty"`
}

// Envelope mirrors the runtime result the capability layer wraps.
type Envelope struct {
	Status      string   `json:"status"`
	Summary     string   `json:"summary"`
	Data        Data     `json:"data"`
	Warnings    []string `json:"warnings"`
	NextActions []string `json:"next_actions"`
}

// Execution is the verify-owned batch seam shared by jacu_verify and
// jacu_apply. Refusal is non-empty only when policy or executor setup blocks
// the batch before ordinary command-result aggregation.
type Execution struct {
	Data    Data
	Refusal string
}

// Verify runs the mission's verification commands, in order, inside the run
// worktree.
func Verify(ctx context.Context, root string, in Input) (result Envelope) {
	result = Envelope{
		Status:      "ok",
		Data:        Data{Verdict: VerdictNotRun, Commands: []Result{}},
		Warnings:    []string{},
		NextActions: []string{},
	}

	run, err := runstate.Load(root, in.RunID)
	if err != nil {
		return blocked(result, refusalFor(in.RunID, err))
	}
	emitGate := func(verdict string) {
		telemetry.EmitBestEffortInput(telemetry.EventInput{
			Timestamp: time.Now().UTC(), ProjectID: telemetry.ProjectID(root), TraceID: telemetry.NewTraceID(),
			RunID: run.RunID, MissionID: run.MissionID, Module: "verify", Stage: "gate",
			Event: telemetry.EventGateDecision, Tool: ToolName, Status: "ok", Verdict: verdict,
		})
	}
	started := time.Now()
	defer func() {
		telemetry.EmitBestEffortInput(telemetry.EventInput{
			Timestamp: time.Now().UTC(), ProjectID: telemetry.ProjectID(root), TraceID: telemetry.NewTraceID(),
			RunID: run.RunID, MissionID: run.MissionID, Event: telemetry.EventVerify,
			Tool: ToolName, Status: result.Status, Verdict: result.Data.Verdict,
			Iteration: 1, Duration: time.Since(started),
		})
	}()
	if run.Status != runstate.StatusOpen && run.Status != runstate.StatusReviewed {
		emitGate("block")
		return blocked(result, fmt.Sprintf("run %s is not open for verification (status %q)", run.RunID, run.Status))
	}

	commands := run.Mission.VerificationCommands
	if len(in.ArgV) > 0 {
		commands = [][]string{append([]string{}, in.ArgV...)}
	}
	if len(commands) == 0 {
		emitGate("warn")
		result.Data.Verdict = VerdictNotRun
		result.Summary = "Mission declares no verification commands."
		result.Warnings = append(result.Warnings, "mission declares no verification commands; nothing was verified")
		result.NextActions = append(result.NextActions, "add verification_commands to the mission and recompile")
		result.Data.EvidenceDigest = digestOf(nil)
		return result
	}

	execution := ExecuteCommands(ctx, root, run, commands)
	if execution.Refusal != "" {
		emitGate("block")
	} else if execution.Data.Verdict == VerdictPass {
		emitGate("pass")
	} else {
		emitGate("warn")
	}
	result.Data = execution.Data
	if execution.Refusal != "" {
		return blocked(result, execution.Refusal)
	}
	result.Summary = summaryFor(result.Data.Verdict, len(result.Data.Commands))
	if result.Data.Verdict == VerdictBlocked {
		return blocked(result, result.Summary)
	}
	if result.Data.Verdict == VerdictFail || result.Data.Verdict == VerdictTimeout {
		result.NextActions = append(result.NextActions, "fix the mission in the worktree and verify again")
	}
	return result
}

// ExecuteCommands applies the complete verify policy and bounded executor to
// an authoritative command batch. The whole batch is checked before the first
// spawn, so a refused later argv cannot leave effects from an earlier one.
func ExecuteCommands(ctx context.Context, root string, run runstate.Run, commands [][]string) Execution {
	data := Data{Verdict: VerdictNotRun, Commands: []Result{}, EvidenceDigest: digestOf(nil)}
	if len(commands) == 0 {
		return Execution{Data: data}
	}

	config, err := LoadConfig(root)
	if err != nil {
		data.Verdict = VerdictBlocked
		return Execution{Data: data, Refusal: "verify policy is unreadable: " + err.Error()}
	}
	allowlist := New(config)
	for _, command := range commands {
		if checkErr := allowlist.Check(command); checkErr != nil {
			emitDenialTelemetry(root, run, denialReason(checkErr), len(command) > 0 && allowlist.KnowsProgram(command[0]))
			data.Commands = append(data.Commands, Result{
				ArgV:   append([]string{}, command...),
				Status: StatusBlocked,
				Reason: checkErr.Error(),
			})
			data.Verdict = VerdictBlocked
			return Execution{Data: data, Refusal: fmt.Sprintf("command refused before execution: %s (%s)",
				strings.Join(command, " "), checkErr.Error())}
		}
	}

	projectID, err := project.ID(root)
	if err != nil {
		data.Verdict = VerdictBlocked
		return Execution{Data: data, Refusal: "project identity unavailable: " + err.Error()}
	}
	runner, cleanup, err := runnerFor(run, projectID, config.PathDirs)
	if err != nil {
		data.Verdict = VerdictBlocked
		return Execution{Data: data, Refusal: err.Error()}
	}
	defer cleanup()

	digests := make([]string, 0, len(commands))
	for _, command := range commands {
		outcome := runner.Run(ctx, command)
		data.Commands = append(data.Commands, outcome)
		data.TotalDurationMs += outcome.DurationMs
		digests = append(digests, outcome.Digest)
		if outcome.Status == StatusTimedOut || outcome.Status == StatusNotRun {
			break
		}
	}
	data.EvidenceDigest = digestOf(digests)
	data.Verdict = aggregate(data.Commands)
	return Execution{Data: data}
}

func emitDenialTelemetry(root string, run runstate.Run, reason string, programKnown bool) {
	telemetry.EmitBestEffortInput(telemetry.EventInput{
		Timestamp: time.Now().UTC(), ProjectID: telemetry.ProjectID(root), TraceID: telemetry.NewTraceID(),
		RunID: run.RunID, MissionID: run.MissionID, Module: "verify", Stage: "denial",
		Event: telemetry.EventVerifyDenial, Tool: ToolName, Status: "blocked", Reason: reason, ProgramKnown: programKnown,
	})
}

func denialReason(err error) string {
	message := err.Error()
	switch {
	case strings.Contains(message, "shell metachar"):
		return "shell_meta"
	case strings.Contains(message, "shell invocation"), strings.Contains(message, "interpreter flag"):
		return "shell_interpreter"
	case strings.Contains(message, "not in allowlist"), strings.Contains(message, "no allowlist"):
		return "not_in_allowlist"
	default:
		return "sandbox"
	}
}

// aggregate applies the decision order of the design. not_run never counts as a
// failure — that is the whole reason it exists.
func aggregate(commands []Result) string {
	if len(commands) == 0 {
		return VerdictNotRun
	}
	seen := map[string]bool{}
	for _, command := range commands {
		seen[command.Status] = true
	}
	switch {
	case seen[StatusBlocked]:
		return VerdictBlocked
	case seen[StatusTimedOut]:
		return VerdictTimeout
	case seen[StatusFailed]:
		return VerdictFail
	case seen[StatusNotRun]:
		return VerdictNotRun
	default:
		return VerdictPass
	}
}

func summaryFor(verdict string, count int) string {
	switch verdict {
	case VerdictPass:
		return fmt.Sprintf("Verification passed: %d command(s).", count)
	case VerdictFail:
		return "Verification failed."
	case VerdictTimeout:
		return "Verification timed out."
	case VerdictNotRun:
		return "Verification did not run."
	default:
		return "Verification blocked."
	}
}

func blocked(result Envelope, summary string) Envelope {
	result.Status = "blocked"
	result.Summary = summary
	if result.Data.Verdict != VerdictBlocked {
		result.Data.Verdict = VerdictBlocked
	}
	if result.Data.EvidenceDigest == "" {
		result.Data.EvidenceDigest = digestOf(nil)
	}
	return result
}

// refusalFor keeps the reason specific. "invalid run_id" and "no such run" are
// different failures and the host reacts to them differently.
func refusalFor(runID string, err error) string {
	if !runstate.ValidRunID(runID) {
		return fmt.Sprintf("invalid run_id %q", runID)
	}
	if os.IsNotExist(err) {
		return fmt.Sprintf("run %s does not exist", runID)
	}
	return fmt.Sprintf("run %s is unreadable: %v", runID, err)
}

// runnerFor builds the executor for a run: the worktree as working directory, a
// synthetic HOME under that run's isolated home so two runs do not share a
// toolchain cache, and a scratch TMPDIR outside the worktree.
func runnerFor(run runstate.Run, projectID string, pathDirs []string) (Runner, func(), error) {
	runHome, err := userstate.RunHome(projectID, run.RunID)
	if err != nil {
		return Runner{}, func() {}, fmt.Errorf("run home unavailable: %w", err)
	}
	toolchainHome := filepath.Join(runHome, "toolchain-home")
	if mkdirErr := os.MkdirAll(toolchainHome, 0o700); mkdirErr != nil {
		return Runner{}, func() {}, fmt.Errorf("prepare toolchain home: %w", mkdirErr)
	}
	scratch, err := os.MkdirTemp("", "jacu-verify-"+run.RunID+"-")
	if err != nil {
		return Runner{}, func() {}, fmt.Errorf("prepare scratch directory: %w", err)
	}
	runner := Runner{
		Worktree:      run.Worktree,
		PathDirs:      append([]string{}, pathDirs...),
		ToolchainHome: toolchainHome,
		ScratchDir:    scratch,
		Timeout:       commandTimeout,
		TailBytes:     defaultTailBytes,
	}
	return runner, func() { _ = os.RemoveAll(scratch) }, nil
}

const commandTimeout = 120 * time.Second

// digestOf hashes the per-command digests into one evidence digest. Hashing the
// hashes keeps the receipt small and still covers every byte produced.
func digestOf(digests []string) string {
	hasher := sha256.New()
	for _, digest := range digests {
		_, _ = hasher.Write([]byte(digest))
		_, _ = hasher.Write([]byte{0})
	}
	return "sha256:" + hex.EncodeToString(hasher.Sum(nil))
}
