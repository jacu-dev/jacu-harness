# security-gates Specification

## Purpose
Define the cross-cutting security invariants enforced across the jacu tools as implemented: run identity validation, mission integrity re-verification, path scope evaluation, a deny-by-default posture, refusal of shell-mediated command execution, environment filtering for subprocesses, and hardened archive filesystem writes. These gates exist so that tampered on-disk state, forged identifiers, or hostile path/command inputs cannot escalate what a tool call is allowed to do.

## Requirements

### Requirement: Run ID format validation
A run ID SHALL be accepted only if it is exactly the prefix `run_` followed by exactly 16 characters from `[0-9a-f]` (total length 20; lowercase hex only — uppercase letters, other characters, and any other length are rejected). `runstate.Load` SHALL reject an invalid run ID with `invalid run_id "<id>"` before touching the filesystem, and SHALL reject a run file whose embedded `run_id` field differs from the requested ID. `jacu_discard` SHALL apply the same format check to its `run_id` input before loading.

#### Scenario: Malformed run ID
- **WHEN** a tool receives a run ID such as `run_XYZ`, `run_` plus 15 hex characters, uppercase hex, or a path-like string
- **THEN** the load fails with `invalid run_id` and no run file is read

#### Scenario: Run file with mismatched embedded ID
- **WHEN** the JSON file `.git/jacu/runs/<id>.json` contains a `run_id` different from `<id>`
- **THEN** loading fails and listing reports that entry as `corrupted`

### Requirement: Run identity re-validation
Before acting on a loaded run, `jacu_diff`, `jacu_apply`, and `jacu_discard` SHALL re-derive the run's expected identity from its run ID and reject divergence: the branch must equal `jacu/run-<suffix>` (suffix taken from the run ID); the worktree must be an absolute path resolving to `<home>/.jacu-harness/worktrees/<project_id>/<run_id>` — compared with `os.SameFile` when both paths exist, otherwise by symlink-resolved normalized paths (case-insensitively on macOS and Windows), never treating an existing path as equal to a missing one; and `base_sha` must be non-empty. On failure, `jacu_diff` and `jacu_apply` SHALL return `status: "blocked"` with `run identity check failed: <cause>`, and `jacu_discard` SHALL fail the discard, so a forged run file cannot point jacu operations at the project root or any directory outside the run's own worktree.

#### Scenario: Forged worktree targeting the project root
- **WHEN** a persisted run file is edited so `worktree` points at the project root (or any path other than the canonical worktree for that run ID)
- **THEN** diff and apply return blocked with "run identity check failed" and no Git operation runs against the forged path

#### Scenario: Branch name mismatch
- **WHEN** a run's `branch` field is not `jacu/run-<suffix>` for its own run ID
- **THEN** identity validation fails before any diff, verification, commit, or discard action

#### Scenario: Relative worktree path
- **WHEN** a run's `worktree` field is not an absolute path
- **THEN** the identity check fails even if the relative path would resolve to the expected location

### Requirement: Mission integrity re-verification
Tools that act on a mission SHALL NOT trust a stored mission object; they SHALL recompile it from the stored `mission_input` and compare identities. `jacu_workspace_open` SHALL block when the recompiled `mission_id` differs from the caller-provided one or when compilation is blocked. `jacu_apply` SHALL, after the digest gate, recompile the run's `mission_input` and block with "mission integrity check failed" when compilation is blocked or the recompiled `mission_id` differs from the run's stored `mission_id`; apply SHALL then use only the recompiled mission for the destructive-risk gate and the verification command list, so editing the persisted run file cannot inject commands, downgrade risk, or alter scope.

#### Scenario: Tampered stored mission at apply time
- **WHEN** the persisted run's `mission` object was modified on disk (for example to add a verification command) but `mission_input` is unchanged
- **THEN** apply executes the commands from the recompiled mission only, ignoring the tampered stored copy

#### Scenario: Tampered mission input at apply time
- **WHEN** the persisted `mission_input` was modified so recompilation yields a different `mission_id` than the run's stored `mission_id`
- **THEN** apply is blocked with "mission integrity check failed" before any verification command runs

### Requirement: Path scope evaluation
Mission compilation SHALL lint every `allowed_paths` and `forbidden_paths` entry with a stay-within-root check and emit BLOCK lint `path_outside_root` on failure: the project root is made absolute and symlink-resolved; the candidate path is cleaned, joined onto the resolved root when relative, and rejected if its path relative to the root starts with `..`; if the candidate exists, its symlink-resolved target must also stay within the root. A path listed (after `filepath.Clean`) in both `allowed_paths` and `forbidden_paths` SHALL emit BLOCK lint `allowed_forbidden_overlap`. At review time, `jacu_diff` SHALL evaluate each changed file against a scope with the rule: a scope (trailing `/` stripped) matches when it is `.`, is `**`, equals the path, or is a `/`-delimited prefix of the path — plain string prefixes without a `/` boundary do not match.

#### Scenario: Traversal in allowed paths
- **WHEN** a mission input lists an allowed or forbidden path such as `../outside` or an absolute path outside the project root
- **THEN** compilation emits BLOCK `path_outside_root` and the mission cannot open a workspace

#### Scenario: Symlink escaping the root
- **WHEN** a listed path exists and is a symlink whose target resolves outside the project root
- **THEN** the path fails the stay-within-root check and compilation is blocked

#### Scenario: Prefix requires a path-separator boundary
- **WHEN** the allowed scope is `internal` and the diff touches `internals/file.go`
- **THEN** the file is classified out of scope, because only `internal` itself or paths under `internal/` match

### Requirement: Deny-by-default posture
The system SHALL fail closed at every gate: a mission with any BLOCK-level lint SHALL not open a workspace; a `risk_hint` outside the enum `safe | write | destructive` SHALL emit BLOCK lint `invalid_risk_hint`; a changed file SHALL be in scope only if it affirmatively matches an allowed scope (unmatched files are reported out of scope); a `destructive` mission SHALL not apply without the explicit `approve_destructive` flag; an empty verification command SHALL fail verification rather than be skipped; any digest, identity, or integrity mismatch SHALL block rather than degrade; and validation error paths (identity check errors, archive validation errors, rollback failures) SHALL abort the operation rather than continue.

#### Scenario: Unknown risk hint
- **WHEN** a mission input carries `risk_hint: "banana"`
- **THEN** compilation emits BLOCK `invalid_risk_hint` and the mission cannot proceed to a workspace

#### Scenario: Empty verification command
- **WHEN** the recompiled mission contains an empty argv array in `verification_commands`
- **THEN** apply blocks with a verification failure instead of treating the command as a no-op

### Requirement: Forbidden shell patterns
Verification commands SHALL be argv arrays and SHALL never be handed to a shell. Mission lint SHALL emit BLOCK `shell_string_command` for a single-element command whose sole element contains whitespace (space, tab, CR, or LF), and BLOCK `shell_interpreter_command` for any command of two or more elements whose first element's lowercased basename is `sh`, `bash`, `zsh`, or `cmd.exe` and whose later arguments include `-c` or `/c` (compared case-insensitively). At apply time, each command SHALL be executed by resolving `command[0]` with `exec.LookPath` and passing the remaining elements directly as argv — no shell ever parses the command line. All internal Git invocations SHALL likewise be direct argv executions of the `LookPath`-resolved `git` binary.

#### Scenario: Shell -c command is blocked at compile time
- **WHEN** a mission input includes a verification command such as `["sh", "-c", "rm -rf /"]` or `["cmd.exe", "/C", "..."]`
- **THEN** compilation emits BLOCK `shell_interpreter_command` and the mission is blocked

#### Scenario: Shell string masquerading as one argument
- **WHEN** a verification command is a single element containing spaces, such as `["go test ./..."]`
- **THEN** compilation emits BLOCK `shell_string_command` requiring an argv array

### Requirement: Environment filtering for subprocesses
Verification commands executed by `jacu_apply` SHALL run with an environment reduced to exactly the allowlisted variables `PATH`, `HOME`, `TMPDIR`, and `LANG` (each included only if set in the server's environment); no other server environment variable SHALL be visible to the command. Each command runs in the run's worktree with a 120-second timeout and stdout/stderr each capped at 8KB. Git subprocesses SHALL run under the same four-variable allowlist, with only explicit additions such as `GIT_INDEX_FILE` for temporary-index diffs.

#### Scenario: Secret in the server environment
- **WHEN** the jacu process was started with a sensitive variable such as an API token in its environment
- **THEN** verification commands and git subprocesses do not receive it; only `PATH`, `HOME`, `TMPDIR`, and `LANG` (plus explicitly added variables for git) are passed

#### Scenario: Runaway verification command
- **WHEN** a verification command hangs or floods output
- **THEN** it is killed at the 120-second timeout and at most 8KB of each output stream is retained

### Requirement: Hardened archive filesystem writes
Discard archives SHALL be written only under `.git/jacu/archive/` through a hardened path: every directory component (`.git`, `.git/jacu`, `.git/jacu/archive`) is required to be a real directory and not a symlink; the archive directory is opened as an `os.Root` so all file operations are confined to it; the directory's identity (same inode, still a non-symlink directory) is re-validated after opening and again immediately before the final rename; the destination file name is derived from a format-validated run ID and refused if an existing destination is a symlink or not a regular file; and content is written to a randomized `O_EXCL` temp file, fsynced, then renamed into place. Reading an archive back for validation SHALL apply the same confinement, require the expected path `.git/jacu/archive/<run_id>.patch` and a recorded digest, require a non-empty regular file, and verify the file's SHA-256 content digest against the recorded `sha256:` value.

Native SDD reads and lock writes SHALL reject symlinked directories or files under `docs/sdd`; lock replacement SHALL write a mode-0600 temporary file, sync it, and rename it atomically while preserving the previous valid lock on failure.

#### Scenario: Native SDD document redirects outside the project
- **WHEN** `docs/sdd/<id>/sdd.md` or an intermediate component is a symlink
- **THEN** SDD lint fails closed and does not read or write the link target

#### Scenario: Native SDD lock replacement fails
- **WHEN** a lock write fails before the final rename
- **THEN** the prior lock remains unchanged and the temporary lock is removed

#### Scenario: Symlinked archive directory
- **WHEN** any component of `.git/jacu/archive` is replaced with a symlink before or during an archive write
- **THEN** the write is refused, so the patch cannot be redirected outside the repository's Git directory

#### Scenario: Archive digest mismatch on recovery validation
- **WHEN** a discarded run's recorded archive file was modified so its content no longer hashes to the recorded digest
- **THEN** archive validation fails with an integrity error instead of treating the archive as a valid recovery point
