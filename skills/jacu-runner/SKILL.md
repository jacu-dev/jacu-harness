---
name: jacu-runner
description: "Use when executing a compiled JACU program or persisted run through an allowlisted Claude or Codex headless provider."
---

# JACU Runner

Use the runner only after the host has compiled the mission/program and
resolved every decision in `program.open_questions`.

## Headless execution

1. Keep the `run_id` from `jacu_workspace_open`; do not reconstruct a worktree
   path or pass a free-form command.
2. Confirm the program has no open questions. An open question is a BLOCK, not
   permission to guess.
3. Choose `claude` or `codex` from the provider decision and run:

   `jacu run --run-id <run_id> --provider <claude|codex>`

4. Treat the JSON result as evidence: status, exit code, duration, tails,
   byte count, truncation, and digest. Never infer success from a prose tail.
5. On `blocked`, `cancelled`, `timed_out`, or `failed`, preserve the run and
   report the sanitized reason. Do not retry a paid session automatically.
6. After a completed provider session, use `jacu_diff` to inspect its changes;
   the runner does not review or apply them.

## Safety contract

- The runner accepts only Claude and Codex, direct argv, and a persisted run.
- The objective travels through stdin; it is not interpolated into argv or a
  shell command.
- Provider login is never started by JACU. Ask the user to authenticate in the
  provider's own terminal when readiness is missing.
- The child receives a positive nine-variable environment allowlist. API keys,
  ambient `GIT_*` repository selectors, and unrelated parent variables do not
  cross the spawn boundary.
- Cancellation and timeout terminate the provider process group. Output tails
  are bounded; the digest and byte count cover the drained streams.
- A completed runner result is not an apply approval. The normal diff, verify,
  review, and apply gates still apply.
