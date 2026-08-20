# workspace Specification

## Purpose
Define the workspace capability: the five logical MCP operations (`jacu_workspace_open`, `jacu_diff`, `jacu_apply`, `jacu_discard`, and canonical `jacu_status`, also exposed as the `jacu_workspace_status` compatibility alias) that run a compiled mission in an isolated Git worktree with a review-then-apply lifecycle. A run moves through the states `open` -> `reviewed` -> `applied`, or ends in `discarded`; any run may be reported `corrupted`. The exact reviewed tree — and nothing else — is what gets committed.
## Requirements
### Requirement: Open preconditions
`jacu_workspace_open` SHALL recompile the mission from `mission_input` and SHALL return `status: "blocked"` (not an error) when: the recompiled `mission_id` differs from the provided `mission_id` ("mission integrity check failed"); the compile result is blocked ("mission has BLOCK lint; fix and recompile"); the mission ceremony is `direct` ("direct ceremony needs no workspace"); the repository has no commits ("repository has no commits"); or the repository has a `.gitmodules` file ("submodules are not supported yet"). When `.gitattributes` contains `filter=lfs`, open SHALL proceed but emit the warning "git-lfs detected; worktree compatibility is not guaranteed".

#### Scenario: Mission integrity mismatch
- **WHEN** the caller passes a `mission_id` that does not equal the ID produced by recompiling `mission_input`
- **THEN** the result is `status: "blocked"` with summary "mission integrity check failed"
- **AND** no worktree, branch, or run state is created

#### Scenario: Direct ceremony needs no workspace
- **WHEN** the recompiled mission classifies as `direct` ceremony
- **THEN** the result is `status: "blocked"` with summary "direct ceremony needs no workspace"

#### Scenario: Repository without commits or with submodules
- **WHEN** the project repository has no `HEAD` commit, or contains a `.gitmodules` file
- **THEN** open is blocked with the corresponding summary

#### Scenario: LFS repository warns but proceeds
- **WHEN** the repository's `.gitattributes` contains `filter=lfs`
- **THEN** open succeeds and the warnings include the git-lfs compatibility warning

### Requirement: Worktree and branch provisioning
On success, `jacu_workspace_open` SHALL generate a run ID `run_<suffix>` where `<suffix>` is 16 lowercase hex characters from 8 cryptographically random bytes, create branch `jacu/run-<suffix>` pinned at the current `HEAD` (the base SHA), and attach it to a worktree at `<home>/.jacu-harness/worktrees/<project_id>/<run_id>`. The branch SHALL be created first via an atomic ref update (`update-ref refs/heads/<branch> <baseSHA> ""`, which fails if the branch already exists) and then attached with `git worktree add`. The worktree SHALL be locked, and the run SHALL be persisted with status `open`, the base SHA, branch, worktree path, mission, and mission input. The result data SHALL contain `run_id`, `branch`, `worktree_path`, and `base_sha`.

#### Scenario: Happy path
- **WHEN** open succeeds for a mission with `light` or `full` ceremony
- **THEN** a locked worktree exists at `<home>/.jacu-harness/worktrees/<project_id>/<run_id>` checked out on branch `jacu/run-<suffix>` at the base SHA
- **AND** the persisted run state has status `open` and matches the returned data

#### Scenario: Two runs are isolated
- **WHEN** open is called twice for the same project
- **THEN** each call produces a distinct run ID, branch, and worktree directory, and neither run interferes with the other

### Requirement: Open failure cleanup
When any step after branch creation fails (worktree add, worktree lock, or run-state save), `jacu_workspace_open` SHALL clean up: unlock and remove the worktree, and delete the branch only if this invocation created it. Cleanup SHALL run under its own 10-second timeout detached from the caller's cancellation.

#### Scenario: Cancelled during worktree add
- **WHEN** the context is cancelled while the worktree is being created
- **THEN** the partially created worktree and the branch created by this invocation are removed

#### Scenario: Pre-existing branch is preserved
- **WHEN** the branch ref already exists (branch creation fails) and worktree add therefore never owns the branch
- **THEN** open fails without deleting the pre-existing branch

### Requirement: Diff review records a digest
`jacu_diff` SHALL load the run by `run_id`, validate the run's identity (returning `status: "blocked"` with "run identity check failed: ..." on mismatch), and compute a snapshot of the worktree against the run's base SHA using a temporary index (`read-tree` base, `add --intent-to-add -A`, then `diff`) so untracked files and deletions are included without mutating the worktree's real index. Diffs SHALL be produced with `--binary --no-color --no-ext-diff --no-renames`. The tool SHALL compute the digest as `sha256:<hex>` over the canonicalized patch — the full patch with all lines starting with `index ` removed — and persist the run as `reviewed` with `reviewed_digest` and `reviewed_at` set. Result data SHALL include `digest`, `files`, `added`, `deleted`, `in_scope`, `out_of_scope`, and `diff`.

#### Scenario: Untracked file and deletion are reviewed
- **WHEN** the worktree contains a new untracked file and a deleted tracked file
- **THEN** both appear in the diff, numstat counts, and file list, and the worktree's real index is not mutated

#### Scenario: Digest is deterministic and canonical
- **WHEN** the same tree is diffed twice, or via the review path and the staged-tree path
- **THEN** the digest is identical, because only `index ` metadata lines are excluded from hashing

#### Scenario: Empty diff is ok with warning
- **WHEN** the worktree has no changes against the base SHA
- **THEN** the result is `status: "ok"` with the warning "no changes yet" and the run still transitions to `reviewed`

#### Scenario: Forged run state is blocked
- **WHEN** the persisted run's branch or worktree does not match the identity derived from its run ID (for example a forged worktree pointing at the project root)
- **THEN** the result is `status: "blocked"` and no diff is computed

### Requirement: Diff scope reporting
`jacu_diff` SHALL classify every changed file against the mission's `allowed_paths`: a file is in scope only if some allowed scope (with any trailing `/` removed) is `.`, is `**`, equals the path, or is a `/`-delimited prefix of the path; otherwise it is out of scope and emits the warning `out-of-scope change: <path>`. A file matching any `forbidden_paths` scope by the same rule SHALL additionally emit `FORBIDDEN path modified: <path>`. Scope violations produce warnings; they do not block the review.

#### Scenario: Change outside allowed paths
- **WHEN** the diff touches a file that matches no allowed scope
- **THEN** the file is listed in `out_of_scope` and an `out-of-scope change` warning is emitted

#### Scenario: Change in a forbidden path
- **WHEN** the diff touches a file under a forbidden scope
- **THEN** a `FORBIDDEN path modified` warning is emitted for that file

### Requirement: Diff output fitting
`jacu_diff` SHALL always digest the full patch, but SHALL truncate the inline `diff` field to a 16KB UTF-8-safe prefix followed by the marker `\n... diff truncated ...\n`, adding the warning "diff exceeds 16KB; inline output truncated". If the JSON-encoded tool result still exceeds the 32KB output limit, the tool SHALL add the warning "diff truncated to fit 32KB encoded output limit" and shrink the inline diff (by binary search over the preview, keeping the truncation marker) until the encoded result fits; if the result exceeds 32KB even with no inline diff, the tool SHALL fail with an error.

#### Scenario: Large diff
- **WHEN** the patch exceeds 16KB
- **THEN** the recorded digest covers the full patch while the inline diff is a truncated prefix ending in the truncation marker

#### Scenario: Metadata alone exceeds the output limit
- **WHEN** the non-diff fields of the encoded result exceed 32KB
- **THEN** the call fails with an error rather than emitting an oversized result

### Requirement: Apply gate sequence
`jacu_apply` SHALL pass, in order, all of the following gates before committing, returning `status: "blocked"` at the first failure: (1) run identity validation; (2) run status is `reviewed` ("diff not reviewed; call jacu_diff first"); (3) the digest of the current worktree diff equals `reviewed_digest` ("worktree changed after review; review the diff again"); (4) mission integrity — the mission is recompiled from the stored `mission_input`, and blocked if compilation is blocked or the recompiled `mission_id` differs from the run's ("mission integrity check failed"); (5) if the recompiled mission's risk is `destructive`, the input flag `approve_destructive` must be true ("destructive mission requires approve_destructive"). The recompiled mission SHALL replace the stored mission for all subsequent apply steps.

#### Scenario: Apply before review
- **WHEN** apply is called on a run still in status `open`
- **THEN** the result is blocked with "diff not reviewed; call jacu_diff first"

#### Scenario: Worktree changed after review
- **WHEN** any file is modified or added in the worktree after `jacu_diff` recorded the digest
- **THEN** apply is blocked with "worktree changed after review; review the diff again" and nothing is committed

#### Scenario: Tampered run state cannot inject verification commands
- **WHEN** the persisted run's stored mission was edited on disk to add or change verification commands
- **THEN** apply recompiles the mission from `mission_input` and runs only the recompiled commands, or blocks with "mission integrity check failed" if the recompiled mission ID differs

#### Scenario: Destructive mission without approval
- **WHEN** the recompiled mission risk is `destructive` and `approve_destructive` is absent or false
- **THEN** apply is blocked with "destructive mission requires approve_destructive"

#### Scenario: Second apply on an applied run
- **WHEN** apply is called again after a successful apply
- **THEN** the call is refused (the run is no longer in `reviewed` status)

### Requirement: Verification command execution
`jacu_apply` SHALL execute the recompiled mission's `verification_commands` through the executor owned by the `verify` capability. Before the first process spawn, it SHALL load the effective project-root verify policy and preflight every argv against the same deny-by-default allowlist used by `jacu_verify`; if any argv is empty, forbidden, or outside the allowlist, apply SHALL return `status: "blocked"` with `verification failed: <command>` and no command in the batch SHALL execute. Allowed commands SHALL run sequentially in the worktree as direct argv exec with the verify limits: a 120-second default per-command timeout, process-group termination on timeout or cancellation, bounded 4KB output tails with full-stream digests and byte counts, a reconstructed `PATH`, a synthetic toolchain `HOME`, a run-scoped scratch `TMPDIR`, and `LANG=C.UTF-8`. The real parent `HOME` and every unrelated parent environment variable SHALL be absent. Any result other than `passed` SHALL block apply without committing. The whole apply tool call runs under a 10-minute timeout. After all commands pass, apply SHALL verify the worktree is unchanged: `HEAD` must still equal the base SHA ("verification commands modified the worktree; review the diff again"), and the diff of the freshly staged tree against the base SHA must still hash to `reviewed_digest` — on mismatch apply SHALL `reset --mixed` the worktree index and block with the same summary.

#### Scenario: Non-allowlisted batch is blocked before spawn
- **WHEN** any verification argv names a binary outside the effective verify allowlist
- **THEN** apply is blocked before executing any command in the batch and no commit is created

#### Scenario: Synthetic HOME replaces the real home
- **WHEN** an allowlisted apply-time verification command reads `HOME` and the parent process has a real user home
- **THEN** the command observes the synthetic toolchain home used by `jacu_verify`
- **AND** the real parent home and unrelated parent variables are absent

#### Scenario: Verification failure
- **WHEN** an allowlisted verification command exits non-zero, times out, is cancelled, or otherwise does not report `passed`
- **THEN** apply is blocked with `verification failed: <command>`, diagnostic output or a structured refusal reason is returned in the data, and no commit is created

#### Scenario: Verification creates a file
- **WHEN** an allowlisted verification command writes a new file into the worktree
- **THEN** apply is blocked with "verification commands modified the worktree; review the diff again" and the temporary staging is reset

#### Scenario: Verification advances HEAD
- **WHEN** an allowlisted verification command commits or otherwise moves the worktree's `HEAD` off the base SHA
- **THEN** apply is blocked with the same summary and nothing is committed by jacu

### Requirement: Apply commit semantics
On success, `jacu_apply` SHALL commit exactly the validated staged tree via `commit-tree <tree> -p <baseSHA>` and advance the branch with a compare-and-swap ref update (`update-ref HEAD <commit> <baseSHA>`), so the commit's parent is the run's base SHA and a concurrently moved branch cannot be overwritten. The commit message SHALL contain the mission objective, the acceptance criteria as a bullet list, a `Verified: ...` summary line (listing each command with `(exit 0)`, or "Verified: no verification commands"), and the trailers `Jacu-Run: <run_id>`, `Jacu-Mission: <mission_id>`, `Jacu-Base: <base_sha>`, and `Assisted-by: <host>` where `<host>` is the MCP client name canonicalized (whitespace/control characters collapsed, trimmed, at most 128 runes, defaulting to "unknown-mcp-client"). The run SHALL transition to `applied` with `applied_commit` recorded; if persisting the applied state fails, apply SHALL roll `HEAD` back to the base SHA via CAS and return an error instructing retry, or fail closed describing manual reconciliation if the rollback also fails. After a successful apply the worktree SHALL be unlocked and removed (a cleanup failure is reported as a warning, not an error) while the branch is kept, and the next actions SHALL include "merge <branch> into main when ready".

#### Scenario: Successful apply
- **WHEN** all gates and verification pass
- **THEN** exactly one commit with parent equal to the base SHA exists on `jacu/run-<suffix>`, the reviewed tree matches the committed tree, the worktree is removed, and the branch survives

#### Scenario: Applied state cannot be persisted
- **WHEN** saving the `applied` run state fails after the commit
- **THEN** `HEAD` is rolled back to the base SHA via compare-and-swap and the error tells the caller to retry apply

#### Scenario: Worktree cleanup failure is non-fatal
- **WHEN** the post-commit worktree unlock or removal fails
- **THEN** the apply result is still `status: "ok"` with a "worktree cleanup failed" warning

#### Scenario: Index drift after validation cannot change the commit
- **WHEN** the index changes between tree validation and commit
- **THEN** the commit still contains the pinned validated tree, not the drifted index

### Requirement: Discard validation and archival
`jacu_discard` SHALL require `run_id` or `gc: true` (error "run_id or gc is required"). A targeted `run_id` SHALL be format-validated and loaded, and the run SHALL pass run identity validation. When the worktree directory exists it must be registered as a Git worktree ("worktree ... is not registered; refusing discard") and its checked-out branch must equal the run's branch. Before removal, a non-empty diff against the base SHA SHALL be archived to `.git/jacu/archive/<run_id>.patch` with a recorded `sha256:` content digest; the archive write SHALL go through a symlink-refusing, `os.Root`-confined directory whose identity is re-validated around the final rename. Discard SHALL then unlock the worktree if locked, remove it via Git, delete the run branch if it exists, prune worktree metadata, and persist the run as `discarded` with the archive reference. When the worktree is already absent, discard SHALL instead validate any recorded archive reference against its expected path and digest, unlock and prune orphaned worktree metadata, delete the branch, and mark the run discarded.

#### Scenario: Discard with unsaved changes
- **WHEN** a run with a dirty worktree is discarded
- **THEN** the patch is archived to `.git/jacu/archive/<run_id>.patch`, its digest recorded, and only then are the worktree and branch removed

#### Scenario: Neither run_id nor gc
- **WHEN** discard is called with an empty `run_id` and `gc` false
- **THEN** the call fails with "run_id or gc is required"

#### Scenario: Unregistered or mismatched worktree
- **WHEN** the run's worktree directory exists but is not a registered Git worktree, or is registered on a different branch than the run's
- **THEN** discard refuses with an error and removes nothing

### Requirement: Discard garbage collection
With `gc: true`, `jacu_discard` SHALL additionally select every run whose status is `corrupted` and every `open` or `reviewed` run whose worktree directory no longer exists, and discard each independently: a per-run failure is recorded in `failures` (with the error, any archive reference, and the actions completed) plus a warning, without aborting the remaining runs. A corrupted entry whose metadata is unreadable and whose run ID is not a valid run ID SHALL have its metadata file `.git/jacu/runs/<name>.json` removed (refusing names that are not plain base names); an unreadable entry with a valid run ID SHALL be reported as a failure ("run metadata is unreadable"). If any failure occurred the result status SHALL be `partial` with the next action "retry gc after resolving the reported run failures"; if nothing was selected the summary SHALL be "No workspace runs discarded.".

#### Scenario: GC sweeps corrupted and orphaned runs
- **WHEN** gc runs while one run is corrupted and another open run's worktree directory was deleted externally
- **THEN** both are discarded and healthy runs are untouched

#### Scenario: Partial GC failure
- **WHEN** one selected run fails to discard while others succeed
- **THEN** the result status is `partial`, the failed run appears in `failures` with its error and completed actions, and the successful runs appear in `runs`

### Requirement: Status reporting
`jacu_status` (and its `jacu_workspace_status` compatibility alias) SHALL keep
the existing persisted-run projection and SHALL additionally project persisted
verify tasks as a bounded `tasks` list. With an optional `task_id`, it SHALL
return only that task and its available result. Status remains read-only and
polling SHALL not alter run or task state.

#### Scenario: Empty status remains compatible
- **WHEN** `jacu_status` receives an empty object
- **THEN** it returns the existing runs and no extra lifecycle mutation

#### Scenario: Task lookup
- **WHEN** `jacu_status` receives a valid `task_id`
- **THEN** it returns the corresponding task state and available result digest

#### Scenario: Missing worktree is reported, not persisted
- **WHEN** an open run's worktree directory has been deleted externally
- **THEN** status reports that run as `corrupted` with a discard-gc warning
  while the run file on disk keeps its original status

#### Scenario: One broken run does not hide healthy runs
- **WHEN** one run's base SHA is unreachable and another run is healthy
- **THEN** the broken run is reported as corrupted and the healthy run is still
  listed with full fields

#### Scenario: Large diff and stale base warnings
- **WHEN** a run's diff exceeds 400 lines or the repository `HEAD` has advanced
  past the run's base SHA
- **THEN** the corresponding warning is emitted alongside the per-run data

### Requirement: Verification tools use existing workspace runs
`jacu_verify` SHALL load an existing workspace run, accept it only while its lifecycle status is `open` or `reviewed`, and execute in that run's worktree. It SHALL block a missing, malformed, applied, discarded, or corrupted run before spawning a process, and SHALL NOT transition the run lifecycle.

#### Scenario: Open or reviewed run is eligible
- **WHEN** either verification tool receives a valid run whose status is `open` or `reviewed`
- **THEN** it may execute an allowlisted argv with cwd set to that run's worktree

#### Scenario: Terminal or invalid run is inert
- **WHEN** either verification tool receives a missing, malformed, applied, discarded, or corrupted run
- **THEN** it returns `blocked`, spawns no process, and leaves persisted run state unchanged

### Requirement: Intra-process serialization only
The workspace tools SHALL be serialized by a single in-process gate: at most one workspace tool call (open, status, diff, apply, or discard) executes at a time within one server process, gate acquisition respects caller cancellation, and each tool runs under its own timeout (open 1 minute, status 30 seconds, diff 1 minute, apply 10 minutes, discard 2 minutes; input capped at 256KB). No cross-process locking is provided: concurrent jacu processes on the same repository are not mutually excluded, and only Git's own worktree/ref locking plus the atomic branch creation and CAS ref updates bound the damage. This is a known limitation; the guarantee is intra-process only.

#### Scenario: Concurrent calls in one process
- **WHEN** two workspace tool calls arrive concurrently in the same server process
- **THEN** they execute one at a time, and a caller whose context is cancelled while waiting receives the cancellation error without holding the gate

#### Scenario: Two processes on one repository
- **WHEN** two jacu processes operate on the same repository
- **THEN** no cross-process mutual exclusion is guaranteed; correctness relies on atomic branch creation failing for duplicate branches and compare-and-swap ref updates refusing lost updates

### Requirement: Policy-gated automatic apply
When the immutable project root contains an autonomy policy, `jacu_apply` SHALL evaluate it before committing. The policy SHALL be loaded outside the writable worktree, use risk tiers exactly `safe`, `write`, and `destructive`, and support `require`, `risk_max`, `max_iterations`, and `on_violation`. `verify_pass` SHALL be true only when the current verify verdict is exactly `pass`; `fail`, `timeout`, `blocked`, and `not_run` SHALL never permit automatic apply. A missing policy SHALL preserve the existing manual review-and-apply behavior.

#### Scenario: Literal verify pass is required
- **WHEN** the policy requires `verify_pass` and verification returns `fail`, `timeout`, `blocked`, or `not_run`
- **THEN** apply is blocked/escalated before commit with the worktree and evidence preserved

#### Scenario: Risk ceiling is enforced
- **WHEN** the mission's derived risk exceeds `risk_max`
- **THEN** apply is blocked/escalated and no commit is created

#### Scenario: Policy cannot be changed in the run worktree
- **WHEN** a run modifies the policy file only inside its writable worktree
- **THEN** apply continues using the project-root policy and does not accept the worktree edit as authorization

### Requirement: Single-use review receipt
When `cross_review` is required, apply SHALL require a valid HMAC-signed receipt containing `run_id`, `diff_digest`, `verdict` (`approve`, `reject`, or `escalate`), `reasons`, and `created_at`. The receipt SHALL be rejected when tampered, reused, or its digest differs from the current reviewed diff. The receipt SHALL be described as a level-2 audit artifact and SHALL NOT claim to prove distinct reviewer and executor sessions.

#### Scenario: Valid approval receipt permits the gate
- **WHEN** a receipt has a valid signature, `approve` verdict, matching run and digest, and has not been consumed
- **THEN** the cross-review requirement passes and the receipt is marked consumed only after successful apply

#### Scenario: Tampered or reused receipt is refused
- **WHEN** receipt bytes are changed or the same receipt is presented twice
- **THEN** apply is blocked/escalated and no commit is created

### Requirement: Audit package and escalation preservation
Every automatic or escalated mission SHALL persist an audit package containing objective, diff digest, verify verdict, evidence digest, receipt reference, iterations, and warnings. A policy violation, merge conflict, or second failure of the same CI check after remediation SHALL escalate the mission while preserving its worktree, last diff, verdict, receipts, and audit package; independent program missions SHALL remain runnable.

#### Scenario: Escalation preserves mission state
- **WHEN** a mission exceeds its iteration budget or encounters a merge conflict
- **THEN** the mission is escalated without deleting its worktree or audit evidence

### Requirement: Apply stays local and never mutates remotes
After a successful apply, whether manual or policy-gated, the result SHALL be a local commit on the run branch. Apply SHALL NOT invoke `git push`, `gh`, a provider API, pull-request creation, merge, or auto-merge. `NextActions` SHALL instruct merging the run branch into `main`. Remote integration is an outer-loop operation. Apply SHALL never create, move, or delete a `v*` tag.

#### Scenario: Policy-gated apply stays local
- **WHEN** policy requirements pass and the run commit succeeds
- **THEN** Apply returns `ok`, `NextActions` request a local merge, and no `git push` or `gh` process is started

#### Scenario: Production tag remains unreachable
- **WHEN** an automatic apply completes
- **THEN** no `v*` tag is created, moved, or deleted

### Requirement: Bounded Remediation Planning

The workspace autonomy layer SHALL classify failed check evidence as lint, test,
build, vulnerability, flaky or other, derive only safe relative paths from
annotations, and create a remediation mission with its own one-iteration
budget. A flaky check SHALL be rerun at most once; the same check failing twice
after correction SHALL escalate and preserve the worktree.

#### Scenario: Real failure creates a scoped mission

- **WHEN** a failed check has valid relative annotations and has not failed
  twice after correction
- **THEN** the result contains objective, check class, allowed paths, evidence
  digest and a dedicated remediation budget

#### Scenario: Repeated failure escalates

- **WHEN** the same check fails twice after correction
- **THEN** no third remediation mission is created and the run is escalated

#### Scenario: Traversal annotation is inert

- **WHEN** an annotation contains an absolute or `..` path
- **THEN** it is excluded from allowed paths and cannot authorize a mission

### Requirement: Injectable Git Boundary

The Git adapter SHALL expose a narrow injectable command boundary for tests
while production continues to execute direct argv with the existing cleaned
Git environment. Test doubles SHALL be able to return stderr/errors without
requiring the real Git binary.

#### Scenario: Git failure is deterministic

- **WHEN** an injected command runner returns a Git error
- **THEN** the capability receives that error and no real Git process is spawned

