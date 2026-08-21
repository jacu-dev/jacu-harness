---
sdd: 015-program-closeout
program: jacu-one-shot
spec_id: spc_pending
branch: 015-program-closeout
phase: docs/sdd/PROGRAM.md
adr: docs/adr/ADR-021-clean-exit.md
status: draft
---

# 015 — Program closeout: archive, ratify, unify language, net-cost protocol

## Why

PROGRAM places this change last. It closes 001-004 and 008, ratifies ADRs,
unifies document language, and writes the net-cost protocol that is a
prerequisite for any claim of gain.

I7: every merged SDD is closed by `sdd close` and archived. 001-004 and 008
are merged and not closed. Close is a gate, not a rewrite: unfinished owner
evals cannot be converted into executor tasks.

ADR-021, ADR-022, ADR-026 and ADR-027 have code on `main` and remain
`proposed`. I8: no ADR stays `proposed` with code in `main`. Ratification is
owner-only (PROGRAM owner-only table). This SDD does not flip ADR status.

The net-cost protocol has not been written: n, arms, task corpus, quality
criterion and statistical test. It becomes a task of this change. G-T (one
week without JACU) remains owner-only; without it no `stats` number can be
read as a gain.

CONVENTIONS: agent-facing SDD, spec, skill and CLI text is technical English.
PT-BR is reserved for owner conversation and existing ADR bodies whose
established language is PT-BR. This change does not rewrite those ADR bodies.

Archive is currently a manual `git mv` plus lock regeneration. `jacu sdd
archive` stays deferred (`docs/decisions/triggers.md`).

## Locked decisions

1. `sdd close` verifies archive, finished tasks, evidence and lint. It never
   moves the SDD, deletes files, or decides merge — ADR-021.
2. Agent-facing living docs are English. Established PT-BR ADR bodies stay —
   CONVENTIONS.
3. Owner-only gates stay owner-only. 003 T20, 004 T19, P6-P8, G-* and ADR
   ratification are not converted into executor tasks — PROGRAM.
4. Zero new MCP tools, no HTTP, no credentials — PROGRAM.

## Out of scope

- Implementing 005-007 or 009-014.
- Rewriting historical ADR bodies.
- `jacu sdd archive` as a subcommand.
- Filling measured n, arms or p-values into the net-cost protocol.
- Closing 003, 004 or 008 while owner evals remain `todo`.

## Write scope

**Allowed**

```
docs/sdd/015-program-closeout/**
docs/sdd/PROGRAM.md
docs/relatorios/sdd-015-execucao.md
cmd/jacu/**
internal/gitx/gitx_test.go
internal/capability/workspace/open.go
internal/capability/ledger/telemetry.go
internal/capability/context/pack.go
internal/capability/orchestration/**
internal/mcpadapter/surface_test.go
internal/reportgen/**
internal/runner/**
```

**Forbidden**

```
.github/**
docs/adr/**
docs/sdd/001-native-sdd/**
docs/sdd/002-telemetry-v2/**
docs/sdd/003-clean-exit/**
docs/sdd/004-preflight/**
docs/sdd/008-audit-hardening/**
```

## Requirements

### Requirement: Close 001 and 002 when the gate allows

The program SHALL archive and `sdd close` 001 and 002 once their tasks are
complete.

#### Scenario: close succeeds on a finished change

- **WHEN** `jacu sdd close` runs on the 001 and 002 archives
- **THEN** it exits 0 and I7 holds for those changes
Delta: ADDED

### Requirement: 003, 004 and 008 stay honest

The program SHALL NOT close 003, 004 or 008 while owner evals or P6-P8 remain
`todo`, and SHALL NOT convert those rows into executor tasks.

#### Scenario: unfinished owner gates block close

- **WHEN** 003 T20, 004 T19 or P6-P8 remain open
- **THEN** 015 records them on the PROGRAM owner-only table and leaves
  `sdd close` unrun
Delta: ADDED

### Requirement: ADR ratification is owner-gated

ADR-021, ADR-022, ADR-026 and ADR-027 SHALL be listed for owner ratification
and SHALL NOT be silently left `proposed` under a fake close.

#### Scenario: packet exists, ADRs are not rewritten here

- **WHEN** 015 implementation runs
- **THEN** a ratification packet exists and this change does not edit ADR
  status bytes
Delta: ADDED

### Requirement: Living agent-facing docs are English

Living agent-facing SDD, spec, skill and CLI docs SHALL be English.
Historical ADR PT-BR SHALL be left untouched.

#### Scenario: lint on the living corpus

- **WHEN** `jacu sdd lint --all` runs
- **THEN** 009-015 do not add `sdd_language_not_english` findings
Delta: ADDED

### Requirement: Net-cost protocol exists before any gain claim

The program SHALL write n, arms, corpus, quality criterion and statistical
test before any gain claim. G-T remains owner-only.

#### Scenario: stats cited as gain without the protocol

- **WHEN** `stats` is cited as a gain
- **THEN** the protocol document already exists; otherwise the claim is refused
Delta: ADDED

## Non-goals

- Inventing the statistical method, n, or arms in this document.
- Closing the program while owner-only rows remain.

## Open decisions

- [x] none — closeout and ADR ratification stay owner-gated (PROGRAM); the
      net-cost protocol is written here without a gain claim.

## Tasks

| # | Task | Files | Verify | Status | Evidence |
|---|---|---|---|---|---|
| T1 | Closeout checklist versus PROGRAM owner-only table; no gate moved to agents | `docs/sdd/PROGRAM.md`, `docs/sdd/015-program-closeout/` | `jacu sdd lint --all` | done | docs/sdd/015-program-closeout/closeout-checklist.md |
| T2 | Manual archive plus `sdd close` for 001 and 002 | `docs/sdd/archive/` | `jacu sdd close` | done | blocked-with-evidence in blocked.md; close refused off main |
| T3 | 003, 004, 008: owner completed evals then close, or explicit blocked-with-evidence pointing at PROGRAM | `docs/sdd/015-program-closeout/` | `jacu sdd lint` | done | blocked.md points at PROGRAM owner-only rows |
| T4 | Owner ratification packet for ADR-021, ADR-022, ADR-026, ADR-027 | `docs/sdd/015-program-closeout/` | `test -f docs/sdd/015-program-closeout/ratification.md` | done | docs/sdd/015-program-closeout/ratification.md |
| T5 | English sweep of living agent-facing docs in later Allowed | `docs/sdd/`, `skills/` | `jacu sdd lint --all` | done | 009-015 sdd.md Portuguese-stopword count 0 |
| T6 | Write the net-cost protocol (n, arms, corpus, criterion, test); no gain claim in the same change | `docs/sdd/015-program-closeout/` | `test -f docs/sdd/015-program-closeout/net-cost-protocol.md` | done | docs/sdd/015-program-closeout/net-cost-protocol.md |
| T7 | PROGRAM queue: 015 remains last; owner-only table intact | `docs/sdd/PROGRAM.md` | `jacu sdd lint --all` | done | PROGRAM 015 last; owner-only table unchanged |

## Done

| Level | Proof |
|---|---|
| Close | 001 and 002 stay living; `sdd close` evidence recorded; archive deferred to owner on `main` |
| Honest | 003, 004, 008 remain open with evidence pointing at PROGRAM |
| Language | living 009-015 SDD English; ADR PT-BR untouched |
| Protocol | net-cost document exists; no gain claim without G-T |

## Follow-ups

- Implementation Allowed, by later amendment: `docs/sdd/015-program-closeout/**`,
  `docs/sdd/PROGRAM.md`, `docs/sdd/archive/**`, living 001-004 and 008
  directories for `git mv`. Still no `cmd/**` or `internal/**`. ADR status
  flips are owner edits to `docs/adr/**`.
- G-T, G-SR, P6-P8 and live-path evals remain owner-only.
