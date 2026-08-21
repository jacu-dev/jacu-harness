# report Specification

## Purpose
Project report projects deterministic workspace, verification, telemetry and delivery state without inventing unavailable evidence.
## Requirements
### Requirement: Versioned report contract
The report model SHALL use `schema_version: "1"`, `kind` equal to `adhoc`,
`plan`, or `audit`, a non-empty title, a summary, and exactly eight typed
blocks named `summary`, `steps`, `decision`, `risks`, `flow`, `chart`, `table`,
and `metrics`. E1 SHALL generate `audit` reports from persisted workspace
state; it SHALL accept the other kinds only for contract validation.

#### Scenario: Audit report has the eight blocks
- **WHEN** the report walker reads a project with zero or more runs
- **THEN** it returns a valid version-one audit report with all eight blocks
  present and JSON arrays encoded as arrays, never null

#### Scenario: Invalid report kind is rejected
- **WHEN** a report has an unknown kind or schema version
- **THEN** validation fails before Markdown projection

### Requirement: Structured state is the source
The audit walker SHALL derive runs, missions, and programs from structured
runstate data. It SHALL never parse Markdown, HTML, log lines, or report text to
reconstruct state, and it SHALL not mutate runstate or the project worktree.

#### Scenario: State survives prose changes
- **WHEN** a run objective or report summary changes without changing its
  structured lifecycle fields
- **THEN** the projected status, identities, and flow remain derived from the
  structured fields and no Markdown is read

### Requirement: reportgen owns Markdown projection
`jacu report` and `jacu_report` SHALL call `internal/reportgen` for the
Markdown projection. That package SHALL contain no MCP types. Structured JSON
in `internal/report` remains the source of state.

#### Scenario: both surfaces call one projector
- **WHEN** `jacu report` and `jacu_report` run
- **THEN** both call `reportgen.Markdown`

### Requirement: Deterministic projections
For the same structured state, canonical JSON, the report digest, and Markdown
output SHALL be byte-identical. Runs, flow nodes, flow edges, metrics, and table
rows SHALL be sorted by stable identifiers, and reports SHALL not contain a
current timestamp.

#### Scenario: Repeated projection is byte-identical
- **WHEN** the same runstate is walked twice
- **THEN** JSON, digest, and Markdown are identical

### Requirement: Safe report text
Report text SHALL be bounded, normalize line controls, and redact secret-like
values and absolute paths. The audit report SHALL not copy verification argv,
stdout, transcripts, or absolute worktree paths. Markdown SHALL escape table
delimiters and remain a projection only; it SHALL never be parsed as state.

#### Scenario: Sensitive mission text is redacted
- **WHEN** structured state contains a secret-like value, newline, or absolute
  path in a text field
- **THEN** the projection replaces the sensitive portion with a redaction
  marker or path marker and emits no raw value

### Requirement: Headless statusline
The `jacu statusline` command SHALL emit one stdout line containing active
mission identity, phase, and program identity when available. Model and cost
fields unavailable in the v1 runstate SHALL be reported as `not measured`; an
idle project SHALL report `idle`. It SHALL not start HTTP, open a browser, or
write a report file.

#### Scenario: Active program is visible
- **WHEN** an open run belongs to a program
- **THEN** statusline includes its run, mission, phase, program, and cursor in
  one line with honest `not measured` fields for model and cost

#### Scenario: Empty workspace is honest
- **WHEN** no persisted runs are present
- **THEN** statusline reports `idle` and does not invent a mission or cost

### Requirement: report --json is quality.json

`jacu report --json` SHALL emit the versioned audit JSON object on stdout.
That object is the `quality.json` artifact. Markdown remains the default
projection and SHALL never be parsed as state. The MCP `jacu_report` envelope
stays a compact summary plus Markdown and digest; it is not `quality.json`.

#### Scenario: CLI JSON is the audit object

- **WHEN** `jacu report --json` runs
- **THEN** stdout is `schema_version` 1, `kind` `audit`, with the eight blocks,
  and is not a capability envelope

#### Scenario: default output stays Markdown

- **WHEN** `jacu report` runs without `--json`
- **THEN** stdout is the deterministic Markdown projection of the same audit

### Requirement: Telemetry metrics projection

The audit report SHALL populate its existing `metrics` block from the local telemetry statistics reader when telemetry data is available. It SHALL include the v1 metric names, the selected time window, and an explicit no-data value when a metric cannot be measured; it SHALL preserve deterministic ordering and report the apply-revert value as heuristic.

#### Scenario: Report includes measured telemetry

- **WHEN** local events exist in the selected window
- **THEN** `jacu_report` and its Markdown projection contain the v1 telemetry metrics in stable order without copying event payloads

#### Scenario: Report is honest with no events

- **WHEN** no local events exist or telemetry is disabled
- **THEN** the report remains valid and identifies telemetry metrics as no-data rather than inventing zero-valued performance claims
