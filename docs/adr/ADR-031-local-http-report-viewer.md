# ADR-031 — Local HTTP report viewer

- Status: proposed; owner ratification required
- Date: 2026-08-21
- Scope: SDD-014 session-scoped localhost viewer

## Decision

`jacu report serve` may bind TCP. That is an exception to the STDIO-only
MCP and no-HTTP program rules. The exception is narrow:

- Bind address is `127.0.0.1` only. Other interfaces, including `0.0.0.0`
  and `::`, are refused before listen.
- Routes are the viewer (`GET /`) and decision write-back
  (`POST /decision`) only. There is no MCP, no daemon, no auth, and no
  cross-host access.
- Lifetime is one planning session: the process exits when the owner
  stops it or the serving context is cancelled.
- `kind: audit` is read-only. Decision writes are refused for audit.
- Headless `jacu report render` binds no port.

This exists because a static HTML file cannot persist a plan choice into
`*.report.json`. The LLM still reads JSON, never HTML.

## Consequences

- Serve is a CLI mode, not a new MCP tool.
- Owner ratification is required before this decision is final.

## Alternatives rejected

- Opening a daemon or binding a public interface.
- Decision write-back through an MCP tool.
