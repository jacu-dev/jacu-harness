# Distribution

JACU distributes as GitHub Release assets (GoReleaser + cosign keyless +
SPDX SBOM) and as `go install github.com/jacu-dev/jacu-harness/cmd/jacu@latest`.

There is no `curl | sh` instruction. Download `scripts/install.sh`, review
it, then run `bash install.sh --version vX.Y.Z`.

The first public release is `v0.2.0`. Tag pushes remain an owner keystroke
(`actor == ecouto`). See [release.md](release.md).

Cloud VMs should prefer a verified release binary (only `github.com`
egress). Build-from-source is the fallback when the module proxy is
reachable; the `go` directive is the floor that compiles with
`GOTOOLCHAIN=local`. Cursor VMs use `.cursor/install.sh`, a thin wrapper
around `scripts/cloud-install.sh`.

See [install.md](install.md) and `SECURITY.md`.
