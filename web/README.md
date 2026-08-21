# Report canvas source

This directory is the later rich canvas for SDD-014 / ADR-032.

- Build only in CI (`npm ci` with the pinned lockfile).
- Never commit `dist/`.
- Never download a public frontend at runtime.
- Headless `jacu report render` uses the Go projector in
  `internal/reportgen` and does not require Node.
