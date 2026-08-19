# Preflight capability

## Purpose

Preflight checks a compiled mission's predictable interruption points before
dispatch. It is fail-closed: an unresolved check blocks dispatch and produces
one reportable interruption batch.

## Check order

The checker evaluates every class before the first verification tool call:

1. `allowlist` — every verification argv is accepted by the same policy used
   by `jacu_verify`.
2. `program_not_on_path` — every authorized executable is present on `PATH`.
3. `path_missing` and `path_not_writable` — required input paths exist, while
   existing paths in the write scope are writable. A missing write target is
   valid because a mission may create a new file.
4. `credential_absent` — required credential presence is available; values are
   never read into the report or telemetry.
5. `network_undeclared` — a declared network need has an explicit declaration.
6. `doc_missing` — required document paths exist; document contents are never
   read by preflight.
7. `open_questions` — unresolved mission questions block dispatch.

All findings are typed closed classes. Their target and detail are diagnostic
report data only; telemetry carries the class and counts, not model-controlled
strings.

## Go contract

`preflight.Check(runstate.MissionSnapshot, preflight.Environment)` returns a
`Report` with `verdict` equal to `pass` or `block`, plus typed findings. An
empty finding set is the only passing state. `AssembleBatch` returns one batch
and sets `Dispatch` false whenever findings exist.

`missioncompile` calls the checker before returning a dispatchable mission. A
preflight finding adds a `BLOCK` lint and returns `blocked` without invoking a
verification or write tool.

## CLI contract

`jacu preflight` accepts repeated `--command` options for bare programs and
`--command-argv` options containing structured JSON argv arrays, plus repeated
`--path`, `--required-path`, `--credential`, `--credential-present`, and `--doc`
options, plus network declaration flags. Network requirements are modeled as
environment state rather than synthetic executable commands.
With `--json`, stdout contains only the JSON `Report`.

- exit `0`: `verdict=pass`;
- exit `1`: `verdict=block`;
- exit `2`: invalid arguments or report encoding failure.

The CLI emits typed `preflight.check` telemetry and, for a block,
`mission.interruption` telemetry through the v2 constructor. Every distinct
failure class is retained as a closed telemetry value. No new MCP tool is
added.

## Security properties

The allowlist is delegated to the verify package and is deny-by-default.
Environment checks use presence and metadata only. Preflight never executes a
verification command, reads credential values, reads document contents, or
accepts a shell string in place of argv.
