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
3. **The runtime never arms auto-merge and never targets a base it was not
   told.** `autonomy_integration.go` loses `gh pr merge --auto`; `--base`
   becomes an explicit input defaulting to the integration branch when one is
   declared, else refusing. Arming auto-merge is the owner's act, in the
   outer loop, as the skill already states.
4. **Prose does not pay the suite.** `ci.yml` gains a `changed-code` job that
   classifies the pull request diff; `verify` runs when code changed; the
   aggregator accepts `skipped` only when the recorded reason is
   `prose-only`, and fails on any other skip. `skipped` is never approval by
   itself.
5. **Skills teach the branch model explicitly.** `jacu-workspace` step 7 and
   `jacu-sdd` gain the integration-branch hand-off; `jacu-autonomy` step 5
   stays as written because it was already right.

## Out of scope

- A permanent `develop` branch, gitflow, release branches.
- Changing `jacu_workspace_open` or `jacu_apply` semantics; they already base
  on HEAD and stop at the local commit.
- Enforcing delivery granularity mechanically (a check that reads the SDD and
  refuses a pull request that does not close a delivery) — follow-up.
- Changes to the sixteen `AGENTS.md` files in the organisation; this SDD
  changes this repository's skills and its own `AGENTS.md` only.

## Write scope

**Allowed**

```
internal/capability/workspace/autonomy_integration.go
internal/capability/workspace/autonomy_integration_test.go
internal/capability/workspace/autonomy_executor.go
internal/capability/workspace/autonomy_apply_test.go
.github/workflows/ci.yml
skills/jacu-workspace/SKILL.md
skills/jacu-sdd/SKILL.md
skills/jacu-mission/SKILL.md
AGENTS.md
docs/reference/cli.md
CHANGELOG.md
docs/sdd/023-integration-branch-per-sdd/**
```

**Forbidden**

```
internal/gitx/**
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

### Requirement: Autonomy never arms auto-merge

The system SHALL NOT run `gh pr merge --auto` from any code path, and SHALL
refuse to create a pull request without an explicit base.

#### Scenario: autonomy integration without a declared base
- **WHEN** `integrateAutonomy` is called and no integration base is declared
- **THEN** it returns `Escalated: true` with a warning naming the missing base and runs no `gh` command
Delta: ADDED

#### Scenario: autonomy integration with a declared base
- **WHEN** an integration base `sdd/<NNN>` is declared
- **THEN** the branch is pushed and a pull request is created against that base; no merge command runs
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

### ENTREGA-1 — the skills teach the model (T1..T3)
After merge: every host that reads the skills works an SDD on `sdd/<NNN>`,
merges runs locally, and opens one pull request per delivery. What goes red
if wrong: `jacu sdd lint --all`, and the living-docs allowlist test.

### ENTREGA-2 — the runtime stops opening pull requests by itself (T4..T6)
After merge: no code path arms auto-merge; the base is explicit. What goes
red if wrong: `go test ./internal/capability/workspace -run Autonomy`.

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
| T4 | Drop `gh pr merge --auto --squash` from `autonomyIntegrationCommands`; add `base string`; refuse when empty | `internal/capability/workspace/autonomy_integration.go` | `go test ./internal/capability/workspace -run Autonomy` | todo | |
| T5 | Thread the base from the program declaration through the executor | `internal/capability/workspace/autonomy_executor.go` | `go test ./internal/capability/workspace -run Autonomy` | todo | |
| T6 | Tests: no base → escalated, no command; base → push + create, no merge | `internal/capability/workspace/autonomy_integration_test.go`, `autonomy_apply_test.go` | `go test ./internal/capability/workspace -run Autonomy -race` | todo | |
| T7 | `changed-code` job in `ci.yml`; `verify` gated on it; aggregator reads the reason | `.github/workflows/ci.yml` | docs-only probe PR under 60s; code PR runs verify | todo | |
| T8 | Probe PR with `verify` skipped for a non-prose reason is refused | `.github/workflows/ci.yml` | probe PR blocked | todo | |
| T9 | `AGENTS.md` of this repository names `sdd/<NNN>`, the local merge, and the one-PR-per-delivery rule; CHANGELOG Unreleased/Changed | `AGENTS.md`, `CHANGELOG.md` | `go run ./cmd/jacu sdd lint --all` | todo | |

## Done

| Level | Proof |
|---|---|
| Core | an SDD with three tasks produces one pull request, opened by the owner, from `sdd/<NNN>`; the run branches never reach `origin` |
| Runtime | `grep -rn 'pr merge' internal/` returns nothing; autonomy without a base escalates |
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
