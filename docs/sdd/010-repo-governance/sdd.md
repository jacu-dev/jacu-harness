---
sdd: 010-repo-governance
program: jacu-one-shot
spec_id: spc_pending
branch: 010-repo-governance
phase: docs/sdd/PROGRAM.md
adr: docs/adr/ADR-007.md
status: draft
---

# 010 — Repo governance: protected paths, per-run home, status, fan-out

## Why

PROGRAM queues this change to put four repository-local governance seams in
one place: `.jacu/protected.json`, per-run `JACU_HOME`, `status` off the write
gate, and a flow fan-out cap.

ADR-007 already carved CODEOWNERS as GitHub enforcement outside the kernel.
That is not this change. CODEOWNERS cannot stop a worktree apply. This SDD is
JACU refusing protected paths inside the worktree.

The document is written now so the queue has no gaps. Implementation follows
016, 017 and 009. Schema, home layout, gate mechanics and the numeric fan-out
cap are implementation decisions, not this document's entry gate.

## Locked decisions

1. CODEOWNERS is GitHub enforcement outside the kernel. This change is JACU
   refusing `.jacu/protected.json` paths in the worktree — ADR-007.
2. Zero new MCP tools. Status stays `jacu_status` — ADR-008.
3. No network, credential or deploy path — PROGRAM invariant.

## Out of scope

- Editing GitHub rulesets, CODEOWNERS, or required reviewers.
- Automatic deletion of foreign or dirty worktrees (`docs/decisions/will-not-do.md`).
- New MCP tools.

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

### Requirement: Protected paths refuse apply

The system SHALL refuse apply when the reviewed tree touches a path listed in
`.jacu/protected.json`.

#### Scenario: a listed path is in the reviewed tree

- **WHEN** apply is requested and the reviewed tree contains a path matching
  `.jacu/protected.json`
- **THEN** apply returns `blocked` and nothing is committed
Delta: ADDED

#### Scenario: CODEOWNERS is not this list

- **WHEN** `.github/CODEOWNERS` lists a path that `.jacu/protected.json` does not
- **THEN** JACU does not refuse that path
Delta: ADDED

### Requirement: Per-run JACU_HOME

Each workspace run SHALL use an isolated `JACU_HOME`, not the process-wide
user directory, for run-owned files.

#### Scenario: two open runs do not share home writes

- **WHEN** two runs are open
- **THEN** neither run writes the other's memory, telemetry or toolchain files
Delta: ADDED

### Requirement: Status is off the write gate

`jacu_status` SHALL not acquire the workspace mutation gate.

#### Scenario: status during apply

- **WHEN** apply holds the mutation gate
- **THEN** status still returns
Delta: ADDED

### Requirement: Flow fan-out is capped

The orchestration scheduler SHALL bound wave width before execute.

#### Scenario: a wave exceeds the cap

- **WHEN** a flow wave would exceed the cap
- **THEN** the flow blocks with a typed finding and does not spawn the extra nodes
Delta: ADDED

## Non-goals

- Choosing the numeric fan-out cap or the `protected.json` schema in this
  document. Those are implementation decisions recorded when code starts.
- Replacing GitHub CODEOWNERS.

## Open decisions

- [x] none — queued from PROGRAM; protected.json schema, per-run JACU_HOME,
      status gate, and fan-out cap are implementation decisions, not this
      document's entry gate.

## Tasks

| # | Task | Files | Verify | Status | Evidence |
|---|---|---|---|---|---|
| T1 | Record the protected-path ADR (fail-closed apply; CODEOWNERS is not this list) | `docs/adr/` | `wc -l` under 120; owner ratifies separately | todo | |
| T2 | RED: apply blocked on a protected path | `internal/capability/workspace/` | `go test ./internal/capability/workspace -race` | todo | |
| T3 | GREEN: load `.jacu/protected.json` and refuse listed paths | `internal/capability/workspace/` | `go test ./internal/capability/workspace -race` | todo | |
| T4 | RED then GREEN: per-run home isolation | `internal/userstate/`, `internal/capability/workspace/` | `go test ./internal/userstate ./internal/capability/workspace -race` | todo | |
| T5 | RED then GREEN: status skips the mutation gate | `internal/capability/workspace/` | `go test ./internal/capability/workspace -race` | todo | |
| T6 | RED then GREEN: flow fan-out cap | `internal/capability/orchestration/` | `go test ./internal/capability/orchestration -race` | todo | |
| T7 | Update living specs after implementation | `docs/sdd/specs/` | `jacu sdd lint --all` | todo | |

## Done

| Level | Proof |
|---|---|
| Core | apply refuses protected paths; two runs isolate `JACU_HOME`; status returns while apply holds the gate; over-cap waves block |

## Follow-ups

- Implementation Allowed, by later amendment: `.jacu/protected.json`,
  `internal/capability/workspace/**`, `internal/capability/orchestration/**`,
  `internal/userstate/**`, `docs/sdd/010-repo-governance/**`, living specs.
- Numeric fan-out cap and `protected.json` schema are chosen in T1/T3, not here.
