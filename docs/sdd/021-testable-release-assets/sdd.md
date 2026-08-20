---
sdd: 021-testable-release-assets
program: jacu-one-shot
spec_id: spc_pending
branch: 021-testable-release-assets
phase: docs/sdd/PROGRAM.md
adr: docs/adr/ADR-007.md
status: draft
---

# 021 — Make release asset collection testable

## Why

The v0.3.0 release failed at `Collect publishable assets`. GoReleaser v2 writes
the Homebrew formula to `dist/homebrew/Formula/jacu.rb`; the step looked only at
`dist/jacu.rb` and `dist/Formula/jacu.rb`.

The path mismatch is the symptom. The defect is that the step was inline shell
in `.github/workflows/release.yml`, so nothing executed it before a tag existed.
The `brews` block landed in #8, *after* v0.2.0 was cut, so v0.3.0 was the first
release ever to run that code — and it ran it on the owner's production tag.

ADR-007 makes tag promotion the one gesture reserved to the owner. A release
path that can only be exercised by that gesture inverts the ADR: it turns the
owner's keystroke into the test environment. This change moves the logic into a
script and covers it in `release-test.sh`, which runs on every pull request.

## Locked decisions

1. Asset collection lives in `scripts/collect-release-assets.sh`. The workflow
   step becomes a single call, so the behaviour is reachable from a laptop and
   from CI without a tag.
2. The formula is located by search, not by a list of candidate paths. A future
   GoReleaser layout change must not break a release.
3. Collection fails closed on a missing required asset. A release without
   `checksums.txt`, the sigstore bundle, `install.sh` or `jacu.rb` is broken in
   a way that only surfaces at install time, on someone else's machine.

## Out of scope

- The GoReleaser configuration itself. `directory: Formula` stays as it is; the
  consumer adapts to the producer.
- Publishing and tap sync, which are unchanged.

## Write scope

**Allowed**

```
scripts/collect-release-assets.sh
scripts/release-test.sh
.github/workflows/release.yml
docs/sdd/021-testable-release-assets/**
CHANGELOG.md
```

**Forbidden**

```
cmd/**
internal/**
.goreleaser.yaml
Formula/**
skills/**
```

## Requirements

### Requirement: Asset collection is executable outside a tag

Release asset collection SHALL be provided by `scripts/collect-release-assets.sh`,
and `.github/workflows/release.yml` SHALL invoke it rather than reimplement it.

#### Scenario: the workflow stops calling the script
- **WHEN** `release.yml` no longer references `scripts/collect-release-assets.sh`
- **THEN** `release-test.sh` exits non-zero
Delta: ADDED

### Requirement: The formula is found in any GoReleaser layout

Collection SHALL locate `jacu.rb` anywhere under the dist directory.

#### Scenario: GoReleaser v2 layout
- **WHEN** the formula is at `dist/homebrew/Formula/jacu.rb`
- **THEN** collection copies it to `jacu.rb` in the output directory
Delta: ADDED

#### Scenario: earlier layouts
- **WHEN** the formula is at `dist/jacu.rb` or `dist/Formula/jacu.rb`
- **THEN** collection copies it to `jacu.rb` in the output directory
Delta: ADDED

#### Scenario: no formula at all
- **WHEN** no `jacu.rb` exists under the dist directory
- **THEN** collection exits non-zero and lists the `.rb` files it did find
Delta: MODIFIED

### Requirement: Only top-level artifacts are published

Collection SHALL copy files from the top level of the dist directory and SHALL
NOT copy per-platform build directories.

#### Scenario: build directory present
- **WHEN** `dist/jacu_darwin_arm64/jacu` exists
- **THEN** the output directory contains no `jacu` binary
Delta: ADDED

### Requirement: Required assets are enforced

Collection SHALL fail when `checksums.txt`, `checksums.txt.sigstore.json`,
`install.sh` or `jacu.rb` is absent from the output.

#### Scenario: incomplete dist
- **WHEN** the dist directory holds only `checksums.txt`
- **THEN** collection exits non-zero naming the missing asset
Delta: ADDED

## Non-goals

- Validating the formula's contents here. `release-test.sh` already asserts
  version/url consistency (SDD-020).

## Open decisions

- [x] Keep `directory: Formula` in `.goreleaser.yaml` — resolved: keep. The
  producer's layout is its own business; the consumer searches.

## Tasks

| # | Task | Files | Verify | Status | Evidence |
|---|---|---|---|---|---|
| T1 | Extract collection into a script | `scripts/collect-release-assets.sh`, `.github/workflows/release.yml` | `bash scripts/release-test.sh` | done | workflow step is one call |
| T2 | Cover all three formula layouts plus the empty case | `scripts/release-test.sh` | `bash scripts/release-test.sh` | done | `release-test: OK` |
| T3 | Repoint the wiring assertions from the YAML to the script | `scripts/release-test.sh` | `bash scripts/release-test.sh` | done | two assertions updated |
| T4 | Record the fix | `CHANGELOG.md` | `go run ./cmd/jacu sdd lint --all` | done | Unreleased/Fixed |

## Done

| Level | Proof |
|---|---|
| Core | `scripts/verify.sh` green; collection proven against `dist/jacu.rb`, `dist/Formula/jacu.rb`, `dist/homebrew/Formula/jacu.rb` and a dist with no formula |

## Follow-ups

- v0.3.0 must be re-cut after this lands: the tag exists but its release run
  failed, so no assets were published.
