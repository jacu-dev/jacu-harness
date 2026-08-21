---
sdd: 014-report-visual
program: jacu-one-shot
spec_id: spc_pending
branch: 014-report-visual
phase: docs/sdd/PROGRAM.md
adr: docs/adr/ADR-010-report-headless.md
status: draft
---

# 014 — Report visual: deterministic HTML factory

## Why

Long LLM prose does not scale for an autonomy model, and audit must be
evidence generated from real state, not a model's self-report. Every relevant
output (asked report, plan, audit) needs rich, standardized HTML owned by the
project (`docs/design/report-visual.md`).

The parent principle: the LLM produces structured CONTENT; a deterministic
machine assembles the report. An LLM never writes report HTML, CSS or JS.

ADR-010 cut v1 to the versioned JSON contract, the audit walker, the
deterministic Markdown projection, `jacu_report` and `jacu statusline`, with
no HTTP, browser, UI, `go:embed`, HTML, JavaScript or frontend dependency in
that phase. That cut defers this factory; it does not revoke it.

Two ADRs are required before coding: a local HTTP exception (bind 127.0.0.1,
viewer and decision routes only, lifetime = planning session) and an embedded
frontend exception (`web/` in the repo, build only in CI, `dist/` never
committed, `go:embed` at release; no runtime download). Those ADRs are T1 and
T2. They are not numbered in this document and are not written in this
authoring batch.

`jacu_report` already exists. Render, export, open and layout are CLI, never
a new MCP tool (ADR-008; PROGRAM forbids a new MCP tool).

## Locked decisions

1. v1 JSON, Markdown, `jacu_report` and statusline stay. Markdown is never
   parsed as state — ADR-010.
2. LLM writes schema-valid JSON. The factory emits HTML. The LLM never writes
   HTML, CSS, JS or diagram syntax — design input.
3. Zero new MCP tools for render, export, open or list — ADR-008. `jacu_report`
   remains validate and read.
4. Download-and-execute of a public frontend at runtime is forbidden — threat
   model; the embed ADR must keep the build in CI.

## Out of scope

- Factory-owned state or a factory database.
- Report editable after execution.
- MCP tools for render, export, open or list.
- LLM writing presentation markup or diagram syntax.
- Mermaid rendered in HTML (it is only a Markdown digest projection).
- Official cloud icons, extra blocks, SQLite (own triggers).
- Writing the two required ADRs in this authoring batch.

## Write scope

**Allowed**

```
docs/sdd/009-core-surface/**
docs/sdd/010-repo-governance/**
docs/sdd/011-workspace-contract/**
docs/sdd/012-structural-debt/**
docs/sdd/013-model-panel/**
docs/sdd/014-report-visual/**
docs/sdd/015-program-closeout/**
docs/sdd/PROGRAM.md
.cursor/agent-board.md
```

**Forbidden**

```
cmd/**
internal/**
.github/**
docs/adr/**
```

## Requirements

### Requirement: Two ADRs gate factory code

The system SHALL not merge factory, HTTP or embed code until both required
ADRs exist and are referenced by this SDD.

#### Scenario: code without the ADRs is not done

- **WHEN** factory, HTTP or embed files are present and the two ADRs are missing
- **THEN** this SDD is not done
Delta: ADDED

### Requirement: Pure factory, JSON in, HTML out

The factory SHALL map schema-valid `*.report.json` (`kind: adhoc|plan|audit`,
eight v1 blocks) to byte-identical HTML. The LLM SHALL NOT emit HTML, CSS or
JS.

#### Scenario: golden HTML is deterministic

- **WHEN** the same JSON is rendered twice
- **THEN** the HTML is byte-identical
Delta: ADDED

#### Scenario: presentation markup is refused

- **WHEN** the LLM emits HTML, CSS or JS
- **THEN** validation fails before render
Delta: ADDED

### Requirement: Headless export binds no port

The system SHALL export static HTML from the embed with no listener.

#### Scenario: CI export

- **WHEN** the HTML export CLI runs in CI
- **THEN** no port is bound
Delta: ADDED

### Requirement: Serve mode is session-scoped localhost after the HTTP ADR

After the HTTP ADR, serve mode SHALL bind 127.0.0.1 only, expose viewer and
decision routes only, and live for one planning session. It SHALL NOT be MCP
and SHALL NOT be a daemon.

#### Scenario: bind address

- **WHEN** serve starts
- **THEN** it listens on 127.0.0.1 and not on another interface
Delta: ADDED

### Requirement: MCP census unchanged

This change SHALL not add render, open, export or list tools.

#### Scenario: governed census

- **WHEN** the governed e2e census runs
- **THEN** it reports 13 tools
Delta: ADDED

## Non-goals

- Replacing ADR-010's JSON contract.
- Clarity-gate mechanics (SDD-005). The visual interview may later feed 005;
  it does not re-specify it here.
- Runtime download of frontend assets.

## Open decisions

- [x] none — ADR-010 defers this factory; local HTTP and embedded frontend
      remain T1 and T2 ADRs, numbered when written, and are not decided here.

## Tasks

| # | Task | Files | Verify | Status | Evidence |
|---|---|---|---|---|---|
| T1 | Write the local HTTP exception ADR (127.0.0.1, session, no daemon); number assigned when written | `docs/adr/` | `wc -l` under 120; owner ratifies separately | todo | |
| T2 | Write the embedded frontend ADR (CI `npm ci`, no runtime download, `go:embed` at release); number assigned when written | `docs/adr/` | `wc -l` under 120; owner ratifies separately | todo | |
| T3 | RED: JSON to HTML golden | `internal/reportgen/` | `go test ./internal/reportgen -race` | todo | |
| T4 | GREEN: headless factory | `internal/reportgen/` | `go test ./internal/reportgen -race` | todo | |
| T5 | Serve mode behind T1 | `internal/reportgen/`, `cmd/jacu/` | `go test ./internal/reportgen ./cmd/jacu -race` | todo | |
| T6 | Measure cold-start p95 before freezing the embed | `docs/relatorios/` | `test -f docs/relatorios/sdd-014-execucao.md` | todo | |
| T7 | MCP census unchanged | `test/e2e/` | `go test -tags=e2e ./test/e2e/ -run Governed` | todo | |

## Done

| Level | Proof |
|---|---|
| Gate | T1 and T2 ADRs exist and are referenced before factory code merges |
| Core | same JSON yields byte-identical HTML; CI export binds no port; 13 tools |

## Follow-ups

- Implementation Allowed, by later amendment after T1/T2: `internal/reportgen/**`,
  `cmd/jacu/`, `skills/jacu-report/**`, `docs/sdd/014-report-visual/**`. `web/**`
  is Allowed only after T2 exists.
- Suggested internal order from the design: contract and headless render, then
  interactive serve, then rich canvas.
- Idle RSS 40MB is not a live budget (floors-and-limits D4 withdrew it).
