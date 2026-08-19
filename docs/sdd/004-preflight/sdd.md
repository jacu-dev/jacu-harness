---
sdd: 004-preflight
program: jacu-one-shot
spec_id: spc_pending
branch: 004-preflight
phase: docs/sdd/PROGRAM.md
adr: docs/adr/ADR-022-preflight.md
status: draft
---

# 004 — Pre-flight

## Why

One-shot means zero *predictable* interruptions (PROGRAM decision 8). Today every
interruption is discovered mid-mission, one at a time, at the worst moment: the
command is not on the allowlist, the path does not exist, the credential is
absent, the spec left a question open. Each one costs a full round trip to the
human, and they arrive serially because nothing looks ahead.

The classes are already enumerated. The telemetry v2 catalogue names them for
`preflight.check`: `allowlist`, `program_not_on_path`, `path_missing`,
`path_not_writable`, `credential_absent`, `network_undeclared`, `doc_missing`,
`open_questions`. Every one is decidable before the first tool call, from the
compiled mission and the local environment, with no model involved.

This change turns a serial trickle of interruptions into one batch asked up
front, and makes `mission.interruption` — the metric the whole program is
measured by — start counting.

## Locked decisions

1. JACU never calls a model to author prose. Pre-flight inspects the compiled
   mission and the environment; it does not ask a model what might go wrong
   (PROGRAM decision 1).
2. Deny-by-default. A check that cannot reach a verdict reports the gap; it
   never assumes the environment is fine (`docs/threat-model.md`).
3. One lint per artifact, typed finding (PROGRAM decision 2).
4. Zero new MCP tools; pre-flight runs inside `jacu_mission_compile` and through
   a CLI subcommand (ADR-008; PROGRAM decision 3).
5. `mission.interruption` and `preflight.check` follow the v2 envelope and carry
   only numbers and closed enums (ADR-020).

## Out of scope

- Fixing what pre-flight finds. It reports and blocks; installing a missing
  program or granting a credential stays with the human.
- Reading a credential's value. Pre-flight checks presence and reachability,
  never content (`docs/threat-model.md`).
- Network probing. `network_undeclared` means the mission declared no network
  need while a step requires one; it does not dial anything.
- Guessing the missing answer for an open question. That is SDD-005's job and it
  is a comprehension gate, not a completion.

## Write scope

**Allowed**

```
docs/sdd/004-preflight/**
docs/sdd/specs/preflight/spec.md
docs/adr/ADR-022-preflight.md
docs/relatorios/sdd-004-execucao.md
docs/evals/preflight.md
internal/capability/preflight/**
internal/capability/missioncompile/**
internal/capability/verify/exec.go
internal/capability/verify/tool_test.go
internal/capability/workspace/apply_test.go
internal/telemetry/**
cmd/jacu/preflight.go
cmd/jacu/preflight_test.go
cmd/jacu/main.go
skills/jacu-mission/SKILL.md
.jacu/verify-allowlist.json
```

**Forbidden**

```
internal/capability/orchestration/**
internal/modelcontrol/**
.github/**
```

The existing workspace fixture `internal/capability/workspace/apply_test.go`
is the single test-only exception recorded in the execution report: preflight
changes the refusal point from apply-time to mission compilation. No workspace
production file is in scope.

## Requirements

### Requirement: Every predictable interruption class is checked before dispatch

The system SHALL evaluate all eight classes before the first tool call and SHALL
report each gap under exactly one class.

#### Scenario: a command outside the allowlist is caught before the run starts

- **WHEN** the mission declares a verification command absent from the allowlist
- **THEN** pre-flight reports `allowlist` naming the missing prefix, and the
  mission does not dispatch

Delta: ADDED

#### Scenario: an unresolvable check reports rather than assumes

- **WHEN** a class cannot be evaluated in this environment
- **THEN** it is reported as unresolved, and pre-flight refuses; it never
  defaults to pass

Delta: ADDED

### Requirement: The human is asked once

The system SHALL collect every gap into a single batch and SHALL present it as
one question set, never one gap at a time.

#### Scenario: three gaps produce one batch

- **WHEN** the allowlist, a path and a credential are all missing
- **THEN** one batch lists all three, and no partial dispatch happened first

Delta: ADDED

#### Scenario: a clean pre-flight asks nothing

- **WHEN** every class passes
- **THEN** the mission dispatches with no human turn

Delta: ADDED

### Requirement: Interruptions are counted

The system SHALL emit `mission.interruption` with `reason` and `stage` every time
a mission stops for a human, and SHALL emit `preflight.check` with `result`,
`missing_class` and `missing_count`.

#### Scenario: a mid-mission stop is counted with its stage

- **WHEN** a mission stops after dispatch for a reason pre-flight did not catch
- **THEN** `mission.interruption` records the reason and the stage, so the class
  pre-flight is missing becomes visible

Delta: ADDED

#### Scenario: no free text reaches the event

- **WHEN** a gap references a program name supplied by the model
- **THEN** the event carries the closed enum and a count, and the name appears
  nowhere in the written bytes

Delta: ADDED

## Non-goals

- Eliminating unforeseen interruptions. The unforeseen still stops, and it stops
  clean, with work archived and a precise report (PROGRAM decision 8).
- Becoming a general environment doctor. `jacu doctor` already exists;
  pre-flight is scoped to one compiled mission.

## Open decisions

- [x] none — resolved before this document was cut. ADR-022 records the eight
  classes, the ask-once contract and the refusal semantics; T1 writes it and the
  owner ratifies separately.

## Tasks

| # | Task | Files | Verify | Status | Evidence |
|---|---|---|---|---|---|
| T1 | Write ADR-022: the eight classes, ask-once, and why an unresolvable check refuses instead of passing | `docs/adr/ADR-022-preflight.md` | `wc -l` under 120; owner ratifies separately | done | `ADR-022-preflight.md` exists, 55 lines; owner ratification remains open |
| T2 | RED: one checker per class, each with a fixture; an unresolvable check reports unresolved and never passes | `internal/capability/preflight/check_test.go` | `go test ./internal/capability/preflight -race` fails on absence | done | RED observed before `d6df49e`; fixtures retained |
| T3 | GREEN: `Check(mission, env) Report` with typed findings | `internal/capability/preflight/check.go` | `go test ./internal/capability/preflight -race` | done | 21 tests passed |
| T4 | RED: the allowlist class reads the same source `jacu_verify` reads, so the two can never disagree | `internal/capability/preflight/allowlist_test.go` | `go test ./internal/capability/preflight -race` fails | done | RED observed before `1ce62dc` |
| T5 | GREEN: allowlist class delegating to the verify package, not reimplementing it | `internal/capability/preflight/allowlist.go` | `go test ./... -race` | done | delegation implemented; 814 tests passed |
| T6 | RED: path, credential and network classes; presence only, never content | `internal/capability/preflight/env_test.go` | `go test ./internal/capability/preflight -race` fails | done | RED observed before `1814aa3` |
| T7 | GREEN: environment classes, fail-closed | `internal/capability/preflight/env.go` | `go test ./internal/capability/preflight -race` | done | 21 tests passed; policy path dirs and required paths covered |
| T8 | RED: `open_questions` blocks while the mission carries an unanswered question | `internal/capability/preflight/questions_test.go` | `go test ./internal/capability/preflight -race` fails | done | RED observed before `524edc4` |
| T9 | GREEN: open-question class reading the compiled mission | `internal/capability/preflight/questions.go` | `go test ./internal/capability/preflight -race` | done | 19 tests passed |
| T10 | RED: three gaps produce exactly one batch and zero dispatch | `internal/capability/preflight/batch_test.go` | `go test ./internal/capability/preflight -race` fails | done | RED observed before `5224a11` |
| T11 | GREEN: single batch assembly | `internal/capability/preflight/batch.go` | `go test ./internal/capability/preflight -race` | done | 19 tests passed |
| T12 | RED: `preflight.check` and `mission.interruption` events, with a fuzz proving no model-controlled string reaches the stream | `internal/capability/preflight/telemetry_test.go` | `go test ./internal/capability/preflight -run Fuzz -fuzztime=30s` fails | done | RED observed before `51eb0de` |
| T13 | GREEN: emission through the v2 constructor | `internal/capability/preflight/telemetry.go` | fuzz clean | done | 3 fuzz targets passed in 30s |
| T14 | RED/GREEN: `jacu_mission_compile` refuses to dispatch while pre-flight has a gap | `internal/capability/missioncompile/**` | `go test ./... -race` | done | full suite: 814 passed; nested MCP telemetry covered |
| T15 | RED/GREEN: `jacu preflight` subcommand, exit codes 0/1/2, `--json` on stdout | `cmd/jacu/preflight.go`, `cmd/jacu/preflight_test.go` | `go test ./cmd/... -race` | done | 21 tests passed; exit contract and environment resolution covered |
| T16 | Authorize the subcommand in the verify allowlist | `.jacu/verify-allowlist.json` | `jacu_verify` returns a verdict for that argv | done | governed verify returned `fail` (exit 1) after accepting argv; installed binary lacked `preflight`; run discarded |
| T17 | Teach the skill to run pre-flight before dispatch | `skills/jacu-mission/SKILL.md` | `go test ./internal/mcpadapter -run Skills -race` | done | 2 tests passed; skill instructs hard preflight gate |
| T18 | Write the living capability spec | `docs/sdd/specs/preflight/spec.md` | `go run ./cmd/jacu sdd lint --all` exits 0 | done | all SDDs linted successfully |
| T19 | Eval on the live path: ten real missions, interruptions per mission before and after, with the class breakdown | `docs/evals/preflight.md` | report with n and the per-class counts | todo | worksheet prepared as `NEEDS-ERICK`; no synthetic results claimed |
| T20 | Confirm the MCP surface is untouched | — | `go test -tags=e2e ./test/e2e/ -run Governed` reports 13 tools | done | 1 governed test passed; 13-tool ratchet |
| T21 | Write the execution report | `docs/relatorios/sdd-004-execucao.md` | PR with the hosted run link | done | report includes hosted CI run 31815198496 from PR #63; T19 and ADR-022 remain human gates |

## Done

| Level | Proof |
|---|---|
| Core | `go test ./internal/capability/preflight -race` green, all eight classes covered; fuzz clean |
| Wiring | a mission with a gap does not dispatch; `jacu preflight` exits 0/1/2 correctly; `bash scripts/verify.sh` green |
| E2E | `go test -tags=e2e ./test/e2e/` green with 13 tools under the ratchet |
| Eval | ten missions on the live path with interruptions per mission recorded before and after |

## Follow-ups

- Auto-remediation for the classes that are safely fixable, such as creating a
  missing writable directory. Needs its own security argument; not here.
- Feeding the observed `missing_class` distribution back into the mission
  template, so the most frequent gap becomes a compile-time field.
