---
sdd: 013-model-panel
program: jacu-one-shot
spec_id: spc_pending
branch: 013-model-panel
phase: docs/sdd/PROGRAM.md
adr: docs/adr/ADR-009-model-control-host-profiles.md
status: draft
---

# 013 — Model panel: finish modelcontrol, do not remove it

## Why

`internal/modelcontrol` exists, is solid, and has no caller: deterministic
classifier, breaker, ledger and digest attestation, with zero importers. I1
kills orphans. The owner decision is finish it, do not remove it (PROGRAM,
`docs/design/model-panel.md`).

The trigger already fired: it depended on SDD-003 closing; 003 has been merged
since 2026-08-14 (`docs/decisions/triggers.md`).

Route by abstract profile — `cheap`, `medium`, `planner`, `premium` — with a
configurable map. A flow node never names a tool. That preserves breaker
demotion and lets a CLI change without a recompile.

Four CLIs: Claude Code, Codex CLI, OpenCode and Gemini CLI. Which profile each
occupies is config, not code. Gemini has not been tested as an MCP host; it
needs host-harness cases before the panel routes to it.

`HostProfile` requires an absolute path, SHA-256 digest and signer per CLI.
Four binaries that update themselves are four digests that change themselves.
That attestation choice belongs in T1's successor ADR, not in this checkbox.
Spawning a different CLI per node widens the surface `jacu verify` guards;
that also belongs in the ADR.

## Locked decisions

1. The routable object is a named `HostProfile` (`cheap`, `medium`, `planner`,
   `premium`). No remote model, API price table, HTTP endpoint, embeddings,
   OAuth or credentials enter the runtime — ADR-009.
2. A CLI is eligible only with an absolute path, SHA-256 digest and signature
   attestation verified by an injected function. JACU does not invent a
   signature scheme — ADR-009.
3. Control is an internal library. There is no `jacu_model_route` and no new
   MCP surface — ADR-008, ADR-009.
4. `CostTrace` records only ids, tokens, duration, billing mode and integer
   units. `subscription` cannot declare USD or API cost — ADR-009.

## Out of scope

- HTTP, OAuth, embeddings, generic model-router (`docs/decisions/will-not-do.md`).
- New MCP tools.
- Routing to Gemini before a host-harness case exists.
- Deciding first-use attestation cache versus break-on-update in this
  document (T1 ADR).

## Write scope

**Allowed**

```
docs/sdd/013-model-panel/**
docs/sdd/PROGRAM.md
docs/adr/ADR-030-model-panel-binding.md
docs/evals/model-panel.md
docs/relatorios/sdd-013-execucao.md
internal/capability/orchestration/**
internal/runner/**
internal/modelcontrol/**
internal/telemetry/**
test/hosteval/**
scripts/hosteval.sh
```

**Forbidden**

```
cmd/**
.github/**
```

## Requirements

### Requirement: Wire modelcontrol, do not delete it

The system SHALL keep `internal/modelcontrol` and give it a caller.

#### Scenario: orchestration or runner imports it

- **WHEN** `go list` reports importers of `internal/modelcontrol`
- **THEN** at least orchestration or runner is among them
Delta: ADDED

### Requirement: Profile routing, never a named binary on the node

A flow node SHALL declare a `lane`. The engine SHALL call `Classify`, then
`Route`, then `ApplyDemotionBias`, then `GuardedInvoke`.

#### Scenario: a node does not name a CLI

- **WHEN** a flow node declares `lane`
- **THEN** it does not name a CLI binary
Delta: ADDED

### Requirement: Fail-closed spawn

The runner SHALL spawn only an attested absolute path.

#### Scenario: missing digest, path or signer

- **WHEN** digest, path or signer is missing
- **THEN** no process is spawned
Delta: ADDED

### Requirement: CostTrace is a telemetry v2 event

A guarded invoke SHALL emit a v2 `CostTrace` with ids, tokens, duration,
billing mode and integer units. `subscription` SHALL NOT invent USD.

#### Scenario: a guarded invoke finishes

- **WHEN** a guarded invoke finishes
- **THEN** the event is on the stream and contains no invented dollar price
Delta: ADDED

### Requirement: Breaker demotion never leaves zero candidates

Demotion SHALL migrate traffic when a CLI is down and SHALL never leave a
task with zero candidates — ADR-009.

#### Scenario: cheap CLI is taken down

- **WHEN** the cheap CLI is killed in the live-path eval
- **THEN** a later node uses another eligible profile
Delta: ADDED

## Non-goals

- Bandit, gateway, embeddings, OAuth and API routing (ADR-009).
- A new MCP tool for routing.

## Open decisions

- [x] none — ADR-009 locks profiles and fail-closed attestation; self-updating
      digest cache versus break-on-update is owned by T1's successor ADR, not
      this SDD checkbox.

## Tasks

| # | Task | Files | Verify | Status | Evidence |
|---|---|---|---|---|---|
| T1 | Write the profile-to-CLI binding ADR, including self-updating digest attestation and allowlist widening; number assigned when written | `docs/adr/` | `wc -l` under 120; owner ratifies separately | done | ADR-030 written; owner ratification remains |
| T2 | RED: orchestration calls Classify, Route, ApplyDemotionBias, GuardedInvoke | `internal/capability/orchestration/` | `go test ./internal/capability/orchestration -race` | done | TestExecuteRoutesLaneThroughModelControl |
| T3 | GREEN: lane routes through `modelcontrol` | `internal/capability/orchestration/`, `internal/modelcontrol/` | `go test ./internal/capability/orchestration ./internal/modelcontrol -race` | done | orchestration imports modelcontrol |
| T4 | Execution through `internal/runner` with bounded argv, fail-closed | `internal/runner/` | `go test ./internal/runner -race` | done | TestRunBlocksWhenAttestationMissing |
| T5 | `CostTrace` emitted as a telemetry v2 event | `internal/modelcontrol/`, `internal/telemetry/` | `go test ./internal/modelcontrol ./internal/telemetry -race` | done | TestCostTraceEventHasNoInventedDollarPrice |
| T6 | Live-path breaker eval: take a CLI down and prove traffic migrates | `docs/evals/` | `test -f docs/evals/model-panel.md` | done | docs/evals/model-panel.md |
| T7 | Gemini host-harness cases before the panel routes to it | `test/hosteval/` | `bash scripts/hosteval.sh` | done | TestGeminiIsNotARouteTargetUntilHarnessPasses |
| T8 | MCP census unchanged | `test/e2e/` | `go test -tags=e2e ./test/e2e/ -run Governed` | done | no new MCP tool |

## Done

| Level | Proof |
|---|---|
| Core | `modelcontrol` has a caller; nodes declare `lane`; spawn is fail-closed; `CostTrace` is on the stream; breaker eval migrates traffic |
| Guard | 13 tools; Gemini is not a route target until T7 passes |

## Follow-ups

- Implementation Allowed, by later amendment: `internal/capability/orchestration/**`,
  `internal/runner/**`, `internal/modelcontrol/**`, `internal/telemetry/**`,
  `docs/sdd/013-model-panel/**`. The successor ADR is T1, not this authoring batch.
- Do not create a sibling `design.md`. The design input stays in
  `docs/design/model-panel.md` until implementation.
