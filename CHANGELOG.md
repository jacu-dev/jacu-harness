# Changelog

All notable changes to JACU are recorded here. The format follows Keep a Changelog, and this project adheres to Semantic Versioning.

## [Unreleased]

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
- Public tree of JACU, a governance harness for coding agents.

### Fixed

- Homebrew tap sync opens a pull request instead of pushing `main`, and
  verifies `jacu.rb` against the signed release checksums.
- Installer ignores draft GitHub Releases and names a missing published
  release instead of a generic github.com fetch error.
- Release workflow attaches `install.sh` and publishes the GitHub Release
  as non-draft so `/releases/latest` resolves.

## [0.2.0] - unreleased

First public release. Private lineage: the former `jacu-mcp` tree, imported as a curated history without copying authorship traces.

### Added

- Public signed GitHub Release install (`scripts/install.sh --version`), with
  offline `JACU_RELEASE_DIR`, checksum + Sigstore verification, and rollback.
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
