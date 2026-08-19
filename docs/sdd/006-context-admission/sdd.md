---
sdd: 006-context-admission
program: jacu-one-shot
spec_id: spc_pending
branch: 006-context-admission
phase: docs/sdd/PROGRAM.md
adr: docs/adr/ADR-024-context-admission.md
status: draft
---

# 006 — Context admission

## Why

Every competing tool cuts context blind. Independent measurement showed what that
buys: RTK measured +7.6% cost at low effort while its own analytics reported
99.8% savings; caveman promised 65% and measured 8.5% (`PROGRAM.md`, Market
position). Compressing the surface without knowing the mission is optimising the
wrong variable, and it is why this program refuses to build a compressor.

JACU is in the one position where the honest version is possible: it holds the
mission contract when it decides what enters context. That turns the question
from "what can I shrink" into "what does this task require, does it fit the
budget, and can I prove nothing required was dropped".

Two of the three gaps the research left unoccupied land here: a per-task context
budget with pre-dispatch backpressure, and mechanical proof that compaction
preserved a fact. Both are refusals, not compressions. A task whose required
context does not fit is refused before dispatch, loudly, instead of being
silently truncated into a mission that will fail three steps later.

## Locked decisions

1. Nothing is cut without knowing the mission. Admission always reads the
   mission contract (PROGRAM decision 2).
2. No compressor is built or vendored. The category optimises the wrong
   variable (`PROGRAM.md`, Out of scope).
3. No counterfactual metric. The ledger never reports what a dropped item
   "would have cost" (PROGRAM decision 5; ADR-020).
4. The pack is deterministic: same mission, same repository state, same bytes.
   A non-reproducible pack cannot be audited (ADR-013).
5. Zero new MCP tools; admission runs inside the existing mission path and a CLI
   subcommand (ADR-008; PROGRAM decision 3).

## Out of scope

- Summarising, compressing or rewriting any content to make it fit. Admission
  selects and refuses; it never transforms.
- Calling a model to decide relevance. Selection is deterministic from the
  mission contract (PROGRAM decision 1).
- A budget in tokens. The unit is bytes, exactly measured; token counts are
  provider-specific estimates and `measurement` exists to keep that distinction
  visible (ADR-020).
- Persisting packs. A pack is derived and reproducible; storing it would create a
  second source of truth.

## Write scope

**Allowed**

```
docs/sdd/006-context-admission/**
docs/sdd/specs/context/spec.md
docs/adr/ADR-024-context-admission.md
docs/relatorios/sdd-006-execucao.md
internal/capability/context/**
internal/capability/ledger/**
internal/capability/missioncompile/**
internal/telemetry/**
cmd/jacu/context.go
cmd/jacu/context_test.go
cmd/jacu/main.go
skills/jacu-mission/SKILL.md
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

### Requirement: The pack is deterministic

The system SHALL produce byte-identical packs for the same mission and
repository state, and SHALL order included items by a total order that does not
depend on filesystem iteration.

#### Scenario: two runs produce identical bytes

- **WHEN** the same mission is packed twice against an unchanged repository
- **THEN** the two packs are byte-identical, including item order

Delta: ADDED

#### Scenario: a changed file changes the pack digest

- **WHEN** an included file changes by one byte
- **THEN** the pack digest changes, so the pack cannot silently go stale

Delta: ADDED

### Requirement: The ledger refuses before dispatch

The system SHALL decide `admit`, `refuse` or `degrade` before the first tool
call, and SHALL refuse when a required item does not fit the budget.

#### Scenario: a required item that does not fit refuses the task

- **WHEN** an item the mission marks required exceeds the remaining budget
- **THEN** the decision is `refuse` with `required_overflow` true, and no tool
  call is made

Delta: ADDED

#### Scenario: optional items degrade instead of refusing

- **WHEN** only optional items exceed the budget
- **THEN** the decision is `degrade`, the dropped count is recorded, and the task
  proceeds

Delta: ADDED

#### Scenario: the refusal names a reason, never a guess

- **WHEN** the ledger refuses
- **THEN** the event carries a closed `reason` enum, `budget_bytes`,
  `requested_bytes` and `remaining_bytes`, and no counterfactual field

Delta: ADDED

### Requirement: Anchor preservation is proven, not asserted

The system SHALL extract anchors from the mission contract and SHALL verify
mechanically that every anchor survives into the pack.

#### Scenario: a dropped anchor fails the check

- **WHEN** an anchor present in the mission is absent from the pack
- **THEN** `context.anchor` reports `fail` with `anchors_lost`, and the task does
  not dispatch

Delta: ADDED

#### Scenario: coverage is measured against what was required

- **WHEN** the pack is built
- **THEN** `context.pack` records `coverage_bps` computed from
  `items_required` and `items_included`, not from total items available

Delta: ADDED

## Non-goals

- Beating a compressor on ratio. The claim is that nothing required was dropped,
  not that the pack is small.
- Working without a mission. Admission has no meaning outside a compiled
  mission and refuses to run standalone.

## Open decisions

- [x] none — resolved before this document was cut. ADR-024 records the byte
  unit, the required/optional distinction, the total order and the anchor
  extraction rule; T1 writes it and the owner ratifies separately.

## Tasks

| # | Task | Files | Verify | Status | Evidence |
|---|---|---|---|---|---|
| T1 | Write ADR-024: bytes as the unit, required versus optional, the total order, anchor extraction, and why nothing is transformed | `docs/adr/ADR-024-context-admission.md` | `wc -l` under 120; owner ratifies separately | todo | |
| T2 | RED: pack determinism — two runs byte-identical, order independent of filesystem iteration | `internal/capability/context/pack_test.go` | `go test ./internal/capability/context -race` fails on absence | todo | |
| T3 | GREEN: deterministic pack with a total order and a digest | `internal/capability/context/pack.go` | `go test ./internal/capability/context -race` | todo | |
| T4 | RED: a one-byte change to an included file changes the digest | `internal/capability/context/digest_test.go` | `go test ./internal/capability/context -race` fails | todo | |
| T5 | GREEN: pack digest over content, not over paths | `internal/capability/context/digest.go` | `go test ./internal/capability/context -race` | todo | |
| T6 | RED: anchor extraction from the mission contract, and a check that fails when an anchor is missing from the pack | `internal/capability/context/anchor_test.go` | `go test ./internal/capability/context -race` fails | todo | |
| T7 | GREEN: anchor extraction and preservation proof | `internal/capability/context/anchor.go` | `go test ./internal/capability/context -race` | todo | |
| T8 | RED: fuzz over arbitrary mission and file input — no panic, always a pack or a typed finding | `internal/capability/context/fuzz_test.go` | `go test ./internal/capability/context -run Fuzz -fuzztime=30s` fails | todo | |
| T9 | GREEN: fix whatever the fuzz finds | `internal/capability/context/**` | fuzz clean | todo | |
| T10 | RED: ledger decides `admit`, `refuse`, `degrade`; required overflow refuses and makes no tool call | `internal/capability/ledger/decide_test.go` | `go test ./internal/capability/ledger -race` fails | todo | |
| T11 | GREEN: ledger with the closed reason enum | `internal/capability/ledger/decide.go` | `go test ./internal/capability/ledger -race` | todo | |
| T12 | RED: the refusal happens **before** dispatch — a test that fails if any tool call was made | `internal/capability/ledger/predispatch_test.go` | `go test ./... -race` fails | todo | |
| T13 | GREEN: pre-dispatch backpressure wired into the mission path | `internal/capability/missioncompile/**`, `internal/capability/ledger/**` | `go test ./... -race` | todo | |
| T14 | RED: `context.pack`, `context.anchor`, `context.handoff` and `ledger.decision` events, with a test proving no counterfactual field exists | `internal/capability/context/telemetry_test.go` | `go test ./internal/capability/context -race` fails | todo | |
| T15 | GREEN: emission through the v2 constructor | `internal/capability/context/telemetry.go`, `internal/capability/ledger/telemetry.go` | `go test ./... -race` | todo | |
| T16 | RED/GREEN: `jacu context pack \| explain`, exit codes 0/1/2, `--json` on stdout | `cmd/jacu/context.go`, `cmd/jacu/context_test.go` | `go test ./cmd/... -race` | todo | |
| T17 | Authorize the subcommand in the verify allowlist | `.jacu/verify-allowlist.json` | `jacu_verify` returns a verdict for that argv | todo | |
| T18 | Teach the skill that a refusal is an answer, not an error to retry around | `skills/jacu-mission/SKILL.md` | `go test ./internal/mcpadapter -run Skills -race` | todo | |
| T19 | Write the living capability spec | `docs/sdd/specs/context/spec.md` | `go run ./cmd/jacu sdd lint --all` exits 0 | todo | |
| T20 | Eval on the live path: ten missions, coverage and refusal rate, with every refusal inspected to confirm it was correct | `docs/evals/context-admission.md` | report with n, coverage distribution and refusal review | todo | |
| T21 | Confirm the MCP surface is untouched | — | `go test -tags=e2e ./test/e2e/ -run Governed` reports 13 tools | todo | |
| T22 | Write the execution report | `docs/relatorios/sdd-006-execucao.md` | PR with the hosted run link | todo | |

## Done

| Level | Proof |
|---|---|
| Core | `go test ./internal/capability/context ./internal/capability/ledger -race` green; fuzz clean; two packs byte-identical |
| Wiring | a required overflow refuses with zero tool calls; `bash scripts/verify.sh` green |
| E2E | `go test -tags=e2e ./test/e2e/` green with 13 tools under the ratchet |
| Eval | ten missions on the live path, every refusal reviewed and confirmed correct |

## Follow-ups

- Cross-mission handoff sizing, once `context.handoff` has data from real
  programs.
- Revisiting the budget default from the observed coverage distribution rather
  than from a number someone picked.
