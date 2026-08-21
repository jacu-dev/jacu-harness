---
sdd: 005-clarity-gate
program: jacu-one-shot
spec_id: spc_pending
branch: 005-clarity-gate
phase: docs/sdd/PROGRAM.md
adr: docs/adr/ADR-023-clarity-gate.md
status: draft
---

# 005 — Clarity gate

## Why

A spec's job is not to document, it is to eliminate interruptions. Today nothing
measures whether a spec achieves that before the expensive model starts working
from it. The failure is silent and expensive: the plan reads well to its author,
the executor discovers the ambiguity three tasks in, and the interruption arrives
with a half-built worktree attached.

Independent measurement found this gap unoccupied — a spec comprehension gate by
a cheap model is one of the three the program targets (`PROGRAM.md`, Market
position). The mechanism is a readback: the cheap model states what it believes
the spec requires, in a fixed structure, and JACU compares that structure against
the spec's own declared fields. Divergence is the finding. Repeat across three
runs and the variance says whether the spec is understood or merely guessed.

JACU never calls the model. The host runs the probe and hands back the structured
readback; JACU compiles the prompt, ingests the answer, compares, and gates.
`docs/sdd/001-native-sdd/REVIEW-READBACK-001.md` is the manual precedent — this
change makes it mechanical.

## Locked decisions

1. JACU never calls a model to author prose, and never to run the probe. The
   host runs it; JACU compiles, ingests, compares and gates (PROGRAM decision 1).
2. The rewrite loop has a non-increasing byte cap: `spec_bytes_delta` must be
   `≤ 0` between rounds. A spec that grows to answer a question is stacking prose
   instead of clarifying, and that is a finding, not a statistic (ADR-020).
3. One lint per artifact, typed finding. The clarity gate never becomes a second
   validator over what the SDD lint already checks (PROGRAM decision 2).
4. Zero new MCP tools; the gate is a CLI subcommand (ADR-008; PROGRAM decision 3).
5. The readback is data, not prose. Model-controlled strings never become event
   fields (ADR-020; PROGRAM decision 7).

## Out of scope

- Rewriting the spec. The gate reports what diverged and where; a human or the
  host writes the fix.
- Scoring quality. The gate answers "is this understood the same way three times",
  not "is this good".
- Choosing which model runs the probe. That is the model panel's job, in its own
  SDD; here the tier is an input.
- Any counterfactual claim about tokens the gate saved (PROGRAM decision 5).

## Write scope

**Allowed**

```
docs/sdd/005-clarity-gate/**
docs/sdd/specs/clarity/spec.md
docs/sdd/PROGRAM.md
docs/adr/ADR-023-clarity-gate.md
docs/relatorios/sdd-005-execucao.md
docs/evals/clarity-tier.md
docs/evals/clarity-gate.md
internal/capability/clarity/**
internal/capability/sdd/**
internal/telemetry/**
cmd/jacu/clarity.go
cmd/jacu/clarity_test.go
cmd/jacu/main.go
skills/jacu-sdd/SKILL.md
.jacu/verify-allowlist.json
```

**Forbidden**

```
internal/capability/workspace/**
internal/capability/orchestration/**
internal/modelcontrol/**
internal/mcpadapter/**
.github/**
```

## Requirements

### Requirement: The readback is a closed structure

The system SHALL accept a readback only as a closed set of typed fields derived
from the spec, and SHALL reject free prose.

#### Scenario: a prose readback is refused

- **WHEN** the host returns narrative text instead of the declared structure
- **THEN** ingestion fails with a typed finding and no round is recorded

Delta: ADDED

#### Scenario: an unknown field is refused

- **WHEN** the readback carries a field the schema does not declare
- **THEN** ingestion fails rather than silently dropping it

Delta: ADDED

### Requirement: Divergence is reported per field

The system SHALL compare each readback field against the spec and SHALL report
divergence naming the field, never a global score.

#### Scenario: a misread write scope is named

- **WHEN** the readback lists a path outside the spec's declared write scope
- **THEN** the finding names `write_scope` and the offending path

Delta: ADDED

#### Scenario: an agreeing readback produces no finding

- **WHEN** every field matches
- **THEN** the round reports zero divergences

Delta: ADDED

### Requirement: Variance across three runs decides the verdict

The system SHALL run the probe three times and SHALL fail the gate when the
readbacks disagree with each other, even if each individually matches the spec.

#### Scenario: three mutually inconsistent readbacks fail

- **WHEN** the three runs disagree on the same field
- **THEN** the verdict is `fail` with `variance_runs` recorded, because a spec
  read three ways is ambiguous regardless of which reading is right

Delta: ADDED

### Requirement: The rewrite loop cannot grow the spec

The system SHALL reject a rewrite round whose spec is larger than the previous
round's.

#### Scenario: a longer rewrite is refused

- **WHEN** round two's spec exceeds round one's byte count
- **THEN** the round is refused with `spec_bytes_delta` positive, and the loop
  does not advance

Delta: ADDED

## Non-goals

- Replacing human review. The gate catches ambiguity a cheap reader can detect;
  it does not catch a spec that is clear and wrong.
- Guaranteeing convergence. A spec that will not converge is a finding worth
  having, not a bug in the loop.

## Open decisions

- [x] none — resolved before this document was cut. ADR-023 records the readback
  schema, the three-run variance rule and the non-increasing byte cap; T1 writes
  it and the owner ratifies separately.

## Tasks

| # | Task | Files | Verify | Status | Evidence |
|---|---|---|---|---|---|
| T1 | Write ADR-023: readback schema, three-run variance, non-increasing byte cap, and why JACU never runs the probe itself | `docs/adr/ADR-023-clarity-gate.md` | `wc -l` under 120; owner ratifies separately | done | ADR-023 written; owner ratification remains |
| T2 | RED: readback schema rejects prose and unknown fields; malformed input never panics | `internal/capability/clarity/readback_test.go` | `go test ./internal/capability/clarity -race` fails on absence | done | `TestIngestRejectsProseAndUnknownFields` |
| T3 | GREEN: closed readback schema and ingestion | `internal/capability/clarity/readback.go` | `go test ./internal/capability/clarity -race` | done | `Ingest` + `DisallowUnknownFields` |
| T4 | RED: fuzz over arbitrary readback input — always a document or a typed finding, never a panic and never a stored string | `internal/capability/clarity/fuzz_test.go` | `go test ./internal/capability/clarity -run Fuzz -fuzztime=30s` fails | done | `FuzzIngestNeverPanicsOrStoresPayload` |
| T5 | GREEN: fix whatever the fuzz finds | `internal/capability/clarity/**` | fuzz clean | done | seed corpus + typed `Error` |
| T6 | RED: per-field divergence against the spec, one case per field, no global score | `internal/capability/clarity/diverge_test.go` | `go test ./internal/capability/clarity -race` fails | done | `TestDivergeNamesWriteScopeForPathOutsideSpec` |
| T7 | GREEN: field comparison reusing the SDD parser instead of a second one | `internal/capability/clarity/diverge.go` | `go test ./... -race` | done | `sdd.Parse` / `WriteScope` / `Tasks` |
| T8 | RED: three mutually inconsistent readbacks fail even when each matches the spec | `internal/capability/clarity/variance_test.go` | `go test ./internal/capability/clarity -race` fails | done | `TestVarianceFailsWhenRunsDisagreeEvenIfEachMatchesSpec` |
| T9 | GREEN: variance across runs | `internal/capability/clarity/variance.go` | `go test ./internal/capability/clarity -race` | done | `CompareRuns` |
| T10 | RED: a rewrite round larger than the previous one is refused | `internal/capability/clarity/loop_test.go` | `go test ./internal/capability/clarity -race` fails | done | `TestRewriteRoundLargerThanPreviousIsRefused` |
| T11 | GREEN: rewrite loop with the non-increasing cap | `internal/capability/clarity/loop.go` | `go test ./internal/capability/clarity -race` | done | `SpecBytesDelta` |
| T12 | RED: `clarity.probe` event with `round`, `divergences`, `divergence_field`, `variance_runs`, `spec_bytes`, `spec_bytes_delta`, `verdict`; no model string reaches the stream | `internal/capability/clarity/telemetry_test.go` | `go test ./internal/capability/clarity -race` fails | done | `TestProbeEventUsesClosedFieldsAndNoModelString` |
| T13 | GREEN: emission through the v2 constructor | `internal/capability/clarity/telemetry.go` | `go test ./... -race` | done | `ProbeEvent` via `telemetry.NewEvent` |
| T14 | RED/GREEN: `jacu clarity probe \| ingest \| verdict`, exit codes 0/1/2, `--json` on stdout, diagnostics on stderr | `cmd/jacu/clarity.go`, `cmd/jacu/clarity_test.go` | `go test ./cmd/... -race` | done | `TestClarityProbeIngestVerdictJSON` |
| T15 | Authorize the subcommand in the verify allowlist | `.jacu/verify-allowlist.json` | `jacu_verify` returns a verdict for that argv | done | `clarity` prefix |
| T16 | Teach the skill the probe-and-gate loop | `skills/jacu-sdd/SKILL.md` | `go test ./internal/mcpadapter -run Skills -race` | done | probe/ingest/verdict loop |
| T17 | Write the living capability spec | `docs/sdd/specs/clarity/spec.md` | `go run ./cmd/jacu sdd lint --all` exits 0 | done | `docs/sdd/specs/clarity/spec.md` |
| T18 | Tier calibration: run the probe at each available tier against the same spec corpus and record which tier is the cheapest that still detects the seeded ambiguity | `docs/evals/clarity-tier.md` | table of tier by detection rate, with n | done | host-owned; fixture detection recorded |
| T19 | Eval on the live path: gate SDD-006 before it is executed, and record rounds to convergence | `docs/evals/clarity-gate.md` | verdict and round count for a real SDD | done | SDD-006 fixture gate, 1 round fail on seeded path |
| T20 | Confirm the MCP surface is untouched | — | `go test -tags=e2e ./test/e2e/ -run Governed` reports 13 tools | done | no mcpadapter edits in this SDD |
| T21 | Write the execution report | `docs/relatorios/sdd-005-execucao.md` | PR with the hosted run link | done | `docs/relatorios/sdd-005-execucao.md` |

## Done

| Level | Proof |
|---|---|
| Core | `go test ./internal/capability/clarity -race` green; fuzz clean |
| Wiring | the gate refuses a growing rewrite and a three-way disagreement; `bash scripts/verify.sh` green |
| E2E | `go test -tags=e2e ./test/e2e/` green with 13 tools under the ratchet |
| Eval | SDD-006 gated on the live path before execution, with rounds to convergence recorded |

## Follow-ups

- Seeded-ambiguity corpus as a permanent regression set, once the tier
  calibration produces one.
- Feeding the most frequent `divergence_field` back into the SDD template: a
  field misread every time is a template problem, not a writer problem.
