---
sdd: 017-installable-cloud
program: jacu-one-shot
spec_id: spc_pending
branch: 017-installable-cloud
phase: docs/sdd/PROGRAM.md
adr: docs/adr/ADR-028-open-source-export.md
status: draft
---

# 017 — Installable release and cloud bootstrap

## Why

After SDD-016 the artifact is public but installing is still a doc page: manual
curls, manual host registration, manual skills copy into four host dirs. The
2026-08-13 host bateria showed onboarding failures presenting as product
failures (Codex truncation). The 2026-08-19 audit reproduced build-from-source
failing in egress-restricted VMs and found two live anchoring defects: a
user-scope server inspecting the wrong directory as `partial`, and the
`claude-desktop` host pack offering no way to anchor cwd. This change delivers
one-command install locally and in cloud VMs, and fixes those defects.
Execution contract: `docs/plans/one-shot-open-source.md` (Phase C).

## Locked decisions

1. `jacu init` is split out of SDD-007 and delivered here; surface-profile work
   stays in 007 behind its entry conditions.
2. `init` writes only into directories the user names; no host auto-detection
   editing configs unprompted — SDD-007 locked decision 4, unchanged.
3. Distribution is verified release binaries first, build-from-source fallback —
   ADR-028 §7.
4. No new MCP tool; everything here is CLI — ADR-008, PROGRAM decision 3.

## Out of scope

- Surface profiles and catalogue compaction (SDD-007 / SDD-012).
- Homebrew tap, package managers beyond `go install` + release script.
- Owner-only evals (G-10a runs after this change, not inside it).

## Write scope

**Allowed**

```
cmd/**
internal/capability/projectinspect/**
internal/capability/preflight/**
internal/mcpadapter/**
scripts/**
test/e2e/**
test/hosteval/**
.cursor/**
docs/**
skills/**
.github/workflows/release.yml
```

**Forbidden**

```
internal/capability/workspace/**
internal/capability/memory/**
internal/modelcontrol/**
```

## Requirements

### Requirement: Release proof closes P6-P8

The system SHALL cut a public release whose assets install end-to-end on a
machine that is not the dev machine: download, cosign verify, `install.sh`,
`jacu doctor` green, recorded as the installable-release report.

#### Scenario: tampered asset is refused

- **WHEN** a downloaded tarball does not match the signed checksum manifest
- **THEN** `install.sh` refuses to alter the destination and exits nonzero

Delta: ADDED

### Requirement: install.sh fetches its own assets

The system SHALL let `install.sh --version vX.Y.Z` download tarball, checksums
and sigstore bundle itself, verify before writing, and keep `--dry-run`,
`--rollback` and `JACU_RELEASE_BASE_URL`.

#### Scenario: offline verification unchanged

- **WHEN** assets are already present locally
- **THEN** the script verifies and installs without downloading

Delta: MODIFIED

### Requirement: jacu init configures a named host in one command

The system SHALL provide `jacu init --host <claude-code|codex|opencode|cursor|claude-desktop>`
that installs the skills into the host's skills directory, emits or applies the
host pack only into files the user names, runs `doctor`, and reports a
machine-readable result.

#### Scenario: unnamed host config is never touched

- **WHEN** `init` runs without an explicit target file for a host that needs one
- **THEN** it prints the snippet and the exact target path instead of editing

Delta: ADDED

### Requirement: claude-desktop host pack anchors the repository

The system SHALL emit, for `claude-desktop`, a configuration that anchors the
server's working directory to a repository path supplied by the user (shell
wrapper `cd <repo> && exec jacu serve`), and `doctor --emit claude-desktop`
SHALL require that path.

#### Scenario: pack without a path fails loudly

- **WHEN** `doctor --emit claude-desktop` runs without a repository path
- **THEN** it exits nonzero naming the missing argument

Delta: MODIFIED

### Requirement: repo-scoped tools block outside a git work tree

The system SHALL make `jacu_project_inspect` and every repo-scoped tool return
`blocked` with a corrective instruction when the process cwd is not inside a git
work tree, instead of inspecting the wrong directory as `partial`.

#### Scenario: user-scope server with wrong cwd

- **WHEN** the server runs with cwd outside any git work tree and a repo-scoped
  tool is called
- **THEN** the envelope is `blocked`, names the cwd, and instructs anchoring

Delta: ADDED

### Requirement: One cloud bootstrap for every VM host

The system SHALL provide `scripts/cloud-install.sh` with a release-binary mode
(cosign-verified, requiring only github.com egress) and a `--from-source`
fallback, ending with `doctor`; `.cursor/install.sh` becomes a thin wrapper.

#### Scenario: egress-restricted VM

- **WHEN** the script runs where only github.com is reachable
- **THEN** release mode succeeds; and when it cannot, the failure names the
  unreachable host

Delta: ADDED

### Requirement: Catalogue survives the host round-trip

The hosteval SHALL assert that every advertised tool description reaches the
host non-truncated and non-empty.

#### Scenario: truncating host is caught

- **WHEN** a host returns a tool list with a shortened description
- **THEN** the matrix fails naming the tool and the observed length

Delta: ADDED

## Non-goals

- Editing host configs the user did not name.
- Any network capability inside the product itself (scripts download; the
  binary does not).

## Open decisions

- [ ] none

## Tasks

| # | Task | Files | Verify | Status | Evidence |
|---|---|---|---|---|---|
| T1 | Self-fetching install.sh + tests | `scripts/install.sh`, `scripts/release-test.sh` | `bash scripts/release-test.sh` | todo | |
| T2 | `jacu init` + per-host e2e | `cmd/jacu/init.go`, `test/e2e/**` | `bash scripts/e2e.sh` | todo | |
| T3 | claude-desktop pack anchoring | `cmd/jacu/hostpack.go` | `go test ./cmd/...` | todo | |
| T4 | cwd guard on repo-scoped tools | `internal/capability/projectinspect/**`, `internal/mcpadapter/**` | `go test ./... -race` | todo | |
| T5 | cloud-install.sh + restricted-VM eval | `scripts/cloud-install.sh` | scripted VM matrix | todo | |
| T6 | Catalogue round-trip assert | `test/hosteval/**` | `bash scripts/hosteval.sh` | todo | |
| T7 | Release v0.2.0 + installable-release report (owner tag) | `.github/workflows/release.yml`, docs | fresh-machine install log | todo | |

## Done

| Level | Proof |
|---|---|
| Core | Fresh VM installs from docs alone via one command per surface; P6-P8 report recorded |

## Follow-ups

- G-10a social pilot (owner-only) is unblocked and should run immediately after.
