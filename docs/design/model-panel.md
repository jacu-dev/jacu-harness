# Entry design — model-panel

> Preserved design input. This is the design; `sdd.md` is written when that
> SDD opens.

## Why it exists

`internal/modelcontrol` exists, is solid and has no caller: deterministic
classifier, breaker, ledger and digest attestation, with **zero importers**.
The owner decision is **finish it, do not remove it**.

## Decision already taken

Route by **abstract profile** — `cheap`, `medium`, `planner`, `premium` —
with a **configurable map**. A flow node never names the tool. That preserves
breaker demotion and lets a CLI change without a recompile.

## Target

Four CLIs: Claude Code, Codex CLI, OpenCode and **Gemini CLI**.
Which profile each occupies is config, not code — that is why routing is by
profile.

**Prerequisite:** Gemini has not been tested as an MCP host. It needs host-
harness cases **before** the panel routes to it.

## Tasks

| # | Task | Note |
|---|---|---|
| 1 | ADR for profile → CLI binding | includes the attestation decision below |
| 2 | Flow node declares `lane`; engine calls `Classify` → `Route` → `ApplyDemotionBias` → `GuardedInvoke` | `internal/capability/orchestration` |
| 3 | Execution through `internal/runner` with bounded argv | fail-closed by default |
| 4 | `CostTrace` emitted as an event | the telemetry-v2 task that only makes sense here |
| 5 | Breaker and demotion with a live-path eval: take a CLI down on purpose and prove traffic migrates | — |

## Allowlist decision

Spawning a different CLI per node **widens the surface `jacu verify` guards**.
That needs an explicit position in the ADR, not an omission.

## Unresolved problem that belongs in the ADR, not later

`HostProfile` requires an absolute path, SHA-256 digest and signer per CLI.
**Four binaries that update themselves are four digests that change
themselves.** Either attestation becomes first-use verification with a cache,
or the panel breaks on every host update. Deciding that is part of task 1.
