---
sdd: 007-surface-profile
program: jacu-one-shot
spec_id: spc_pending
branch: 007-surface-profile
phase: docs/sdd/PROGRAM.md
adr: docs/adr/ADR-025-surface-profile.md
status: draft
---

# 007 — Surface profile

## Why

**This change is deferred.** It is written now so the queue has no gaps, not
because it is next. Do not open it before 006 closes and the entry conditions
below hold.

The `tools/list` catalogue is at 20.476 bytes against a 20.480 cap: four bytes of
slack, ratcheted literally in the `e2e` job. Every capability since ADR-008 has
entered through a CLI subcommand instead of a tool, which is the right answer and
also an admission that the surface is full. A surface profile lets a host see
only the tools it actually uses, which reclaims catalogue budget without removing
a capability from anyone.

The second half is onboarding. Installing JACU today means reading
`docs/install.md` and copying skills into four host directories by hand. The
2026-08-13 host bateria found that Codex silently truncates skill descriptions
under catalogue pressure and stopped routing entirely — an onboarding failure
that looked like a product failure. `jacu init` makes setup a command that
can be tested, which is the only way that class of bug gets caught before a user
finds it.

**Entry conditions.** Do not start until: 006 has closed; the `$defs`/`$ref`
catalogue compaction has been evaluated and either applied or rejected on
evidence; and there is a measured reason to believe a profile helps, not a
suspicion. Opening this change to buy catalogue slack that a schema refactor
would give for free is the wrong order.

## Locked decisions

1. A profile hides tools from a host; it never removes a capability from the
   binary. Profiles are presentation, not feature flags (ADR-008).
2. The 14-tool absolute ceiling and the 20 KiB `tools/list` ratchet stay. A
   profile reduces what a given host sees; it does not raise the cap (ADR-008).
3. Zero new MCP tools, including for profile management. Profiles are set by CLI
   and config (ADR-008; PROGRAM decision 3).
4. `jacu init` writes only inside directories the user names. It never edits
   a host config it was not pointed at (`docs/threat-model.md`).
5. Nothing ships without an eval on the live path (PROGRAM decision 10).

## Out of scope

- Removing any tool from the binary.
- Per-tool authorization or entitlement. A profile is not a permission model, and
  presenting it as one would be a security claim nothing enforces.
- Auto-detecting installed hosts and editing their configs unprompted.
- Raising the tool ceiling or the catalogue cap.

## Write scope

**Allowed**

```
docs/sdd/007-surface-profile/**
docs/sdd/specs/surface/spec.md
docs/adr/ADR-025-surface-profile.md
docs/install.md
docs/relatorios/sdd-007-execucao.md
internal/mcpadapter/**
internal/capability/surface/**
cmd/jacu/init.go
cmd/jacu/init_test.go
cmd/jacu/main.go
test/e2e/**
skills/using-jacu/SKILL.md
.jacu/verify-allowlist.json
```

**Forbidden**

```
internal/capability/workspace/**
internal/capability/memory/**
internal/modelcontrol/**
.github/**
```

## Requirements

### Requirement: A profile narrows the advertised catalogue only

The system SHALL advertise only the tools a profile names, and SHALL keep every
tool reachable through the CLI regardless of profile.

#### Scenario: a narrowed host sees fewer tools

- **WHEN** a host connects under a profile naming five tools
- **THEN** `tools/list` advertises exactly those five, and the catalogue byte
  count falls below the full-surface figure

Delta: ADDED

#### Scenario: a hidden capability is still reachable by CLI

- **WHEN** a tool is absent from the active profile
- **THEN** the equivalent `jacu` subcommand still works, because the profile
  is presentation and not a feature flag

Delta: ADDED

#### Scenario: the default profile is the full surface

- **WHEN** no profile is configured
- **THEN** all 13 tools are advertised and the existing ratchet applies unchanged

Delta: ADDED

### Requirement: Init writes only where it was pointed

The system SHALL write host configuration only into directories named by the
user, and SHALL report any pre-existing entry instead of overwriting it.

#### Scenario: an existing MCP entry is reported, not replaced

- **WHEN** the target host config already registers a server under the same name
- **THEN** init reports the conflict, changes nothing, and exits non-zero

Delta: ADDED

#### Scenario: a backup precedes every edit

- **WHEN** init modifies a host config
- **THEN** the prior file is preserved beside it before the write

Delta: ADDED

### Requirement: Skill descriptions survive catalogue pressure

The system SHALL place each skill's routing trigger within the first words of its
description.

#### Scenario: a truncated description still routes

- **WHEN** a host shortens descriptions to fit its skill budget
- **THEN** the trigger remains inside the retained prefix, and the host-eval
  harness routes the case

Delta: ADDED

## Non-goals

- Making the tool surface unlimited. The cap is the product's discipline, not an
  obstacle to route around.
- Replacing `docs/install.md`. Init automates the mechanical part; the
  document still explains what is happening.

## Open decisions

- [x] none — resolved as deferral. This change does not open until the entry
  conditions in Why hold. ADR-025 records the profile semantics and is written by
  T1 when the change opens, not before.

## Tasks

| # | Task | Files | Verify | Status | Evidence |
|---|---|---|---|---|---|
| T1 | Write ADR-025: profile as presentation, the default full surface, and why profiles are not a permission model | `docs/adr/ADR-025-surface-profile.md` | `wc -l` under 120; owner ratifies separately | todo | |
| T2 | RED: profile narrows `tools/list` to the named set; the default profile advertises all 13 | `internal/capability/surface/profile_test.go` | `go test ./internal/capability/surface -race` fails on absence | todo | |
| T3 | GREEN: profile resolution and catalogue filtering | `internal/capability/surface/profile.go`, `internal/mcpadapter/**` | `go test ./... -race` | todo | |
| T4 | RED: a tool hidden by profile is still reachable through its CLI subcommand | `internal/capability/surface/reach_test.go` | `go test ./... -race` fails | todo | |
| T5 | GREEN: prove the separation between advertised surface and capability | `internal/capability/surface/**` | `go test ./... -race` | todo | |
| T6 | RED: e2e ratchet gains a per-profile byte budget so a narrowed profile cannot silently grow | `test/e2e/surface_test.go` | `go test -tags=e2e ./test/e2e/` fails | todo | |
| T7 | GREEN: per-profile ratchet alongside the existing 20 KiB cap | `test/e2e/surface_test.go` | `go test -tags=e2e ./test/e2e/` | todo | |
| T8 | RED: init reports a conflicting host entry and changes nothing; every edit is preceded by a backup | `cmd/jacu/init_test.go` | `go test ./cmd/... -race` fails | todo | |
| T9 | GREEN: `jacu init`, writing only into named directories | `cmd/jacu/init.go`, `cmd/jacu/main.go` | `go test ./cmd/... -race` | todo | |
| T10 | Move each skill's routing trigger into the first words of its description | `skills/**` | `go test ./internal/mcpadapter -run Skills -race` | todo | |
| T11 | Prove the trigger survives truncation using the host-eval harness against a full host catalogue | `test/hosteval/**` | `bash scripts/hosteval.sh --case 4.1-inspect` passes without reducing the host catalogue | todo | |
| T12 | Update the installation document to describe init and the profiles | `docs/install.md` | human read | todo | |
| T13 | Write the living capability spec | `docs/sdd/specs/surface/spec.md` | `go run ./cmd/jacu sdd lint --all` exits 0 | todo | |
| T14 | Eval on the live path: a person who is not the owner installs from the release using only init and the document | `docs/evals/init.md` | outcome recorded, including where they got stuck | todo | |
| T15 | Write the execution report | `docs/relatorios/sdd-007-execucao.md` | PR with the hosted run link | todo | |

## Done

| Level | Proof |
|---|---|
| Core | `go test ./internal/capability/surface -race` green; default profile advertises 13 |
| Wiring | a hidden tool remains reachable by CLI; init refuses a conflicting entry; `bash scripts/verify.sh` green |
| E2E | `go test -tags=e2e ./test/e2e/` green with the full-surface ratchet and a per-profile budget |
| Eval | an install by someone other than the owner, from the release, using only init and the document |

## Follow-ups

- `$defs`/`$ref` compaction of the catalogue. Its trigger already fired — four
  bytes of slack remain — and it may make profiles unnecessary, which is exactly
  why it is evaluated before this change opens.
- Profile presets per host once the eval shows which tools each host actually
  calls; guessing them now would encode a suspicion as a default.
