---
sdd: 023-integration-branch-per-sdd
program: jacu-one-shot
spec_id: spc_pending
branch: 023-integration-branch-per-sdd
phase: docs/sdd/PROGRAM.md
adr: none
status: draft
---
# 023 — One integration branch per SDD, one pull request per delivery

## Why

Measured on 2026-08-21 across the `jacu-dev` organisation: 166 pull requests
merged in one day. In `ecouto`, five of seventeen touched the same
`AGENTS.md` and one (#98) fixed the one before it (#97). In `infra`, six
pull requests edited the same file. A documentation-only pull request in
this repository paid 3m38s of `verify`. The owner's words: *"if a comma
changes, the LLM opens a pull request; a pull request delivers value, not
edits."*

The rule already exists. `AGENTS.md` says *pull request is a unit of value,
not of edit*, and *an SDD task is not a merge point*. It is prose, and
nothing in the toolchain distinguishes a delivery from a comma.

Three facts decide the design:

1. **JACU does not require `main`.** `jacu_workspace_open` bases the run on
   `git.RevParseHead(root)` — the HEAD of the checkout, whatever branch it is.
   `jacu_apply` commits to the run branch and stops; its `next_actions` says
   *"merge … into main when ready"* as text, not as a check. A checkout
   sitting on an integration branch therefore opens runs from it and merges
   them back locally with no pull request at all. The tool already supports
   the workflow the owner wants; the skills never say so.
2. **The autonomy integration contradicts its own skill.**
   `skills/jacu-autonomy/SKILL.md` step 5 says the runtime *"does not push,
   open a PR, or arm auto-merge. Remote integration stays in the outer
   loop."* `internal/capability/workspace/autonomy_integration.go` pushes,
   runs `gh pr create --base main` and `gh pr merge --auto --squash`. One of
   the two is wrong, and PROGRAM invariant I10 says the document must not
   contradict the code. The code is the one that opens pull requests for
   commas.
3. **Prose pays for the test suite.** `ci.yml` runs the full `verify` for
   every pull request. `ecouto` and `bem-te-vi` already carry a
   `mudou-codigo` job that classifies the diff and lets the aggregator accept
   a *reasoned* skip; this repository does not, and it is the one where a
   docs-only pull request is most common.

A permanent `develop` branch was considered and rejected: it would need its
own ruleset in sixteen repositories, `main-verde` and `guardiao` only watch
`main`, and the gate would fire once, late, on a large pull request — the
failure mode the SDD "deliveries" exist to prevent. The integration branch
here is short-lived and dies with its SDD.

## Locked decisions

1. **An SDD is worked on one integration branch, `sdd/<NNN>`, created from
   `main` and deleted after its last delivery merges.** JACU runs open from
   that branch's HEAD; each applied run branch is merged locally into
   `sdd/<NNN>`; no pull request is opened per run or per task.
2. **A pull request is opened per delivery, from `sdd/<NNN>` to `main`,
   never per task.** The delivery is the smallest set of tasks that leaves
   `main` working and delivers something observable — as `AGENTS.md` already
   defines. The SDD's `## Entregas` (or `## Deliveries`) section names them.
3. **The runtime never arms auto-merge and never opens a pull request on its
   own.** `autonomy_integration.go` loses both `gh pr create` and
   `gh pr merge --auto`. After a policy-gated apply, the autonomy executor
   integrates **locally**: fast-forward or merge of the run branch into
   `sdd/<NNN>`, then the next mission opens from the new HEAD. Nothing
   reaches `origin` during the program. This is what makes one-shot
   autonomous: a program of eight missions produces eight local merges and
   zero pull requests.
3a. **Delivery is an explicit act, by CLI, never by MCP tool.** A new
   subcommand `jacu deliver [--base main] [--title …]` pushes `sdd/<NNN>`,
   opens the pull request for the current delivery and prints its URL; it
   never arms auto-merge. It runs when the owner asks, or when a program
   declares `deliver_at_end: true` and its last mission applied with verdict
   `pass`. PROGRAM: new capability enters through a CLI subcommand; the MCP
   catalogue does not grow.
4. **Prose does not pay the suite.** `ci.yml` gains a `changed-code` job that
   classifies the pull request diff; `verify` runs when code changed; the
   aggregator accepts `skipped` only when the recorded reason is
   `prose-only`, and fails on any other skip. `skipped` is never approval by
   itself.
5. **Skills teach the branch model explicitly.** `jacu-workspace` step 7 and
   `jacu-sdd` gain the integration-branch hand-off; `jacu-autonomy` step 5
   gains the local merge and `jacu deliver`; `jacu-mission` gains the three
   questions that separate a mission from a fragment.
6. **Every skill claim is checked against the code in this SDD**, and what
   does not match is corrected here, not deferred. The review is in the
   section below.

## Skill review against the code (2026-08-21)

All twelve skills were read and each operational claim was checked against
`cmd/jacu` and `internal/`.

| Skill | Claim | Code | Verdict |
|---|---|---|---|
| `jacu-autonomy` §5 | runtime "does not push, open a PR, or arm auto-merge" | `autonomy_integration.go:46-49` pushes, creates, arms `--auto --squash` | **code wrong**; fixed by decision 3 (T4–T6) |
| `jacu-autonomy` §7, §9 | "escalates to Erick", "until Erick runs the eval sheet" | public repository, skill read by every host | **personal name in a public skill**; replace with "the owner" (T10) |
| `jacu-workspace` §1 | `jacu_status` "takes no arguments" | `status.go:62` accepts `task_id`; `jacu-verify` §2 relies on it | **contradiction between two skills**; fixed in this delivery |
| `jacu-workspace` §1 | legacy alias `jacu_workspace_status` "is equivalent" | `tool.go:87` registers it as a **second MCP tool** | true — and it costs catalogue bytes under the 20 KiB ratchet while SDD-009 is fighting for them. Thirteen tools on the wire, not twelve; SDD-009's follow-up saying "the number is stale" was wrong, PROGRAM was right (T11) |
| `jacu-sdd` §9 | `sdd close` "never … deletes user-created paths" | `sdd.go:93-97` runs `cleanexit.Remove` on JACU-owned leftovers | true but incomplete; reworded in this delivery |
| `jacu-sdd` | `sdd status` is a summary | aborts on the first unlocked directory (SDD-009 T10) | documented there |
| `jacu-runner` | "positive nine-variable environment allowlist" | `runner.go:305`: nine names | matches |
| `jacu-model` | "at least 10 samples and failure rate strictly above 40%" | `resilience.go:36`: `TotalRuns >= 10 && rate > 0.40` | matches |
| `jacu-orchestration` §1 | capabilities `mission, workspace, verify, review, apply` | `graph.go:13-17` | matches |
| `jacu-verify` §3 | five verdicts | `core.go:24-28` | matches |
| `jacu-verify` | "a cancelled run is `not_run`, never `fail`" | task status `cancelled` exists; verdict mapping not located | **unverified**; T12 adds the test or fixes the prose |
| `jacu-report` | `report render` / `report serve` on 127.0.0.1 | `report_visual.go:31-33`, `--port` | matches; "eight v1 blocks" unverified (T12) |
| `jacu-memory` | save refreshes only the sentinelled `AGENTS.md` region, checksum-guarded | `bridge.go:85-91` | matches |
| `jacu-inspect`, `using-jacu` | routing only | — | no operational claim to check |

## Out of scope

- A permanent `develop` branch, gitflow, release branches.
- Changing `jacu_workspace_open` or `jacu_apply` semantics; they already base
  on HEAD and stop at the local commit.
- Enforcing delivery granularity mechanically (a check that reads the SDD and
  refuses a pull request that does not close a delivery) — follow-up.
- A new MCP tool for delivery. Delivery is CLI-only by PROGRAM rule and
  because a tool that pushes is exactly the capability the MCP surface must
  not have.
- Changes to the sixteen `AGENTS.md` files in the organisation; this SDD
  changes this repository's skills and its own `AGENTS.md` only.

## Write scope

**Allowed**

```
internal/capability/workspace/autonomy_integration.go
internal/capability/workspace/autonomy_integration_test.go
internal/capability/workspace/autonomy_executor.go
internal/capability/workspace/autonomy_apply_test.go
internal/capability/workspace/tool.go
internal/capability/missioncompile/program.go
internal/gitx/merge.go
internal/gitx/merge_test.go
cmd/jacu/deliver.go
cmd/jacu/deliver_test.go
cmd/jacu/main.go
.github/workflows/ci.yml
skills/jacu-workspace/SKILL.md
skills/jacu-sdd/SKILL.md
skills/jacu-mission/SKILL.md
skills/jacu-autonomy/SKILL.md
skills/jacu-verify/SKILL.md
skills/jacu-report/SKILL.md
AGENTS.md
docs/reference/cli.md
CHANGELOG.md
docs/sdd/023-integration-branch-per-sdd/**
```

**Forbidden**

```
internal/gitx/** except merge.go
internal/capability/workspace/open.go
internal/capability/workspace/apply.go
.github/workflows/verify.yml
.github/workflows/guardiao.yml
scripts/**
```

## Requirements

### Requirement: Runs integrate locally into the SDD branch

The system SHALL open runs from the checkout's HEAD and SHALL document that
the checkout is expected to sit on `sdd/<NNN>` while an SDD is being worked.

#### Scenario: task applied on an integration branch
- **WHEN** the checkout is on `sdd/023` and a run is applied
- **THEN** the commit lands on the run branch based on `sdd/023`'s HEAD, and the skill's next step is a local merge into `sdd/023`, not a pull request
Delta: ADDED

### Requirement: Autonomy integrates locally and never reaches origin

The system SHALL NOT run `gh pr create` or `gh pr merge` from any autonomy
code path. After a policy-gated apply it SHALL merge the run branch into the
integration branch locally.

#### Scenario: program of N missions
- **WHEN** a program with `deliver_at_end` unset applies N missions with verdict `pass`
- **THEN** `sdd/<NNN>` advances by N local merges, `origin` is untouched, and no `gh` command runs
Delta: ADDED

#### Scenario: local merge conflicts
- **WHEN** the run branch does not merge cleanly into `sdd/<NNN>`
- **THEN** the mission escalates with the worktree preserved; dependent missions wait, independent ones continue
Delta: ADDED

### Requirement: Delivery is an explicit CLI act

The system SHALL provide `jacu deliver` that pushes the integration branch
and opens one pull request, and SHALL NOT arm auto-merge.

#### Scenario: owner delivers
- **WHEN** `jacu deliver --base main` runs on `sdd/<NNN>` with a clean tree
- **THEN** the branch is pushed, one pull request is created, its URL is printed, and no merge command runs
Delta: ADDED

#### Scenario: program delivers at end
- **WHEN** a program declares `deliver_at_end: true` and its last mission applied with `pass`
- **THEN** the executor runs the same code path as `jacu deliver` once, after the last local merge
Delta: ADDED

#### Scenario: deliver with a dirty tree or no integration branch
- **WHEN** the checkout is not on `sdd/<NNN>` or has uncommitted changes
- **THEN** `jacu deliver` exits 2 naming the condition and runs no `git push`
Delta: ADDED

### Requirement: Prose-only pull requests skip the suite with a recorded reason

#### Scenario: docs-only diff
- **WHEN** a pull request changes only `*.md`, `docs/**`, `skills/**`, `LICENSE`, `CODE_OF_CONDUCT.md`
- **THEN** `verify` is skipped, the aggregator reads reason `prose-only` and passes, and the run completes in under one minute
Delta: ADDED

#### Scenario: any other skip
- **WHEN** `verify` is skipped for any reason other than `prose-only`
- **THEN** the aggregator fails
Delta: ADDED

#### Scenario: push to main
- **WHEN** the event is not `pull_request`
- **THEN** `verify` runs unconditionally
Delta: ADDED

## Non-goals

- Merge queue; not available on this plan.
- Reducing `verify` duration for code changes.

## Open decisions

- none — the integration branch name is `sdd/<NNN>` to match the SDD
  directory prefix, and the `autonomy_integration.go` functions keep their
  signatures with one added `base string` parameter.

## Entregas

### ENTREGA-1 — the skills teach the model (T1..T3, T10)
After merge: every host that reads the skills works an SDD on `sdd/<NNN>`,
merges runs locally, and opens one pull request per delivery. What goes red
if wrong: `jacu sdd lint --all`, and the living-docs allowlist test.

### ENTREGA-2 — one-shot autonomy: local merges, `jacu deliver` at the end (T4..T6a, T11, T12)
After merge: a program runs end to end with zero pull requests; `jacu
deliver` opens the one pull request when the owner or the program asks. What
goes red if wrong: `go test ./internal/capability/workspace -run Autonomy`
and `go test ./cmd/jacu -run Deliver`.

### ENTREGA-3 — prose is cheap (T7..T8)
After merge: a docs-only pull request finishes in under a minute, and a
non-prose skip fails the aggregator. What goes red if wrong: a probe pull
request with a `skipped` verify and no `prose-only` reason.

## Tasks

| # | Task | Files | Verify | Status | Evidence |
|---|---|---|---|---|---|
| T1 | `jacu-workspace` step 7: hand off to a local merge into `sdd/<NNN>`; pull request only per delivery; never `--auto` from the session | `skills/jacu-workspace/SKILL.md` | `go run ./cmd/jacu sdd lint --all` | done | edited with this SDD |
| T2 | `jacu-sdd`: step 0 creates `sdd/<NNN>` from `main`; step 10 deletes it after the last delivery; deliveries section is mandatory for ceremony `full` | `skills/jacu-sdd/SKILL.md` | `go run ./cmd/jacu sdd lint --all` | done | edited with this SDD |
| T3 | `jacu-mission`: compile refuses an objective that is a fragment of a delivery (no observable outcome) with a WARN naming the delivery | `skills/jacu-mission/SKILL.md` | `go run ./cmd/jacu sdd lint --all` | done | edited with this SDD |
| T4 | Replace `autonomyIntegrationCommands` (push + `pr create` + `pr merge --auto`) with a local merge of the run branch into the integration branch through `gitx`; conflict → escalate with worktree preserved | `internal/capability/workspace/autonomy_integration.go`, `internal/gitx/**` (merge primitive only) | `go test ./internal/capability/workspace -run Autonomy` | todo | |
| T5 | Executor: next mission opens from the advanced HEAD; `deliver_at_end` in the program schema | `internal/capability/workspace/autonomy_executor.go`, `internal/capability/missioncompile/program.go` | `go test ./internal/capability/workspace -run Autonomy` | todo | |
| T6 | Tests: N missions → N local merges, zero `gh`; conflict → escalated; `deliver_at_end` → one deliver call | `internal/capability/workspace/autonomy_integration_test.go`, `autonomy_apply_test.go` | `go test ./internal/capability/workspace -run Autonomy -race` | todo | |
| T6a | `jacu deliver [--base main] [--title …] [--json]`: preconditions, push, `gh pr create`, print URL, never `--auto`; exit codes documented | `cmd/jacu/deliver.go`, `cmd/jacu/deliver_test.go`, `docs/reference/cli.md` | `go test ./cmd/jacu -run Deliver` | todo | |
| T7 | `changed-code` job in `ci.yml`; `verify` gated on it; aggregator reads the reason | `.github/workflows/ci.yml` | docs-only probe PR under 60s; code PR runs verify | todo | |
| T8 | Probe PR with `verify` skipped for a non-prose reason is refused | `.github/workflows/ci.yml` | probe PR blocked | todo | |
| T9 | `AGENTS.md` of this repository names `sdd/<NNN>`, the local merge, `jacu deliver`, and the one-PR-per-delivery rule; CHANGELOG Unreleased/Changed | `AGENTS.md`, `CHANGELOG.md` | `go run ./cmd/jacu sdd lint --all` | todo | |
| T10 | `jacu-autonomy`: §5 describes the local merge and `jacu deliver`; §7 and §9 lose the personal name | `skills/jacu-autonomy/SKILL.md` | `go run ./cmd/jacu sdd lint --all` | done | edited with this SDD |
| T11 | Decide the legacy tool alias `jacu_workspace_status`: drop it (frees catalogue bytes for SDD-009) or keep it and say why in `tool.go` | `internal/capability/workspace/tool.go`, `skills/jacu-workspace/SKILL.md` | `go test ./test/e2e -run TestGovernedChange` | todo | |
| T12 | Prove or fix the two unverified skill claims: cancelled verify → `not_run`; report has eight v1 blocks | `internal/capability/verify/task_test.go`, `internal/report/report_test.go`, or the two skills | `go test ./internal/capability/verify ./internal/report` | todo | |

## Done

| Level | Proof |
|---|---|
| Core | an SDD with three tasks produces one pull request, opened by the owner, from `sdd/<NNN>`; the run branches never reach `origin` |
| Runtime | `grep -rn 'gh.*pr' internal/` returns nothing; a program of N missions leaves `origin` untouched; `jacu deliver` is the only path that pushes |
| Esteira | docs-only pull request: `verify` skipped with reason `prose-only`, aggregator green in under a minute; any other skip is red |

## Follow-ups

- Mechanical delivery check: the `guardiao` reads the SDD path named in the
  pull request body and refuses a pull request whose diff does not close a
  declared delivery. Out of scope here because it needs the SDD parser on the
  hosted runner.
- Propagate the `sdd/<NNN>` rule to the organisation-wide `AGENTS.md`
  (fifteen repositories) in one verified round — the propagation script from
  the 21/08 audit, not by hand.
- `PROGRAM.md` out-of-scope list says *"no new MCP tool"*; this SDD adds
  none, but the `changed-code` job is a second place where skip reasons are
  encoded — consider moving the classification into `jacu preflight` so the
  CLI and the workflow share one definition.
