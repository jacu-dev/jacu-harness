# ADR-028 — Open-source export and rename to JACU (jacu-harness)

- Status: accepted
- Date: 2026-08-19
- Scope: program-wide; governs SDD-016 and SDD-017

## Context

The product must be installable by third parties with one command and usable
inside LLM cloud VMs. A private repository blocked `go install`, tokenless
release downloads and the G-10a social pilot. The private history contained
AI-authorship traces and mixed languages. Rewriting that history in place
would have been leak-prone and would have invalidated signed tags.

## Decision

1. **Brand.** The product is **JACU**, described as "a governance harness for
   coding agents". Binary and commands are `jacu`.
2. **Public repository.** `jacu-dev/jacu-harness`, module
   `github.com/jacu-dev/jacu-harness`.
3. **Fresh curated history.** This public tree starts from an exported HEAD as
   Conventional Commits by area. The former private repository is archived
   read-only and is never rewritten.
4. **Provenance policy.** Single author identity
   `Erick Soares do Couto <ecouto123@gmail.com>`. Zero AI attribution in
   commits, PRs, code comments and docs. Enforced by `provenance-lint`.
   References to Claude, Codex and Cursor as supported hosts are product
   domain and stay.
5. **English only** in this repository.
6. **User-level directory.** Runtime state lives under `~/.jacu-harness`, with
   a one-time migration from the previous user-level directory.
7. **Distribution.** GitHub Releases with goreleaser, cosign keyless and SBOM.
   The `go` directive is `1.25.0` with no toolchain pin, bound by
   `modelcontextprotocol/go-sdk v1.7.0`.
8. **CI self-containment.** Verify is vendored. The required check name
   `verify / verify` is preserved.

## Consequences

- `go install github.com/jacu-dev/jacu-harness/cmd/jacu@latest` becomes possible.
- Public history does not carry pre-export blame; the private archive keeps it.
- The rename is the export itself, not a commit in the former private tree.
