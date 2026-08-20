---
sdd: 020-derived-release-version
program: jacu-one-shot
spec_id: spc_pending
branch: 020-derived-release-version
phase: docs/sdd/PROGRAM.md
adr: docs/adr/ADR-028-open-source-export.md
status: draft
---

# 020 — Derive the release version instead of hardcoding it

## Why

`scripts/release-test.sh` asserted the formula by grepping for literal strings
containing `0.2.0`. That has two costs. The small one is that every release
requires editing the test. The large one is that the assertion is the wrong
shape: it checks whether the expected version appears *somewhere* in the file,
so a formula whose `version` field and download urls disagree passes — and that
is exactly the defect that ships a `brew install` resolving to a 404.

The same literal leaked into the instructional docs, where it is worse than
noise: `README.md` and `docs/install.md` told readers to `go install ...@v0.2.0`
and to pin `--version v0.2.0`, which freezes new users on an old release
forever. `docs/release.md` is a process checklist and had the number baked into
every step, so following it literally re-cuts the same tag.

This change makes the test derive the version from the artifact under test and
turns the docs' version references into placeholders or `latest`.

## Locked decisions

1. The formula's own `version` field is the source of truth for the test. The
   test asserts internal consistency, never a literal number.
2. Instructional docs teach `latest` where the tool supports it, and `vX.Y.Z`
   where a concrete tag must be typed.
3. Historical statements keep their numbers. `docs/distribution.md` saying the
   first public release is `v0.2.0` is a fact, not an instruction.

## Out of scope

- The `Formula/jacu.rb` snapshot contents. It is refreshed from the published
  release per `docs/release.md` step 5, not hand-edited here.
- `CHANGELOG.md` version sections, which are records.

## Write scope

**Allowed**

```
scripts/release-test.sh
README.md
CONTRIBUTING.md
docs/README.md
docs/install.md
docs/release.md
docs/cursor-cloud.md
docs/sdd/020-derived-release-version/**
```

**Forbidden**

```
cmd/**
internal/**
.goreleaser.yaml
Formula/**
.github/workflows/**
```

## Requirements

### Requirement: Formula assertions derive the version

`scripts/release-test.sh` SHALL read the version from the formula's `version`
field and SHALL NOT contain a literal release number.

#### Scenario: formula declares a version the urls do not use
- **WHEN** the formula declares `version "A"` and any download url carries a
  different version
- **THEN** `release-test.sh` exits non-zero and prints every version found in
  the urls
Delta: MODIFIED

#### Scenario: every url is stale in the same way
- **WHEN** the formula declares `version "A"` and all urls carry version `B`
- **THEN** `release-test.sh` exits non-zero rather than passing on internal
  agreement
Delta: ADDED

#### Scenario: a platform tarball is missing
- **WHEN** the formula omits the asset for one of the four platforms
- **THEN** `release-test.sh` exits non-zero naming the missing asset
Delta: MODIFIED

### Requirement: sha256 pins are distinct

`scripts/release-test.sh` SHALL reject a formula that reuses one sha256 across
platform tarballs.

#### Scenario: copy-pasted digest
- **WHEN** two platform entries pin the same sha256
- **THEN** `release-test.sh` exits non-zero
Delta: ADDED

### Requirement: Docs do not pin a stale release

Instructional documents SHALL NOT instruct the reader to install a specific
historical version.

#### Scenario: source install instruction
- **WHEN** a reader follows the `go install` line in `README.md`
- **THEN** the command targets `@latest`
Delta: MODIFIED

## Non-goals

- Automating the `Formula/jacu.rb` refresh. That stays a documented owner step.

## Open decisions

- [x] Keep `v0.2.0` in `docs/distribution.md` — resolved: keep. It states which
  release was first, which does not age.

## Tasks

| # | Task | Files | Verify | Status | Evidence |
|---|---|---|---|---|---|
| T1 | Derive the version and assert url/version consistency | `scripts/release-test.sh` | `bash scripts/release-test.sh` | done | `release-test: OK`; three-case probe covering consistent, mixed, and uniformly stale formulas |
| T2 | Reject duplicated sha256 pins | `scripts/release-test.sh` | `bash scripts/release-test.sh` | done | distinct-count check |
| T3 | Teach `latest` and placeholders in the docs | `README.md`, `docs/install.md`, `docs/cursor-cloud.md`, `docs/release.md`, `docs/README.md`, `CONTRIBUTING.md` | `grep -rn '0\.2\.0'` over instructional docs | done | no hits outside historical statements |
| T4 | Document the Cursor cloud entry point added by SDD-018 | `docs/cursor-cloud.md` | manual read | done | environment.json section |

## Done

| Level | Proof |
|---|---|
| Core | `scripts/verify.sh` green; the consistency check was proven against a deliberately corrupted formula, not only against the good one |

## Follow-ups

- Refresh `Formula/jacu.rb` from the published release after the next tag, per
  `docs/release.md` step 5.
