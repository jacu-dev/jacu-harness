# Architecture

One core, three surfaces. The core is a set of governed capabilities over a
git repository; the surfaces are the **CLI** (`jacu <command>`), the **MCP
stdio server** (`jacu serve`) and, after SDD-009, a Go **library**. MCP is a
surface, not the identity (README).

## What the core guarantees

Two honest enforcement levels ([threat-model.md](threat-model.md)):

1. **Workspace boundary — guaranteed.** Every mutation happens in an isolated
   git worktree on its own branch. Apply only when the digest matches the
   reviewed diff. This holds even against an uncooperative host: it is
   git/filesystem, not an agreement.
2. **Per-call gating — cooperative.** Gates only see calls the host routes
   through JACU. Level 1 is what makes that safe.

The binary has no network, no credential and no deploy path. Deny-by-default
on path, command, env and network.

## The governed run

```
objective ──▶ mission_compile ──▶ workspace_open ──▶ edits in worktree
                                        │
                                     verify ──▶ diff (digest) ──▶ human review
                                        │                            │
                                     discard ◀── abandon      apply (digest match)
```

- `mission_compile` turns objective, scope and verification argv into a
  governed mission. Verification commands are checked against the allowlist
  (`.jacu/verify-allowlist.json`) — known executable + argv, never `sh -c`.
- `workspace_open` creates the run: a worktree on its own branch, a `run_id`,
  persisted run state under `.git/jacu`.
- `verify` executes the mission's checks inside the worktree. Only `pass`
  proceeds to review.
- `diff` returns the scoped patch and its digest. Any later edit invalidates
  the review.
- `apply` re-validates and commits the reviewed tree; `discard` abandons the
  run. Both are `risk: write` with `destructiveHint` — nothing in the runtime
  declares `risk: destructive` without a recorded decision.

## Package map

| Package | Responsibility |
|---|---|
| `cmd/jacu` | argv parsing, one `case` per subcommand; stdout discipline (protocol vs prose) |
| `internal/mcpadapter` | MCP stdio server; registers the tool surface over the capabilities |
| `internal/capability/missioncompile` | objective → governed mission; ceremony and risk classification |
| `internal/capability/workspace` | worktree lifecycle, boundary, diff/digest, apply/discard, autonomy policy |
| `internal/capability/verify` | allowlisted command execution, retention, run association |
| `internal/capability/preflight` | environment checks before dispatch |
| `internal/capability/memory` | durable knowledge store with lint, promotion and recall caps |
| `internal/capability/orchestration` | bounded multi-step flows: graph, fan-in, panel |
| `internal/capability/projectinspect` | read-only project analysis |
| `internal/capability/sdd` | native SDD documents: scaffold, lint, lock, status |
| `internal/capability/cleanexit` | receipts and removal of JACU-owned state |
| `internal/capability/storage` | inspect/prune of owned local storage |
| `internal/runstate` | persisted run state, versioned schema, file locks |
| `internal/telemetry` | local JSONL event store; never leaves the machine |
| `internal/modelcontrol` | host profiles, routing, cost and resilience dials |
| `internal/runner` | headless provider execution (`jacu run`) |
| `internal/provenance` | authorship-trace scanning for files and history |
| `internal/gitx` | git plumbing seams, testable |
| `skills/` + `skillset.go` | shipped SKILL.md files, embedded in the binary |

## State locations

| Location | Contents | Lifecycle |
|---|---|---|
| `.git/jacu` (per repo) | run state, receipts | owned; `jacu status`, `jacu storage` |
| `~/.jacu-harness` (per user) | telemetry, tool caches | owned; one-time migration from the previous directory |
| worktrees (per run) | isolated change | released by `apply` or `discard` |

## Where decisions live

Behavior traces back to records, not memory:

- [docs/adr/](adr/) — architecture decisions (ADR-001 … ADR-028).
- [docs/sdd/](sdd/) — specs and their locks; [PROGRAM.md](sdd/PROGRAM.md) is
  the program, its invariants and the definition of done.
- [docs/decisions/](decisions/) — triggers, refusals
  ([will-not-do.md](decisions/will-not-do.md)) and acceptance floors.

## Reference

- [reference/cli.md](reference/cli.md) — every subcommand.
- [reference/mcp-tools.md](reference/mcp-tools.md) — the frozen MCP tool surface.
