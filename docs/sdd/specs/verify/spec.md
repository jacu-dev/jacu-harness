# verify Specification

## Purpose
Allowlisted, bounded execution of verification and diagnostic argv inside an open run worktree, with stable evidence that downstream autonomy can gate on.
## Requirements

Terminal task payload is retained for 24 hours. After expiry, retention clears input and result payload fields while preserving identity, status, timestamps, reason, digest, expiry and `payload_pruned_at`; compact terminal metadata is retained for 30 days and capped at 1,000 records. Queued/running and corrupt records are preserved.
### Requirement: Verify Runs Mission Commands Only
`jacu_verify` SHALL accept only `run_id` and execute exactly the compiled mission's `verification_commands`, in order, with cwd set to the run worktree. It SHALL preflight every argv against policy before the first spawn, continue after non-zero exits, and stop after timeout or cancellation.

#### Scenario: Mission commands execute in order
- **WHEN** a valid open or reviewed run declares allowlisted verification commands
- **THEN** they execute in mission order inside the worktree
- **AND** a non-zero exit is recorded without skipping later commands

#### Scenario: Refused batch is atomic before spawn
- **WHEN** any declared command fails the allowlist preflight
- **THEN** the aggregate verdict is `blocked` and no command in the batch executes

### Requirement: Ad-hoc Command Door
`jacu_verify` SHALL accept `run_id` and an optional `argv` array. Without `argv`, it runs the mission commands; with one non-empty `argv`, it executes that single diagnostic inside the run worktree under the same allowlist, binary resolution, environment, timeout, and output limits. It SHALL never accept a shell string, and there SHALL be no separate MCP command tool.

#### Scenario: Single diagnostic command
- **WHEN** `jacu_verify` receives an allowlisted diagnostic argv for an open or reviewed run
- **THEN** it returns the shared verification data shape containing one command record

#### Scenario: Shell invocation is inert
- **WHEN** argv names a shell or interpreter pattern such as `sh -c` or `cmd.exe /C`
- **THEN** the result is `blocked` and no process is spawned

### Requirement: Deny-by-default Allowlist
The system SHALL read optional project policy from `.jacu/verify-allowlist.json` at the project root, never from the writable worktree. The effective entries SHALL be `(curated global allowlist union project allow) minus project deny`, with deny taking precedence. Each entry names a bare program and may require an exact argv prefix; project `path_dirs` only contribute absolute directories to binary resolution. Production deploy/push, secret-manager, arbitrary-network, and dependency-install commands SHALL be absent from the global list.

#### Scenario: Binary not allowed
- **WHEN** a bare program matches neither a global nor project allow entry, or is named by project deny
- **THEN** the verdict is `blocked` and nothing executes

#### Scenario: Deny overrides both allow sources
- **WHEN** a program appears in project deny and also in the global list or project allow
- **THEN** the effective allowlist contains no entry for that program

### Requirement: Bounded Executor
The system SHALL execute direct argv with a 120-second default per-command timeout, process-group kill on expiry, and a minimal explicit environment consisting only of reconstructed `PATH`, synthetic toolchain `HOME`, run-scoped scratch `TMPDIR`, and `LANG=C.UTF-8`. Binary resolution SHALL use the reconstructed PATH and refuse a binary inside the worktree or JACU state directory.

#### Scenario: Timeout kills the process group
- **WHEN** a command exceeds its timeout
- **THEN** its process group is killed and the command status is `timed_out`

#### Scenario: Synthetic environment protects the real home
- **WHEN** a command reads HOME and TMPDIR
- **THEN** it observes the synthetic toolchain home and run scratch directory
- **AND** the real parent HOME and unrelated parent variables are absent

#### Scenario: Oversized output retains evidence
- **WHEN** stdout or stderr exceeds the tail cap
- **THEN** the bounded tail is retained, `truncated` is true, `bytes_out` counts all bytes, and the command `digest` covers the complete streams

### Requirement: Stable Result Contract
The data SHALL expose `verdict: pass | fail | timeout | blocked | not_run`, `commands`, `evidence_digest`, and `total_duration_ms`. Each command record SHALL use `argv`, `status`, optional `exit_code`, `duration_ms`, `stdout_tail`, `stderr_tail`, `truncated`, `bytes_out`, `digest`, and optional `reason`; command status SHALL be `passed | failed | timed_out | blocked | not_run`. Each command digest covers complete output, and the evidence digest covers ordered command digests. Output fitting MAY drop tails but SHALL retain argv, status, exit code, duration, byte count, and digests.

#### Scenario: Aggregate verdicts
- **WHEN** all executed commands pass
- **THEN** verdict is `pass`
- **WHEN** any command exits non-zero
- **THEN** verdict is `fail`
- **WHEN** a command times out
- **THEN** verdict is `timeout`

#### Scenario: Nothing executed is not a pass
- **WHEN** a mission declares no verification commands or execution is cancelled before spawning
- **THEN** verdict is `not_run`, never `pass`

### Requirement: Requires an Open Workspace Run
Both tools SHALL require a valid run whose lifecycle status is `open` or `reviewed`. Missing, malformed, applied, discarded, or corrupted runs SHALL be blocked before process execution.

#### Scenario: Invalid or terminal run
- **WHEN** either tool receives a missing, malformed, applied, discarded, or corrupted run
- **THEN** it returns `blocked` and spawns no process

### Requirement: No New Persistent State
Verification SHALL return evidence without mutating the persisted run lifecycle or writing a verification-history record. Commands MAY modify their run worktree as ordinary test/build tools do, so callers remain responsible for reviewing the resulting diff.

#### Scenario: Tool result is not persisted
- **WHEN** either verification tool completes
- **THEN** the persisted run keeps its prior lifecycle state and no separate verification record is written

### Requirement: Async Verify Uses a Persisted Task
`jacu_verify` SHALL remain synchronous by default and SHALL create a persisted
task when `async` is true. The task SHALL execute only the existing mission
verification path for the supplied `run_id` and SHALL return task metadata before
the operation finishes.

#### Scenario: Async start is non-blocking
- **WHEN** an eligible run is called with `async: true`
- **THEN** the response contains a `task_id` and a non-terminal task status
- **AND** the request does not wait for the verification command to finish

#### Scenario: Synchronous compatibility
- **WHEN** `async` is absent or false
- **THEN** `jacu_verify` returns the existing verification envelope directly

### Requirement: Task State Is Forward-Only and Recoverable
Tasks SHALL persist states `queued`, `running`, `done`, `failed`, `cancelled`, or
`timeout` under `.git/jacu/tasks`, with schema version, timestamps, result digest,
and bounded result. Terminal state and result SHALL be immutable.

#### Scenario: Orphaned task
- **WHEN** a server starts with a queued or running persisted task
- **THEN** it marks that task failed with an orphaned reason before serving calls

#### Scenario: Invalid transition
- **WHEN** a terminal task is asked to transition again
- **THEN** the transition is rejected and the persisted result is unchanged

### Requirement: Task Scheduling Is Bounded
The scheduler SHALL use FIFO ordering and a configured small active-task limit;
MCP input SHALL NOT select a capability, command, concurrency, or timeout.

#### Scenario: FIFO queue
- **WHEN** more tasks are started than the active limit
- **THEN** later tasks remain queued and start in creation order

### Requirement: Task Cancellation and Timeout Are Cooperative
Queued cancellation SHALL not spawn a process. Running cancellation and total
timeout SHALL cancel the existing verify context and SHALL terminate its process
group, eventually producing `cancelled` or `timeout` respectively.

#### Scenario: Cancel a running verify
- **WHEN** a running task receives `cancel: true`
- **THEN** its process group is stopped and its terminal task state is cancelled

### Requirement: Task Status Uses Canonical Workspace Status
`jacu_status` SHALL accept an optional `task_id` and return task metadata and the
bounded verify result while its TTL is valid. Without `task_id`, it SHALL retain
the existing run projection. No task-specific MCP tool SHALL be registered.

#### Scenario: Poll and retrieve result
- **WHEN** a caller polls a task id
- **THEN** it can distinguish queued/running from terminal state and receives the
  same verify envelope after completion
