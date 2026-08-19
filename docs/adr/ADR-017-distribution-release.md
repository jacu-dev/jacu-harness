# ADR-017: Reproducible signed tag-only distribution

- Status: accepted
- Date: 2026-08-11
- Decider: Erick

## Context

The binary already receives a version via `ldflags`, but there was no safe
path for an external machine to install, verify, update and roll back a
version. ADR-007 reserves production promotion to an owner-created `v*` tag
and keeps CI as the only automatic integration gate.

## Decision

1. GoReleaser produces one binary per `darwin|linux` × `amd64|arm64`, with
   `CGO_ENABLED=0`, `-trimpath`, `-buildvcs=false` and version injected by
   ldflag. The workflow does not publish artifacts from a branch or from a
   `workflow_dispatch` dry-run.
2. Release fires only on `push` of `v*`. A job also refuses tags whose actor
   is not the owner. Final authority remains the tag ruleset, not this
   workflow condition.
3. The `sha256` checksum of every asset is signed with cosign keyless using
   GitHub Actions OIDC. Syft publishes an SPDX JSON SBOM. No private key
   exists in the repository or as a workflow secret.
4. The installer downloads an explicit version, verifies the checksum-file
   signature and the tarball checksum before install, keeps the previous
   binary and offers `--dry-run` and `--rollback`. Documentation never uses
   `curl | sh`.
5. `doctor --emit` generates deterministic snippets for Claude Code, Codex
   CLI, opencode, Cursor and Claude Desktop. An MCP registry is out of this
   phase.

## Consequences

- The release build does not need a macOS runner; the four targets
  cross-compile on the Linux runner.
- Private-repository install may change only the download base via
  `JACU_RELEASE_BASE_URL`; verification stays mandatory.
- Owner-present piloting is documented as an external social gate; mechanical
  tests do not pretend a developer machine is a virgin machine.
