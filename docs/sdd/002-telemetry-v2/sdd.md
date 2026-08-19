---
sdd: 002-telemetry-v2
program: jacu-one-shot
spec_id: spc_pending
branch: 002-telemetry-v2
phase: docs/sdd/PROGRAM.md
adr: docs/adr/ADR-020-observabilidade.md
status: draft
---

# 002 — Telemetry v2

## Why

The v1 stream answers "did a tool run and did it pass". It cannot answer what a
mission cost in context, which gate held, or how often JACU stopped. Those three
are the program's own success metrics, so the product currently cannot measure
whether it works.

Most of the work is wiring, not instrumenting. `tool-envelope/spec.md` already
logs input and encoded output bytes and throws them away. `internal/runstate`
already carries digests and a program cursor. The design inventory
(design input, removed in the plan purge) verified this against the repository.

One thing must land before any schema work. `internal/runstate/state.go` imports
`internal/capability/missioncompile` and embeds `missioncompile.Input` and
`missioncompile.Mission` directly in the persisted `Run`. The persistence layer
depends on a capability, and the on-disk shape changes whenever that capability
gains a field while `CurrentSchemaVersion` stays `"1"`. There is no golden
fixture anywhere in the repository to catch it — `find . -type d -name testdata`
returns nothing. Freezing a v2 schema on top of that inversion pays for it
forever, so Block 0 breaks it first.

## Locked decisions

1. Sanitization is an invariant, not a level — content is unrepresentable in the
   constructor at every level including `full` (ADR-020; PROGRAM decision 7).
2. No counterfactual metric, ever. `SavingsUnits` renders only beside a named
   baseline profile (ADR-020; PROGRAM decision 5).
3. Telemetry is local-first with no upload channel (ADR-018, ADR-020; PROGRAM
   decision 6).
4. `detail` is typed per event, never a flat struct of optional fields, so each
   payload keeps its own closed constructor (ADR-020).
5. Every event carrying a cost number carries `measurement` naming how it was
   obtained (ADR-020).
6. Zero new MCP tools; the surface stays at 13 with the 20 KiB ratchet
   (ADR-008; PROGRAM decision 3).

## Out of scope

- Instrumenting a module that does not exist yet. `clarity.*`, `preflight.*`,
  `context.*`, `ledger.*` and `mission.interruption` ship with SDD-004, 005 and
  006, each with the module that emits it.
- Wiring `internal/modelcontrol`. The package has no consumer anywhere in the
  repository, so a `modelcontrol.route` event would be a green unit test that
  never fires in production. The owner decided on 2026-08-14 that the package
  stays, unwired, and that the routing work is its own SDD.
- An OpenTelemetry SDK inside the binary (ADR-018 rejected it).
- The paired net-cost eval. It is research with a pre-registered protocol, not a
  row in a task table; see Follow-ups.

## Write scope

**Allowed**

```
docs/sdd/002-telemetry-v2/**
docs/sdd/specs/telemetry/spec.md
docs/adr/ADR-020-observabilidade.md
docs/telemetry.md
docs/relatorios/sdd-002-execucao.md
internal/telemetry/**
internal/runstate/**
internal/runtime/**
internal/report/**
internal/capability/report/**
internal/capability/workspace/**
internal/capability/verify/**
cmd/jacu/stats.go
cmd/jacu/stats_test.go
```

**Forbidden**

```
internal/modelcontrol/**
internal/mcpadapter/**
internal/capability/orchestration/**
skills/**
.github/**
```

## Requirements

### Requirement: Versioned event envelope

The system SHALL write events carrying `schema_version`, `level`, `module`,
`stage` and `status`, and SHALL read a v1 stream without loss.

#### Scenario: a v1 line is read by the v2 reader

- **WHEN** the reader encounters a line written by schema v1
- **THEN** it yields the event with the v2-only fields reported as `no-data`,
  never as a zero value

Delta: ADDED

#### Scenario: a torn final line does not abort the read

- **WHEN** the last line of the stream is a partial write
- **THEN** the reader skips it, counts it, and returns every readable event

Delta: ADDED

### Requirement: Content is unrepresentable at every level

The system SHALL make prompts, diffs, outputs, file paths and free text
impossible to construct into an event, including at `level: full`.

#### Scenario: full level offers no content field

- **WHEN** an event is constructed at `level: full`
- **THEN** the constructor exposes no field accepting model-controlled text, and
  the test proving it fails to compile if one is added

Delta: ADDED

#### Scenario: a refused program name never reaches the stream

- **WHEN** `verify.denial` records a command the allowlist refused
- **THEN** the event carries `reason` and `program_known`, and the adversarial
  program name appears nowhere in the written bytes

Delta: ADDED

### Requirement: Cost numbers name their measurement

The system SHALL refuse to construct an event carrying a cost or size number
without a `measurement` of `exact_bytes`, `cli_reported_tokens` or
`estimated_tokens`.

#### Scenario: a byte count without measurement is a construction error

- **WHEN** `runtime.tool_call` is built with `output_bytes` and no `measurement`
- **THEN** construction returns an error and nothing is written

Delta: ADDED

### Requirement: Run state persistence owns its own shape

The system SHALL persist run state through a type owned by `internal/runstate`,
and SHALL detect any change to the on-disk shape against a golden fixture.

#### Scenario: a capability field change is caught

- **WHEN** a field is added to `missioncompile.Mission`
- **THEN** the run-state golden test fails until the mapping and
  `CurrentSchemaVersion` are updated deliberately

Delta: ADDED

### Requirement: Module scorecard reports honestly

The system SHALL print one section per module under `stats --full`, showing
`no-data` where no event exists and the `measurement` on every cost line.

#### Scenario: an unmeasured module is not reported as zero

- **WHEN** a module has emitted no event in the window
- **THEN** its section reads `no-data`, never `0`

Delta: ADDED

#### Scenario: savings render only beside a named baseline

- **WHEN** `SavingsUnits` is rendered without a baseline profile
- **THEN** rendering fails the test that guards it

Delta: ADDED

## Non-goals

- Estimating cost in currency. `CostTrace` has no USD field and that is a
  decision from E7, not an omission.
- Claiming a saving. Gain is proven by a paired eval, never by telemetry.
- Measuring an interactive session's total spend. JACU does not call the model
  in that regime and cannot know.

## Open decisions

- [x] none — resolved in ADR-020 and in the owner decisions of 2026-08-14.
  D4 is settled here by writing it down: RSS idle and runtime overhead do **not**
  become v2 fields and leave the budget list, because `runtime.health` carries
  `panic_recovered`, `timeout` and `spec_invalid` and no memory field. A budget
  nobody measures is an intention.

## Tasks

| # | Task | Files | Verify | Status | Evidence |
|---|---|---|---|---|---|
| T1 | RED: golden fixture of the persisted run-state JSON; the test fails when the on-disk shape changes | `internal/runstate/golden_test.go`, `internal/runstate/testdata/run-v1.json` | `go test ./internal/runstate -race` fails on absence | done | `go test ./internal/runstate -race -run TestPersistedRunStateMatchesGolden -count=1` → failed: `open testdata/run-v1.json: no such file or directory` |
| T2 | GREEN: `runstate.MissionSnapshot` DTO plus explicit capability type aliases; `runstate` no longer imports `missioncompile` | `internal/runstate/state.go`, `internal/runstate/mission.go`, `internal/capability/missioncompile/types.go` | `go test ./... -race` | done | `go test ./... -race` → `Go test: 757 passed in 18 packages` |
| T3 | Prove the inversion is gone | `internal/runstate/**` | `go list -deps ./internal/runstate \| grep -c capability` reports 0 | done | `go list -deps ./internal/runstate \| grep -c capability` → `0` |
| T4 | RED: v2 envelope with `schema_version`, `level`, `module`, `stage`; reader accepts v1 and marks absent fields `no-data` | `internal/telemetry/event_test.go` | `go test ./internal/telemetry -race` fails | done | `go test ./internal/telemetry -race -run TestV2EnvelopeIsPresentAndV1ReadsAsNoData -count=1` → failed to compile because the v2 fields were absent |
| T5 | GREEN: v2 envelope and tolerant reader | `internal/telemetry/event.go`, `internal/telemetry/store.go` | `go test ./internal/telemetry -race` | done | `go test ./internal/telemetry -race` → `Go test: 16 passed in 1 packages` |
| T6 | RED: fixture captured from a real v1 stream, sanitized, proves the reader against production bytes rather than against its own assumption | `internal/telemetry/testdata/events-v1.jsonl`, `internal/telemetry/store_test.go` | `go test ./internal/telemetry -race` fails | done | `go test ./internal/telemetry -race -run TestReaderAcceptsSanitizedRealV1Corpus -count=1` → failed: `open testdata/events-v1.jsonl: no such file or directory` |
| T7 | GREEN: reader passes the real v1 corpus | `internal/telemetry/store.go` | `go test ./internal/telemetry -race` | done | `go test ./internal/telemetry -race` → `Go test: 17 passed in 1 packages` |
| T8 | RED: content is unrepresentable at `full`; the test fails to compile if a text field is added | `internal/telemetry/event_test.go` | `go test ./internal/telemetry -race` fails | done | `go test ./internal/telemetry -race -run TestFullEventConstructorHasNoContentSurface -count=1` → failed to compile: `undefined: NewFullEvent` |
| T9 | GREEN: constructor closed per level | `internal/telemetry/event.go` | `go test ./internal/telemetry -race` | done | `go test ./internal/telemetry -race` → `Go test: 18 passed in 1 packages` |
| T10 | RED: `measurement` mandatory on any event carrying a cost or size number | `internal/telemetry/measurement_test.go` | `go test ./internal/telemetry -race` fails | done | `go test ./internal/telemetry -race -run TestMeasuredBytesRequireMeasurement -count=1` → failed to compile: `EventInput`/`Event` lacked `OutputBytes` |
| T11 | GREEN: construction error when it is absent | `internal/telemetry/event.go` | `go test ./internal/telemetry -race` | done | `go test ./internal/telemetry -race` → `Go test: 19 passed in 1 packages` |
| T12 | RED: `runtime.tool_call` carries `input_bytes`, `output_bytes`, `capped`, `degraded_partial`, taken from the pipeline point that already logs them | `internal/runtime/pipeline_test.go` | `go test ./internal/runtime -race` fails | done | `go test ./internal/runtime -race -run TestExecuteEmitsOneSanitizedTelemetryEvent -count=1` → failed to compile: `Event` lacked `Capped` and `DegradedPartial` |
| T13 | GREEN: move the measurement from the log into the event; the log stays | `internal/runtime/pipeline.go` | `go test ./internal/runtime -race` | done | `go test ./internal/runtime -race` → `Go test: 8 passed in 1 packages` |
| T14 | RED: `gate.decision` for every existing gate, with `verdict` ordered `pass<warn<require_approval<block` | `internal/capability/workspace/telemetry_test.go` | `go test ./... -race` fails | done | RED commit `018b296` failed to compile before `EventGateDecision`; order test added in `internal/capability/workspace/telemetry_test.go` |
| T15 | GREEN: emission at the gates | `internal/capability/workspace/telemetry.go`, `internal/capability/verify/**` | `go test ./... -race` | done | GREEN commit `3f7a433`; `jacu_diff`, `jacu_apply`, and `jacu_verify` emit `gate.decision`; `go test ./... -race` -> `Go test: 763 passed in 18 packages` |
| T16 | RED: fuzz proves `verify.denial` never writes the refused program name, using adversarial names | `internal/telemetry/fuzz_test.go` | `go test ./internal/telemetry -run Fuzz -fuzztime=30s` fails | done | RED commit `256859a` failed to compile because the denial event and typed detail were absent |
| T17 | GREEN: denial records reason and `program_known` only | `internal/capability/verify/**` | fuzz clean | done | GREEN commit `7a7298f`; `go test ./internal/telemetry -run=^$ -fuzz=FuzzVerifyDenialNeverWritesProgramName -fuzztime=30s` -> `Go test: 1 passed in 1 packages`; refused program names are not event fields |
| T18 | RED/GREEN: `workspace.apply`, `escalation` and `review.disagreement` carry their typed detail | `internal/capability/workspace/**` | `go test ./... -race` | done | RED commit `0b53a2e` failed before typed detail fields; GREEN commit `57cb3d9`; apply emits `auto`, `intervention`, `diff_bytes`, `files_changed`, receipt disagreement emits `resolved`; `go test ./... -race` -> `Go test: 770 passed in 18 packages` |
| T19 | RED: `stats --full` prints a section per module, `no-data` where empty, `measurement` on every cost line | `cmd/jacu/stats_test.go` | `go test ./cmd/... -race` fails | done | RED commit `f2dec44` failed before `parseStatsArgs` and `FormatFullStats` existed |
| T20 | GREEN: the scorecard | `cmd/jacu/main.go`, `internal/telemetry/stats.go` | `go test ./cmd/... -race` | done | GREEN commit `2cfb3b9`; `go test ./cmd/jacu ./internal/telemetry -race` -> `Go test: 43 passed in 2 packages`; `stats --full` emits module sections, `no-data`, and named measurements |
| T21 | RED/GREEN: the `metrics` block of `jacu_report` gains the user-level fields | `internal/report/report.go`, `internal/report/report_test.go` | `go test ./internal/report -race` | done | RED commit `003675e`; GREEN commit `36d24ad`; adds mission bytes, interruption and clean-exit metrics with honest no-data; `go test ./... -race` -> `Go test: 772 passed in 18 packages` |
| T22 | Update the living capability spec and the operator doc | `docs/sdd/specs/telemetry/spec.md`, `docs/telemetry.md` | `go run ./cmd/jacu sdd lint --all` exits 0 | done | GREEN commit `af3e110`; v2 envelope, gate/denial/review detail, full scorecard and report metrics documented; `go run ./cmd/jacu sdd lint --all` exited 0 |
| T23 | Confirm the MCP surface is untouched | — | `go test -tags=e2e ./test/e2e/ -run Governed` reports 13 tools under the ratchet | done | `go test -tags=e2e ./test/e2e/ -run Governed` -> `Go test: 1 passed in 1 packages`; governed test asserts the expected 13-tool surface and budget |
| T24 | Write the execution report | `docs/relatorios/sdd-002-execucao.md` | PR with the hosted run link | done | Report commit `284ebe3`; draft PR #61; hosted run `31803875261` for `9b0c4e3` passed all eight required jobs |

## Done

| Level | Proof |
|---|---|
| Core | `go test ./internal/telemetry ./internal/runstate -race` green; fuzz clean |
| Wiring | `go test ./... -race` green; `stats --full` prints every module; `go run ./cmd/jacu sdd lint --all` exits 0 |
| E2E | `go test -tags=e2e ./test/e2e/` green with 13 tools under the ratchet |
| Eval | one real mission through the live path, its events read back by `stats --full`, with the run link in the report |

Repository gate, all green: `gofmt -l`, `go vet ./...`, `golangci-lint run`,
`go test ./... -race`, `bash scripts/hygiene.sh`, `bash scripts/verify.sh`.

## Follow-ups

- OTLP exporter as a separate binary or subcommand, outside the main binary as
  ADR-018 requires.
- `docs/evals/custo-liquido-protocolo.md`: the paired net-cost eval needs
  n, arms, task corpus and quality criterion registered **before** it runs, or
  it produces a number nobody can defend. Written before 002 closes, executed
  after.
- Wiring `internal/modelcontrol` into the orchestration route path — its own
  SDD, after 003.
