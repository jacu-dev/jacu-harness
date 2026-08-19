# Distribution

JACU distributes as GitHub Release assets (GoReleaser + cosign keyless +
SPDX SBOM). The easy install paths are Homebrew
(`brew install --formula` the `Formula/jacu.rb` in this repo) and
`scripts/install.sh` downloaded with curl, reviewed, then run.

There is no `curl | sh` instruction. Draft GitHub Releases are not
installable: `/releases/latest` ignores them, and the installer says so.

The first public release is `v0.2.0`. Tag pushes remain an owner keystroke
(`actor == ecouto`). The publish job attaches `install.sh` and forces the
GitHub Release out of draft. See [release.md](release.md).

Cloud VMs should prefer a verified release binary (only `github.com`
egress). Build-from-source is the fallback when the module proxy is
reachable; the `go` directive is the floor that compiles with
`GOTOOLCHAIN=local`. Cursor VMs use `.cursor/install.sh`, a thin wrapper
around `scripts/cloud-install.sh`.

See [install.md](install.md) and `SECURITY.md`.
