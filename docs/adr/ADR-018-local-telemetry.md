# ADR-018: Sanitized local telemetry to measure outcomes

> Extended by ADR-020 (per-module schema v2, levels `off | user | full`,
> larger event catalogue). ADR-020 does not revoke this; it replaces the field
> contract and the opt-out mode described below.

- Status: accepted for implementation
- Date: 2026-08-12
- Deciders: Erick / JACU execution plan

## Context

JACU needs numbers for “is JACU good; does it bring gain?” Runtime logs debug
a call but are not a durable, queryable local series for first-pass
verification, remediation, escalation, auto-apply, time-to-apply, throughput,
tool duration and reverts. The telemetry path must preserve the local-first
threat model: an event never carries a prompt, diff, output, file path or free
text.

## Decision

Add capability `telemetry` as a local append-only JSONL sink. Files live at
`~/.jacu-harness/telemetry/events-YYYY-MM.jsonl`; the directory is `0700`,
files are `0600`, writes are atomic newline appends protected by `flock`, and
the writer garbage-collects files older than the twelve newest months during
its write flow.

Events are constructed from a closed typed allowlist only:

`ts`, `project_id`, `trace_id`, optional `run_id`, optional `mission_id`,
`event`, optional `tool`, `status`, optional `duration_ms`, optional
`ceremony`, optional `risk`, optional `verdict`, optional `iteration`,
optional `exit_reason`.

The event enum is `tool_call`, `verify`, `apply`, `discard`, `remediation`,
`escalation`, or `flow_node`. Unknown fields and unknown enum values are
rejected. Fuzz tests must demonstrate that mission content cannot leak into
encoded events. Telemetry is enabled by default, disabled only by
`JACU_TELEMETRY=off`, and is always best-effort: an append, lock, encoding or
GC failure logs a warning and never fails the observed operation.

The runtime pipeline emits one `tool_call` event at the existing per-call
logging boundary, reusing the same safe facts. Runner/autonomy paths emit
closed events for verification, apply, discard, flow nodes, remediation
iterations, escalation and automatic-apply outcomes. No new sensitive field is
added to make those measurements.

Consumption stays outside the MCP tool surface. `jacu stats [--since 30d]`
prints v1 metrics and `jacu_report` projects the same metrics: first-pass
verify percentage, remediation iterations per mission, escalation percentage,
auto-apply without intervention percentage, mission-to-apply p50/p95,
missions/day, p95 per tool, and apply-reverted percentage. Revert percentage
is derived by scanning Git history for revert commits referencing the
`Jacu-Run` trailer; it is a documented heuristic, not a runtime event.

OpenTelemetry is outside the binary. The SDK is a heavy new dependency,
commonly presumes a collector, and conflicts with the zero-dependency
local-first contract. A future separate exporter may read this JSONL and emit
OTLP; that exporter is not part of this ADR's implementation.

## Alternatives considered

- **OpenTelemetry SDK in the binary:** rejected for dependency weight,
  collector assumptions and local-first scope.
- **Remote analytics or collector:** rejected because telemetry is local and
  sanitized by product requirement.
- **SQLite or another embedded database:** rejected because JSONL is
  sufficient, inspectable, append-only and avoids a new persistence
  dependency.
- **Generic map or free-form event payload:** rejected because sanitization
  would be a convention instead of a construction invariant.
- **New MCP stats tool:** rejected by ADR-008's tool-surface ceiling; CLI and
  existing report are sufficient.

## Consequences

The telemetry series is inspectable and resilient to operation failures, but
it is local to each machine and has no automatic cross-machine aggregation.
Empty or disabled data is reported as no-data, not as zero. Revert derivation
can misclassify history and must never become an execution gate.
