# Contributing

## Ground rules

- **Do not add a new MCP tool.** New capability enters as a CLI subcommand.
  The MCP surface is frozen by design (ADR-008); widening it needs a recorded
  decision, not a PR.
- **No network, no credential, no deploy.** That boundary is the product
  (README). A PR that makes the binary speak to the network will be refused.
- Security-relevant paths are deny-by-default and fail closed
  ([docs/threat-model.md](docs/threat-model.md)). A change that relaxes a gate
  needs an ADR.
- Refusals and floors are recorded in [docs/decisions/](docs/decisions/).
  Check [will-not-do.md](docs/decisions/will-not-do.md) before proposing.

## Setup

Go is the only toolchain. The floor is in `go.mod`; CI pins the newest patch of
that minor via `.go-version`.

```sh
git clone https://github.com/jacu-dev/jacu-harness
cd jacu-harness
go build ./cmd/jacu
```

## Layout

| Path | What lives there |
|---|---|
| `cmd/jacu/` | CLI entrypoint and subcommands |
| `internal/capability/` | one package per governed capability (workspace, verify, memory, ...) |
| `internal/mcpadapter/` | the MCP stdio server over the capabilities |
| `internal/runstate/`, `internal/telemetry/` | persisted local state, versioned schemas |
| `skills/` | shipped SKILL.md files, embedded via `skillset.go` |
| `docs/adr/`, `docs/sdd/` | decisions and specs; behavior changes trace back here |
| `test/e2e/`, `test/hosteval/` | end-to-end and host evaluation suites |

## Verify

```sh
bash scripts/verify.sh
go test ./... -race
```

PRs must keep the `verify / verify` check green. That check also runs
golangci-lint, govulncheck, gitleaks, hygiene, e2e and MCP smoke — running
`scripts/verify.sh` locally before pushing saves a round trip.

Behavior changes come with a test. Parsers and other input boundaries prefer a
fuzz target next to the unit tests (see the existing `fuzz_test.go` files).

## Commits

Conventional Commits 1.0.0, English only, imperative subject ≤ 72 characters.

Allowed types: `feat`, `fix`, `docs`, `style`, `refactor`, `test`, `chore`, `ci`, `build`, `perf`, `revert`.

## Provenance

Public history has a single author identity. Authorship traces are refused by CI:

- AI `Co-Authored-By` trailers
- `noreply@anthropic.com` and `cursoragent@cursor.com`
- the phrase `Generated with`
- the robot emoji

Host names such as Claude, Codex and Cursor are product domain, not traces.
Check your branch before opening the PR:

```sh
go run ./cmd/jacu provenance --history origin/main..HEAD --json
```

## Review model

PRs merge on green CI with 0 required approvals (ADR-007). The exceptions are
the paths in [.github/CODEOWNERS](.github/CODEOWNERS) — CI, scripts, lint
config and the verify allowlist — which require owner review, because they are
the gate itself.
