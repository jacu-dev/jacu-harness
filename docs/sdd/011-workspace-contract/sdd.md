---
sdd: 011-workspace-contract
program: jacu-one-shot
spec_id: spc_pending
branch: 011-workspace-contract
phase: docs/sdd/PROGRAM.md
adr: docs/adr/ADR-010-report-headless.md
status: draft
---

# 011 — Workspace contract: report JSON as quality.json, context --sdd

## Why

PROGRAM queues this change to make two host-facing contracts explicit:
`report --json` as `quality.json`, and `context --sdd`.

ADR-010 already shipped the versioned JSON audit contract, the Markdown
projection, `jacu_report` and `jacu statusline`. Markdown is a projection and
must never be parsed as state. Hosts that want a machine artifact still have
no named JSON file and no CLI that admits the active SDD into context.

The document is written now so the queue has no gaps. Implementation follows
009. Flag grammar and file placement are implementation decisions, not this
document's entry gate. SDD-006 owns admission, ledger and pack; this change
does not.

## Locked decisions

1. Structured JSON is the source of report state. Markdown is a projection
   only — ADR-010.
2. Zero new MCP tools. `jacu_report` already exists. This change is CLI
   contract, not a catalogue slot — ADR-008.
3. `context --sdd` is a CLI flag. It does not add an MCP tool — ADR-008.

## Out of scope

- Visual HTML factory (SDD-014).
- Context admission, per-task budget, ledger (SDD-006).
- New MCP tools.
- Treating Markdown as state.

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

### Requirement: report --json is the quality.json artifact

`jacu report --json` SHALL emit the versioned audit JSON (`schema_version`,
`kind: audit`, eight blocks) on stdout, and that object SHALL be the
`quality.json` artifact.

#### Scenario: JSON is the contract, not Markdown

- **WHEN** `jacu report --json` runs
- **THEN** stdout parses as the ADR-010 audit JSON and is not Markdown
Delta: ADDED

#### Scenario: quality.json is that object

- **WHEN** a consumer asks for `quality.json`
- **THEN** it receives this JSON object, not the `.md` projection
Delta: ADDED

### Requirement: context --sdd admits the active native SDD

`jacu context --sdd` SHALL emit the active living SDD path and the document
for the host.

#### Scenario: a living SDD is active

- **WHEN** `--sdd` runs with a living `docs/sdd/<NNN>-<slug>/sdd.md`
- **THEN** stdout names that path and includes the document
Delta: ADDED

#### Scenario: no active SDD

- **WHEN** `--sdd` runs and no living SDD is active
- **THEN** the command exits non-zero with a typed diagnostic on stderr
Delta: ADDED

### Requirement: Both subcommands accept --json

Both `report` and `context` SHALL accept `--json` and keep diagnostics on
stderr (I3).

#### Scenario: --json is documented and machine-readable

- **WHEN** either subcommand is invoked with `--json`
- **THEN** stdout is JSON and exit codes match the documented contract
Delta: ADDED

## Non-goals

- Inventing a second report schema. ADR-010's eight-block audit is the contract.
- Compaction, budget, or refusal of context (SDD-006).

## Open decisions

- [x] none — queued from PROGRAM; quality.json placement and context --sdd
      grammar are implementation decisions, not this document's entry gate.

## Tasks

| # | Task | Files | Verify | Status | Evidence |
|---|---|---|---|---|---|
| T1 | RED: `jacu report --json` is schema-valid audit JSON | `cmd/jacu/`, `internal/report/` | `go test ./cmd/jacu ./internal/report -race` | todo | |
| T2 | GREEN: `--json` vs default Markdown projection | `cmd/jacu/`, `internal/report/` | `go test ./cmd/jacu ./internal/report -race` | todo | |
| T3 | RED then GREEN: `jacu context --sdd` | `cmd/jacu/` | `go test ./cmd/jacu -race` | todo | |
| T4 | Skills and CLI docs; MCP census unchanged | `skills/`, `docs/` | `go test ./internal/mcpadapter -run Skills -race` | todo | |
| T5 | Update the report spec after implementation | `docs/sdd/specs/report/spec.md` | `jacu sdd lint --all` | todo | |

## Done

| Level | Proof |
|---|---|
| Core | `jacu report --json` is the audit JSON named `quality.json`; `jacu context --sdd` admits or refuses with a typed exit |

## Follow-ups

- Implementation Allowed, by later amendment: `cmd/jacu/`, `internal/report/`,
  `docs/sdd/011-workspace-contract/**`, `docs/sdd/specs/report/spec.md`,
  `skills/`, `docs/reference/cli.md`.
- SDD-006 still owns admission, ledger and pack.
