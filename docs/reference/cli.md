# CLI reference

`jacu <command>`. Unknown argv prints usage to stderr — stdout belongs to the
protocol when a host launches the binary.

Most commands resolve the project root from the working directory; `status`
and `init` are deliberately global.

## serve

Speak MCP over stdio. This is what an MCP host runs — never call it by hand
expecting prose. Register per host with `jacu doctor --emit <host>` or
`jacu init`. Logs go to stderr; stdout is protocol-only.

## doctor

```
jacu doctor
jacu doctor --emit <host> [--repo PATH]
```

Without flags: report versions and environment. With `--emit`: print a host
pack (e.g. `claude-code`) that registers `jacu serve` for that host.

## init

Install skills and emit/apply a host pack into named paths.

```
jacu init --host <claude-code|claude-desktop|codex|cursor|generic|opencode>
         [--skills-dir DIR] [--config FILE] [--from DIR] [--repo PATH]
         [--dry-run] [--json]
```

Without `--config`, the pack is printed with the exact target path and that
file is not edited. `--json` prints only a JSON object.

## status

```
jacu status [--json]
```

List every run still holding a worktree, across all projects. Global on
purpose: requiring a repository would reproduce the blindness it exists to fix.

## inspect

```
jacu inspect [--json] [--events] [--input JSON]
```

Inspect the open project without opening a workspace. Same core as
`jacu_project_inspect`. `--json` prints the capability envelope; `--events`
streams telemetry v2 NDJSON while the command runs (stderr when combined with
`--json`, otherwise stdout).

Exit codes: `0` ok/accepted/partial, `1` blocked/failed, `2` usage.

## compile

```
jacu compile [--json] [--events] [--input JSON]
```

Compile a mission from the same JSON schema as `jacu_mission_compile`.

## workspace

```
jacu workspace open|status|diff|apply|discard [--json] [--events] [--run-id ID] [--input JSON]
```

Drive the core loop without an MCP host. `workspace status` is the per-repo
MCP `jacu_status` surface; `jacu status` stays the global parked-run scan.
Apply records host `jacu-cli`.

## memory

```
jacu memory save|recall [--json] [--events] [--input JSON]
```

Save or recall project memory. Same schemas as `jacu_memory_save` and
`jacu_memory_recall`.

## verify

```
jacu verify [--json] [--events] [--run-id ID] [--async] [--cancel] [--task-id ID] [--input JSON]
```

Run the mission's verification commands. `--events` emits a `verify` envelope
with `status: running` before the batch finishes.

## flow

```
jacu flow [--json] [--events] [--run-id ID] [--async] [--input JSON]
```

Execute a compiled orchestration graph. Same core as `jacu_flow_run`. Never
confused with `jacu run`, which is the headless provider.

## report

```
jacu report [--json] [--events]
```

Project structured workspace state as deterministic Markdown. Headless
projection of the audit state — same data the `jacu_report` MCP tool serves.
`--json` prints the capability envelope instead of Markdown.

## statusline

Print one honest active-run status line for the current project.

## stats

```
jacu stats [--full] [--since 30d]
```

Print local telemetry metrics for the current project. Telemetry is local
JSONL under `~/.jacu-harness`; nothing leaves the machine
([telemetry.md](../telemetry.md)).

## run

Execute one persisted run through an allowlisted headless provider
(`--run-id`, `--provider`, `--model`). See the runner spec
([sdd/specs/runner/spec.md](../sdd/specs/runner/spec.md)).

## preflight

Check the compiled mission's environment before dispatch: required tools,
allowlist coverage, open questions. Exit code is the verdict.

## provenance

```
jacu provenance --files --json
jacu provenance --history <base>..<head> --json
```

Scan files and commit history for authorship traces. The refused patterns are
listed in CONTRIBUTING → Provenance. CI runs both on every PR.

## storage

```
jacu storage inspect|prune [--dry-run] [--json]
```

Inspect or clean JACU-owned local storage. `prune` touches only owned paths
(`.git/jacu`, `~/.jacu-harness`) — never the working tree.

## sdd

```
jacu sdd new <slug>
jacu sdd lint [<directory>] [--all] [--json] [--write-lock]
jacu sdd status
jacu sdd close <directory>
```

Author, lint and inspect native SDD documents under `docs/sdd/`
([sdd/CONVENTIONS.md](../sdd/CONVENTIONS.md)).

## version

Print the build version stamped at build time (`-X main.Version=...`).
