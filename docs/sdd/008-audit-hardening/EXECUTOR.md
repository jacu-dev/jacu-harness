# Executor contract — SDD-008 audit hardening

This file narrows the global rules in `docs/sdd/EXECUTOR.md` for SDD-008. It
does not replace or weaken them. When the two documents differ on authority,
this file is stricter: the SDD-008 executor is local-only and never publishes.

## Fixed read order

Before changing a file, read:

1. `docs/sdd/PROGRAM.md`
2. `docs/sdd/CONVENTIONS.md`
3. `docs/sdd/EXECUTOR.md`
4. this file
5. `docs/sdd/008-audit-hardening/sdd.md`
6. `docs/adr/ADR-026-audit-hardening-delivery.md`
7. `docs/adr/ADR-027-owned-storage-lifecycle.md`
8. `docs/ameacas.md` and `docs/hygiene.md`
9. `plans/README.md`, then the individual plan named by the current task only
10. the existing ADR and living specs cited by that plan

Do not preload historical reports or archived SDDs. They are evidence, not the
current execution contract, and they are outside write scope.

## Entry gate

Start only when all conditions are true:

- the owner explicitly ratified SDD-008 and ADR-026/ADR-027 for local execution;
- the current branch is exactly `codex/008-audit-hardening`;
- `git merge-base HEAD origin/main` is
  `4a846296f5efa6305ae0ee03fe420844f1b28ed2`;
- the SDD, both ADRs, audit plans and SDD lock are already committed;
- the worktree is clean;
- `go run ./cmd/jacu sdd lint docs/sdd/008-audit-hardening` exits 0.

If any condition is false, stop with a precise report. Do not recreate or
rewrite the input contract to make the gate pass.

## Authority

You may:

- edit only paths in the SDD Allowed write scope;
- execute repository-local tests and read-only diagnostics;
- create local Conventional Commits at the boundaries defined below;
- update only SDD task status/evidence and regenerate its lock after the
  corresponding task is objectively complete;
- continue an independent task after a blocked task when the dependency graph
  permits it.

You must never:

- run `git push`, `gh pr create`, `gh pr merge`, `gh api --method` or any other
  command that mutates hosted state;
- edit a GitHub ruleset, branch protection, repository secret or workflow run;
- create, move, sign or delete any tag;
- publish a release, package or deployment;
- merge, rebase onto an unreviewed remote tip, amend a pushed commit or force
  update a reference;
- use the user's real `~/.jacu-harness`, worktrees, Git objects, editor data, memory
  or telemetry as mutable test fixtures. The bounded `storage inspect` and
  `storage prune --dry-run` smoke commands may observe typed metadata only; do
  not record paths, payloads or user content and never use `--apply` for smoke;
- weaken an allowlist, required job, sanitizer, timeout, retention invariant or
  preservation rule to obtain a green result.

Read-only network research is allowed only when a plan requires upstream
verification. Prefer the primary project documentation or release page and
record the exact URL and version in the execution report. It grants no external
mutation authority.

## One-shot state machine

Run one task at a time in the SDD table. The dependency waves are:

1. baseline and supply chain: T1–T3;
2. fail-closed filesystem, Git and network boundaries: T4–T11;
3. bounded task and telemetry persistence: T12–T15;
4. deterministic watcher/status/test/hygiene gates: T16–T21;
5. unified storage lifecycle: T22–T23;
6. living truth, operator documentation and local closeout: T24–T25.

The dependencies in the table are authoritative. A failed RED test is expected
only when it fails for the intended missing behavior. An unrelated failure is a
blocker, not RED evidence. A GREEN task starts only after its RED commit exists.

For a blocked task:

1. leave its artifacts and user data untouched;
2. record command, exit code, bounded output and impact in
   `docs/relatorios/sdd-008-audit-hardening-execution.md`;
3. mark the task `blocked` with the report anchor;
4. skip every declared dependant;
5. continue the next independent task;
6. never infer, mock or fabricate human/hosted evidence.

## Command and file discipline

- Prefix shell commands with `rtk` as required by the repository environment.
- Use direct argv, temporary test-owned directories and fake clocks; do not use
  the user's home, global Git configuration, credentials or live storage.
- Keep production source files under 200 lines. Split by responsibility when a
  touched file would exceed the limit; do not move unrelated code merely to hit
  the number.
- Use `gofmt`; do not hand-format generated content.
- Use `git diff --check`, `git status --short`, `git diff --stat` and a scoped
  name-only review before every commit.
- Never use destructive broad commands, unresolved globs, `rm -rf`, force branch
  deletion or symlink-following traversal.

## Commit policy

Use local commits only. Behavioral changes use separate RED and GREEN commits:

- `test(<scope>): reproduce <failure>` for a failing regression;
- `fix(<scope>): <behavior>` or `feat(<scope>): <capability>` for the passing
  implementation and matching contract update.

Mechanical baseline, CI, hygiene and documentation tasks may use one focused
commit because their Verify command is observational rather than a unit RED.
Do not combine independent modules into one commit. Do not amend completed task
commits. The expected series is reviewable and may be squash-merged later; the
executor does not perform that squash.

After each task:

1. run its exact Verify command;
2. append exact evidence to the execution report;
3. update the task status/evidence pointer in `sdd.md`;
4. run `go run ./cmd/jacu sdd lint docs/sdd/008-audit-hardening --write-lock`;
5. review the lock and diff;
6. create the local commit;
7. confirm the worktree is clean before starting the next task.

T25 has one closing exception to avoid making corpus lint observe its own
uncommitted evidence: run the full source gates on the clean source HEAD, write
and commit the report/task evidence, then run the final clean-tree
`sdd lint --all`, `git diff --check` and `git status --short` without editing
anything afterward. Put that last exact output in the final handoff message.

## Final handoff

T25 is the executor's terminal state. The final report must contain:

- starting base SHA and ending HEAD SHA;
- ordered local commit list;
- every task status and exact evidence command/output;
- targeted, repeatability, canonical, e2e and SDD-lint results;
- proof that the MCP tool count remains 13 and the catalogue stays within the
  existing 20 KiB ratchet;
- changed paths compared with the SDD scope;
- user/foreign/ambiguous artifacts observed but not changed;
- blocked tasks and downstream tasks skipped because of them;
- explicit statements: `push_not_performed`, `pr_not_created`,
  `hosted_state_not_mutated`, `tag_not_created`, `release_not_published`.

Stop after the clean local handoff. Independent validation and promotion are a
different authority described in the SDD. A passing local report is not
permission to continue into either phase.
