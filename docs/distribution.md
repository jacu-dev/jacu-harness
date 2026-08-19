# Distribution

JACU distributes as GitHub Release assets (goreleaser + cosign keyless + SBOM) and as `go install github.com/jacu-dev/jacu-harness/cmd/jacu@latest`.

There is no `curl | sh` instruction. Download, review, then run `scripts/install.sh`.

The first public release is `v0.2.0`. Tag pushes remain an owner keystroke (`actor == ecouto`).

Cloud VMs should prefer a verified release binary (only `github.com` egress). Build-from-source is the fallback when the module proxy is reachable; the `go` directive is the floor that compiles with `GOTOOLCHAIN=local`.

See `docs/install.md` and `SECURITY.md`.
