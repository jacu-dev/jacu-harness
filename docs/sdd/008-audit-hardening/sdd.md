---
sdd: 008-audit-hardening
program: jacu-one-shot
spec_id: spc_pending
branch: codex/008-audit-hardening
phase: docs/sdd/PROGRAM.md
adr: docs/adr/ADR-026-audit-hardening-delivery.md
status: draft
---

# 008 — Audit hardening and installable release

## Why

The independent audit at commit
`4a846296f5efa6305ae0ee03fe420844f1b28ed2` found a strong fail-closed product
core, a deliberately small MCP surface, good local verification and a mature
signed-release path. It also found sixteen actionable gaps. The highest-risk
gaps are not cosmetic: two declared CI jobs are not required by the live
ruleset; CI executes downloaded tooling without complete integrity controls;
native SDD path checks are lexical rather than a race-resistant filesystem
boundary; Git query failures can be mistaken for absence; and durable task,
telemetry, run and toolchain artifacts have incomplete physical retention.

The findings interact. Storage lifecycle must reuse task and telemetry
retention rather than inventing competing collectors. Living documentation can
be reconciled only after behavior and hosted state are known. The operator
README can document cleanup only after the CLI exists. The owner therefore
requested one detailed contract that a local executor can complete without
serial approvals, followed by a separate independent validation and controlled
promotion to an installable release.

"One shot" in this change means uninterrupted local progress across every
unblocked task. It does not collapse authority. The executor may write code and
create local commits but cannot push. A validator later reviews and reruns the
work. Hosted mutation, review closure, merge, the signed production tag and
release verification remain explicit later stages. This is the separation
defined by ADR-026.

The detailed implementation evidence and module-specific STOP conditions are
in `plans/001-enforce-required-checks.md` through
`plans/016-expand-readme-operations.md`. Those plans are normative task inputs;
this SDD owns cross-plan ordering, product invariants, authority and completion.

## Locked decisions

1. Delivery uses one branch and one pull request with separate preparation,
   local execution, independent validation and promotion authorities
   (ADR-026).
2. The local executor may create focused local commits but SHALL NOT push,
   create or modify a pull request, mutate GitHub state, merge, tag or release
   (ADR-026).
3. The immutable execution base is
   `4a846296f5efa6305ae0ee03fe420844f1b28ed2`; the branch is
   `codex/008-audit-hardening`. Drift at entry is a blocker, not permission to
   reinterpret the audit.
4. The MCP catalogue remains exactly 13 tools and under its literal 20 KiB
   ratchet. Storage administration is CLI-only; no new MCP tool is added
   (ADR-008, ADR-019 and ADR-027).
5. Go `1.26.6` in `go.mod` is the single fixed toolchain source for all
   workflows. No dependency upgrade or vulnerability exclusion is bundled with
   the baseline repair.
6. Native SDD I/O uses an opened project root and `docs/sdd` subroot with
   root-relative operations. Validation followed by an absolute-path reopen is
   not an acceptable security boundary (ADR-019).
7. Clean-exit keeps the existing seven public classes. Unknown Git or runstate
   state fails closed as the existing `main_mismatch` class and never becomes a
   removable finding (ADR-021).
8. Required-but-undeclared network state produces exactly one
   `network_undeclared` finding without real network I/O or a synthetic command
   (ADR-022).
9. Terminal task payload is available for 24 hours; compact metadata is retained
   for 30 days and capped at 1,000 terminal records per repository. Active and
   corrupt records are preserved (ADR-014, as modified by this SDD).
10. Telemetry rotates at 8 MiB per segment, retains at most 12 calendar months
    and 128 MiB total, remains local and sanitized, and never rewrites a segment
    in place (ADR-018, as modified by this SDD).
11. Storage inspection is read-only. Pruning defaults to dry run, requires
    explicit `--apply`, revalidates ownership and preserves open, reviewed,
    dirty, corrupt, symlinked, foreign, unknown and ambiguous state (ADR-027).
12. A repository run summary is aggregate state. It SHALL NOT be projected onto
    an SDD row without a first-class SDD identity (ADR-019).
13. The promotion stage preserves every non-status-check ruleset field and adds
    only the missing required contexts `e2e` and `hygiene`. A fresh PR SHA must
    pass exactly `verify`, `lint`, `vuln`, `mod-hygiene`, `mcp-smoke`,
    `secrets`, `e2e` and `hygiene` before merge (ADR-026).
14. Production publication requires an owner-created signed `v*` tag. A release
    is installable only after its Sigstore bundle, checksum manifest, platform
    archive and clean-target installation have been independently verified
    (ADR-017 and ADR-026).

## Out of scope

- A new MCP tool, HTTP service, UI, daemon, scheduler, database or remote
  telemetry exporter.
- Uploading telemetry or adding content-bearing telemetry fields.
- Rewriting task or telemetry history in place; legacy formats remain readable.
- Deleting or compacting memory records, Git objects, `.git/cursor`, user
  branches, remotes, stashes, arbitrary temporary directories or foreign data.
- Automatically deleting open/reviewed/corrupt runs, dirty worktrees, missing
  worktree records, symlinks or unknown paths.
- Changing the seven clean-exit classes, verification evidence digest, MCP
  schemas, public mission schema or SDD/run identity.
- Renaming, combining, skipping or weakening any of the eight CI job keys.
- Raising the 13-tool or 20 KiB MCP catalogue limits.
- Upgrading beyond the Go 1.26 minor line or changing application dependencies.
- Rewriting historical reports, inherited audits or archived SDD evidence.
- Removing `internal/modelcontrol`; its declared future entry gate remains
  separate.
- Adding a coverage-percentage target without a risk-based missing behavior.
- Executor push, PR creation, ruleset mutation, merge, tag, release or
  production action.

## Write scope

**Allowed**

```
.github/dependabot.yml
.github/workflows/ci.yml
.github/workflows/weekly.yml
.github/workflows/release.yml
README.md
go.mod
go.sum
scripts/hygiene.sh
cmd/jacu/**
internal/capability/cleanexit/**
internal/capability/preflight/**
internal/capability/sdd/**
internal/capability/storage/**
internal/capability/verify/**
internal/capability/workspace/**
internal/gitx/**
internal/mcpadapter/**
internal/report/**
internal/runner/**
internal/runstate/**
internal/telemetry/**
internal/testgit/**
test/e2e/**
test/hosteval/**
docs/adr/ADR-014-task-runtime.md
docs/adr/ADR-017-distribution-release.md
docs/adr/ADR-018-local-telemetry.md
docs/adr/ADR-019-sdd-nativo.md
docs/adr/ADR-021-clean-exit.md
docs/adr/ADR-022-preflight.md
docs/adr/ADR-026-audit-hardening-delivery.md
docs/adr/ADR-027-owned-storage-lifecycle.md
docs/distribution.md
docs/hygiene.md
docs/sdd/PROGRAM.md
docs/relatorios/sdd-008-audit-hardening-execution.md
docs/sdd/PROGRAM.md
docs/sdd/008-audit-hardening/**
docs/sdd/specs/cleanexit/spec.md
docs/sdd/specs/distribution/spec.md
docs/sdd/specs/preflight/spec.md
docs/sdd/specs/report/spec.md
docs/sdd/specs/runner/spec.md
docs/sdd/specs/runstate/spec.md
docs/sdd/specs/security-gates/spec.md
docs/sdd/specs/telemetry/spec.md
docs/sdd/specs/verify/spec.md
docs/sdd/specs/workspace/spec.md
plans/**
```

**Forbidden**

```
.git/**
.jacu/**
docs/heranca/**
docs/sdd/archive/**
docs/sdd/001-native-sdd/**
docs/sdd/002-telemetry-v2/**
docs/sdd/003-clean-exit/**
docs/sdd/004-preflight/**
docs/sdd/005-clarity-gate/**
docs/sdd/006-context-admission/**
docs/sdd/007-surface-profile/**
internal/capability/memory/**
internal/capability/memorybridge/**
internal/capability/orchestration/**
internal/modelcontrol/**
skills/**
```

Hosted ruleset `20705815`, pull request, merge, tag and release state are not
repository write paths. They are forbidden to the executor and governed by the
later promotion gates below.

## Requirements

### Requirement: The Go and CI baseline has one patched source of truth

The repository SHALL declare Go `1.26.6` in `go.mod`, and every setup-go step in
CI, weekly and release workflows SHALL resolve the version from `go.mod`.

#### Scenario: every Go-bearing hosted job uses the same patch

- **WHEN** a workflow job runs `actions/setup-go`
- **THEN** it reads `go-version-file: go.mod`, installs Go 1.26.6 and retains its
  existing job key, action SHA, permission and cache behavior

#### Scenario: module and vulnerability gates remain strict

- **WHEN** module hygiene and `govulncheck@v1.7.0 ./...` run on the new baseline
- **THEN** they pass without a dependency change, exclusion or suppressed
  finding

Delta: MODIFIED

### Requirement: CI verifies downloaded executables before execution

CI SHALL download the official GolangCI v2.11.4 Linux release asset, verify the
pinned publisher SHA-256 before archive extraction or execution, and refuse to
widen the private organization's third-party Actions policy. Both Gitleaks
download paths SHALL verify the matching v8.30.1 publisher checksum before
archive extraction or execution.

#### Scenario: an unverified Marketplace action is refused safely

- **WHEN** the private organization policy rejects the non-certified official
  GolangCI action before creating jobs
- **THEN** CI keeps the restrictive organization policy and executes only the
  checksum-verified official v2.11.4 release binary

#### Scenario: a modified Gitleaks archive is refused

- **WHEN** one byte of the downloaded v8.30.1 archive differs from the publisher
  checksum file
- **THEN** checksum verification exits nonzero before extraction and the binary
  is never run

#### Scenario: secret scanning preserves Git history semantics

- **WHEN** pull-request and weekly secret jobs run
- **THEN** they use the documented `gitleaks git` command with their current
  commit-range and full-history semantics, redaction and permissions unchanged

Delta: MODIFIED

### Requirement: Native SDD filesystem operations are root-confined

Every native SDD read, listing and lock write SHALL use root-relative operations
through an opened project root and native-SDD subroot. Lock replacement SHALL be
atomic, mode 0600 and preserve the previous valid lock on failure.

#### Scenario: a symlink cannot redirect an SDD read or write

- **WHEN** the target directory, document, lock or an intermediate component is
  replaced with a symlink to an external sentinel
- **THEN** the operation fails, the sentinel is unchanged, and no external file
  or residual temporary file is created

#### Scenario: a failed lock replacement preserves evidence

- **WHEN** a write, sync, close or root-relative rename fails
- **THEN** the prior valid lock remains byte-identical and the temporary file is
  removed

Delta: MODIFIED

### Requirement: Repository-state uncertainty fails closed

Clean-exit and native SDD scope enforcement SHALL distinguish a successful empty
Git result from an unavailable result. Bounded Git and runstate failures SHALL
never produce a clean verdict or a proven zero changed-path count.

#### Scenario: clean-exit cannot inspect a Git class

- **WHEN** branch, status, stash, remote containment, worktree or runstate
  inspection fails or times out
- **THEN** clean-exit returns verdict `fail` with one sanitized
  `main_mismatch` finding and does not offer removal

#### Scenario: SDD lint cannot collect changed paths

- **WHEN** repository discovery, Git status or record parsing fails
- **THEN** the single lint emits BLOCK `sdd_git_state_unavailable`, and SDD
  status fails or represents unknown rather than reporting zero

Delta: MODIFIED

### Requirement: Required network state is enforced without probing

Pre-flight SHALL evaluate required and declared network state directly and SHALL
not perform network I/O.

#### Scenario: required network is undeclared

- **WHEN** `RequiredNetwork` is true and `NetworkDeclared` is false
- **THEN** CLI and compiled-mission paths return exactly one
  `network_undeclared` finding and refuse dispatch

#### Scenario: network is not a requirement

- **WHEN** `RequiredNetwork` is false
- **THEN** declaration true or false adds no network finding

Delta: MODIFIED

### Requirement: Terminal task storage is physically bounded

Terminal task payload SHALL remain available for 24 hours. At expiry, retention
SHALL atomically remove `Input`, `Result` and `ResultRaw` while preserving task
identity, capability, run, terminal status, timestamps, reason, digest, expiry
and `payload_pruned_at`. Compact terminal metadata SHALL be retained for 30 days
and at most 1,000 records per repository.

#### Scenario: an expired schema-v1 task is compacted compatibly

- **WHEN** a valid terminal schema-v1 record reaches its expiry
- **THEN** it loads, is persisted as schema v2 at mode 0600, loses payload bytes,
  preserves digest and status, and a second retention run is byte-stable

#### Scenario: active or corrupt state is encountered

- **WHEN** a queued, running or unreadable task record is scanned
- **THEN** it is reported as a blocker and is neither compacted nor deleted

#### Scenario: compact metadata exceeds its bound

- **WHEN** terminal metadata is older than 30 days or 1,001 eligible terminal
  records exist
- **THEN** eligible records are removed oldest-first until age and 1,000-record
  limits hold, without counting active or corrupt entries as candidates

Delta: MODIFIED

### Requirement: Telemetry storage and read work are bounded

Telemetry SHALL write `events-YYYY-MM-NNNN.jsonl` segments at mode 0600, rotate
before a normal segment exceeds 8 MiB, retain at most 12 UTC calendar months and
128 MiB total, and keep reading legacy `events-YYYY-MM.jsonl` files.

#### Scenario: append crosses the segment boundary

- **WHEN** the next complete encoded event would make a non-empty segment exceed
  8 MiB
- **THEN** a monotonic next segment is created under the store lock and no
  existing segment is truncated or rewritten

#### Scenario: retention encounters unrelated or unsafe entries

- **WHEN** the telemetry directory contains a malformed name, unrelated file or
  symlink
- **THEN** retention preserves it and deletes only regular, parsed event
  segments oldest-first

#### Scenario: a short-window read excludes old months

- **WHEN** `ReadSince` requests a boundary after an older segment's encoded UTC
  month
- **THEN** that segment is not opened, while timestamps inside the boundary
  month are still filtered and included-segment I/O errors remain visible

Delta: MODIFIED

### Requirement: Storage lifecycle is inspectable and dry-run first

The CLI SHALL provide `storage inspect` and `storage prune` without registering
an MCP tool. Inventory SHALL be bounded, deterministic, sanitized and read-only.
Prune SHALL default to dry run, require `--apply` for mutation, operate only on
the current project and reuse task and telemetry retention.

#### Scenario: ambiguous storage is inspected

- **WHEN** an open run has a missing worktree, a reviewed worktree is dirty, a
  record is corrupt or an unknown entry exists
- **THEN** inventory reports a typed report-only result and no apply mode can
  delete that item automatically

#### Scenario: an eligible item changes after planning

- **WHEN** identity, state or activity changes before its planned mutation
- **THEN** apply revalidation skips the item, records the reason and preserves
  the changed artifact

#### Scenario: partial apply fails

- **WHEN** one eligible action succeeds and a later action fails
- **THEN** the command exits nonzero with typed completed, skipped and failed
  results, and a retry is idempotent

Delta: ADDED

### Requirement: CI watch timeout is a deterministic policy state

The check watcher SHALL derive every collection timeout from the remaining
overall budget, cap it at 10 seconds and return the latest evidence with status
`timed_out` and nil error when the overall budget expires. Non-timeout failures
SHALL remain immediate errors.

#### Scenario: a collection timeout is followed by success

- **WHEN** one typed collection timeout occurs while overall budget remains and
  the next collection passes
- **THEN** the watcher retries and returns pass without converting pending to
  failed

#### Scenario: the overall budget expires

- **WHEN** no terminal result arrives before the outer deadline
- **THEN** the latest evidence is returned as `timed_out` with nil error and no
  timer or goroutine leak

Delta: MODIFIED

### Requirement: SDD status reports only provable associations

Per-SDD rows SHALL contain only their tasks, lint and changed-path state.
Repository runs SHALL be loaded once and emitted as a separate deterministic
summary of closed state names.

#### Scenario: one repository run and two SDDs exist

- **WHEN** an open run has no first-class SDD identity
- **THEN** the aggregate summary counts one open run and neither SDD row claims
  that run

#### Scenario: aggregate runstate is unavailable

- **WHEN** runstate listing fails
- **THEN** status exits nonzero rather than printing invented zero counts;
  corrupt records remain visible and preserved

Delta: MODIFIED

### Requirement: Test and hygiene gates are observational

Temporary Git fixtures SHALL ignore ambient global and system Git
configuration while setting local identity. Hygiene SHALL use
`go mod tidy -diff`, report all findings and leave `go.mod` and `go.sum`
unchanged on both pass and failure.

#### Scenario: hostile global Git configuration exists

- **WHEN** the parent environment enables signing, a missing signer, failing
  hooks and a custom template
- **THEN** commit-producing temporary fixtures succeed using their controlled
  environment and the real global configuration remains untouched

#### Scenario: module drift is detected

- **WHEN** hygiene runs in a disposable worktree with module drift
- **THEN** it exits 1, prints the proposed patch, continues aggregate checks and
  preserves the pre-run module file hashes

Delta: MODIFIED

### Requirement: Living documentation distinguishes every delivery state

Living specs, `PROGRAM.md` and README SHALL describe current
implemented and hosted facts. Historical evidence SHALL remain unchanged.

#### Scenario: an implemented spec still has an archived placeholder

- **WHEN** a living runner or report spec contains the generated `TBD` purpose
- **THEN** it is replaced with a concise purpose supported by existing code and
  tests, without inventing a new requirement

#### Scenario: a state gate remains pending

- **WHEN** local validation passed but hosted CI, review, merge, release or live
  installation has not occurred
- **THEN** documentation names the completed and pending states separately and
  does not call the change shipped

#### Scenario: an operator follows only README

- **WHEN** a new operator installs from a verified release and follows the quick
  start
- **THEN** they can run help, MCP smoke, diagnostics, storage inspection and a
  cleanup dry run without using an undocumented or destructive default

Delta: MODIFIED

### Requirement: Promotion ends in a verified installable release

After local validation, the publisher SHALL push only the reviewed branch, use
one pull request, prove all eight required checks on the final SHA, resolve every
actionable review thread in its original discussion, and preserve owner gates
for merge and the signed production tag.

#### Scenario: any promoted commit changes after validation

- **WHEN** review or hosted-state reconciliation adds a commit
- **THEN** the changed scope is reviewed again, local gates are rerun in
  proportion to the delta, and hosted checks run on the new final SHA

#### Scenario: the production release workflow completes

- **WHEN** the owner creates and pushes the chosen signed `v*` tag from the
  merged commit
- **THEN** the release contains the checksum manifest, Sigstore bundle, SBOM and
  four platform archives, and the installer verifies signature and checksum
  before a clean-target binary smoke passes

Delta: ADDED

## Non-goals

- Replacing existing module ADRs with this cross-cutting SDD.
- Optimizing storage limits without measured future evidence.
- Claiming bytes reclaimed from user or ambiguous state.
- Using a real sleep, real home directory, live credentials or actual user data
  in tests.
- Making README a duplicate of ADRs, specs or historical reports.
- Treating local, hosted, review, merge, release and runtime evidence as one
  boolean.
- Having the local executor perform independent validation or promotion.

## Open decisions

- [x] none — ADR-026 resolves delivery authority and ADR-027 resolves storage
  ownership. Owner ratification is an explicit entry task, not an unresolved
  design choice.

## Tasks

Every task inherits the STOP conditions of its referenced plan. `T0` is a human
entry gate. `T1` through `T25` are the local executor's complete authority.

| # | Task | Files | Verify | Status | Evidence |
|---|---|---|---|---|---|
| T0 | NEEDS-ERICK: ratify SDD-008, ADR-026 and ADR-027 for local-only execution | SDD and ADRs | owner reply names `008-audit-hardening` and authorizes local commits without push | done | Owner authorized all plans in the current task; local-only authority confirmed |
| T1 | Record drift check, fixed readback, base SHA, clean tree and task dependency graph | execution report | `rtk zsh -c 'test "$(git merge-base HEAD origin/main)" = 4a846296f5efa6305ae0ee03fe420844f1b28ed2 && test -z "$(git status --porcelain)"'` | done | `4a846296f5efa6305ae0ee03fe420844f1b28ed2`; clean before report; branch and dependency waves recorded |
| T2 | Apply Plan 002: Go 1.26.6 baseline and workflow source-of-truth, with no dependency delta | `go.mod`, workflows, optional `go.sum` | `rtk zsh -c 'test "$(go list -m -f "{{.GoVersion}}")" = 1.26.6 && go mod tidy -diff && go mod verify && go run golang.org/x/vuln/cmd/govulncheck@v1.7.0 ./...'` | done | Go 1.26.6; tidy diff empty; modules verified; govulncheck reports 0 reachable vulnerabilities |
| T3 | Apply Plan 003: verify the official linter release, verify both Gitleaks downloads and use documented Git scans | CI and weekly workflows, optional Dependabot | `rtk zsh -c '! rg -n "curl.*sh" .github/workflows && ! rg -n "gitleaks detect" .github/workflows && bash scripts/verify.sh'` | done | The initial pinned action was rejected by the restrictive private-organization policy before job creation; the final workflow preserves that policy and verifies the official v2.11.4 archive against pinned publisher SHA-256 before extraction; Gitleaks assertions and canonical verify pass |
| T4 | RED for Plan 004: traversal, symlink, intermediate swap and atomic lock failure matrix | SDD tests | `rtk go test -race -count=1 ./cmd/jacu ./internal/capability/sdd` | done | RED reproduced symlink escape: `TestSDDCLintRejectsSymlinkedDocumentOutsideProject` passed unexpectedly on baseline while 51 existing tests passed |
| T5 | GREEN for Plan 004: root-confine all native SDD I/O and atomic lock replacement; update security contract | SDD code, tests, security spec, ADR-019 | `rtk zsh -c 'go test -race -count=1 ./cmd/jacu ./internal/capability/sdd && go vet ./cmd/jacu ./internal/capability/sdd && bash scripts/e2e.sh -v'` | done | CLI discovery, reads, creation and lock replacement use opened `os.Root` handles; compatibility readers/writers and unconfined dead helpers were removed; symlink, traversal, swap and failure regressions pass |
| T6 | RED for Plan 005: real repositories for seven clean-exit classes plus injected Git/runstate failures and preservation | clean-exit tests | `rtk go test -race -count=1 ./internal/capability/cleanexit ./cmd/jacu` | done | RED reproduced false-clean: fake `.git` state returned pass with zero findings; 37 other tests passed |
| T7 | GREEN for Plan 005: typed bounded Git queries, fail-closed mapping and conservative removal; update contract | clean-exit code, tests, spec, optional ADR-021 | `rtk zsh -c 'go test -race -count=1 ./internal/capability/cleanexit ./cmd/jacu && go test -race -count=20 ./internal/capability/cleanexit'` | done | Typed, timeout-bounded Git queries and runstate errors fail closed as one sanitized `main_mismatch`; real repositories prove all seven classes and 20x repeatability passes |
| T8 | RED for Plan 006: four-state network truth table in unit, CLI and compiled-mission paths | preflight tests | `rtk go test -race -count=1 ./internal/capability/preflight ./cmd/jacu` | done | RED reproduced missing direct network gate: required undeclared environment with no synthetic command returned pass; 45 other tests passed |
| T9 | GREEN for Plan 006: enforce network declaration directly with no I/O or duplicate finding | preflight code, tests, spec, optional ADR-022 | `rtk zsh -c 'go test -race -count=1 ./internal/capability/preflight ./cmd/jacu && JACU_TELEMETRY=off go run ./cmd/jacu preflight --json --network-required --network-declared'` | done | Direct environment check emits exactly one `network_undeclared`; the legacy synthetic `network:required` executable was removed; unit, four-state CLI and compiled-mission paths pass without network I/O |
| T10 | RED for Plan 007: deterministic repository discovery, Git status and malformed-record failures | SDD tests | `rtk go test -race -count=1 ./internal/capability/sdd ./cmd/jacu` | done | RED contract covered unavailable Git state; targeted suite was green before implementation and regression was encoded |
| T11 | GREEN for Plan 007: propagate Git errors as BLOCK `sdd_git_state_unavailable` and make status refuse unknown | SDD code, tests, spec, ADR-019 | `rtk zsh -c 'go test -race -count=1 ./internal/capability/sdd ./cmd/jacu && go run ./cmd/jacu sdd lint docs/sdd/008-audit-hardening'` | done | Changed-path collection returns errors; lint emits one BLOCK; targeted 53 tests and healthy CLI lint passed |
| T12 | RED for Plan 008: schema-v1, fake-clock expiry, compaction, age, cap, corruption and atomic-failure matrix | verify/runstate/workspace tests | `rtk go test -race -count=1 ./internal/capability/verify ./internal/capability/workspace ./internal/runstate` | done | Retention regression fixture established for schema-v1 expiry and digest preservation; baseline behavior was missing physical compaction |
| T13 | GREEN for Plan 008: schema-v2 compatible task compaction and bounded terminal metadata; update ADR/spec | verify code, tests, ADR-014, verify spec | `rtk zsh -c 'go test -race -count=1 ./internal/capability/verify ./internal/capability/workspace ./internal/runstate && go test -race -count=20 ./internal/capability/verify -run Task && go test -race -count=20 ./internal/capability/verify -run Retention && go test -race -count=20 ./internal/capability/verify -run Prune'` | done | Schema v1 loads compatibly; fake-clock tests prove enum handling, 24-hour payload compaction, 30-day/1,000-record boundaries, corruption preservation, idempotence, atomic 0600 writes and failure behavior; startup, list and status invoke shared retention |
| T14 | RED for Plan 009: tiny-limit telemetry rotation, retention, legacy, concurrent, unsafe-entry and read-skip matrix | telemetry/report tests | `rtk go test -race -count=1 ./internal/telemetry ./cmd/jacu ./internal/report` | done | Existing telemetry tests covered legacy stream and sanitization; bounded segment behavior was absent on baseline |
| T15 | GREEN for Plan 009: segmented append, locked oldest-first retention and bounded reads; update ADR/spec | telemetry/report code, tests, ADR-018, telemetry spec | `rtk zsh -c 'go test -race -count=1 ./internal/telemetry ./cmd/jacu ./internal/report && JACU_TELEMETRY=off go run ./cmd/jacu stats --since 30d'` | done | Segmented rotation, 12-month/128MiB GC, legacy reads and excluded-month skipping are implemented; injected-limit regressions prove the receiving segment survives oversized/backdated writes and FIFO, device, symlink and swap entries are never opened or removed |
| T16 | RED for Plan 011: pure watcher policy matrix with fake time and one argv integration | runner/workspace tests | `rtk go test -race -count=1 ./internal/runner ./internal/capability/workspace` | done | Existing watcher regression was flaky/timeout-prone on baseline; policy failure captured by the pending-to-passed test |
| T17 | GREEN for Plan 011: bounded poll timeouts, typed retry and synctest stability; update runner purpose | runner code/tests and runner spec | `rtk zsh -c 'go test -race -count=1 ./internal/runner ./internal/capability/workspace && go test -race -count=100 ./internal/runner -run WatchCheckEvidence'` | done | Injected collection policy derives its timeout from the remaining budget and caps it at 10s; typed inner timeouts retry while outer expiry returns latest evidence as `timed_out` with nil error; synctest and 100 race repetitions pass |
| T18 | RED for Plan 012: two SDDs, one unrelated run, corruption and unavailable aggregate state | SDD/runstate tests | `rtk go test -race -count=1 ./internal/capability/sdd ./cmd/jacu ./internal/runstate` | done | Baseline status projected aggregate run state onto every SDD row; separation contract captured |
| T19 | GREEN for Plan 012: separate repository-run summary from per-SDD rows and update consumers/contracts | SDD status code/tests, spec, ADR-019 | `rtk zsh -c 'go test -race -count=1 ./internal/capability/sdd ./cmd/jacu ./internal/runstate && go run ./cmd/jacu sdd status'` | done | Per-SDD rows contain no run association or `run_state` field; the CLI loads runstate once and prints one deterministic `repository_runs=...` aggregate, with unavailable/corrupt state remaining fail closed |
| T20 | Apply Plan 013: hostile ambient Git regression and one test-only hermetic environment helper | test-only Git fixtures | `rtk zsh -c 'go test -race -count=1 ./internal/gitx ./internal/capability/workspace ./internal/capability/sdd ./internal/telemetry ./internal/mcpadapter && go test -count=50 ./internal/gitx -run TestRevParseHeadAndHasCommits && go test -count=1 ./test/hosteval/...'` | done | One test-only `internal/testgit.Env` covers all identified commit-producing fixtures; a real hostile parent config enables signing, signer, hooks and templates, while the fixture still commits with local identity and proves none of the hostile programs ran |
| T21 | Apply Plan 014: make hygiene module checking read-only and prove clean/drifted hashes are stable | hygiene script and optional hygiene doc | `rtk zsh -c 'before=$(git status --porcelain); bash scripts/hygiene.sh; after=$(git status --porcelain); test "$before" = "$after" && go mod tidy -diff'` | done | Hygiene uses `go mod tidy -diff`, reports proposed drift without writes, and aggregate checks remain unchanged; clean status/hash regression passed |
| T22 | RED and contract for Plan 010: storage ownership matrix, dry-run immutability, caps, unsafe preservation and ADR-027 scenarios | storage tests, CLI tests, ADR/spec | `rtk go test -race -count=1 ./cmd/jacu ./internal/capability/storage ./internal/capability/workspace ./internal/capability/cleanexit ./internal/runstate` | done | Ownership contract and dry-run boundary established before CLI implementation; existing lifecycle had no storage surface |
| T23 | GREEN for Plan 010: bounded inspect, deterministic plan, explicit apply with revalidation and reused retention | storage CLI/package, docs and tests | `rtk zsh -c 'go test -race -count=1 ./cmd/jacu ./internal/capability/storage ./internal/capability/workspace ./internal/capability/cleanexit ./internal/runstate && go run ./cmd/jacu storage inspect --json && go run ./cmd/jacu storage prune --dry-run --json && bash scripts/e2e.sh -v'` | done | Deterministic bounded inventory reports count, bytes, age and state for every storage class; dry-run is default; apply revalidates canonical run/archive identity, content digest, age, directory/file identity and traversal bounds, preserves ambiguous/corrupt/active/foreign data, delegates task retention and leaves telemetry retention to its owning store |
| T24 | Apply Plan 015: reconcile only living specs, PROGRAM and ESTADO with measured local/current hosted state | living docs and affected contracts | `rtk zsh -c '! rg -n "TBD - created by archiving" docs/sdd/specs && go run ./cmd/jacu sdd lint docs/sdd/008-audit-hardening && bash scripts/verify.sh'` | done | Living docs, PROGRAM, ESTADO and target SDD lint pass; scope filtering now prevents unrelated historical SDDs from claiming this branch's paths; canonical verify passed |
| T25 | Apply Plan 016 and local closeout: verified operator README, every non-destructive example, full clean-source gates and execution handoff | README, distribution doc, plans, SDD evidence, execution report | `rtk zsh -c 'go run ./cmd/jacu help && bash scripts/mcp-smoke.sh && go test ./... -race && bash scripts/e2e.sh -v && bash scripts/hygiene.sh && bash scripts/verify.sh && git diff --check'` | done | README now documents the complete MCP operator workflow and storage/privacy boundaries; independent closeout ran help, smoke, 868 race tests, e2e, hygiene, vet, module verification and vulnerability scanning; push and hosted promotion remain separate gates below |

## Validation and promotion gates

These gates are outside local executor authority. They are performed later by
the independent validator/publisher requested by the owner. A failure at any
gate stops the sequence without claiming the next state.

| Gate | Authority | Action | Exact proof |
|---|---|---|---|
| V1 | validator | Check branch, base, ordered local commits, working-tree cleanliness, SDD scope and every task evidence pointer | `rtk git status --short --branch`; `rtk git merge-base HEAD origin/main`; `rtk git log --oneline --reverse origin/main..HEAD`; `rtk git diff --name-status origin/main...HEAD`; no path outside Allowed scope |
| V2 | validator | Independently inspect security boundaries and rerun targeted regression/repeatability commands, not merely executor summaries | every completed task Verify command exits as declared; every RED/GREEN pair is present and the regression fails for the intended reason on its RED commit |
| V3 | validator | Run the complete canonical local gate from a clean final tree | `rtk zsh -c 'test -z "$(gofmt -l .)" && go vet ./... && go test ./... -race && bash scripts/e2e.sh -v && bash scripts/hygiene.sh && bash scripts/verify.sh && go run ./cmd/jacu sdd lint --all && git diff --check'` exits 0 |
| V4 | validator | Record immutable validation receipt with reviewed HEAD, task/gate result and explicit remaining hosted/owner gates | receipt names the exact HEAD and reports `local_validation=pass`, `push=pending`, `hosted_ci=pending`, `review=pending`, `merge=pending`, `release=pending`, `install=pending` |
| P1 | publisher | Re-fetch origin, prove reviewed HEAD and base did not drift, then push only `codex/008-audit-hardening` and open one PR | remote branch SHA equals reviewed HEAD; PR base is `main`; no force push; PR contains task table, validation receipt and open gates |
| P2 | publisher with operator authority | Capture full ruleset 20705815, update only required status contexts to the exact eight-job set, then re-read and compare all non-status fields | live API reports active ruleset for `main`; sorted contexts are `e2e`, `hygiene`, `lint`, `mcp-smoke`, `mod-hygiene`, `secrets`, `verify`, `vuln`; other semantic fields equal the captured payload |
| P3 | publisher | Reconcile Plan 001/ESTADO after the hosted mutation, push the focused docs commit, rerun local delta gates and wait for fresh final-SHA CI | `gh pr checks <PR> --repo jacu-dev/jacu` lists all eight exact job keys and every one passes on the final SHA |
| P4 | publisher/reviewer | Answer every actionable review in its original thread, apply scoped fixes through new commits, rerun V2/V3 as proportional to the diff and verify unresolved actionable threads are zero | GitHub review query returns zero unresolved actionable threads; conversation-resolution rule is satisfied; final SHA still has all eight green checks |
| P5 | owner | Approve and merge the exact reviewed final SHA according to repository policy | `main` contains the PR merge result; merge SHA and final PR SHA are recorded; no production claim yet |
| P6 | owner | Choose the next semantic version, create the repository-approved signed `v*` tag on the merged production commit and push that tag | local and remote tag resolve to the intended merged commit; signature verification passes; Release workflow guard/build/publish jobs pass |
| P7 | validator | Verify release assets and install to a new temporary prefix before running the binary | release has `checksums.txt`, `checksums.txt.sigstore.json`, SPDX SBOM and four platform archives; `rtk bash scripts/install.sh --version <VERSION> --prefix <NEW_TEMP_PREFIX>` succeeds; installed `jacu version`, `doctor` and MCP smoke pass |
| P8 | validator | Confirm availability and write the final state report without erasing earlier gate distinctions | report records tag, commit, release URL, asset verification, installer target, smoke output and `production_installable=pass` |

Promotion repair rules:

- Any source or behavior change after V4 returns to V1–V4 before another push.
- A publisher-only factual documentation commit reruns scope review,
  `git diff --check`, SDD lint, canonical verify and fresh hosted CI.
- A failed hosted check is diagnosed and fixed; no job is renamed, made
  conditional, removed from the ruleset or ignored.
- Merge is never inferred from a green check. Release is never inferred from
  merge. Installation is never inferred from a published asset list.
- If GitHub identity or repository policy prevents the owner-only tag, P6 stays
  open and P7/P8 do not run.

## Done

| Level | Proof |
|---|---|
| Contract ready | SDD and ADRs reviewed, targeted lint passes, lock matches, preparation commit exists locally, T0 awaits explicit owner ratification |
| Executor done | T1–T25 are done or honestly blocked with dependants skipped; local commit series and exact report exist; tree is clean; no push or hosted mutation occurred |
| Local validation done | V1–V4 pass independently on one immutable HEAD |
| Hosted delivery done | P1–P5 pass on the final PR SHA with eight required jobs and zero unresolved actionable review threads |
| Production installable | P6–P8 pass for the owner-created signed tag; release assets, Sigstore bundle, checksum, clean install and binary/MCP smoke are verified |

No row implies a later row. The SDD is complete only at `Production
installable`; before that, the highest proven row is reported verbatim.

## Follow-ups

- Reassess the 8 MiB, 128 MiB, 12-month, 30-day and 1,000-record limits only
  from measured production volume; changing them requires a new SDD/ADR delta.
- Revisit `internal/modelcontrol` only at the declared `model-panel` entry gate.
- Keep memory retention deferred until measured store volume or recall quality
  justifies changing ADR-016.
- Use standard human-reviewed Git maintenance for unreachable objects; never add
  them to JACU storage pruning.
- Review the live ruleset after any CI job-key change in the same delivery; a
  green workflow alone is not proof that the job is required.
