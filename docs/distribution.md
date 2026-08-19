# Distribution

JACU distributes as GitHub Release assets (GoReleaser + cosign keyless +
SPDX SBOM). The recommended one-command install is

`go install github.com/jacu-dev/jacu-harness/cmd/jacu@v0.2.0`.

There is no `curl | sh` instruction. For a verified binary, download
`scripts/install.sh`, review it, then run it (omit `--version` for latest).

The first public release is `v0.2.0`. Tag pushes remain an owner keystroke
(`actor == ecouto`). See [release.md](release.md).

Cloud VMs should prefer a verified release binary (only `github.com`
egress). Build-from-source is the fallback when the module proxy is
reachable; the `go` directive is the floor that compiles with
`GOTOOLCHAIN=local`. Cursor VMs use `.cursor/install.sh`, a thin wrapper
around `scripts/cloud-install.sh`.

See [install.md](install.md) and `SECURITY.md`.
