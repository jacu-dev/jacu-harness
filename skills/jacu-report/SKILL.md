---
name: jacu-report
description: "Use when the host needs a deterministic report projection, HTML factory, or statusline for the open project."
---

# Project a headless report

Call `jacu_report` or `jacu report --json` to read the structured workspace state.

- Generate only schema-valid JSON with the eight v1 blocks. Never emit HTML,
  CSS, JS, React, or diagram syntax. Validation refuses presentation markup
  before render.
- `jacu report --json` emits the `quality.json` artifact: the ADR-010 audit
  JSON (`schema_version`, `kind: audit`, eight blocks) on stdout.
- `jacu report render --input FILE` exports self-contained HTML and binds no
  port. `jacu report serve --input FILE` is session-scoped on 127.0.0.1.
- `jacu_report` still returns the compact envelope with Markdown and digest.
- Treat that JSON as the source of truth. Markdown and HTML are output only.
  Read decisions from JSON, never from HTML.
- Use the projection for runs, missions, programs, flow, and measured counts.
- The metrics block includes local telemetry v1 measurements when available; `jacu stats [--since 30d]` is the CLI diagnostic for the same data.
- Treat `apply_reverted_pct_heuristic` as a Git-history heuristic, never as an execution gate.
- Do not infer model or cost values when the report says `not measured`.
- Preserve warnings and the digest when presenting the result.
