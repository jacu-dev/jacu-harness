// Package telemetry contains the local, sanitized measurement stream.
//
// The public event constructor intentionally accepts only typed closed values.
// Prompts, diffs, outputs, paths, and free text have no field in this package.
package telemetry

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"

	"github.com/jacu-dev/jacu-harness/internal/project"
)

const (
	CurrentSchemaVersion = "2"
	NoData               = "no-data"
	LevelUser            = "user"
	LevelFull            = "full"

	EventToolCall            = "tool_call"
	EventVerify              = "verify"
	EventApply               = "apply"
	EventDiscard             = "discard"
	EventRemediation         = "remediation"
	EventEscalation          = "escalation"
	EventFlowNode            = "flow_node"
	EventGateDecision        = "gate.decision"
	EventVerifyDenial        = "verify.denial"
	EventReviewDisagreement  = "review.disagreement"
	EventCleanExitClose      = "cleanexit.close"
	EventPreflightCheck      = "preflight.check"
	EventMissionInterruption = "mission.interruption"
	EventClarityProbe        = "clarity.probe"
)

// Event is the only record written to the local stream. Keep this struct
// closed: adding a field is a telemetry contract change and needs a test.
type Event struct {
	SchemaVersion   string    `json:"schema_version"`
	Timestamp       time.Time `json:"ts"`
	Level           string    `json:"level"`
	ProjectID       string    `json:"project_id"`
	TraceID         string    `json:"trace_id"`
	RunID           string    `json:"run_id,omitempty"`
	MissionID       string    `json:"mission_id,omitempty"`
	ProgramID       string    `json:"program_id,omitempty"`
	Module          string    `json:"module"`
	Stage           string    `json:"stage"`
	Event           string    `json:"event"`
	Tool            string    `json:"tool,omitempty"`
	Status          string    `json:"status"`
	DurationMs      int64     `json:"duration_ms,omitempty"`
	Measurement     string    `json:"measurement,omitempty"`
	InputBytes      int64     `json:"input_bytes,omitempty"`
	OutputBytes     int64     `json:"output_bytes,omitempty"`
	Capped          bool      `json:"capped,omitempty"`
	DegradedPartial bool      `json:"degraded_partial,omitempty"`
	Ceremony        string    `json:"ceremony,omitempty"`
	Risk            string    `json:"risk,omitempty"`
	Verdict         string    `json:"verdict,omitempty"`
	Iteration       int       `json:"iteration,omitempty"`
	ExitReason      string    `json:"exit_reason,omitempty"`
	Reason          string    `json:"reason,omitempty"`
	ProgramKnown    bool      `json:"program_known"`
	Auto            bool      `json:"auto,omitempty"`
	Intervention    bool      `json:"intervention,omitempty"`
	DiffBytes       int64     `json:"diff_bytes,omitempty"`
	FilesChanged    int       `json:"files_changed,omitempty"`
	Resolved        string    `json:"resolved,omitempty"`
	Result          string    `json:"result,omitempty"`
	FailureClass    string    `json:"failure_class,omitempty"`
	Round           int       `json:"round,omitempty"`
	Divergences     int       `json:"divergences,omitempty"`
	DivergenceField string    `json:"divergence_field,omitempty"`
	VarianceRuns    int       `json:"variance_runs,omitempty"`
	SpecBytes       int64     `json:"spec_bytes,omitempty"`
	SpecBytesDelta  int64     `json:"spec_bytes_delta,omitempty"`
}

// EventInput uses a duration instead of an unbounded free-form duration
// string. Every field maps to an allowlisted Event field.
type EventInput struct {
	SchemaVersion   string
	Timestamp       time.Time
	Level           string
	ProjectID       string
	TraceID         string
	RunID           string
	MissionID       string
	ProgramID       string
	Module          string
	Stage           string
	Event           string
	Tool            string
	Status          string
	Duration        time.Duration
	Measurement     string
	InputBytes      int64
	OutputBytes     int64
	Capped          bool
	DegradedPartial bool
	Ceremony        string
	Risk            string
	Verdict         string
	Iteration       int
	ExitReason      string
	Reason          string
	ProgramKnown    bool
	Auto            bool
	Intervention    bool
	DiffBytes       int64
	FilesChanged    int
	Resolved        string
	Result          string
	FailureClass    string
	Round           int
	Divergences     int
	DivergenceField string
	VarianceRuns    int
	SpecBytes       int64
	SpecBytesDelta  int64
}

var safeIdentifier = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.:-]{0,127}$`)

var allowedEvents = map[string]struct{}{
	EventToolCall: {}, EventVerify: {}, EventApply: {}, EventDiscard: {},
	EventRemediation: {}, EventEscalation: {}, EventFlowNode: {}, EventGateDecision: {}, EventVerifyDenial: {}, EventReviewDisagreement: {}, EventCleanExitClose: {}, EventPreflightCheck: {}, EventMissionInterruption: {}, EventClarityProbe: {},
}

var allowedLevels = map[string]struct{}{LevelUser: {}, LevelFull: {}, NoData: {}}
var allowedModules = map[string]struct{}{
	"runtime": {}, "mission": {}, "workspace": {}, "verify": {}, "memory": {}, "report": {},
	"orchestration": {}, "modelcontrol": {}, "context": {}, "ledger": {}, "clarity": {},
	"preflight": {}, "sdd": {}, "cleanexit": {}, "eval": {}, "telemetry": {}, NoData: {},
}
var allowedStages = map[string]struct{}{
	"tool_call": {}, "verify": {}, "apply": {}, "discard": {}, "remediation": {}, "escalation": {}, "gate": {}, "denial": {}, "review": {}, "close": {},
	"flow_node": {}, "emit": {}, "preflight": {}, "interruption": {}, "probe": {}, NoData: {},
}
var allowedMeasurements = map[string]struct{}{
	"exact_bytes": {}, "cli_reported_tokens": {}, "estimated_tokens": {}, NoData: {},
}

var allowedStatuses = map[string]struct{}{
	"ok": {}, "blocked": {}, "partial": {}, "failed": {}, "accepted": {}, "escalated": {},
	"completed": {}, "timed_out": {}, "cancelled": {}, "queued": {}, "running": {},
	"done": {}, "passed": {}, "pass": {}, "fail": {}, "timeout": {}, "not_run": {},
	"applied": {}, "discarded": {},
}

var allowedCeremonies = map[string]struct{}{"direct": {}, "light": {}, "full": {}}
var allowedRisks = map[string]struct{}{"safe": {}, "write": {}, "destructive": {}}
var allowedVerdicts = map[string]struct{}{
	"pass": {}, "fail": {}, "timeout": {}, "blocked": {}, "not_run": {},
	"warn": {}, "require_approval": {}, "block": {},
}

var gateVerdictOrder = map[string]int{
	"pass":             0,
	"warn":             1,
	"require_approval": 2,
	"block":            3,
}

func GateVerdictRank(verdict string) int {
	return gateVerdictOrder[verdict]
}

var allowedExitReasons = map[string]struct{}{
	"completed": {}, "failed": {}, "timed_out": {}, "cancelled": {}, "blocked": {},
	"escalated": {}, "reverted": {}, "no_data": {}, "unavailable": {},
}
var allowedDenialReasons = map[string]struct{}{
	"not_in_allowlist": {}, "shell_meta": {}, "shell_interpreter": {}, "prefix_mismatch": {}, "sandbox": {},
}
var allowedResolved = map[string]struct{}{"require_approval": {}, "escalated": {}}
var allowedResults = map[string]struct{}{"pass": {}, "fail": {}}
var allowedDivergenceFields = map[string]struct{}{
	"write_scope": {}, "forbidden_paths": {}, "requirements": {}, "out_of_scope": {}, "tasks": {},
}
var allowedFailureClasses = map[string]struct{}{
	"branch_local": {}, "branch_remote": {}, "worktree": {}, "untracked": {}, "stash": {}, "run_open": {}, "main_mismatch": {},
	"allowlist": {}, "program_not_on_path": {}, "path_missing": {}, "path_not_writable": {}, "credential_absent": {}, "network_undeclared": {}, "doc_missing": {}, "open_questions": {},
}

func NewEvent(input EventInput) (Event, error) {
	schemaVersion := input.SchemaVersion
	if schemaVersion == "" {
		schemaVersion = CurrentSchemaVersion
	}
	level := input.Level
	if level == "" {
		level = LevelUser
	}
	module := input.Module
	if module == "" {
		module = defaultModule(input.Event)
	}
	stage := input.Stage
	if stage == "" {
		stage = input.Event
	}
	event := Event{
		SchemaVersion: schemaVersion, Timestamp: input.Timestamp.UTC(), Level: level,
		ProjectID: input.ProjectID, TraceID: input.TraceID,
		RunID: input.RunID, MissionID: input.MissionID, ProgramID: input.ProgramID,
		Module: module, Stage: stage, Event: input.Event, Tool: input.Tool,
		Status: input.Status, DurationMs: input.Duration.Milliseconds(), Ceremony: input.Ceremony,
		Measurement: input.Measurement, InputBytes: input.InputBytes, OutputBytes: input.OutputBytes,
		Capped: input.Capped, DegradedPartial: input.DegradedPartial,
		Risk: input.Risk, Verdict: input.Verdict,
		Iteration: input.Iteration, ExitReason: input.ExitReason,
		Reason: input.Reason, ProgramKnown: input.ProgramKnown,
		Auto: input.Auto, Intervention: input.Intervention, DiffBytes: input.DiffBytes,
		FilesChanged: input.FilesChanged, Resolved: input.Resolved,
		Result: input.Result, FailureClass: input.FailureClass,
		Round: input.Round, Divergences: input.Divergences, DivergenceField: input.DivergenceField,
		VarianceRuns: input.VarianceRuns, SpecBytes: input.SpecBytes, SpecBytesDelta: input.SpecBytesDelta,
	}
	if err := event.Validate(); err != nil {
		return Event{}, err
	}
	return event, nil
}

// NewFullEvent exposes the same closed typed surface at the owner's detail
// level. It deliberately reuses EventInput: there is no full-level escape hatch
// for prompts, diffs, outputs, paths, or free text.
func NewFullEvent(input EventInput) (Event, error) {
	input.Level = LevelFull
	return NewEvent(input)
}

func NewToolCallEvent(projectID, traceID, tool, status string, durationMs int64) (Event, error) {
	return NewEvent(EventInput{
		Timestamp: time.Now().UTC(), ProjectID: projectID, TraceID: traceID,
		Module: "runtime", Stage: EventToolCall, Event: EventToolCall, Tool: tool, Status: status,
		Duration: time.Duration(durationMs) * time.Millisecond,
	})
}

func NewTraceID() string {
	var raw [8]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "tr_0000000000000000"
	}
	return "tr_" + hex.EncodeToString(raw[:])
}

// ProjectID derives the stable local identity used by the rest of JACU. A
// registration path cannot return an error without changing the MCP startup
// contract, so an invalid root becomes a bounded sentinel and telemetry is
// still isolated from the governed result.
func ProjectID(root string) string {
	if id, err := project.ID(root); err == nil {
		return id
	}
	return "prj_unknown"
}

func (event Event) Validate() error {
	if event.SchemaVersion != "1" && event.SchemaVersion != CurrentSchemaVersion {
		return fmt.Errorf("telemetry schema_version is unsupported: %q", event.SchemaVersion)
	}
	if _, ok := allowedLevels[event.Level]; !ok {
		return fmt.Errorf("telemetry level is not allowlisted: %q", event.Level)
	}
	if _, ok := allowedModules[event.Module]; !ok {
		return fmt.Errorf("telemetry module is not allowlisted: %q", event.Module)
	}
	if _, ok := allowedStages[event.Stage]; !ok {
		return fmt.Errorf("telemetry stage is not allowlisted: %q", event.Stage)
	}
	if event.Measurement != "" {
		if _, ok := allowedMeasurements[event.Measurement]; !ok {
			return fmt.Errorf("telemetry measurement is not allowlisted: %q", event.Measurement)
		}
	}
	if event.InputBytes < 0 || event.OutputBytes < 0 {
		return errors.New("telemetry byte count is negative")
	}
	if event.DiffBytes < 0 || event.FilesChanged < 0 {
		return errors.New("telemetry typed detail count is negative")
	}
	if (event.InputBytes > 0 || event.OutputBytes > 0) && event.Measurement == "" {
		return errors.New("telemetry byte count requires measurement")
	}
	if event.Timestamp.IsZero() || event.Timestamp.Location() == nil {
		return errors.New("telemetry timestamp is required")
	}
	if err := validateIdentifier("project_id", event.ProjectID); err != nil {
		return err
	}
	if err := validateIdentifier("trace_id", event.TraceID); err != nil {
		return err
	}
	for name, value := range map[string]string{"run_id": event.RunID, "mission_id": event.MissionID, "tool": event.Tool} {
		if value != "" {
			if err := validateIdentifier(name, value); err != nil {
				return err
			}
		}
	}
	if _, ok := allowedEvents[event.Event]; !ok {
		return fmt.Errorf("telemetry event is not allowlisted: %q", event.Event)
	}
	if _, ok := allowedStatuses[event.Status]; !ok {
		return fmt.Errorf("telemetry status is not allowlisted: %q", event.Status)
	}
	if event.DurationMs < 0 || event.DurationMs > 24*60*60*1000 {
		return errors.New("telemetry duration_ms is outside bounds")
	}
	if event.Ceremony != "" {
		if _, ok := allowedCeremonies[event.Ceremony]; !ok {
			return fmt.Errorf("telemetry ceremony is not allowlisted: %q", event.Ceremony)
		}
	}
	if event.Risk != "" {
		if _, ok := allowedRisks[event.Risk]; !ok {
			return fmt.Errorf("telemetry risk is not allowlisted: %q", event.Risk)
		}
	}
	if event.Verdict != "" {
		if _, ok := allowedVerdicts[event.Verdict]; !ok {
			return fmt.Errorf("telemetry verdict is not allowlisted: %q", event.Verdict)
		}
	}
	if event.Reason != "" {
		if _, ok := allowedDenialReasons[event.Reason]; !ok {
			return fmt.Errorf("telemetry denial reason is not allowlisted: %q", event.Reason)
		}
	}
	if event.Resolved != "" {
		if _, ok := allowedResolved[event.Resolved]; !ok {
			return fmt.Errorf("telemetry resolved value is not allowlisted: %q", event.Resolved)
		}
	}
	if event.Result != "" {
		if _, ok := allowedResults[event.Result]; !ok {
			return fmt.Errorf("telemetry result is not allowlisted: %q", event.Result)
		}
	}
	if event.FailureClass != "" {
		if _, ok := allowedFailureClasses[event.FailureClass]; !ok {
			return fmt.Errorf("telemetry failure_class is not allowlisted: %q", event.FailureClass)
		}
	}
	if event.Iteration < 0 || event.Iteration > 1000 {
		return errors.New("telemetry iteration is outside bounds")
	}
	if event.Round < 0 || event.Round > 1000 {
		return errors.New("telemetry round is outside bounds")
	}
	if event.Divergences < 0 || event.Divergences > 1000 || event.VarianceRuns < 0 || event.VarianceRuns > 3 {
		return errors.New("telemetry clarity count is outside bounds")
	}
	if event.SpecBytes < 0 || event.SpecBytesDelta < -10_000_000 || event.SpecBytesDelta > 10_000_000 {
		return errors.New("telemetry spec byte count is outside bounds")
	}
	if event.DivergenceField != "" {
		if _, ok := allowedDivergenceFields[event.DivergenceField]; !ok {
			return fmt.Errorf("telemetry divergence_field is not allowlisted: %q", event.DivergenceField)
		}
	}
	if event.ExitReason != "" {
		if _, ok := allowedExitReasons[event.ExitReason]; !ok {
			return fmt.Errorf("telemetry exit_reason is not allowlisted: %q", event.ExitReason)
		}
	}
	return nil
}

func validateIdentifier(name, value string) error {
	if !safeIdentifier.MatchString(value) {
		return fmt.Errorf("telemetry %s is not a bounded identifier", name)
	}
	if strings.ContainsAny(value, "\\/ \t\r\n") {
		return fmt.Errorf("telemetry %s contains forbidden text", name)
	}
	return nil
}

func DecodeEvent(encoded []byte) (Event, error) {
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	var event Event
	if err := decoder.Decode(&event); err != nil {
		return Event{}, fmt.Errorf("decode telemetry event: %w", err)
	}
	if event.SchemaVersion == "" {
		event.SchemaVersion = "1"
		event.Level = NoData
		event.Module = NoData
		event.Stage = NoData
		event.Measurement = NoData
	}
	var trailing any
	if err := decoder.Decode(&trailing); err == nil {
		return Event{}, errors.New("decode telemetry event: trailing JSON")
	} else if !errors.Is(err, io.EOF) {
		return Event{}, fmt.Errorf("decode telemetry event: %w", err)
	}
	if err := event.Validate(); err != nil {
		return Event{}, err
	}
	return event, nil
}

func defaultModule(event string) string {
	switch event {
	case EventToolCall:
		return "runtime"
	case EventVerify:
		return "verify"
	case EventVerifyDenial:
		return "verify"
	case EventReviewDisagreement:
		return "workspace"
	case EventCleanExitClose:
		return "cleanexit"
	case EventClarityProbe:
		return "clarity"
	case EventApply, EventDiscard, EventRemediation, EventEscalation:
		return "workspace"
	case EventFlowNode:
		return "orchestration"
	case EventGateDecision:
		return "workspace"
	default:
		return "telemetry"
	}
}

func encodeEvent(event Event) ([]byte, error) {
	if err := event.Validate(); err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(event)
	if err != nil {
		return nil, fmt.Errorf("encode telemetry event: %w", err)
	}
	return append(encoded, '\n'), nil
}
