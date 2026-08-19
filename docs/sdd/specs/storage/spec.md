# Storage lifecycle Specification

## Purpose

Define the bounded, CLI-only inventory and explicit cleanup of storage that
JACU can prove it owns for the project resolved from the current working
directory. This capability does not add an MCP tool or a background collector.

## Requirements

### Requirement: dry-run and bounded inventory

`storage inspect` and `storage prune --dry-run` SHALL be read-only. Output
SHALL contain sanitized typed counts, bytes and deterministic oldest/newest age
seconds per class, SHALL NOT expose absolute paths or payload contents, and
SHALL cap traversal depth, globally visited entries per tree and reported
skipped/failed references. Branching SHALL NOT reset the traversal budget.
Symlinks and unknown entries SHALL be reported but never followed.

#### Scenario: unsafe or oversized tree

- **WHEN** an owned root contains a symlink, unknown entry or exceeds an
  inventory bound
- **THEN** the result reports a bounded skip reason and produces no delete
  action for that entry

### Requirement: canonical run and archive ownership

A run record SHALL be eligible only when `runstate.Load` accepts its canonical
`run_<16 lowercase hex>` identity, it is `applied` or `discarded`, its creation
and record modification are at least 30 days old, its branch/base/worktree
identity matches the current project, and the worktree is absent. Any load
error or corrupt/partial/legacy JSON SHALL remain report-only.

An archive SHALL be eligible only when that canonical terminal run names
exactly `.git/jacu/archive/<run_id>.patch`, supplies a non-empty
`sha256:<hex>` digest, and the old non-empty regular file hashes to that exact
digest. Generic filenames and unmatched or tampered patches SHALL be preserved.

#### Scenario: archive changes after planning

- **WHEN** a matched archive is replaced, modified or changed into a symlink
  after plan construction
- **THEN** apply skips both the archive and its dependent run record and leaves
  the replacement target untouched

#### Scenario: archive disappears outside apply

- **WHEN** a matched archive disappears after planning but before its archive
  action succeeds
- **THEN** the archive action and dependent run action are skipped
- **AND** absence alone is never treated as proof of a successful removal

### Requirement: class-specific apply revalidation

Mutation SHALL require `storage prune --apply`. Immediately before every
action, apply SHALL reacquire the relevant lock and revalidate class identity,
confinement, file type, age and current content/state. Archive actions precede
dependent run actions. Toolchain removal SHALL recheck 30-day tree idleness and
the absence of open/reviewed/corrupt runs and queued/running/corrupt tasks.
Unknown task statuses, invalid task schemas and inconsistent task identities
SHALL also block toolchain removal; only a valid terminal task is inactive.
Recursive toolchain removal SHALL use an opened confined root and SHALL
revalidate each child identity through that root immediately before removal; a
directory-to-symlink swap during traversal SHALL fail without following the
replacement or changing its external target.
Empty project parents SHALL be removed only if their original directory
identity is unchanged and they remain empty. A changed candidate SHALL be
skipped; an operation error SHALL be reported as failed and produce a nonzero
CLI result.

#### Scenario: empty parent gains an unknown child

- **WHEN** an empty owned parent gains any child between planning and apply
- **THEN** immediate revalidation skips removal and preserves the child

### Requirement: retention ownership remains single-source

Storage SHALL represent task cleanup as one aggregate action and SHALL delegate
apply to `verify.RetainTasksAt`, which owns task status, compaction, TTL and
metadata-cap decisions under the runstate lock. Storage SHALL NOT parse task
records to decide retention candidates or delete task files individually.

Telemetry SHALL be inventory/report-only in storage. Segment rotation,
12-month retention and the 128 MiB cap SHALL remain exclusively owned by the
telemetry Store write/GC path.

#### Scenario: task and telemetry inventory in dry-run

- **WHEN** task and telemetry files are present
- **THEN** dry-run changes neither store
- **AND** apply delegates only task retention to its owning API and does not
  mutate telemetry
