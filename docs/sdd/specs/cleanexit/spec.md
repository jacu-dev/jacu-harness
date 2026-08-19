# cleanexit Specification

## Purpose

Provide a conservative, typed close check for JACU work: detect leftovers,
remove only artifacts recorded as JACU-created, and preserve anything that may
belong to the user.

## Requirements

### Requirement: Leftovers have one closed failure class

The detector SHALL classify findings as exactly one of `branch_local`,
`branch_remote`, `worktree`, `untracked`, `stash`, `run_open`, or
`main_mismatch`. Unknown repository state SHALL fail closed without panicking.
Git query and runstate errors SHALL remain distinct from successful empty output. Any unavailable branch, status, stash, remote, worktree or runstate query SHALL produce one sanitized `main_mismatch` finding and a `fail` verdict.

### Requirement: Cleanup is ownership bounded

Removal SHALL operate only on a recorded JACU worktree, a merged `jacu/` local
branch, or its terminal run-state entry. User untracked files, remote branches,
unmerged branches, stashes, and ambiguous state SHALL be preserved and reported.

### Requirement: Close verifies instead of archiving

`sdd close` SHALL refuse an unfinished task, missing evidence, a lint BLOCK, or
a missing manually-created archive. It SHALL not move the SDD, touch a remote,
create a production tag, or add an MCP tool.

### Requirement: Receipts and telemetry are sanitized

The receipt SHALL contain only a verdict, failure classes, and removed artifact
identifiers. `cleanexit.close` telemetry SHALL use the closed result and failure
class enums and SHALL never include free text.

## Scenarios

#### Scenario: user file survives

- **WHEN** a user-created untracked file exists
- **THEN** close reports `untracked` and the file remains on disk

#### Scenario: locked orphan is visible

- **WHEN** a locked worktree is not referenced by an open run
- **THEN** the report names `worktree`, its path, and `locked=true`

#### Scenario: unmerged branch is not deleted

- **WHEN** a `jacu/` branch has commits absent from main
- **THEN** cleanup reports `branch_local` and preserves the branch
