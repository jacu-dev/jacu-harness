---
sdd: 018-cloud-dev-environment
program: jacu-one-shot
spec_id: spc_pending
branch: 018-cloud-dev-environment
phase: docs/sdd/PROGRAM.md
adr: docs/adr/ADR-028-open-source-export.md
status: draft
---

# 018 — Cloud dev environment bootstrap

## Why

SDD-017 made JACU installable in a cloud VM, but a cloud session still has to be
told what to run and when. Claude Code on the web and Cursor cloud agents share
one shape — a build step that is snapshotted, and a start step that runs per
session — and today neither is wired: `.cursor/install.sh` covers only the
Cursor build step, and nothing covers Claude Code at all.

Two defects found on 2026-08-20 make this more than convenience. First, the
Claude Code environment cache is a filesystem snapshot that survives edits to a
repo script, so a session can boot on a stale build step with no signal. Second,
`~/.gitignore_global` commonly ignores `.cursor/`, and git does not descend into
an excluded directory, so `.cursor/environment.json` silently never reaches the
repository and the Cursor VM boots with no bootstrap at all — the failure is
invisible in `git status`.

This change adds one entry point that serves both platforms, and makes the
existing eval prove the wiring instead of proving an unused wrapper.

## Locked decisions

1. One script, two phases: `--phase image` persists to disk, `--phase session`
   runs per session. The split mirrors both platforms rather than either one.
2. The image phase never exits non-zero. On Claude Code a failing setup script
   means the session never starts, so failures are recorded and surfaced by the
   session phase.
3. The session phase compares a hash of the script against a stamp and re-runs
   the image phase when they diverge. This is the only defence against the
   snapshot cache serving a stale build.
4. No credential, no vault read, no decryption. A cloud session holds no secret,
   so there is nothing to leak — ADR-028 §7.

## Out of scope

- The other repositories in the owner's stack. This change is the reference
  implementation; replication is manual and per-repo.
- Cursor's secrets store. Deliberately unused: this repository needs no
  credential to build or test.
- Removing `.cursor/install.sh`. It stays as the Cursor build-step wrapper.

## Write scope

**Allowed**

```
scripts/dev-setup.sh
scripts/cloud-install-eval.sh
.cursor/environment.json
.claude/settings.json
.gitignore
docs/sdd/018-cloud-dev-environment/**
```

**Forbidden**

```
cmd/**
internal/**
.github/workflows/**
skills/**
```

## Requirements

### Requirement: Two-phase bootstrap entry point

The repository SHALL provide `scripts/dev-setup.sh` accepting `--phase` with
`image`, `session`, or `all`, and the image phase SHALL exit zero even when a
provisioning step fails.

#### Scenario: image phase survives a broken toolchain
- **WHEN** `dev-setup.sh --phase image` runs with a `go` that exits non-zero
- **THEN** the script exits zero and records the failure for the session phase
Delta: ADDED

#### Scenario: session phase reports what the image phase could not do
- **WHEN** the image phase recorded a failure and the session phase runs
- **THEN** the session phase prints the recorded failure
Delta: ADDED

### Requirement: Stale snapshot self-healing

The session phase SHALL compare a hash of `dev-setup.sh` against the stamp
written by the image phase and SHALL re-run the image phase when they differ.

#### Scenario: script edited after the snapshot was taken
- **WHEN** the session phase runs and the stamp does not match the script hash
- **THEN** the image phase runs again before the session continues
Delta: ADDED

### Requirement: Local sessions are unaffected

`dev-setup.sh --if-remote` SHALL produce no output and take no action when the
host is not an agent VM.

#### Scenario: invoked on the maintainer machine
- **WHEN** `dev-setup.sh --phase session --if-remote` runs on Darwin without
  `CLAUDE_CODE_REMOTE` or `CURSOR_AGENT`
- **THEN** the command exits zero having printed nothing
Delta: ADDED

### Requirement: Both cloud entry points stay wired

`scripts/cloud-install-eval.sh` SHALL fail when `.cursor/environment.json` is
missing, is not valid JSON, lacks `install` or `start`, or does not reference
`scripts/dev-setup.sh`.

#### Scenario: environment file drops out of the repository
- **WHEN** `.cursor/environment.json` is absent from the checkout
- **THEN** `cloud-install-eval.sh` exits non-zero naming the missing file
Delta: ADDED

## Non-goals

- Replacing `scripts/cloud-install.sh`. `dev-setup.sh` calls it.
- Running services. This repository is Go-only and needs none.

## Open decisions

- [x] Keep `.cursor/install.sh` alongside `.cursor/environment.json` — resolved:
  keep. The eval now proves both, and removing the wrapper is unrelated churn.

## Tasks

| # | Task | Files | Verify | Status | Evidence |
|---|---|---|---|---|---|
| T1 | Add the two-phase bootstrap script | `scripts/dev-setup.sh` | `bash scripts/dev-setup.sh --phase session --if-remote` | done | no output on Darwin |
| T2 | Wire both platforms | `.cursor/environment.json`, `.claude/settings.json` | `bash scripts/cloud-install-eval.sh` | done | `cloud-install-eval: OK` |
| T3 | Re-include `.cursor/` against the global ignore, and cover run artifacts | `.gitignore` | `git check-ignore -v .cursor/environment.json` | done | no longer ignored |
| T4 | Extend the eval to prove the new wiring | `scripts/cloud-install-eval.sh` | `bash scripts/verify.sh` | done | `verify: OK` |

## Done

| Level | Proof |
|---|---|
| Core | `scripts/verify.sh` green; `cloud-install-eval: OK` with the four added checks |

## Follow-ups

- Replicate the two-phase pattern in the other repositories of the stack.
- Add `install.sh --version` + `jacu init` to the image phase once a session can
  fetch a signed release from outside this checkout, so cloud sessions regain
  deterministic gates instead of prompt-only rules.
