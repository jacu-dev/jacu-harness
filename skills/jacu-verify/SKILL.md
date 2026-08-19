---
name: jacu-verify
description: "Use while a run is open to execute the mission's verification commands, or to run one allowlisted diagnostic command inside the worktree."
---

# Verify a change before asking for approval

Verification runs inside the open run's worktree. It never runs a shell.

1. Call `jacu_verify` with the active `run_id`. Without `argv`, it runs the
   mission's own `verification_commands`, in order. For one diagnostic test or
   linter, pass an `argv` array in the same call; it uses the identical policy.
   For a measured long operation, add `async: true`; the response is an
   accepted task, not a verification verdict.

2. For an async task, poll `jacu_status` with `task_id` until the task is
   `done`, `failed`, `cancelled`, or `timeout`. The `result` on a completed
   task is the same verify envelope and its `data.verdict` is the authority.
   If cancellation is needed, call `jacu_verify` with that `task_id` and
   `cancel: true`, then keep polling until the terminal task state is visible.

3. Read `data.verdict`. It has five values, and only one of them is approval:

   - `pass` — every command succeeded.
   - `fail` — a command exited non-zero. Fix the code in `worktree_path` and
     call `jacu_verify` again.
   - `timeout` — a command exceeded its limit and its process group was killed.
   - `blocked` — the governance layer refused. **Nothing ran at all**, not even
     the commands before the refused one.
   - `not_run` — nothing was verified, usually because the mission declares no
     verification commands. **This is not a pass.** Do not present it as one.

   Never derive approval from the absence of `fail`. Read the verdict.

4. `pass` is what precedes `jacu_diff`. Verify first, present the diff second —
   a diff offered for approval without a verdict is asking the human to be the
   test suite.

5. Diagnostic `argv` is always an **array**, never a shell string. The same
   allowlist and limits apply; there is no second command tool.

## When it refuses

`blocked` means the command is outside the project's allowlist, or the argv
looks like a shell: an interpreter, a metacharacter, a `..` path component, an
absolute path in an argument. **Stop and report the refusal to the human.**

Never work around it — not by rephrasing the command, not by using a different
interpreter, not by running it through a tool outside the jacu. If the command
is legitimate, the answer is for the human to add it to the project's
`.jacu/verify-allowlist.json`, which is read from the project root and never
from the worktree.

## Limits, stated plainly

Verification is **not a sandbox**. There is no network denial and no filesystem
confinement; the command runs as your user. What the jacu governs is the argv —
which program, with which arguments — on top of a worktree boundary that git
guarantees. Escape by a path that does not look like a path (an environment
variable, a config file read from the worktree) is not detected.

`stdout_tail` and `stderr_tail` are the **tail** of the output and may be
truncated; `evidence_digest` always covers the complete output. Timing out is
not the same as failing, and a cancelled run is `not_run`, never `fail`.
