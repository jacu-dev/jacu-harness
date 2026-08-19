# Floors and limits — what counts as passing

Agreed numbers that serve as acceptance criteria.

## Autonomy eval floors

These four are the valid floors (decision D2 of 2026-08-14) and replace any other set:

| Metric | How to measure | Floor |
|---|---|---|
| Complete program without intervention | 1 real program of ≥ 3 missions, end to end | 100% of missions end in `applied` or an explicit escalation — none silent |
| Improper auto-apply | audit every automatic apply against the policy table | **zero** |
| Batch review cost | owner time to approve the whole program | < 10 min for 3 missions |
| Useful escalations | % of escalations the human agrees should have escalated | ≥ 80% |

**The second line is the one that kills:** a single auto-apply that should not have happened invalidates autonomy entirely, regardless of the other three.

## Output cap — the decision that replaced the artifact store

The budget is a **pure cap with digest**, not an artifact store.

The handler self-truncates by priority: first the tails of commands that passed (which nobody reads), then those that failed, **always preserving** `argv`, `status`, `exit_code`, `duration_ms`, `truncated` and the digest. It marks `truncated: true` and emits WARN naming the cut.

The generic `runtime.Execute` behaviour — zero `data` and degrade to `partial` — remains a safety net and **must not be reachable** by this tool. That is a test assertion, not an intention.

## A verdict is a struct, never a string

`summary` is a presentation projection and never a decision input. Whoever decides reads `verdict` and `commands[].status`.

Heritage: jacu-code decided error by literal equality with a phrase — adding duration to the formatter would have silently inverted the verdict of every successful command.

## What verify does NOT promise

Written here so it is not promised in the skill or the docs:

- **It is not a sandbox.** No network denial, no filesystem confinement. The command runs as the user's uid and may write outside the worktree. What is delivered is **argv governance** on top of a git-guaranteed workspace boundary.
- **Writes outside the worktree are checked only lexically** — absolute arguments and `..`. Detection, not containment; the OS enforces. An allowed command that writes outside through a path that does not look like a path (environment variable, config read from the worktree, symlink) passes.
- **Windows is out.** The executor is unix; `Setpgid` and `Kill(-pgid)` have no direct equivalent. On Windows the tool answers `blocked`.

## What the pipeline does NOT run — and when

Decision of 2026-08-15. Before that the pipeline was unconditional: any PR paid for all eight jobs.

The `changes` job now reads the PR diff and decides. Three rules make this safe, and none is optional:

- **`verify` is never skipped.** It runs `sdd lint --all`, and prose is exactly what breaks the SDD linter.
- **`checks` is never skipped.** A secret can enter through any file, including `.md`.
- **`.github/` does not count as prose.** Workflow changes are validated by the workflows.

## Measuring size and deciding removal are different walks

`treeStats` answers a **safety** question. `measureTree` answers a **size** question. They must not be fused.

## Standing performance budgets

| Budget | Value | State |
|---|---|---|
| Cold start | p95 < 150 ms | measured: p50 12 ms, p95 19 ms (n=20) |
| Operation | p95 < 1 s | verified in e2e |
| RSS idle · runtime overhead | — | **withdrawn** by decision D4: a budget nobody measures is an intention, not a budget |
