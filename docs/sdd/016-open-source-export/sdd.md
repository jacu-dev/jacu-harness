---
sdd: 016-open-source-export
program: jacu-one-shot
spec_id: spc_pending
branch: 016-open-source-export
phase: docs/sdd/PROGRAM.md
adr: docs/adr/ADR-028-open-source-export.md
status: draft
---

# 016 — Open-source export: sanitize, rename, publish

## Why

The program's distribution promise (one-command install, cloud-VM usage, G-10a
social pilot) is blocked by the private repository. ADR-028 decides the export:
a new public repo `jacu-dev/jacu-harness` with fresh curated history, brand
JACU, binary `jacu`. This change executes the sanitization sweep and the export
itself. The full execution contract is `docs/plans/one-shot-open-source.md`
(Phases A and B); this SDD is its normative form.

## Locked decisions

1. Brand JACU, binary `jacu`, repo/module `jacu-harness` — ADR-028 §1-2.
2. Fresh curated history; the private repo is archived, never rewritten — ADR-028 §3.
3. Provenance policy with CI enforcement (`provenance-lint`) — ADR-028 §4.
4. English only in the public repo; exclusions of ADR-028 §6 do not ship.
5. Vendored verify workflow, check name `verify / verify` kept — ADR-028 §8.

## Out of scope

- Any functional code change beyond the mechanical rename (SDD-009 does the
  surface work afterwards, in the public repo).
- Release cutting and installers (SDD-017).
- History rewrite of the private repository.

## Write scope

**Allowed**

```
docs/**
scripts/export/**
.github/**
go.mod
go.sum
cmd/**
internal/**
skills/**
.cursor/**
README.md
AGENTS.md
CLAUDE.md
CONTRIBUTING.md
SECURITY.md
CHANGELOG.md
```

**Forbidden**

```
.jacu/verify-allowlist.json
```

## Requirements

### Requirement: Full-history secrets sweep gates the export

The system SHALL run `gitleaks git` over the complete private history with the
repository's `.gitleaks.toml` and SHALL block export while any unresolved
finding exists.

#### Scenario: a finding blocks

- **WHEN** the history scan reports a non-allowlisted finding
- **THEN** the export does not proceed and the finding is recorded in the
  archive's `sanitization-report.md`

Delta: ADDED

### Requirement: AI-trace inventory reaches zero in the exported tree

The system SHALL classify every match of the trace patterns (AI `Co-Authored-By`
trailers, `noreply@anthropic.com`, `cursoragent@cursor.com`, "Generated with",
robot emoji) as `product` or `trace`, and the exported tree and its new history
SHALL contain zero `trace` classifications.

#### Scenario: product references survive, traces do not

- **WHEN** the exported tree is scanned with the trace patterns
- **THEN** remaining matches are only host-support references (host packs,
  runner, skills) and the new `git log` has zero matches. Root host-instruction
  files (`AGENTS.md`, `CLAUDE.md`, and siblings) do not ship.

Delta: ADDED

### Requirement: Mechanical rename is grep-verifiable

The system SHALL rename module `github.com/jacu-dev/jacu-harness` to
`github.com/jacu-dev/jacu-harness`, `./cmd/jacu` to `./cmd/jacu`, binary to
`jacu`, and user dir `~/.jacu-harness` to `~/.jacu-harness` with a transparent
one-time migration, such that a case-insensitive tree search for `jacu`
returns only the migration shim and the CHANGELOG lineage line.

#### Scenario: old user dir migrates once

- **WHEN** `jacu` starts and `~/.jacu-harness` exists while `~/.jacu-harness` does not
- **THEN** the directory is moved, the move is logged, and subsequent starts do
  not repeat it

Delta: ADDED

### Requirement: Public history is curated Conventional Commits

The system SHALL import the exported tree as a small series of Conventional
Commits by area, all authored as `Erick Soares do Couto <ecouto123@gmail.com>`,
in English.

#### Scenario: provenance-lint accepts the import

- **WHEN** the `provenance-lint` CI job runs over the imported history
- **THEN** it passes: zero AI attribution, zero non-English subjects, zero
  non-conforming subjects

Delta: ADDED

### Requirement: CI is self-contained and gates the first PR

The system SHALL vendor the verify workflow into the public repo, keep the check
name `verify / verify`, add `provenance-lint` and the commit-convention check,
and the first PR SHALL run the full gate green.

#### Scenario: private reusable workflow is unreachable

- **WHEN** public CI runs without access to `jacu-dev/jacu-dev-ci`
- **THEN** the verify gate still executes fully from vendored definitions

Delta: ADDED

### Requirement: go.mod floor supports restricted VMs

The system SHALL set the `go` directive to the lowest version that compiles and
passes `go test ./... -race`, with no toolchain pin. The floor is a floor: CI
SHALL build it with the newest patch of the floor's own minor line, pinned in
`.go-version`, and SHALL NOT resolve the toolchain from `go.mod`.

#### Scenario: build on an older preinstalled toolchain

- **WHEN** the tree is built with the floor Go version and `GOTOOLCHAIN=local`
- **THEN** the build and tests succeed without downloading a toolchain

#### Scenario: the floor's own patch releases are not the build toolchain

- **WHEN** the floor names a patch release that a later patch has superseded
- **THEN** CI builds with the later patch and the claim still holds, because
  patch releases add no API and the `go` directive keeps the language version
  at the floor; resolving the toolchain from `go.mod` instead would pin CI to
  a release whose own advisories `govulncheck` reports as findings

Delta: ADDED

## Non-goals

- Surface-profile work (SDD-007) and `Run()` refactor (SDD-009).
- Translating archived historical evidence.
- Publishing `docs/heranca/`, `docs/relatorios/`, `docs/sdd/archive/`.

## Open decisions

- [ ] none — ADR-028 resolves them; license confirmation is an owner keystroke
      recorded at export.

## Tasks

| # | Task | Files | Verify | Status | Evidence |
|---|---|---|---|---|---|
| T1 | History secrets sweep + sanitization report | archive only | `gitleaks git --no-banner` | done | `docs/relatorios/sdd-016-open-source-export-sanitization.md`: gitleaks v8.30.1, 384 commits, exit 0, 0 findings |
| T2 | Trace inventory and product/trace classification, fix all traces | tree-wide | `bash scripts/export/trace-scan.sh` | done | `internal/provenance` + `provenance-lint` in verify.yml; traces must be 0 in the exported tree |
| T3 | Export script: tree export minus ADR-028 §6 exclusions | `scripts/export/**` | `bash scripts/export/export.sh --dry-run` | done | `internal/export` + `cmd/jacu-export`; wrappers under `scripts/export/` (do not ship) |
| T4 | Mechanical rename (module, cmd dir, binary, user dir shim) | tree-wide | `bash scripts/verify.sh` | done | rename is the export transform; private tree unchanged; `TestExportRealHead` |
| T5 | go.mod floor lowering | `go.mod` | `GOTOOLCHAIN=local go test ./... -race` | done | `go 1.25.0`, no `toolchain` line; SDK binding recorded in ADR-028 §7; the CI toolchain lives in `.go-version` (go1.25.13) because `setup-go` reads a floor as an exact version |
| T6 | Translate living PT-BR docs; write README/CONTRIBUTING/SECURITY/CHANGELOG | `docs/**`, root | `bash scripts/hygiene.sh` | done | English overlays for shipping ADRs 001–020, threat-model and design docs; `scripts/host-smoke/` excluded as owner-present eval evidence; `PortugueseInventory` empty is a `TestExportRealHead` gate; README/SECURITY overlays and in-tree English CONTRIBUTING/CHANGELOG already existed |
| T7 | Vendor verify workflow + provenance-lint + commit check | `.github/**` | first public PR green | in-progress | `.github/workflows/verify.yml` vendored; `ci.yml` calls it; `.go-version` pins the gate's toolchain in verify, release and weekly; public PR remains owner |
| T8 | Curated import series; archive private repo with ARCHIVED.md | new repo | `git log` scan = zero traces | todo | `docs/export/import-playbook.md` + `internal/export/commitplan.go` |

## Done

| Level | Proof |
|---|---|
| Core | Public repo green on `verify / verify`; trace scans zero; tree search for `jacu` clean |

## Follow-ups

- SDD-017 makes the public artifact installable (release proof, init, cloud bootstrap).
