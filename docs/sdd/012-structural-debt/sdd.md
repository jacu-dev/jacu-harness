---
sdd: 012-structural-debt
program: jacu-one-shot
spec_id: spc_pending
branch: 012-structural-debt
phase: docs/sdd/PROGRAM.md
adr: docs/adr/ADR-008-tool-surface.md
status: draft
---

# 012 — Structural debt: scope, reportgen, ratchet, compaction, stats

## Why

PROGRAM queues this change to pay structural debt that later SDDs would
otherwise copy: `internal/scope`, `reportgen`, the I6 size ratchet, an
orchestration boundary test, `$defs`/`$ref` catalogue compaction, and `stats`
without `git log`.

`ScopesConflict` already lives in `internal/capability/workspace` and is
shared with SDD lint (SDD-001). Moving it to `internal/scope` is the named
home, not a second verdict.

`internal/telemetry/stats.go` still executes `git log` for the revert
heuristic. I4 requires git exec to live in `internal/gitx`; this change goes
further and stops using `git log` for stats at all. Missing signal is
`available=false`, never a silent zero.

The `$defs`/`$ref` trigger already fired: 20,476 of 20,480 bytes of
`tools/list` headroom (`docs/decisions/triggers.md`). Compaction is required
before the next catalogue consumer. SDD-007 stays deferred until this
evaluation lands.

The document is written now so the queue has no gaps. Implementation follows
009. Compaction design and the byte ceiling value are implementation
decisions; the ceiling may only decrease (I6).

## Locked decisions

1. One write-scope verdict, one package. SDD lint and workspace apply already
   share `ScopesConflict` — SDD-001. This change relocates that function to
   `internal/scope`.
2. Zero new MCP tools. Catalogue compaction uses `$defs`/`$ref`, not a new
   tool — ADR-008. The 14-tool ceiling and 20 KiB ratchet stay.
3. Report Markdown remains a projection of structured JSON — ADR-010.
   `reportgen` is that projector extracted, not the visual factory (SDD-014).
4. I4: no `exec` of `git` outside `internal/gitx`. I6: no non-test file above
   the ceiling; ratchet decreasing only — PROGRAM.

## Out of scope

- Surface profiles (SDD-007).
- Visual HTML factory (SDD-014).
- New MCP tools, HTTP, credentials.
- Raising the tool ceiling or the `tools/list` cap.

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

### Requirement: internal/scope owns ScopesConflict

The system SHALL house the single `ScopesConflict` function in
`internal/scope`. SDD lint, workspace apply, and any other caller SHALL use
that package.

#### Scenario: no second copy

- **WHEN** sdd, workspace, and orchestration classify a path
- **THEN** they call `internal/scope`, not a copy
Delta: ADDED

### Requirement: reportgen owns headless projection

The system SHALL extract JSON-to-Markdown projection into `reportgen` with no
MCP types in that package.

#### Scenario: both surfaces call one projector

- **WHEN** `jacu report` and `jacu_report` run
- **THEN** both call `reportgen`
Delta: ADDED

### Requirement: I6 size ratchet

`scripts/verify.sh` SHALL fail when a non-test file exceeds the ceiling, and
the ceiling SHALL not increase.

#### Scenario: a seeded oversize file is refused

- **WHEN** a seeded oversize non-test file is added
- **THEN** `bash scripts/verify.sh` exits non-zero
Delta: ADDED

### Requirement: Orchestration boundary test

The suite SHALL fail when orchestration grows a concern outside sequencing.

#### Scenario: a seeded extra concern is refused

- **WHEN** a seeded extra type or file is added to orchestration
- **THEN** the boundary test fails
Delta: ADDED

### Requirement: Catalogue compaction via $defs/$ref

The MCP `tools/list` catalogue SHALL be compacted with `$defs`/`$ref` so I9
headroom is at least 2 KiB and the tool count stays at 13.

#### Scenario: census and headroom

- **WHEN** the governed e2e census runs
- **THEN** it reports 13 tools and at least 2 KiB of catalogue headroom
Delta: ADDED

### Requirement: stats without git log

`jacu stats` SHALL compute its revert heuristic without executing `git log`,
including via `internal/gitx`. Missing signal SHALL be `available=false`,
never a silent zero.

#### Scenario: no git log in stats

- **WHEN** `go test ./internal/telemetry` runs
- **THEN** `internal/telemetry/stats.go` does not invoke `git log`
Delta: ADDED

## Non-goals

- Designing the `$defs` rewrite in this document. The trigger fired; the
  compaction design is implementation.
- The visual factory (SDD-014). `reportgen` stays headless Markdown/JSON.

## Open decisions

- [x] none — queued from PROGRAM; `$defs` already fired (`docs/decisions/triggers.md`)
      but the compaction design is not this document; size ceiling value and
      stats-without-git-log algorithm are implementation decisions.

## Tasks

| # | Task | Files | Verify | Status | Evidence |
|---|---|---|---|---|---|
| T1 | RED: `ScopesConflict` lives in `internal/scope` | `internal/scope/` | `go test ./internal/scope ./internal/capability/sdd ./internal/capability/workspace -race` | todo | |
| T2 | GREEN: move `ScopesConflict` to `internal/scope` | `internal/scope/`, `internal/capability/sdd/`, `internal/capability/workspace/` | `go test ./internal/scope ./internal/capability/sdd ./internal/capability/workspace -race` | todo | |
| T3 | Extract `internal/reportgen` | `internal/reportgen/`, `internal/report/` | `go test ./internal/reportgen ./internal/report -race` | todo | |
| T4 | I6 script and ratchet in `scripts/verify.sh` | `scripts/verify.sh` | `bash scripts/verify.sh` | todo | |
| T5 | Orchestration boundary test | `internal/capability/orchestration/` | `go test ./internal/capability/orchestration -run Boundary -race` | todo | |
| T6 | `$defs`/`$ref` compaction | `internal/mcpadapter/` | `go test -tags=e2e ./test/e2e/ -run Governed` | todo | |
| T7 | Remove stats `git log` revert heuristic | `internal/telemetry/stats.go` | `go test ./internal/telemetry -race` | todo | |
| T8 | Update living specs after implementation | `docs/sdd/specs/` | `jacu sdd lint --all` | todo | |

## Done

| Level | Proof |
|---|---|
| Core | one `internal/scope`; `reportgen` is the projector; I6 ratchet fails on oversize; orchestration boundary holds; catalogue has 13 tools and I9 headroom; stats has no `git log` |

## Follow-ups

- Implementation Allowed, by later amendment: `internal/scope/**`,
  `internal/reportgen/**`, `internal/report/**`, `internal/telemetry/**`,
  `internal/capability/**`, `internal/mcpadapter/**`, `scripts/verify.sh`,
  `docs/sdd/012-structural-debt/**`, living specs.
- SDD-007 may open only after this compaction is applied or rejected on
  evidence.
