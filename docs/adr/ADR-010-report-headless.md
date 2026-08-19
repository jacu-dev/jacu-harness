# ADR-010: Headless reports in v1

- Status: accepted
- Date: 2026-08-11
- Decider: Erick

## Context

A visual factory with HTML, local HTTP and an embedded frontend is out of the
v1 critical path. v1 needs a headless projection of state and a statusline.
MCP context surface and frontend toolchains must not enter that path.

## Decision

v1 implements only the versioned JSON contract, the audit walker, the
deterministic Markdown projection, capability `jacu_report` and command
`jacu statusline`. There is no HTTP, browser, UI, `go:embed`, HTML,
JavaScript or frontend dependency in this phase.

Embedded frontend, HTML rendering, interactive plan mode and any HTTP server
are post-v1. Each requires a new decision and its own gates. No consumer may
treat Markdown as the source of state.

## Consequences

- The report is useful in CI, terminal and LLM review without growing a
  network surface.
- Digest and goldens cover the projection that exists in v1.
- A rich visual experience is deferred on purpose, not partially simulated
  with HTML or authored Markdown.
