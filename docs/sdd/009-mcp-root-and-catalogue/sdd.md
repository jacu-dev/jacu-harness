---
sdd: 009-mcp-root-and-catalogue
program: jacu-one-shot
spec_id: spc_pending
branch: 009-mcp-root-and-catalogue
phase: docs/sdd/PROGRAM.md
adr: none
status: draft
---
# 009 — MCP root from the host, honest errors, and a catalogue that can speak

## Why

Three things were measured on 2026-08-21 against the shipped binary and the
MCP best-practice checklist, and each one is a defect the host can see.

**The server does not know where the repository is.** `projectRoot()` in
`cmd/jacu/main.go` is `filepath.Abs(".")`. There is no `--root`, no
environment override, and `ServerCapabilities{}` is empty, so the server never
asks the host for its `roots`. Every host that registers `jacu serve` globally
(Cursor `~/.cursor/mcp.json`, Claude Code `~/.claude.json`) spawns it with the
host's own working directory — on the owner's Mac that is `$HOME`. Measured:
`jacu storage inspect` walked `/Users/ecouto` and hung; `verify.NewTaskManager`
panics inside `NewServer` when that directory is not a project, and the host
reports "server crashed" with no message. SDD-019 and SDD-022 fixed the binary
name and the host registration; neither touched the root, and the root is what
breaks the session in every host that is not started from inside the checkout.

**Failure is reported as success.** No handler in `internal/capability` sets
`IsError` on the `CallToolResult`. A capability that fails returns
`status: "failed"` inside the envelope, with a summary such as "capability
execution failed" and no `next_actions`. Clients that honour the protocol flag
(Claude Code, Cursor) read that as a successful call and move on. The
protocol is explicit that tool execution errors go in the result with
`isError: true`.

**The catalogue cannot describe itself.** Tool descriptions are one word:
`Inspect.`, `Flow.`, `Compile.`, `Open.`, `Diff.`, `Apply.`, `Discard.`, `Run
checks.`, `Save memory.`, `Recall memory.`, `Status.`, `Show report.`. Input
schemas carry no `description`, no `required`, no `enum`
(`verifyInputSchema()`, `memory.Input.kind`, `memory.Input.source`). The
reason is structural, not taste: `tools/list` sits 4 bytes under the 20 KiB
ratchet in `test/e2e/mcp_test.go`, and the comment there records that
descriptions are 4 % of the weight while the **same envelope output schema is
repeated verbatim in every tool**. The budget is being spent on duplication,
and discoverability is what pays for it.

Two smaller items surfaced on the same pass and belong here because they are
the same surface:

- `jacu_memory_recall` truncates at 32 KiB with a warning and offers no
  `limit`/`cursor`/`has_more`. Once memory grows, the agent only sees what fit.
- `jacu sdd status` aborts on the first SDD directory without `sdd.lock.json`
  ("read lock: openat …: no such file or directory"), which is the state
  `jacu sdd new` leaves behind. The tool that reports status cannot run while
  a new SDD is being written.

## Locked decisions

1. **Root resolution order is: host `roots` → `--root` flag → `JACU_ROOT` →
   `git rev-parse --show-toplevel` from cwd → refuse.** Never the bare cwd,
   never `$HOME`. Refusal is a structured error on stderr naming the four
   sources tried. — PROGRAM invariant "governs one change inside one
   repository": a server that cannot name the repository must not start.
2. **`roots` is consumed, not polled.** The server declares interest at
   initialize, calls `roots/list` once after the handshake, and subscribes to
   `roots/list_changed`. With more than one root, the first `file://` root that
   contains a `.git` wins and the others are listed in a warning; with zero,
   the next source in decision 1 applies. No new MCP tool is added
   (PROGRAM: new capability enters through a CLI subcommand; this is
   transport negotiation inside `mcpadapter`).
3. **Any envelope with `status: failed` or `status: blocked` sets
   `IsError: true` on the wire**, keeps the envelope as `structuredContent`,
   and carries at least one `next_actions` entry. `blocked` stays
   distinguishable from `failed` inside the envelope; the flag says "do not
   treat this as done", the status says why.
4. **The envelope output schema is emitted once, by reference.** Every tool's
   `OutputSchema` becomes `{"$ref": "#/$defs/envelope", ...}` with only the
   `data` member inlined per tool. The 20 KiB ratchet is not raised; the bytes
   recovered pay for descriptions and schema annotations.
5. **Every tool description states what the tool does, what it needs, and what
   it returns, in at most three sentences.** Every input property carries a
   `description`; enumerated strings carry `enum`; mandatory members are in
   `required`. The hosteval catalogue assert is extended to fail on an empty
   or single-token description.
6. **`NewServer` never panics.** Construction errors return to `serve`, which
   logs them to stderr and exits non-zero with a message a human can act on.

## Out of scope

- HTTP or SSE transport, daemon mode. Vetoed program-wide; stdio stays.
- Any new MCP tool. The catalogue keeps twelve entries.
- Changing the envelope fields. Only how the schema is serialised changes.
- Host pack editing (`jacu init`) — done in SDD-019/022.
- Raising `maxToolsListBytes`.

## Write scope

**Allowed**

```
cmd/jacu/main.go
cmd/jacu/sdd.go
internal/mcpadapter/**
internal/capability/*/tool.go
internal/capability/verify/register.go
internal/capability/memory/tool.go
internal/capability/memory/search.go
internal/capability/memory/types.go
internal/runtime/pipeline.go
internal/capability/sdd/**
test/e2e/mcp_test.go
test/hosteval/catalogue.go
docs/reference/cli.md
docs/cursor-cloud.md
CHANGELOG.md
docs/sdd/009-mcp-root-and-catalogue/**
```

**Forbidden**

```
internal/gitx/**
internal/capability/*/core logic outside tool.go
.goreleaser.yaml
Formula/**
scripts/**
```

## Requirements

### Requirement: The server resolves the repository from the host

The system SHALL determine the project root from the MCP host's declared
roots before falling back to any local source, and SHALL refuse to serve
when no source yields a Git repository.

#### Scenario: host declares one root
- **WHEN** the host answers `roots/list` with one `file://` root containing `.git`
- **THEN** every capability operates on that directory regardless of the process cwd
Delta: ADDED

#### Scenario: host declares no roots and cwd is the home directory
- **WHEN** `roots/list` is empty or unsupported, no `--root`/`JACU_ROOT` is set, and cwd is not inside a Git worktree
- **THEN** `jacu serve` exits non-zero with a message listing the four sources tried, and no directory is walked
Delta: ADDED

#### Scenario: root changes mid-session
- **WHEN** the host sends `roots/list_changed`
- **THEN** the server re-resolves and subsequent calls use the new root; open runs bound to the previous root report `blocked`
Delta: ADDED

### Requirement: Failure is visible on the wire

The system SHALL set `isError: true` on every tool result whose envelope
status is `failed` or `blocked`.

#### Scenario: a capability fails
- **WHEN** any tool handler produces `status: failed`
- **THEN** the `CallToolResult` has `IsError: true`, the envelope is still present as structured content, and `next_actions` is non-empty
Delta: ADDED

### Requirement: The catalogue describes every tool and input

The system SHALL publish a `tools/list` in which every description has at
least one full sentence, every input property has a description, and the
total stays under the existing byte ratchet.

#### Scenario: catalogue assert
- **WHEN** `go test ./test/e2e -run TestGovernedChange` and `./test/hosteval -run TestCatalogue` run
- **THEN** both pass with `tools/list` ≤ 20 KiB, and the assert fails on any description of fewer than two tokens
Delta: ADDED

#### Scenario: envelope by reference
- **WHEN** `tools/list` is inspected
- **THEN** the envelope schema appears once under `$defs` and each tool references it
Delta: ADDED

### Requirement: Recall paginates

The system SHALL accept `limit` and `cursor` on `jacu_memory_recall` and
return `has_more` and `next_cursor`.

#### Scenario: more than one page
- **WHEN** matching records exceed `limit`
- **THEN** the result carries `has_more: true` and a `next_cursor` that yields the remainder without repetition
Delta: ADDED

### Requirement: `sdd status` tolerates an unlocked SDD

#### Scenario: freshly created SDD
- **WHEN** a directory under `docs/sdd` has `sdd.md` and no `sdd.lock.json`
- **THEN** `jacu sdd status` reports it as `unlocked` and continues to the next directory
Delta: ADDED

## Non-goals

- Multi-root sessions serving several repositories at once.
- Markdown rendering of envelopes; the envelope stays JSON.

## Open decisions

- none — `--root`/`JACU_ROOT` apply to every subcommand that calls
  `projectRoot()` (`serve`, `report`, `storage`, `preflight`, `provenance`,
  `sdd`, `stats`, `headless`), because they share the function and the
  `storage inspect` hang was on a non-serve path.

## Tasks

| # | Task | Files | Verify | Status | Evidence |
|---|---|---|---|---|---|
| T1 | `projectRoot()` accepts `--root`, `JACU_ROOT`, then `git rev-parse --show-toplevel` via `gitx`; refuses otherwise with the list of sources tried | `cmd/jacu/main.go` | `go test ./cmd/jacu/... -run TestProjectRoot` | todo | |
| T2 | `mcpadapter` declares roots interest, calls `roots/list` after initialize, subscribes to `list_changed`, passes the resolved root to capability registration lazily | `internal/mcpadapter/server.go` | `go test ./internal/mcpadapter/... -run TestRoots` | todo | |
| T3 | `NewServer` returns `error`; `serve` logs and exits non-zero | `internal/mcpadapter/server.go`, `cmd/jacu/main.go` | `go test ./internal/mcpadapter/...` | todo | |
| T4 | e2e: start the binary with cwd=`$HOME`-like temp dir and a host root pointing at a project; the mission reaches Git | `test/e2e/mcp_test.go` | `go test ./test/e2e -run TestRootFromHost` | todo | |
| T5 | `IsError: true` for `failed`/`blocked` in the shared wrapper path; `next_actions` mandatory on failure | `internal/runtime/pipeline.go`, `internal/capability/*/tool.go` | `go test ./internal/... -run TestIsError` | todo | |
| T6 | Envelope output schema by `$ref`; per-tool `data` inlined | `internal/capability/*/tool.go`, `internal/capability/verify/register.go` | `go test ./test/e2e -run TestGovernedChange` | todo | |
| T7 | Full descriptions for the twelve tools; `description`/`enum`/`required` on every input schema | `internal/capability/*/tool.go` | `go test ./test/hosteval -run TestCatalogue` | todo | |
| T8 | Catalogue assert rejects descriptions under two tokens and any property without `description` | `test/hosteval/catalogue.go` | `go test ./test/hosteval -run TestCatalogue` | todo | |
| T9 | `limit`/`cursor`/`has_more`/`next_cursor` on recall | `internal/capability/memory/tool.go`, `search.go`, `types.go` | `go test ./internal/capability/memory/... -run TestRecallPaginates` | todo | |
| T10 | `sdd status` reports `unlocked` instead of aborting | `cmd/jacu/sdd.go`, `internal/capability/sdd/**` | `go run ./cmd/jacu sdd status` with an unlocked directory present | todo | |
| T11 | Docs: root resolution order in `docs/reference/cli.md` and `docs/cursor-cloud.md`; CHANGELOG under Unreleased/Fixed | `docs/reference/cli.md`, `docs/cursor-cloud.md`, `CHANGELOG.md` | `go run ./cmd/jacu sdd lint --all` | todo | |

## Done

| Level | Proof |
|---|---|
| Core | `jacu serve` started from `$HOME` by a host that declares a root completes a governed change; started with no root from `$HOME` it exits with the four-source message and walks nothing |
| Wire | every `failed`/`blocked` envelope carries `isError: true`; `tools/list` ≤ 20 KiB with full descriptions and the envelope under `$defs` |
| Hygiene | `jacu sdd status` runs over a tree containing an unlocked SDD; `sdd lint --all` passes |

## Follow-ups

- **Close the seven SDDs that are 100 % done and still `draft`** (002, 008,
  018, 019, 020, 021, 022): PROGRAM I7 says every merged SDD is closed by `sdd
  close` and archived; `sdd close` requires `docs/sdd/archive/<dir>` to exist,
  and the archive directory does not exist in the tree. Decide whether
  `close` creates it or the rule changes.
- **005, 006, 007, 017 show 0 tasks done while their code is on `main`**
  (PRs #24, #25, #31 import or finish them). PROGRAM I10 ("no living document
  contradicts the code") is violated by the SDD table itself. Either mark the
  tasks with evidence or close them as superseded by the import commit.
- **001, 003, 004, 016 each have one open task** with no owner listed.
- The PROGRAM says the catalogue is "one slot below the 14-tool limit" with
  twelve tools registered; the number is stale in one of the two places.
- Numbering: `sdd new` filled 009 because 009–015 are absent from the tree
  although PR #19 "create native SDD documents 009-015" merged. Confirm they
  were intentionally removed (archive does not exist) before a second `sdd
  new` reuses 010.
