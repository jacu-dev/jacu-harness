# JACU

A governance harness for coding agents. It compiles missions, isolates the work,
verifies declared scope, and preserves auditable local run state.

Three surfaces over one core: the **CLI**, the **MCP server** (`serve`), and — after
SDD-009 — a Go **library**. MCP is a surface, not the identity.

**It has no network, no credential and no deploy.** If JACU fails in every way it can,
production does not fall. That boundary is the design, not an omission.

Install from a verified GitHub Release (`scripts/install.sh --version v0.2.0`,
after reviewing the script) or with:

```sh
go install github.com/jacu-dev/jacu-harness/cmd/jacu@latest
```

There is no `curl | sh` installer. See [docs/install.md](docs/install.md) and
[docs/distribution.md](docs/distribution.md).

## Quick start

```sh
jacu doctor
jacu serve
```

For a host configuration, use `jacu init --host cursor` (or another supported
host) or `jacu doctor --emit claude-code`.

## Core MCP workflow

The names below are MCP tools called by the configured host, not shell
commands:

1. `jacu_mission_compile` turns the objective, scope and verification argv into
   a governed mission.
2. `jacu_workspace_open` creates the isolated run. Keep its `run_id` and make
   every change only inside the returned `worktree_path`.
3. `jacu_verify` runs the mission checks. Only a `pass` verdict may proceed to
   review.
4. `jacu_diff` returns the scoped patch and digest for human review. Any later
   edit invalidates that review and requires a new diff.
5. After explicit approval, `jacu_apply` validates and commits the reviewed
   tree. Use `jacu_discard` instead when abandoning the run.

Never apply an unreviewed diff. `jacu_apply` may run the mission's verification
commands for up to 10 minutes; MCP hosts must allow at least that long
(`MCP_TOOL_TIMEOUT` in Claude Code).

## Diagnostics and local data

Start with `jacu status`. It lists every run that still owns a worktree, in
every project. After installation, run `jacu doctor` and `jacu storage inspect --json`.

Runtime state lives under `.git/jacu`. User-level telemetry and tool caches live
under `~/.jacu-harness`, with a one-time migration from the previous user-level directory.

The program, its invariants and the definition of done live in
[docs/sdd/PROGRAM.md](docs/sdd/PROGRAM.md). Triggers, refusals and acceptance
floors live in [docs/decisions/](docs/decisions/).
