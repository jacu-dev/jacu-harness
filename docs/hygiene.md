# Hygiene — no junk, no hurry

Permanent cleanup rules for the repository and the machine.

## Repository

- `.gitignore` from commit 1: `bin/`, `dist/`, `*.test`, `coverage.out`, `*.prof`. Binaries are never committed.
- No leftover "temporary" files: a draft dies on the branch or becomes a real document.
- A new dependency is one line of justification in the PR/commit.
- Dead code does not sleep. Git history is the museum.
- MCP surface: every phase serializes the canonical `tools/list` response, measures UTF-8 bytes, and records tool count + size. Growth must match approved capability.

## CI gate and local pre-flight

- A green `CI` workflow on GitHub Actions for the PR SHA is the required gate before merge to `main`.
- `scripts/verify.sh` is a recommended local pre-flight; it never replaces Actions.
- "CI-equivalent" is only for when GitHub is unavailable, as a documented exception. It does not authorize merge.

## Phase-close checklist (beyond the technical gate)

Automated since 2026-08-11: `scripts/hygiene.sh`. It runs by machine, on every PR: `go mod tidy` is a no-op, no bare `TODO`/`FIXME`, no tracked build artifact, no tracked executable binary.

Tool count and `tools/list` size are measured by the e2e job.
