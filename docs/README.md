# Documentation map

Start with the repository [README](../README.md), then:

## Using JACU

- [install.md](install.md) — build, install, register with a host.
- [distribution.md](distribution.md) — releases, verification, channels.
- [reference/cli.md](reference/cli.md) — every `jacu` subcommand.
- [reference/mcp-tools.md](reference/mcp-tools.md) — the frozen MCP tool surface.

## Understanding JACU

- [architecture.md](architecture.md) — one core, three surfaces; package map; state locations.
- [threat-model.md](threat-model.md) — what is guaranteed vs cooperative; controls per threat.
- [telemetry.md](telemetry.md) — local-only metrics.
- [hygiene.md](hygiene.md) — repository hygiene rules enforced by CI.

## Why it is this way

- [adr/](adr/) — architecture decision records.
- [sdd/PROGRAM.md](sdd/PROGRAM.md) — the program, invariants, definition of done.
- [sdd/specs/](sdd/specs/) — per-capability specs.
- [decisions/](decisions/) — triggers, acceptance floors, and
  [will-not-do.md](decisions/will-not-do.md): refusals recorded so they stay refused.
