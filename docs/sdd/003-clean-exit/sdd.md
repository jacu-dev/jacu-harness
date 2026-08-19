---
sdd: 003-clean-exit
program: jacu-one-shot
spec_id: spc_pending
branch: 003-clean-exit
phase: docs/sdd/PROGRAM.md
adr: docs/adr/ADR-021-clean-exit.md
status: draft
---

# 003 — Clean exit

## Why

PROGRAM decision 3 says JACU wears a suit: delivery means landing on `main` with
no leftover branch, worktree, or stray file. Nothing enforces it, and the
repository proves the cost. At the time this document was cut there was an
orphaned worktree at `~/.jacu-harness/worktrees/prj_8dbdc74c3e474de9/run_544a9a0476da708e`,
commit `0583892`, still marked `locked`, from a run nobody remembers. It survived
because no code looks for it and no report mentions it.

An SDD also has no way to close. `CONVENTIONS.md` describes archiving as a manual
`git mv` plus a lock regeneration, and 001 deferred `sdd archive` as a
subcommand. That is fine as a procedure and terrible as a gate: the only thing
checking that a change actually finished is a person remembering to look.

This change makes leaving the room a verified step. It also carries the boundary
test that stops `internal/capability/workspace` — 7.418 lines, the largest slice
in the repository and the one this change touches hardest — from growing
sideways while nobody is watching.

## Locked decisions

1. JACU deletes only what JACU created: worktree, ephemeral branch, run state.
   Anything else is reported and never removed (PROGRAM decision 9).
2. Production tagging stays manual; clean exit never touches a `v*` tag
   (ADR-007; PROGRAM decision 3).
3. Zero new MCP tools. Clean exit reaches the host as a `jacu` CLI
   subcommand authorized through the `jacu_verify` allowlist (ADR-008; PROGRAM
   decision 3).
4. One lint per artifact, typed finding `{code, severity, target, message}`
   (PROGRAM decision 2).
5. Nothing closes without an eval that passes through the live path (PROGRAM
   decision 10).

## Out of scope

- Deleting anything JACU did not create. An untracked file the user wrote is
  reported, never removed, even when it is obviously garbage.
- Force-deleting a branch with unmerged commits. That is data loss wearing a
  cleanup costume; it escalates.
- Touching the remote. Clean exit reports a surviving remote branch and stops.
- `sdd archive` as an automated mover. Archiving stays the manual `git mv` of
  `CONVENTIONS.md`; `sdd close` verifies it happened, it does not do it.

## Write scope

**Allowed**

```
docs/sdd/003-clean-exit/**
docs/sdd/specs/cleanexit/spec.md
docs/evals/clean-exit.md
docs/adr/ADR-021-clean-exit.md
docs/relatorios/sdd-003-execucao.md
internal/capability/cleanexit/**
internal/capability/sdd/**
internal/capability/workspace/boundary_test.go
internal/runstate/**
cmd/jacu/sdd.go
cmd/jacu/sdd_test.go
cmd/jacu/main.go
skills/jacu-sdd/SKILL.md
scripts/verify.sh
.jacu/verify-allowlist.json
```

**Forbidden**

```
internal/capability/memory/**
internal/capability/orchestration/**
internal/modelcontrol/**
internal/mcpadapter/**
.github/**
```

## Requirements

### Requirement: Clean exit detects every class of leftover

The system SHALL classify a workspace as clean or, when not, SHALL report each
leftover under exactly one of `branch_local`, `branch_remote`, `worktree`,
`untracked`, `stash`, `run_open`, `main_mismatch`.

#### Scenario: an orphaned locked worktree is found

- **WHEN** a worktree exists under the project's worktree root with no open run
  referencing it
- **THEN** the check reports `worktree` with the path and the lock state, and the
  verdict is `fail`

Delta: ADDED

#### Scenario: a clean tree passes

- **WHEN** the branch is merged, the worktree removed, no run open and `main`
  matches the remote
- **THEN** the verdict is `pass` and no failure class is reported

Delta: ADDED

### Requirement: Clean exit removes only what JACU created

The system SHALL remove only a worktree, ephemeral branch or run-state entry it
created, and SHALL report anything else without touching it.

#### Scenario: a user file is reported, never removed

- **WHEN** an untracked file JACU did not create is present
- **THEN** it is reported under `untracked` and still exists after the command
  returns

Delta: ADDED

#### Scenario: an unmerged branch escalates instead of being deleted

- **WHEN** the ephemeral branch carries commits absent from `main`
- **THEN** the command refuses to delete it, reports `branch_local`, and exits
  non-zero

Delta: ADDED

### Requirement: An SDD cannot close over unfinished work

The system SHALL refuse `sdd close` while the change has a task that is not
`done` or `blocked`, a `done` task without evidence, or a BLOCK from the lint.

#### Scenario: a task in flight blocks the close

- **WHEN** any task is `doing`
- **THEN** `sdd close` exits 2 with a typed finding naming the task

Delta: ADDED

#### Scenario: closing requires the archive to exist

- **WHEN** the change directory has not been moved under `docs/sdd/archive/`
- **THEN** `sdd close` reports the expected destination and exits non-zero

Delta: ADDED

### Requirement: The workspace slice declares its dependencies

The system SHALL fail the build when `internal/capability/workspace` gains an
internal import not on its declared list.

#### Scenario: a new internal import is rejected

- **WHEN** a file in the workspace slice imports an internal package outside the
  declared set
- **THEN** the boundary test fails, naming the package and the file

Delta: ADDED

## Non-goals

- A general repository cleaner. Clean exit is scoped to what one run created.
- Garbage collecting old runs on a schedule. Retention is a separate concern.
- Deciding whether work should merge. Clean exit runs after that decision.

## Open decisions

- [x] none — resolved before this document was cut. ADR-021 records the
  failure-class enum and the delete-only-what-we-created boundary; T1 writes it
  and the owner ratifies separately, exactly as ADR-019 was handled for 001.

## Tasks

| # | Task | Files | Verify | Status | Evidence |
|---|---|---|---|---|---|
| T1 | Write ADR-021: the seven failure classes, the delete boundary, and why `sdd close` verifies the archive instead of performing it | `docs/adr/ADR-021-clean-exit.md` | `wc -l` under 120; owner ratifies separately | done | `wc -l docs/adr/ADR-021-clean-exit.md` -> `51`; ADR written in commit `8129e33`; owner ratification remains a hard-stop human gate |
| T2 | RED: detector per failure class, each with a fixture repository; an unknown state never panics | `internal/capability/cleanexit/detect_test.go` | `go test ./internal/capability/cleanexit -race` fails on absence | done | RED commit `269e43d`; failed to compile with `undefined: Detect` |
| T3 | GREEN: `Detect(project) Report` returning typed findings | `internal/capability/cleanexit/detect.go` | `go test ./internal/capability/cleanexit -race` | done | GREEN commit `df78c3c`; `go test ./internal/capability/cleanexit -race` -> `Go test: 9 passed in 1 packages` |
| T4 | RED: orphaned locked worktree, reproducing `run_544a9a0476da708e`, is reported as `worktree` with its lock state | `internal/capability/cleanexit/worktree_test.go` | `go test ./internal/capability/cleanexit -race` fails | done | RED commit `37d5baa` failed because `Finding.Locked` was absent |
| T5 | GREEN: worktree detection including locked entries | `internal/capability/cleanexit/worktree.go` | `go test ./internal/capability/cleanexit -race` | done | GREEN commit `3fd0310`; `go test ./internal/capability/cleanexit -race` -> `Go test: 10 passed in 1 packages` |
| T6 | RED: removal touches only JACU-created paths; a user file survives and an unmerged branch escalates | `internal/capability/cleanexit/remove_test.go` | `go test ./internal/capability/cleanexit -race` fails | done | RED commit `af7b3d1` failed because `Remove` was absent |
| T7 | GREEN: bounded removal, fail-closed on anything ambiguous | `internal/capability/cleanexit/remove.go` | `go test ./internal/capability/cleanexit -race` | done | GREEN commit `7ac1456`; `go test ./internal/capability/cleanexit ./internal/runstate -race` -> `Go test: 48 passed in 2 packages` |
| T8 | RED: the clean-exit receipt records verdict, classes and the paths removed, and carries no free text | `internal/capability/cleanexit/receipt_test.go` | `go test ./internal/capability/cleanexit -race` fails | done | RED commit `2171939` failed because `NewReceipt` was absent |
| T9 | GREEN: receipt as an audit artifact | `internal/capability/cleanexit/receipt.go` | `go test ./internal/capability/cleanexit -race` | done | GREEN commit `dff0354`; `go test ./internal/capability/cleanexit -race` -> `Go test: 12 passed in 1 packages` |
| T10 | RED: `cleanexit.close` telemetry event with `result` and `failure_class`, per the v2 catalogue | `internal/capability/cleanexit/telemetry_test.go` | `go test ./internal/capability/cleanexit -race` fails | done | RED commit `077a013` failed because the event and typed fields were absent |
| T11 | GREEN: emission through the v2 constructor | `internal/capability/cleanexit/telemetry.go` | `go test ./... -race` | done | GREEN commit `af92a76`; `go test ./internal/capability/cleanexit -race` -> `Go test: 13 passed in 1 packages` |
| T12 | RED: `sdd close` exit codes 0/1/2; refuses on a task in flight, on `done` without evidence, on a lint BLOCK, and on a missing archive | `cmd/jacu/sdd_test.go` | `go test ./cmd/... -race` fails | done | RED commit `1ea230f` failed because `sddClose` was absent |
| T13 | GREEN: `jacu sdd close`, wired into `main.go` | `cmd/jacu/sdd.go`, `cmd/jacu/main.go` | `go test ./cmd/... -race` | done | GREEN commit `929caed`; `go test ./cmd/jacu ./internal/capability/cleanexit -race` -> `Go test: 32 passed in 2 packages` |
| T14 | RED: boundary test pinning the internal imports of `internal/capability/workspace` | `internal/capability/workspace/boundary_test.go` | `go test ./internal/capability/workspace -race` fails when a package is added | done | RED commit `2d49896` failed because the boundary scanner was absent |
| T15 | GREEN: declared list matching today's imports, with the rationale in a comment | `internal/capability/workspace/boundary_test.go` | `go test ./... -race` | done | GREEN commit `23ec4c5`; boundary test passes with the seven current internal imports |
| T16 | Authorize `jacu` prefix `["sdd","close"]` in the verify allowlist | `.jacu/verify-allowlist.json` | `jacu_verify` with that argv returns a verdict rather than an allowlist denial | done | Dedicated run `run_13e89b635aa877c8`; `jacu_verify` accepted the argv and returned typed verdict `fail` (exit 2: installed worktree binary reported unknown subcommand), evidence digest `sha256:21cf0f05134979c79b1c98896801dbb9994bac946336a6006ee9387c1f8df5b`; run discarded afterward |
| T17 | Add the clean-exit gate to the verify script | `scripts/verify.sh` | `bash scripts/verify.sh` | done | GREEN commit `e075dd6`; `bash scripts/verify.sh` -> `verify: OK` |
| T18 | Teach the skill the close step | `skills/jacu-sdd/SKILL.md` | `go test ./internal/mcpadapter -run Skills -race` | done | GREEN commit `7894257`; `go test ./internal/mcpadapter -run Skills -race` -> `Go test: 2 passed in 1 packages` |
| T19 | Write the living capability spec | `docs/sdd/specs/cleanexit/spec.md` | `go run ./cmd/jacu sdd lint --all` exits 0 | done | GREEN commit `fb1da3a`; `go run ./cmd/jacu sdd lint --all` exited 0 |
| T20 | Eval on the live path: close SDD-002 with this command and record the receipt | `docs/evals/clean-exit.md` | receipt attached, `sdd close` exits 0 on a real change | todo | Owner action remains required: manual archive preparation and ADR-021 ratification; current attempt is recorded as exit 2 and no receipt/deletion occurred |
| T21 | Confirm the MCP surface is untouched | — | `go test -tags=e2e ./test/e2e/ -run Governed` reports 13 tools | done | `go test -v -tags=e2e ./test/e2e/ -run Governed` -> 1 governed test passed; the test's 13-tool ratchet remained green |
| T22 | Write the execution report | `docs/relatorios/sdd-003-execucao.md` | PR with the hosted run link | done | Report committed; hosted CI run `31807729312` passed all 8 required jobs for SHA `91e5019` |

## Done

| Level | Proof |
|---|---|
| Core | `go test ./internal/capability/cleanexit -race` green, every failure class covered |
| Wiring | `sdd close` refuses on each blocking condition and exits 0 on a clean change; `bash scripts/verify.sh` green |
| E2E | `go test -tags=e2e ./test/e2e/` green with 13 tools under the ratchet |
| Eval | SDD-002 closed through this command on the live path, receipt in the report |

## Follow-ups

- `sdd archive` as a subcommand, still deferred: revisit when an eval shows the
  manual `git mv` costs more than the CLI surface it would add.
- Retention for old runs and worktrees, once clean exit shows how many accumulate.
- Extending the boundary test to `internal/capability/orchestration`, the other
  coupling hub.
