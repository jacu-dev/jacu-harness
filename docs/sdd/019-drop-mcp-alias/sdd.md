---
sdd: 019-drop-mcp-alias
program: jacu-one-shot
spec_id: spc_pending
branch: 019-drop-mcp-alias
phase: docs/sdd/PROGRAM.md
adr: docs/adr/ADR-028-open-source-export.md
status: draft
---

# 019 — Drop the jacu-mcp compatibility alias

## Why

The product renamed `jacu-mcp` to `jacu`. SDD-017 shipped a compatibility
symlink so existing host configs kept working across the rename, and the binary
prints `jacu-mcp is deprecated; use jacu` when invoked through it. That alias is
now load-bearing in eleven places: two packaging manifests, one argv branch,
two installers, three install checks, and three test files. Every future change
to the install path pays for a name the product no longer uses.

The 2026-08-20 owner migration exercised the removal end to end on the only
known installation: all three hosts (`claude-code`, `cursor`, `codex`) were
re-pointed to `jacu`, and no `.jacu-mcp` path remains on that machine. The alias
now has no known consumer.

This change removes the alias from the shipped artifact and from the checks that
require it, leaving one binary name.

## Locked decisions

1. Removal is a breaking change for any host config still invoking `jacu-mcp`;
   it lands in the next minor, not as a patch on v0.2.0 — ADR-028 §7.
2. The install path keeps exactly one binary name. Installers do not create the
   alias and the smoke checks do not tolerate it.
3. `internal/userstate.Legacy` goes with it — owner decision, 2026-08-20. The
   automatic migration failed closed when both directories existed, and that
   failure mode cost more than the migration returned: it surfaced as
   `home directory unavailable`, sent the binary to its cwd fallback, and wrote
   `.jacu-harness/` into five package directories of this very checkout.
4. An existing `jacu-mcp` in the install prefix is left untouched. It is the
   user's file; the installer stops creating it and never deletes it.

## Out of scope

- The published v0.2.0 release and the tap formula already in
  `jacu-dev/homebrew-jacu`. Both keep the alias; only future releases drop it.
- Host-side config repair. `jacu init` already writes `jacu`.
- `Migrate` itself, which stays as a generic utility for a future rename.

## Write scope

**Allowed**

```
cmd/jacu/**
internal/userstate/**
scripts/install.sh
scripts/cloud-install.sh
scripts/install-smoke.sh
scripts/cloud-install-eval.sh
scripts/release-test.sh
.goreleaser.yaml
Formula/jacu.rb
docs/install.md
docs/sdd/019-drop-mcp-alias/**
CHANGELOG.md
.gitignore
```

**Forbidden**

```
internal/capability/**
internal/mcpadapter/**
internal/runner/**
.github/workflows/**
skills/**
```

## Requirements

### Requirement: Single installed binary name

The installers SHALL install exactly one executable named `jacu` and SHALL NOT
create a `jacu-mcp` symlink.

#### Scenario: release install leaves no alias
- **WHEN** `scripts/install.sh --version vX.Y.Z` completes into a prefix
- **THEN** the prefix contains a regular file `jacu` and no `jacu-mcp` entry
Delta: MODIFIED

#### Scenario: from-source install leaves no alias
- **WHEN** `scripts/cloud-install.sh --from-source` completes into a prefix
- **THEN** the prefix contains a regular file `jacu` and no `jacu-mcp` entry
Delta: MODIFIED

### Requirement: No argv-name dispatch

The binary SHALL behave identically regardless of the filename used to invoke
it, and SHALL NOT emit a deprecation notice for any name.

#### Scenario: invocation through a renamed copy
- **WHEN** the binary is invoked through a symlink named `jacu-mcp`
- **THEN** stdout is the normal command output and stderr contains no
  `deprecated` notice
Delta: REMOVED

### Requirement: Packaging emits no alias

The release manifests SHALL NOT declare a `jacu-mcp` symlink.

#### Scenario: generated formula has one binary
- **WHEN** `scripts/release-test.sh` renders the formula from `.goreleaser.yaml`
- **THEN** the rendered formula contains no `install_symlink` for `jacu-mcp`
Delta: MODIFIED

### Requirement: A pre-existing alias is left alone

Installing over a prefix that already contains a `jacu-mcp` entry SHALL neither
delete nor modify that entry.

#### Scenario: install over an older layout
- **WHEN** `scripts/install.sh` installs into a prefix holding a regular file
  named `jacu-mcp`
- **THEN** `jacu` is installed and the `jacu-mcp` file keeps its original
  contents and file type
Delta: ADDED

### Requirement: No automatic user-state migration

`userstate.Dir` SHALL resolve the state directory without attempting to migrate
a legacy directory, and SHALL NOT fail when an unrelated dot-directory exists in
the home directory.

#### Scenario: a retired state directory is present
- **WHEN** `~/.jacu-mcp` exists alongside `~/.jacu-harness`
- **THEN** `userstate.Dir` returns `~/.jacu-harness` without error
Delta: REMOVED

## Non-goals

- Preserving a migration path for host configs. `jacu init` is the supported
  repair, and it already exists.
- Renaming `.jacu-harness`, the module, or the repository.

## Open decisions

- [x] Remove `internal/userstate.Legacy` in the same change — resolved by the
  owner on 2026-08-20: remove it. The failure mode was demonstrated on the only
  known installation, and the migration protects a population that is not known
  to exist.

## Tasks

| # | Task | Files | Verify | Status | Evidence |
|---|---|---|---|---|---|
| T1 | Drop the argv-name branch and its test | `cmd/jacu/main.go`, `cmd/jacu/alias_test.go` | `go test ./cmd/jacu/...` | done | `ok cmd/jacu` |
| T2 | Stop creating the alias in both installers | `scripts/install.sh`, `scripts/cloud-install.sh` | `bash scripts/install-smoke.sh` | done | no alias in prefix |
| T3 | Invert the alias checks to assert absence, and prove a pre-existing alias survives | `scripts/install-smoke.sh`, `scripts/cloud-install-eval.sh`, `scripts/release-test.sh` | `bash scripts/cloud-install-eval.sh` | done | `cloud-install-eval: OK` |
| T4 | Drop `install_symlink` from packaging | `.goreleaser.yaml`, `Formula/jacu.rb` | `bash scripts/release-test.sh` | done | `release-test: OK` |
| T5 | Retire the user-state migration | `internal/userstate/userstate.go` | `go test ./internal/userstate/...` | done | `ok internal/userstate` |
| T6 | Narrow the living-docs allowlist and update install docs | `cmd/jacu/rename_test.go`, `docs/install.md` | `go test ./cmd/jacu/...` | done | allowlist no longer excepts the alias |
| T7 | Record both breaking changes | `CHANGELOG.md` | `go run ./cmd/jacu sdd lint --all` | done | Unreleased/Removed |
| T8 | Ignore the root-built binary, found while running T1 | `.gitignore` | `go build ./cmd/jacu && git status --short` | done | clean tree after a root build |

## Done

| Level | Proof |
|---|---|
| Core | `scripts/verify.sh` green with no `jacu-mcp` reference outside frozen ADRs and the export contract |

## Follow-ups

- Drop the alias from `jacu-dev/homebrew-jacu` when the next release ships; the
  tap currently mirrors the v0.2.0 formula, which still declares it.
- `docs/install.md` documents `brew install jacu-dev/jacu/jacu`; the tap formula
  and this repository disagree about the alias until that release lands.
