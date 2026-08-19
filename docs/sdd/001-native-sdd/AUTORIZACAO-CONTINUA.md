# Standing authorization — run to the end

The owner authorizes continuous execution. Do not ask for confirmation between tasks.

## Pre-approved, no confirmation needed

- Create and edit any file inside the declared scope of the current SDD.
- Run tests, builds, linters, formatters, and the SDD lint.
- Commit per task group; open draft pull requests.
- Add tasks the contract requires, as long as the addition is recorded in `sdd.md`.
- Move on to the next open plan when the current one is done or blocked.

## Per-task discipline is unchanged

The task gate stays: the `Verify` command must pass before the next task starts, with
the command and its output pasted into the Evidence cell. Green gate, next task, no
pause. Red gate, stop that task.

Never fabricate evidence, never weaken a gate to make a task pass, never mark done
without the exact `Verify` command.

## Hard stops — the only reasons to interrupt

1. `NEEDS-ERICK` tasks.
2. ADR ratification.
3. Deleting any file.
4. Anything touching production, money, or real user data.
5. Merging a pull request.
6. A conflict with a locked decision, or a spec the change would contradict.

When one is hit: do not stop the world. Record the blocker, skip to the next task that
does not depend on it, and keep going. Accumulate blockers in a list.

## Reporting

One report at the end, not one per task. It contains: tasks done with evidence, tasks
blocked and why, and every human gate collected in a single list so the owner answers
them in one pass.

For a task whose `Verify` is `human read`: do not paste the whole file into the report.
Give the path, a five-line summary, and what specifically needs a human eye.
