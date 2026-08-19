# Design input — 009 core-surface

> Ready to be promoted to `docs/sdd/009-core-surface/` with `jacu sdd new`
> once the two Open decisions below are answered. Design approval is the entry
> gate of the change (CONVENTIONS.md).

## Why

Today the CLI and the MCP surfaces are disjoint. `cmd/jacu` imports `cleanexit`,
`missioncompile`, `preflight`, `sdd`, `storage` and `report`; it does not import
`workspace`, `verify`, `memory`, `orchestration` or `projectinspect`. The core loop —
open a workspace, verify, review, apply — exists **only as MCP tools**. Anything that is
not an MCP host cannot drive JACU through its own core.

That is a boundary problem, not a feature gap. The capabilities carry the transport in
their signature: `func RegisterTool(server *mcp.Server, root string)`. `mcpadapter`
correctly contains no domain logic; the converse is false — the domain contains transport.

One capability already shows the shape that works: `cmd/jacu/main.go` and
`internal/capability/report/tool.go` both call `headlessreport.BuildAudit(root)`. This
change generalises the one thing already done right.

The name follows the same correction. `jacu` describes a transport that is about to
become one surface out of three.

## Locked decisions

1. JACU governs and measures one change inside one repository, and does not act on the
   world outside it. This change adds surfaces, never reach (PROGRAM invariant).
2. Zero new MCP tools. Every new entry point is a CLI subcommand (ADR-008).
3. Inputs and results are not redesigned. The pure core reuses the existing typed
   `Input`/`Result` structs and their JSON schemas; the CLI is `unmarshal -> Run -> marshal`.
   One schema, one validation, one test surface.
4. `--events` reuses the telemetry v2 envelope. No second event vocabulary.

## Out of scope

- HTTP server, listener, port, daemon.
- Any credential, token or network call.
- New MCP tool, or widening the tool catalogue.
- Behaviour change in any capability. This change moves code and adds surfaces; a
  behaviour delta here is a bug.

## Write scope

**Allowed**
```
go.mod
cmd/
internal/
docs/
skills/
scripts/
.goreleaser.yaml
README.md
AGENTS.md
CLAUDE.md
```

**Forbidden**
```
docs/adr/
docs/relatorios/
docs/evals/
docs/heranca/
docs/sdd/archive/
```

## Requirements

### Requirement: Every capability has a transport-free core

The system SHALL expose, for each MCP capability, a function whose signature contains no
type from the MCP SDK, and all surfaces SHALL call it.

#### Scenario: the domain does not import the transport
- **WHEN** `go list -deps` runs over any package under `internal/capability/`
- **THEN** the MCP SDK appears only in that package's `tool.go` wrapper
Delta: ADDED

#### Scenario: CLI and MCP produce the same result
- **WHEN** the same input runs through the CLI subcommand and through the MCP tool
- **THEN** the emitted `Result` is byte-identical
Delta: ADDED

### Requirement: Every capability is reachable without an MCP host

The system SHALL provide a CLI subcommand for every MCP capability, accepting `--json`
and documenting its exit codes.

#### Scenario: the core loop runs from a shell
- **WHEN** a mission is compiled, a workspace opened, verified, diffed and applied using
  only the binary
- **THEN** every step succeeds and emits machine-readable output
Delta: ADDED

### Requirement: Progress is observable while the work happens

The system SHALL, under `--events`, write the telemetry envelope as NDJSON to stdout
during execution, not only at the end.

#### Scenario: a long run is followed live
- **WHEN** a verify that takes longer than one second runs with `--events`
- **THEN** events appear on stdout before the process exits
Delta: ADDED

### Requirement: The name matches what the thing is

The system SHALL be distributed as module `github.com/jacu-dev/jacu-harness` (ADR-028)
with binary `jacu`, and the MCP server SHALL be one subcommand of it.

#### Scenario: MCP is a surface, not the identity
- **WHEN** the binary is invoked without arguments
- **THEN** the help lists `serve` alongside the other subcommands, with no claim that the
  program is an MCP server
Delta: ADDED

## Non-goals

- Moving packages out of `internal/` to publish a Go library. That is a later change, and
  it only pays when a second Go program needs to import JACU.
- Any hexagonal refactor of the core. `gitx` and `os/exec` stay concrete; the hermetic git
  fixtures are a better guarantee than a mock would be.

## Open decisions

- [x] Binary name `jacu` while the sibling product repository is also called `jacu`.
      Resolved by ADR-028 / SDD-016: the binary is `jacu`; the command rename landed
      in this repository. Module, repository and user-home changes remain export scope.
- [x] Whether the rename lands as one commit or as the last task, after the surface work
      is green. Resolved by ADR-028 / SDD-016: the binary/command half landed in this
      repository; module, repository and user-home changes remain owned by the export.

## Tasks

| # | Task | Files | Verify | Status | Evidence |
|---|---|---|---|---|---|
| T1 | RED: table test asserting every MCP capability has `Run`, a tool wrapper and a CLI subcommand | `internal/mcpadapter/surface_test.go` | `go test ./internal/mcpadapter/ -run Surface` fails | todo | |
| T2 | Extract `Run` in `projectinspect`; `tool.go` becomes a wrapper | `internal/capability/projectinspect/` | `go test ./... -race` | todo | |
| T3 | Extract `Run` in `missioncompile` | `internal/capability/missioncompile/` | `go test ./... -race` | todo | |
| T4 | Extract `Run` in `workspace` for all six tools | `internal/capability/workspace/` | `go test ./... -race` | todo | |
| T5 | Extract `Run` in `memory` | `internal/capability/memory/` | `go test ./... -race` | todo | |
| T6 | Extract `Run` in `verify` | `internal/capability/verify/` | `go test ./... -race` | todo | |
| T7 | Extract `Run` in `orchestration` | `internal/capability/orchestration/` | `go test ./... -race` | todo | |
| T8 | CLI subcommands for the six capabilities, each accepting `--json` | `cmd/jacu/` | `bash scripts/verify.sh` | todo | |
| T9 | Exit-code contract, documented once and asserted | `cmd/`, `docs/` | table test over the command registry | todo | |
| T10 | GREEN: T1 passes | `internal/mcpadapter/surface_test.go` | `go test ./internal/mcpadapter/ -run Surface` | todo | |
| T11 | Invariant I5 in CI: no MCP type outside `mcpadapter` and `tool.go` | `.github/workflows/ci.yml`, `scripts/verify.sh` | job fails on a seeded violation | todo | |
| T12 | Invariant I4 in CI: no `exec` of `git` outside `internal/gitx`; migrate the four current violators | `internal/capability/cleanexit/`, `internal/capability/sdd/`, `internal/telemetry/`, `scripts/` | job fails on a seeded violation | todo | |
| T13 | `--events` writes the telemetry envelope as NDJSON to stdout during execution | `internal/runtime/pipeline.go`, `cmd/` | e2e asserting an event before exit | todo | |
| T14 | Rename: module path, imports, binary, `cmd/jacu` (binary/command done here; module/imports remain SDD-016) | whole tree | `go build ./...`, `bash scripts/verify.sh` | done | command `jacu` shipped; module/repo remain SDD-016 |
| T15 | Rename: goreleaser, install.sh, MCP server `Implementation.Name`, host configuration docs, the twelve skills (binary/command done here; export integration remains SDD-016) | `.goreleaser.yaml`, `scripts/`, `skills/`, `docs/` | `bash scripts/e2e.sh` | done | command `jacu` shipped; module/repo remain SDD-016 |
| T16 | ADR-028 recording the surface decision and the rename (binary/command done here; module/repo remain SDD-016) | `docs/adr/ADR-028-core-surface.md` | ADR exists and is referenced by this SDD | done | command `jacu` shipped; module/repo remain SDD-016 |
| T17 | Write the execution report | `docs/relatorios/sdd-009-execucao.md` | PR with the hosted run link | todo | |

## Done

| Level | Proof |
|---|---|
| Core | the six capabilities expose `Run`; T1 green |
| Surface | the whole loop runs from a shell with `--json`; `--events` streams |
| Name | command is `jacu`; no living surface teaches the retired binary name |
| Guard | I4 and I5 fail the build on a seeded violation |

## Follow-ups

- Moving packages out of `internal/` for library consumption; only when a second Go
  program needs it.
- `jacu sdd archive` as a subcommand; its trigger is unchanged.
