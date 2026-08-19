# tool-envelope Specification

## Purpose
Define the shared response envelope and the runtime pipeline (`internal/runtime/`) every jacu tool executes through, plus the MCP registration invariants enforced at the adapter boundary (`internal/mcpadapter/`). Every tool call, regardless of capability, produces the same envelope shape, passes the same spec validation, input cap, timeout, panic recovery, output cap, and structured logging, and is registered on the server under the same annotation and schema rules.

## Requirements

### Requirement: Uniform response envelope
Every tool SHALL return a single envelope object with exactly these fields: `status` (string), `summary` (non-empty string), `data` (tool-specific object), `artifacts` (array of strings), `warnings` (array of strings), `next_actions` (array of strings), and `trace_id` (string with prefix `tr_` followed by 16 lowercase hex characters, or `tr_unavailable` when randomness fails). At the MCP boundary, `artifacts`, `warnings`, and `next_actions` SHALL be serialized as empty JSON arrays (`[]`), never `null`, even when the runtime produced nil slices; inside the runtime `Result`, `data` is `omitempty`, so a nil `data` is omitted from the runtime's own JSON encoding while tool envelopes at the adapter always carry a concrete `data` object. The envelope SHALL be delivered both as MCP structured content and as a JSON-encoded `TextContent` block containing the same envelope.

#### Scenario: Envelope shape on any call
- **WHEN** any registered tool is called successfully over MCP
- **THEN** the structured content is an object with `status`, non-empty `summary`, object `data`, array-valued `artifacts`, `warnings`, and `next_actions`, and a `trace_id` prefixed `tr_`

#### Scenario: Null-vs-empty arrays
- **WHEN** a handler returns nil for `artifacts`, `warnings`, or `next_actions`
- **THEN** the client observes empty arrays, not null

#### Scenario: Text and structured content agree
- **WHEN** a tool result is returned
- **THEN** decoding the `TextContent` JSON yields the same envelope as the structured content

### Requirement: Status vocabulary
The envelope `status` SHALL be one of exactly four values with these meanings: `ok` (the tool completed and `data` is complete), `blocked` (a policy or validation gate refused the operation before doing work — oversized input, or a capability-level lint of level BLOCK — with diagnostic details in `data` where the capability provides them), `partial` (the tool completed but the runtime discarded oversized `data` to honor the output cap), and `failed` (the tool spec was invalid, the handler returned an error, or the handler panicked).

#### Scenario: Blocked by lint
- **WHEN** a capability's validation gate fires a BLOCK-level lint
- **THEN** the envelope status is `blocked` and the lint details are carried in `data`

#### Scenario: Handler failure
- **WHEN** a handler returns an error or panics
- **THEN** the envelope status is `failed` with summary `capability execution failed` and no partial data

### Requirement: Tool spec validation gates execution
Every capability SHALL declare a `ToolSpec` with name, version, risk, read-only, idempotent, open-world flags, a timeout, and positive input/output byte caps, and the runtime SHALL validate it on every execution before running the handler. Validation SHALL require: name matching `^jacu_[a-z0-9_]+$`, a positive timeout, positive `MaxInputBytes` and `MaxOutputBytes`, risk one of `safe`, `write`, or `destructive`, and that a `safe` tool is read-only. An invalid spec SHALL produce status `failed` with summary `invalid tool specification` without invoking the handler.

#### Scenario: Invalid spec never executes
- **WHEN** a capability has an empty name, a non-`jacu_` name, a zero timeout, an unknown risk, or is `safe` but not read-only
- **THEN** execution returns status `failed` and the handler is not called

#### Scenario: Safe implies read-only
- **WHEN** a spec declares risk `safe` with `ReadOnly` false
- **THEN** spec validation rejects it

### Requirement: Runtime pipeline order and protections
For every call the runtime SHALL, in order: validate the spec; reject input larger than `MaxInputBytes` with status `blocked` and summary `input exceeds tool limit` without invoking the handler; run the handler under a context bounded by the spec timeout; convert a handler error into status `failed`; recover any handler panic into status `failed`; assign a fresh `trace_id` when the result has none; apply the output cap; and emit one structured log record per execution containing the tool name, `trace_id`, status, duration, input bytes, and encoded output bytes.

#### Scenario: Oversized input blocked
- **WHEN** the JSON-encoded input exceeds the tool's `MaxInputBytes`
- **THEN** the result is `blocked` and the handler never runs

#### Scenario: Timeout enforced
- **WHEN** a handler waits on its context and the spec timeout is 200ms
- **THEN** the context is cancelled within the timeout and the resulting error yields status `failed`

#### Scenario: Panic recovery
- **WHEN** a handler panics
- **THEN** the call returns status `failed` instead of crashing the server

#### Scenario: Trace id always present
- **WHEN** any execution completes by any path
- **THEN** the result carries a `trace_id` matching `tr_` plus 16 hex characters

### Requirement: Per-tool output caps with exact byte limits
Each tool SHALL declare `MaxOutputBytes` measured against the JSON-encoded runtime result. The caps SHALL be 16384 bytes (16KB) by default — used by `jacu_project_inspect`, `jacu_mission_compile`, `jacu_memory_save`, `jacu_workspace_open`, `jacu_status` and its `jacu_workspace_status` alias, `jacu_apply`, and `jacu_discard` — and 32768 bytes (32KB) for the two tools that return bulk review data: `jacu_diff` and `jacu_memory_recall`.

#### Scenario: Default cap
- **WHEN** `jacu_memory_save` produces its result
- **THEN** the runtime caps the encoded result at 16384 bytes

#### Scenario: Raised cap for bulk tools
- **WHEN** `jacu_diff` or `jacu_memory_recall` produces its result
- **THEN** the cap applied is 32768 bytes

### Requirement: Output overflow degrades to partial with zeroed data
When the JSON-encoded result exceeds `MaxOutputBytes`, the runtime SHALL degrade the result in place: `status` becomes `partial`, `data` is reset to null (dropped from the runtime encoding via `omitempty`), and the warning `output exceeded inline limit; data reset to empty` is appended, while `summary`, `trace_id`, and the remaining envelope fields are preserved. The oversized data is discarded outright — as current behavior there is no artifact store to spill it to, so the `artifacts` field is not populated with any overflow reference. A result that fails to JSON-encode SHALL become status `failed` with summary `capability output is not serializable`, keeping its `trace_id`. Capabilities that need to keep `data` under overflow (such as `jacu_memory_recall`) SHALL shrink their own payload below the cap before returning, since the generic degradation preserves nothing of `data`.

#### Scenario: Oversized output becomes partial
- **WHEN** a handler returns a result whose encoding exceeds the cap
- **THEN** the caller receives status `partial` with the overflow warning, no `data`, and an encoded size far below the original (bounded by the small fixed envelope; verified in tests at no more than twice the cap)

#### Scenario: No artifact spill
- **WHEN** the overflow degradation fires
- **THEN** the discarded data is not written anywhere and `artifacts` gains no entry pointing at it

#### Scenario: Unserializable output
- **WHEN** a handler returns a result that cannot be marshalled to JSON
- **THEN** the result is status `failed` with summary `capability output is not serializable` and the original `trace_id`

### Requirement: MCP registration invariants
The server SHALL register exactly 13 tools with unique names (`jacu_project_inspect`, `jacu_mission_compile`, `jacu_workspace_open`, `jacu_status`, `jacu_workspace_status`, `jacu_diff`, `jacu_apply`, `jacu_discard`, `jacu_memory_save`, `jacu_memory_recall`, `jacu_verify`, `jacu_report`, `jacu_flow_run`). `jacu_workspace_status` is a compatibility alias for `jacu_status`; `jacu_verify` accepts optional diagnostic `argv` and there is no separate `jacu_run_command` registration. `jacu_report` is a read-only audit projection and is the only report tool. `jacu_flow_run` is the only orchestration submission tool; it does not create node-specific tools. Every tool SHALL carry annotations that mirror its runtime spec — `readOnlyHint`, `idempotentHint`, and explicit (non-nil) `destructiveHint` and `openWorldHint` — and a concrete output schema whose `data` property is a typed object with non-empty properties, never an open-ended `data`. The verification tool SHALL expose a concrete result schema whose `data` object names `verdict`, `commands`, `evidence_digest`, and `total_duration_ms`; its verdict enum SHALL contain exactly `pass`, `fail`, `timeout`, `blocked`, and `not_run`. Every tool description SHALL be a single short sentence on one line. The server SHALL advertise implementation name `jacu`, SHALL support protocol versions `2026-07-28` and `2025-11-25`, and SHALL NOT advertise the logging capability.

#### Scenario: Tool census and annotation mirror
- **WHEN** a client lists tools
- **THEN** exactly 13 uniquely named tools are returned, each with explicit destructive and open-world hints matching its spec
- **AND** `jacu_verify` is not read-only, not idempotent, not destructive, and closed-world

#### Scenario: Concrete verification data schemas
- **WHEN** either verification tool's output schema is inspected
- **THEN** `properties.data` names `verdict`, `commands`, `evidence_digest`, and `total_duration_ms`
- **AND** `data.verdict` advertises exactly `pass`, `fail`, `timeout`, `blocked`, `not_run`

#### Scenario: Report metadata is safe and concrete
- **WHEN** a client lists tools and inspects `jacu_report`
- **THEN** it is read-only, idempotent, closed-world, non-destructive, and its
  output data names `report`, `markdown`, and `digest`

#### Scenario: No logging capability
- **WHEN** a client completes initialization
- **THEN** the server capabilities contain no logging capability

### Requirement: STDIO transport keeps stdout protocol-clean
In `serve` mode the server SHALL run over the MCP STDIO transport, where stdout carries only single-line JSON-RPC frames. All diagnostics — including the runtime's per-execution structured log line — SHALL go to stderr, and the project root SHALL be resolved to an absolute, symlink-evaluated path before the server starts.

#### Scenario: Logs never corrupt the frame stream
- **WHEN** the server executes tools while serving over STDIO
- **THEN** execution logs are written to stderr and stdout carries only protocol frames

#### Scenario: Clean shutdown
- **WHEN** the serve process receives SIGINT or SIGTERM
- **THEN** the server stops without reporting a serve failure
