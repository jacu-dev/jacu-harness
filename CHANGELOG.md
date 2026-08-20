# Changelog

All notable changes to JACU are recorded here. The format follows Keep a Changelog, and this project adheres to Semantic Versioning.

## [Unreleased]

### Added

- `.mcp.json` and `.cursor/mcp.json`, so a cloud session gets the `jacu` MCP
  server without hand configuration. The image phase of `scripts/dev-setup.sh`
  also writes the approval into `~/.claude/settings.json`: since Claude Code
  v2.1.196 a cloned repository cannot approve its own MCP servers, so a
  committed config alone would sit at `Pending approval` forever in a VM where
  nobody can accept the workspace trust dialog. Approval names the `jacu`
  server rather than enabling all project servers, and preserves any existing
  settings.

### Fixed

- Release asset collection moved from inline workflow shell into
  `scripts/collect-release-assets.sh`, and is covered by `release-test.sh`
  across every GoReleaser formula layout. The inline version looked for the
  Homebrew formula at `dist/jacu.rb` and `dist/Formula/jacu.rb` while
  GoReleaser v2 writes `dist/homebrew/Formula/jacu.rb`; because the `brews`
  block landed after 0.2.0 was cut, the first release to run that code was
  0.3.0 and it failed there. Collection now locates the formula by search and
  fails closed when a required asset is missing.

## [0.3.0] - 2026-08-20

### Removed

- **Breaking:** the `jacu-mcp` compatibility alias. Installers no longer create
  the symlink, packaging no longer declares it, and the binary no longer
  branches on its own filename. A host config that still launches `jacu-mcp`
  must be repointed with `jacu init --host <host>`. An existing `jacu-mcp` file
  in the install prefix is left untouched — it is the user's, not ours.
- **Breaking:** automatic migration of the `~/.jacu-mcp` user-state directory.
  `userstate.Legacy` and `EnsureMigrated` are gone; `Migrate` remains as a
  generic utility for a future rename. The migration failed closed when both
  directories existed, which surfaced as `home directory unavailable` and sent
  the binary to its cwd fallback, writing `.jacu-harness/` into checkouts.
  Anyone still holding `~/.jacu-mcp` should move it to `~/.jacu-harness` by
  hand.

### Added

- Weekly free hardening on public runners: 3m fuzz per target, `go test
  -race` on macOS, and a published-release install smoke
  (`scripts/install-smoke.sh`). Not on the pull-request gate.
- Homebrew tap `jacu-dev/homebrew-jacu`: first install is
  `brew install jacu-dev/jacu/jacu`; afterwards `brew install jacu` and
  `brew upgrade jacu`. GoReleaser writes `jacu.rb`; the tap syncs it
  from the GitHub Release.
- `scripts/dev-setup.sh`, a two-phase bootstrap for agent VMs and any clean
  Linux checkout. `--phase image` persists to disk and never exits non-zero,
  because on Claude Code a failing setup script means the session never starts;
  `--phase session` runs per session and re-runs the image phase when the
  script's hash no longer matches the snapshot it was built from. Wired to
  Cursor through `.cursor/environment.json` and to Claude Code through a
  `SessionStart` hook in `.claude/settings.json`. No credential is involved.

### Fixed

- Homebrew tap sync opens a pull request instead of pushing `main`, and
  verifies `jacu.rb` against the signed release checksums.
- Installer ignores draft GitHub Releases and names a missing published
  release instead of a generic github.com fetch error.
- Release workflow attaches `install.sh` and publishes the GitHub Release
  as non-draft so `/releases/latest` resolves.
- `scripts/release-test.sh` derives the release version from the formula
  instead of matching a hardcoded number, and now rejects a formula whose
  `version` field and download urls disagree — the case that ships a `brew
  install` resolving to a 404. It also rejects a sha256 reused across
  platforms.
- Instructional docs no longer pin a historical release. `README.md` and
  `docs/install.md` teach `go install ...@latest`, and `docs/release.md` uses
  `vX.Y.Z` placeholders instead of re-cutting the same tag.
- `.gitignore` covers `.jacu-harness/` run artifacts and the binary left by
  `go build ./cmd/jacu` in the module root. It also re-includes `.cursor/`,
  which a global ignore rule had been hiding — git does not descend into an
  excluded directory, so `.cursor/environment.json` was silently never
  committed.

## [0.2.0] - 2026-08-19

First public release. Private lineage: the former `jacu-mcp` tree, imported as a curated history without copying authorship traces.

### Added

- Public signed GitHub Release install (`scripts/install.sh --version`), with
  offline `JACU_RELEASE_DIR`, checksum + Sigstore verification, rollback, and
  a `jacu-mcp` compatibility symlink (removed again in 0.3.0).
- `jacu init --host` for Claude Code, Claude Desktop, Codex, Cursor, OpenCode
  and generic stdio hosts. `--json` is exclusive machine-readable output.
- Cursor / cloud VM bootstrap: `scripts/cloud-install.sh` and
  `.cursor/install.sh`. Failed fetches name the unreachable host.
- Repo-scoped MCP tools return `blocked` when `jacu serve` is not inside a
  git work tree.
- Hosteval catalogue assert: a truncated or empty tool description fails
  naming the tool and the observed length.

### Changed

- Installer fetches the public `jacu-dev/jacu-harness` release first via curl.
  `gh` is a fallback for the same repository.
