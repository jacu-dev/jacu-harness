# ADR-027 — Owned local storage lifecycle

- Status: proposed; owner ratification required
- Date: 2026-08-15
- Scope: SDD-008 local storage inspection and pruning

## Decision

JACU exposes local storage lifecycle management as CLI-only commands:

- `jacu storage inspect --json` is read-only.
- `jacu storage prune --dry-run --json` is the default planning path.
- Mutation requires the explicit `--apply` flag and applies only to the project
  resolved from the current working directory.

Inventory output is deterministic, bounded and sanitized. It reports typed
owner class, state, count, age, estimated bytes and proposed action without
payload contents, absolute paths or unbounded entry lists.

Deletion is permitted only when structured state and path identity prove that
JACU created and still owns the artifact. Every candidate is revalidated while
holding the relevant existing lock immediately before mutation. A candidate
that changed after planning is skipped and reported. Symlinks are never
followed. Partial failure returns a nonzero result that distinguishes completed,
skipped and failed actions, and rerunning is idempotent.

The lifecycle is:

- terminal applied or discarded run metadata and its matching JACU archive are
  eligible after 30 days;
- an archive is eligible only when a canonical terminal run still exists, its
  `archive_patch` is exactly `.git/jacu/archive/<run_id>.patch`, and the file's
  SHA-256 equals that run's non-empty `archive_digest`; generic filenames,
  legacy partial JSON and corrupt runstate are never ownership proof;
- a project toolchain home is eligible after 30 idle days only when no open or
  reviewed run and no active task exists;
- empty JACU project parent directories are eligible only after all known owned
  children are gone and no unknown entry exists;
- task apply delegates atomically to `verify.RetainTasksAt`, reusing the exact
  ADR-014 compaction, TTL and metadata-cap policy rather than parsing or
  deleting individual task files in storage;
- telemetry is inventory/report-only here: mutating retention remains
  exclusively owned by the ADR-018 `Store.Emit`/`gcLocked` path, so storage
  does not create a second event collector or retention policy.

Archive actions execute before their corresponding run metadata action. Run
metadata with a referenced archive is removed only after the matching archive
action was recorded as successfully applied by the same invocation and
immediate revalidation also proves that the path remains absent. External
removal is not success. Consequently an archive failure, disappearance, digest
change, directory swap or symlink swap preserves the run record that proves
recovery ownership.

All recursive inventory and removal shares one global 2,048-entry budget per
tree; branching cannot reset the counter. Inventory includes deterministic
oldest/newest age seconds per class using the invocation clock. Unknown task
statuses, invalid schemas and corrupt task records conservatively count as
active for toolchain-cache eligibility.

Recursive toolchain removal is rooted in an opened `os.Root` descriptor. Every
child is lstat/revalidated against the opened parent immediately before removal;
a directory or child swapped to a symlink during traversal fails the action and
cannot redirect deletion outside the project-owned cache.

Open, reviewed, corrupted, dirty, foreign, ambiguous, symlinked or unknown state
is report-only. An open run with a missing worktree and a reviewed dirty
worktree are explicitly ineligible for automatic deletion. The CLI may name the
existing explicit recovery or discard path, but it does not take that decision.

Git objects, `.git/cursor`, user branches, remotes, stashes, memory records,
arbitrary temporary directories and files without proven JACU ownership are
outside this lifecycle. The capability does not add an MCP tool, daemon,
scheduler, database, remote service or background goroutine.

## Consequences

- Operators can identify which JACU-owned producer consumes space before
  authorizing any deletion.
- Dry-run and apply share one typed plan, reducing the chance that displayed and
  executed behavior diverge.
- Ambiguous recoverable work consumes space until a human resolves it; safety is
  preferred over automatic reclamation.
- New storage producers must define ownership, retention, inventory,
  preservation tests and revalidation before they can participate in pruning.
- The MCP catalogue remains at 13 tools and under the existing 20 KiB ratchet.

## Alternatives rejected

- **General disk cleaner:** rejected because JACU cannot prove ownership of
  unrelated Git, editor or user data.
- **Automatic scheduled pruning:** rejected because it adds background mutation
  and removes the operator's dry-run review point.
- **Force removal of dirty or missing-worktree runs:** rejected because those
  shapes may contain recoverable or user-owned work.
- **Independent task and telemetry cleanup implementations:** rejected because
  duplicated retention logic would drift from their owning modules.
- **New MCP cleanup tool:** rejected because the operation is administrative,
  the programme forbids surface growth, and explicit CLI invocation is safer.
