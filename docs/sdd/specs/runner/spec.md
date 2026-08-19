# runner Specification

## Purpose
Run bounded host-side GitHub check collection and process execution with explicit timeout, retry and sanitized evidence semantics.
## Requirements
### Requirement: Bounded Headless Provider Runner

The headless runner SHALL accept only the providers `claude` and `codex`, build
provider-specific direct argv, send the objective through stdin, and execute in
the persisted run worktree. It SHALL never invoke a shell, accept an arbitrary
binary, or perform interactive login.

#### Scenario: Provider argv is direct and bounded

- **WHEN** a valid run is executed for Claude or Codex
- **THEN** the child receives the provider's fixed headless argv, the worktree
  cwd, and the objective via stdin

#### Scenario: Unready plan is refused before spawn

- **WHEN** the run's plan has one or more open decisions
- **THEN** `run` returns a sanitized blocked result and spawns no provider

### Requirement: Process Limits and Environment

The runner SHALL use a process group, propagate context cancellation and a
timeout, drain stdout and stderr concurrently, keep bounded tails, count all
bytes, and produce a digest. The child environment SHALL contain only
`PATH`, `HOME`, `CODEX_HOME`, `LANG`, `LC_ALL`, `LC_CTYPE`, `TMPDIR`, `TMP`, and
`TEMP` from the positive allowlist; provider API keys and all `GIT_*` variables
SHALL be absent.

#### Scenario: Cancellation kills the process group

- **WHEN** the caller cancels while a provider and descendant are running
- **THEN** the result is `cancelled` and no descendant remains

#### Scenario: Large output does not deadlock

- **WHEN** a provider writes more than the tail cap
- **THEN** the process completes or fails without hanging, `truncated` is true,
  `bytes_out` counts the stream, and the digest is present

### Requirement: Git Environment Isolation

Every Git subprocess SHALL remove `GIT_DIR`, `GIT_WORK_TREE`, `GIT_INDEX_FILE`,
`GIT_OBJECT_DIRECTORY`, `GIT_ALTERNATE_OBJECT_DIRECTORIES`, `GIT_COMMON_DIR`,
`GIT_NAMESPACE`, `GIT_PREFIX`, and `GIT_CEILING_DIRECTORIES` from inherited
environment before appending an explicit internal temporary-index override.

#### Scenario: Ambient Git location cannot redirect a command

- **WHEN** the parent environment points the nine variables at a victim repo
- **THEN** Git operations continue to target the selected repository and the
  temporary index override still works for read-only diffing

### Requirement: Collect Bounded Check Evidence

The runner SHALL use direct allowlisted `gh` argv to collect PR check JSON and,
for failed Actions jobs, a bounded failed-log tail and structured annotations.
It SHALL return a digest over the complete captured streams and SHALL distinguish
pending checks from failed checks.

#### Scenario: Failed check has evidence

- **WHEN** `gh pr checks` reports a failed Actions job with a valid link
- **THEN** the runner collects its run log tail and annotations
- **AND** returns the job/run identifiers and a non-empty evidence digest

#### Scenario: Pending check is not a failure

- **WHEN** one or more checks are pending and no check has failed
- **THEN** the result is pending with no remediation failure

### Requirement: Safe GitHub Arguments

The runner SHALL reject invalid refs, non-numeric run/job identifiers and
repository/path values that are not derived from a validated GitHub Actions
link before spawning `gh`. It SHALL never invoke a shell or print credentials.

#### Scenario: Malformed job link

- **WHEN** a failed check has a malformed or non-GitHub link
- **THEN** the runner records a bounded collection warning and does not call an
  annotation endpoint with attacker-controlled path components
