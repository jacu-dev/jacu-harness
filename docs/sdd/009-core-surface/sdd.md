---
sdd: 009-core-surface
program: jacu-one-shot
spec_id: spc_pending
branch: 009-core-surface
phase: docs/sdd/PROGRAM.md
adr: docs/adr/ADR-028-open-source-export.md
status: draft
---

# 009 — Core surface: transport-free Run, CLI parity, events

## Why

The CLI and the MCP surfaces are disjoint. `cmd/jacu` imports `cleanexit`,
`missioncompile`, `preflight`, `sdd`, `storage` and `report`; it does not import
`workspace`, `verify`, `memory`, `orchestration` or `projectinspect`. The core
loop — open a workspace, verify, review, apply — exists only as MCP tools.
Anything that is not an MCP host cannot drive JACU through its own core.

That is a boundary problem, not a feature gap. The capabilities carry the
transport in their signature: `func RegisterTool(server *mcp.Server, root string)`.
`mcpadapter` correctly contains no domain logic; the converse is false — the
domain contains transport.

One capability already shows the shape that works: `cmd/jacu/main.go` and
`internal/capability/report/tool.go` both call `headlessreport.BuildAudit(root)`.
This change generalises the one thing already done right.

The name follows the same correction. MCP is a surface, not the identity
(PROGRAM, ADR-028). Implementation follows `docs/design/core-surface.md` after
016 and 017.

## Locked decisions

1. JACU governs and measures one change inside one repository, and does not act
   on the world outside it. This change adds surfaces, never reach — PROGRAM
   invariant.
2. Zero new MCP tools. Every new entry point is a CLI subcommand — ADR-008.
3. Inputs and results are not redesigned. The pure core reuses the existing
   typed `Input`/`Result` structs and their JSON schemas; the CLI is
   unmarshal, Run, marshal. One schema, one validation, one test surface.
4. `--events` reuses the telemetry v2 envelope. No second event vocabulary —
   ADR-020.
5. Binary `jacu`, module `github.com/jacu-dev/jacu-harness` — ADR-028. The
   command rename already landed; module and user-home remain SDD-016.

## Out of scope

- HTTP server, listener, port, daemon.
- Any credential, token or network call.
- New MCP tool, or widening the tool catalogue.
- Behaviour change in any capability. This change moves code and adds
  surfaces; a behaviour delta here is a bug.
- Deleting `git log` from `stats` (SDD-012). This change may wrap remaining
  git exec through `internal/gitx` (I4).

## Write scope

**Allowed**

```
docs/sdd/009-core-surface/**
docs/sdd/010-repo-governance/**
docs/sdd/011-workspace-contract/**
docs/sdd/012-structural-debt/**
docs/sdd/013-model-panel/**
docs/sdd/014-report-visual/**
docs/sdd/015-program-closeout/**
docs/sdd/PROGRAM.md
.cursor/agent-board.md
```

**Forbidden**

```
cmd/**
internal/**
.github/**
docs/adr/**
```

## Requirements

### Requirement: Every capability has a transport-free core

The system SHALL expose, for each MCP capability, a function whose signature
contains no type from the MCP SDK, and all surfaces SHALL call it.

#### Scenario: the domain does not import the transport

- **WHEN** `go list -deps` runs over any package under `internal/capability/`
- **THEN** the MCP SDK appears only in that package's `tool.go` wrapper
Delta: ADDED

#### Scenario: CLI and MCP produce the same result

- **WHEN** the same input runs through the CLI subcommand and through the MCP tool
- **THEN** the emitted `Result` is byte-identical
Delta: ADDED

### Requirement: Every capability is reachable without an MCP host

The system SHALL provide a CLI subcommand for every MCP capability, accepting
`--json` and documenting its exit codes.

#### Scenario: the core loop runs from a shell

- **WHEN** a mission is compiled, a workspace opened, verified, diffed and
  applied using only the binary
- **THEN** every step succeeds and emits machine-readable output
Delta: ADDED

### Requirement: Progress is observable while the work happens

The system SHALL, under `--events`, write the telemetry envelope as NDJSON to
stdout during execution, not only at the end.

#### Scenario: a long run is followed live

- **WHEN** a verify that takes longer than one second runs with `--events`
- **THEN** events appear on stdout before the process exits
Delta: ADDED

### Requirement: The name matches what the thing is

The system SHALL be distributed as module `github.com/jacu-dev/jacu-harness`
(ADR-028) with binary `jacu`, and the MCP server SHALL be one subcommand of it.

#### Scenario: MCP is a surface, not the identity

- **WHEN** the binary is invoked without arguments
- **THEN** the help lists `serve` alongside the other subcommands, with no claim
  that the program is an MCP server
Delta: ADDED

### Requirement: Invariants I4 and I5 fail the build on a seeded violation

CI SHALL fail when an MCP type appears outside `mcpadapter` and `tool.go`, and
when `git` is executed outside `internal/gitx`.

#### Scenario: a seeded MCP-type leak is refused

- **WHEN** a non-wrapper capability file imports the MCP SDK
- **THEN** `scripts/verify.sh` exits non-zero
Delta: ADDED

## Non-goals

- Moving packages out of `internal/` to publish a Go library. That is a later
  change, and it only pays when a second Go program needs to import JACU.
- Any hexagonal refactor of the core. `gitx` and `os/exec` stay concrete.

## Open decisions

- [x] none — resolved by ADR-028 / SDD-016: binary is `jacu`; module, repository
      and user-home remain export scope.

## Tasks

| # | Task | Files | Verify | Status | Evidence |
|---|---|---|---|---|---|
| T1 | RED: table test asserting every MCP capability has `Run`, a tool wrapper and a CLI subcommand | `internal/mcpadapter/surface_test.go` | `go test ./internal/mcpadapter/ -run Surface` | todo | |
| T2 | Extract `Run` in `projectinspect`; `tool.go` becomes a wrapper | `internal/capability/projectinspect/` | `go test ./... -race` | todo | |
| T3 | Extract `Run` in `missioncompile` | `internal/capability/missioncompile/` | `go test ./... -race` | todo | |
| T4 | Extract `Run` in `workspace` for all six tools | `internal/capability/workspace/` | `go test ./... -race` | todo | |
| T5 | Extract `Run` in `memory` | `internal/capability/memory/` | `go test ./... -race` | todo | |
| T6 | Extract `Run` in `verify` | `internal/capability/verify/` | `go test ./... -race` | todo | |
| T7 | Extract `Run` in `orchestration` | `internal/capability/orchestration/` | `go test ./... -race` | todo | |
| T8 | CLI subcommands for the six capabilities, each accepting `--json` | `cmd/jacu/` | `bash scripts/verify.sh` | todo | |
| T9 | Exit-code contract, documented once and asserted | `cmd/`, `docs/` | `go test ./cmd/jacu -race` | todo | |
| T10 | GREEN: T1 passes | `internal/mcpadapter/surface_test.go` | `go test ./internal/mcpadapter/ -run Surface` | todo | |
| T11 | Invariant I5 in CI: no MCP type outside `mcpadapter` and `tool.go` | `scripts/verify.sh` | `bash scripts/verify.sh` | todo | |
| T12 | Invariant I4 in CI: no `exec` of `git` outside `internal/gitx`; wrap remaining callers | `internal/capability/cleanexit/`, `internal/capability/sdd/`, `internal/telemetry/` | `bash scripts/verify.sh` | todo | |
| T13 | `--events` writes the telemetry envelope as NDJSON to stdout during execution | `internal/runtime/pipeline.go`, `cmd/` | `go test -tags=e2e ./test/e2e/` | todo | |
| T14 | Rename: binary and command `jacu` | `cmd/jacu/` | `go build ./...` | done | command `jacu` shipped; module and repo remain SDD-016 |
| T15 | Rename: installers, MCP `Implementation.Name`, host docs, skills | `scripts/`, `skills/`, `docs/` | `bash scripts/e2e.sh` | done | command `jacu` shipped; module and repo remain SDD-016 |
| T16 | ADR-028 records the surface decision and the rename | `docs/adr/ADR-028-open-source-export.md` | `test -f docs/adr/ADR-028-open-source-export.md` | done | ADR-028 accepted; command `jacu` shipped |

## Done

| Level | Proof |
|---|---|
| Core | the six capabilities expose `Run`; T1 green |
| Surface | the whole loop runs from a shell with `--json`; `--events` streams |
| Name | command is `jacu`; no living surface teaches the retired binary name |
| Guard | I4 and I5 fail the build on a seeded violation |

## Follow-ups

- Moving packages out of `internal/` for library consumption; only when a
  second Go program needs it.
- `jacu sdd archive` as a subcommand; its trigger is unchanged.
- Execution report `docs/relatorios/sdd-009-execucao.md` after implementation
  (out of this authoring write scope).
- When 009 opens for implementation, replace this authoring write-scope with
  the implementation set: `cmd/`, `internal/`, `docs/sdd/009-core-surface/**`,
  `docs/sdd/specs/**`, `docs/sdd/PROGRAM.md`, `skills/`, `scripts/`,
  `.goreleaser.yaml`, `README.md`, `AGENTS.md`, `CLAUDE.md`.
