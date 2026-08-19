# ADR-020: Complete per-module observability

- Status: accepted
- Date: 2026-08-13
- Ratified: 2026-08-13
- Decider: Erick

> Succeeds ADR-018 (local telemetry v1). Does not revoke it: extends the same
> stream, keeping local-first, sanitization by construction and best-effort.

## Context

Telemetry v1 delivered a local, construction-sanitized stream with 13 fields
and 7 event types, read by `jacu stats` and projected in the `metrics` block
of `jacu_report`. It answers process questions — first-pass verify,
remediation iterations, escalation, auto-apply, missions/day, p95 per tool.

It does not answer product questions: net cost versus not using JACU; which
gate holds what, how often; where the context budget overflows; which module
gains and which pays; how often JACU interrupted a mission, and why; whether
evals cover the live path.

The mechanical reason: `Event` carries no size, cost, budget, gate or module
field. Two measurements already exist and are discarded: per-call input and
encoded output bytes on stderr, and `CostTrace` fields that never reach the
stream.

## Decision

Extend the local stream to complete per-module observability at two detail
levels, with no remote collection and no sanitization loosening.

### 1. Two levels, one stream

`JACU_TELEMETRY=off | user | full`, default `user`.

- `user` — what an operator needs: mission cost and size, interruptions,
  first-pass, plan-to-apply time, clean exit.
- `full` — owner mode: internals of each module for the scorecard.

### 2. Sanitization is an invariant, not a setting

`full` adds **numbers and closed enums. Never content.** Prompt, diff,
output, path and free text remain **unrepresentable in the event
constructor**, at every level.

Corollary: a string from a model never becomes a field. An allowlist denial
records `reason` (enum) and `program_known` (bool) — **never** the refused
program name, which is model-controlled text.

Debugging content, if ever needed, is another mechanism: an explicit dump,
off by default, outside the event constructor, and it is not called
telemetry.

### 3. No number without provenance

Every event with a cost number carries `measurement`:

| Value | Meaning |
|---|---|
| `exact_bytes` | bytes JACU delivered; measured, exact |
| `cli_reported_tokens` | tokens reported by the host CLI in a governed run |
| `estimated_tokens` | estimate with a declared estimator and declared margin |

Without `measurement`, the field is not emitted.

### 4. Counterfactuals are forbidden

The product **never** reports “tokens saved”, “would have cost” or equivalent.
JACU did not observe the world in which it did not exist.

`CostTrace.SavingsUnits` may be rendered only as *relative to profile
`baseline_profile_id`*, with the baseline named beside the number, and never
aggregated into a headline.

Gain claims are made by **paired eval**, not by telemetry.

### 5. Two declared measurement regimes

- **Governed** (`jacu run` headless, orchestration, clarity probe): JACU
  starts the CLI and captures `CostTrace`. There is a real token count.
- **Interactive** (MCP session): JACU is called, it does not call. It measures
  **its own bytes** and cannot know total session spend.

The scorecard labels both. A “net cost with JACU versus without” eval is
valid only in the governed regime, where both arms are measurable.

### 6. Module is a first-class dimension

The event gains `module` (closed enum) and `stage`. `tool` is not enough:
ledger, context pack and clarity probe act without a tool call.

### 7. A field exists to answer a named question

No field enters without the question it answers written in the design.

### 8. Versioned schema

The event gains `schema_version`. `stats` reads older versions without
breaking and labels missing fields as `no-data` — never as zero.

### 9. No client collection

Nothing leaves the machine. There is no upload channel, consent, remote
retention or PII review, because there is no transmission. The owner's
dataset is dogfood.

The OTLP exporter sanctioned by ADR-018 remains **outside the binary** and
serves the owner's machine: it reads JSONL and emits OTLP to a local
collector of theirs.

## Options considered

- **A — keep v1 and derive from structured logs:** stderr is not retained or
  queryable.
- **B — OpenTelemetry SDK in the binary:** rejected again by ADR-018
  (dependency weight, collector assumption, local-first).
- **C — extend the local stream at two levels (chosen):** keeps local-first,
  sanitization and best-effort; reuses `stats` and `jacu_report`.
- **D — local SQLite instead of JSONL:** new dependency and a binary file
  that is harder to audit by eye. Revisit only if dogfood volume makes
  `stats` slow.

## Consequences

Easier: prove or refute a net-cost claim with a public method (paired eval);
see which gate holds what; decide on an artifact store from `partial`
degradation counts; prioritize from a per-module scorecard.

Harder: publish a pretty number (counterfactuals forbidden; `no-data` is
honest); add a field on impulse (each needs a written question).

`CostTrace.SavingsUnits` needs a render rule before it appears in any report.
