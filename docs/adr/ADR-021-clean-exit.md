# ADR-021 — Clean exit boundary

- Status: proposed — owner ratification required
- Date: 2026-08-14
- Scope: `jacu sdd close` and the clean-exit capability

## Context

JACU creates ephemeral branches, worktrees and run-state entries while it
governs a change. A completed mission must leave the repository on the merged
main line without silently deleting user work. The existing repository has no
typed check for these leftovers and no close gate for an SDD archive.

## Decision

Clean exit classifies every non-clean result as exactly one of:

`branch_local`, `branch_remote`, `worktree`, `untracked`, `stash`, `run_open`,
`main_mismatch`.

The cleaner may remove only artifacts recorded as JACU-created: an ephemeral
worktree, its ephemeral branch when merged, and the matching run-state entry.
User-created untracked files, unrelated branches, remote branches, ambiguous
paths, and unmerged commits are reported and preserved. An unmerged local
branch escalates rather than being force-deleted.

`sdd close` verifies that the SDD has been archived manually, that every task is
`done` or `blocked` with evidence, and that lint has no BLOCK. It does not move
the SDD, delete files, touch remotes, create tags, or decide whether a change
should merge. Exit codes are 0 for a clean close, 1 for a detected clean-exit
failure, and 2 for a close-contract refusal.

The MCP surface does not gain a tool. The CLI subcommand is authorized through
the existing verify allowlist and emits only typed, sanitized clean-exit
telemetry.

## Consequences

The boundary is conservative: uncertainty produces a finding and preserves
data. Cleanup is auditable through a typed receipt containing the verdict,
failure classes and bounded removed-artifact identifiers. Production tagging
and remote cleanup remain explicit human operations.

## Rejected alternatives

- A general repository garbage collector: it cannot distinguish user work from
  JACU work safely.
- Automatic SDD archiving: a close check must verify the human archive rather
  than hide a move inside the command.
- Force-deleting unmerged branches: this turns a cleanup convenience into data
  loss.
