---
sdd: 022-mcp-reachable-from-cloud
program: jacu-one-shot
spec_id: spc_pending
branch: 022-mcp-reachable-from-cloud
phase: docs/sdd/PROGRAM.md
adr: docs/adr/ADR-008.md
status: draft
---

# 022 — Make the MCP server reachable from a cloud session

## Why

SDD-018 gave cloud VMs a bootstrap that installs the `jacu` binary, and its
follow-up deferred host registration until a signed release existed outside this
checkout. v0.3.0 exists, so that block is gone — but installing the binary was
never the hard part.

The product is a stdio MCP server. A cloud session only benefits from it if the
host launches `jacu serve`. Two facts make that harder than committing a config:

1. Claude Code reads project MCP servers from `.mcp.json`, but since v2.1.196 a
   **cloned repository cannot approve its own servers**. `enabledMcpjsonServers`
   committed to `.claude/settings.json` is ignored in an untrusted folder, and
   the server sits at `Pending approval`. Every cloud checkout is untrusted, and
   no human is present to accept the workspace trust dialog.
2. Approvals from `~/.claude/settings.json` *do* apply in an untrusted folder,
   and the image phase runs as root before Claude Code starts.

So the repository declares the server, and the image phase writes the approval
into the session user's settings. Without the second half the first half is
decorative: the tools never appear and the session silently runs without the
gates the harness exists to provide.

## Locked decisions

1. `.mcp.json` and `.cursor/mcp.json` are committed and declare `jacu serve`
   by name, not by path. The binary is on PATH; a path would break the moment
   the install prefix differs.
2. The approval is written only when the session is an agent VM, and names the
   `jacu` server explicitly. `enableAllProjectMcpServers` is not used: it would
   auto-approve unrelated servers in any other checkout on that VM.
3. Approval never clobbers an existing `~/.claude/settings.json`. Unparseable
   or non-object settings are left untouched and reported.

## Out of scope

- Codex and OpenCode host packs. `jacu init --host` already covers them for a
  human-driven install; no cloud surface reads them today.
- Registering the server on the developer's Mac, which `jacu init` already did.

## Write scope

**Allowed**

```
.mcp.json
.cursor/mcp.json
.gitignore
scripts/dev-setup.sh
scripts/cloud-install-eval.sh
docs/sdd/022-mcp-reachable-from-cloud/**
docs/cursor-cloud.md
CHANGELOG.md
```

**Forbidden**

```
cmd/**
internal/**
.goreleaser.yaml
Formula/**
```

## Requirements

### Requirement: The repository declares the MCP server

`.mcp.json` and `.cursor/mcp.json` SHALL declare `mcpServers.jacu` with command
`jacu` and args `["serve"]`, and both SHALL be tracked by git.

#### Scenario: a global ignore rule hides the config
- **WHEN** `.cursor/mcp.json` is matched by any gitignore rule
- **THEN** `cloud-install-eval.sh` exits non-zero, because a config that never
  reaches the VM is the same as no config
Delta: ADDED

#### Scenario: the declared command drifts
- **WHEN** either file declares a command other than `jacu serve`
- **THEN** `cloud-install-eval.sh` exits non-zero naming the file
Delta: ADDED

### Requirement: Cloud sessions approve the project server

On an agent VM, the image phase SHALL add `jacu` to `enabledMcpjsonServers` in
`~/.claude/settings.json`.

#### Scenario: fresh VM
- **WHEN** the image phase runs with no `~/.claude/settings.json`
- **THEN** the file is created enabling the `jacu` server
Delta: ADDED

#### Scenario: settings already exist
- **WHEN** `~/.claude/settings.json` holds unrelated keys and another approved
  server
- **THEN** both are preserved and `jacu` is appended
Delta: ADDED

#### Scenario: developer machine
- **WHEN** the script runs on Darwin without `CLAUDE_CODE_REMOTE` or
  `CURSOR_AGENT`
- **THEN** no settings file is written
Delta: ADDED

### Requirement: The session reports reachability

The session phase SHALL state whether `jacu` is on PATH and whether the project
MCP approval is present.

#### Scenario: binary installed but not on PATH
- **WHEN** `jacu` exists in the install prefix but not on PATH
- **THEN** the session phase says so and names the directory, because the host
  launches the server by name
Delta: ADDED

## Non-goals

- Making the harness gates mandatory in cloud sessions. They become available;
  enforcement is still the host's.

## Open decisions

- [x] `enableAllProjectMcpServers` vs naming the server — resolved: name it.
  Blanket approval would auto-trust any `.mcp.json` in any other repository
  cloned onto the same VM.

## Tasks

| # | Task | Files | Verify | Status | Evidence |
|---|---|---|---|---|---|
| T1 | Declare the server for both hosts | `.mcp.json`, `.cursor/mcp.json` | `bash scripts/cloud-install-eval.sh` | done | both parsed and asserted |
| T2 | Un-ignore `.cursor/mcp.json` | `.gitignore` | `git check-ignore -v .cursor/mcp.json` | done | no longer ignored |
| T3 | Write the approval from the image phase | `scripts/dev-setup.sh` | `bash scripts/cloud-install-eval.sh` | done | fresh + existing + Darwin cases |
| T4 | Report reachability in the session phase | `scripts/dev-setup.sh` | manual read of session output | done | PATH and approval lines |
| T5 | Cover all of it | `scripts/cloud-install-eval.sh` | `bash scripts/verify.sh` | done | negatives proven by removing and corrupting `.mcp.json` |

## Done

| Level | Proof |
|---|---|
| Core | `scripts/verify.sh` green; eval proves missing config, wrong command, gitignored config, fresh approval, preserved approval, and Darwin no-op |

## Follow-ups

- Replicate `.mcp.json` plus the approval step in the other repositories of the
  stack once their `dev-setup.sh` exists.
