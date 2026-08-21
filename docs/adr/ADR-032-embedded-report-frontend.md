# ADR-032 — Embedded report frontend

- Status: proposed; owner ratification required
- Date: 2026-08-21
- Scope: SDD-014 factory assets and release embed

## Decision

The report factory maps schema-valid JSON to self-contained HTML. The LLM
never authors HTML, CSS, JS, or diagram syntax. v1 ships a Go projector
so headless export and golden tests need no Node runtime.

`web/` may hold a later rich canvas (Vite, React, xyflow, elkjs). That
tree is source only. CI may run `npm ci` against a pinned lockfile.
`web/dist/` is never committed. `go:embed` of a CI-built `dist/` is
allowed at release only. Runtime download of a public frontend is
forbidden.

Cold-start of the Go projector is measured before any embed is frozen
into the critical path. If the embed misses the floor, it stays lazy
and headless export keeps using the Go projector.

## Consequences

- CI export stays a pure function: JSON in, HTML out, no listener.
- Owner ratification is required before this decision is final.

## Alternatives rejected

- Downloading frontend assets at runtime.
- Letting the LLM emit presentation markup.
- Committing `web/dist/` or fetching npm packages inside `jacu serve`.
