# telemetry Specification

## Purpose
Provide trustworthy local measurements of governed JACU work while ensuring that telemetry is sanitized by construction, local-only, and unable to change the outcome of the operation it observes.
## Requirements
Normal segments use `events-YYYY-MM-NNNN.jsonl` and rotate before 8 MiB; legacy `events-YYYY-MM.jsonl` remains readable. Retention keeps at most twelve UTC calendar months and 128 MiB total, deleting oldest complete regular segments only. Reads skip segments strictly older than the requested UTC month.
### Requirement: Local append-only event stream

The telemetry capability SHALL write one JSON object per line to `~/.jacu-harness/telemetry/events-YYYY-MM.jsonl` or its rotated `events-YYYY-MM-NNNN.jsonl` successor. The telemetry directory SHALL have mode `0700`, regular event files SHALL have mode `0600`, writes SHALL be atomic appends protected by an inter-process lock, and the writer SHALL retain at most twelve UTC calendar months and 128 MiB of regular segments. The segment receiving an append is retained even when one defensive oversized event temporarily exceeds the total cap.

#### Scenario: Default write creates a protected monthly stream

- **WHEN** a valid event is emitted with telemetry enabled and no monthly file exists
- **THEN** the capability creates the directory and file with the required modes and appends exactly one newline-terminated JSON object

#### Scenario: Concurrent writers do not interleave records

- **WHEN** multiple processes emit events to the same monthly stream concurrently
- **THEN** every line decodes as one complete event and no event bytes are interleaved

#### Scenario: Retention removes only old regular segments

- **WHEN** a write completes while retention limits are exceeded
- **THEN** the oldest complete regular segments are removed without following symlinks and the receiving segment remains available

### Requirement: Closed sanitized event contract

Every event SHALL contain only the allowlisted fields `schema_version`, `level`, `ts`, `project_id`, `trace_id`, optional `run_id`, optional `mission_id`, optional `program_id`, `module`, `stage`, `event`, optional `tool`, `status`, optional `duration_ms`, optional `measurement`, byte counts, typed apply detail, optional `ceremony`, `risk`, `verdict`, `iteration`, `exit_reason`, denial `reason`, and typed review resolution. `event` SHALL be one of `tool_call`, `verify`, `apply`, `discard`, `remediation`, `escalation`, `flow_node`, `gate.decision`, `verify.denial`, or `review.disagreement`. Values SHALL be typed bounded identifiers or closed enums; prompts, diffs, outputs, file paths, and free text SHALL be unrepresentable in the event constructor.

#### Scenario: Unknown fields are rejected

- **WHEN** an event payload contains a key outside the allowlist
- **THEN** validation rejects the payload and the writer emits no line

#### Scenario: Mission content never reaches telemetry

- **WHEN** fuzz input contains arbitrary mission text, paths, diffs, output, or prompt-like values
- **THEN** event construction either rejects the input or emits only the closed allowlisted fields and none of that content appears in the encoded event

#### Scenario: Invalid enum is rejected

- **WHEN** an event uses an unknown event kind, status, ceremony, risk, verdict, or exit reason
- **THEN** validation rejects the event before filesystem access

### Requirement: Best-effort emission and opt-out

Telemetry SHALL be enabled by default when `JACU_TELEMETRY` is unset or has any value other than `off`. `JACU_TELEMETRY=off` SHALL suppress event writes. A filesystem, lock, encoding, or retention error SHALL produce a bounded warning through the existing logger and SHALL NOT change the governed operation's result.

#### Scenario: Opt-out suppresses local writes

- **WHEN** `JACU_TELEMETRY=off` is set before an operation
- **THEN** the operation proceeds and no telemetry directory or event file is required

#### Scenario: Store failure does not fail a tool call

- **WHEN** telemetry cannot append because its destination or lock is unavailable
- **THEN** the observed tool or runner returns its original result and a warning is recorded

### Requirement: Runtime and autonomy coverage

The runtime SHALL emit a `tool_call` event for each capability invocation using the same tool, trace, status, and duration facts already available at the pipeline boundary. Verification, apply, discard, flow-node, remediation, escalation, and auto-apply outcomes SHALL emit their corresponding event kinds with no additional sensitive fields.

#### Scenario: One runtime invocation produces one tool-call event

- **WHEN** a capability returns any supported status
- **THEN** telemetry records one `tool_call` event with the capability name, trace id, status, and bounded duration while preserving the original result

#### Scenario: Remediation and escalation are measurable

- **WHEN** autonomy retries remediation, escalates, or completes an automatic apply
- **THEN** telemetry records the corresponding closed event with iteration or verdict fields sufficient for v1 statistics

### Requirement: Local statistics diagnostic

The CLI SHALL expose `jacu stats` with an optional `--since 30d` duration filter. It SHALL read only local telemetry and print v1 metrics for first-pass verify percentage, remediation iterations per mission, escalation percentage, auto-apply without intervention percentage, mission-to-apply p50/p95, missions per day, p95 duration per tool, and heuristic apply-revert percentage. Empty input SHALL be reported honestly without invented values.

#### Scenario: Stats uses the default window

- **WHEN** `jacu stats` runs without `--since`
- **THEN** it reads the default thirty-day window and prints all v1 metric names with measured values or an explicit no-data representation

#### Scenario: Invalid duration is rejected

- **WHEN** `--since` is missing a duration or contains an unsupported value
- **THEN** the CLI exits non-zero with a bounded usage error and does not read or write event data

### Requirement: Heuristic revert derivation

The telemetry statistics reader SHALL derive apply reverts by scanning Git history for revert commits whose message references the `Jacu-Run` trailer. The result SHALL be labeled heuristic, SHALL not require a collection event, and SHALL not claim certainty when Git history is unavailable.

#### Scenario: Revert trailer is counted

- **WHEN** Git history contains a revert commit referencing a `Jacu-Run` trailer within the selected window
- **THEN** the statistics include it in the heuristic reverted-apply numerator

#### Scenario: Missing Git history remains honest

- **WHEN** the selected project has no readable Git history
- **THEN** the heuristic metric is reported as unavailable or no-data and other telemetry metrics remain readable

### Requirement: Full module scorecard

The CLI SHALL accept `jacu stats --full`, print one section per known module,
print `no-data` for empty modules, and print the measurement beside every byte
or cost line. The report metrics block SHALL expose user-level mission bytes,
interruptions, and clean-exit fields as no-data until their emitting modules
exist.
