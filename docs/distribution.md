# Distribution

```
Go
  → single binary (CGO_ENABLED=0)
  → GitHub Actions + GoReleaser
  → GitHub Release (signed tarballs + checksums + SBOM)
       ├── Homebrew tap  (jacu-dev/homebrew-jacu ← jacu.rb)
       └── install.sh    (review, then run; no curl|sh)
```

GoReleaser builds the four `darwin|linux` × `amd64|arm64` archives and
the Homebrew formula. Cosign keyless signs `checksums.txt`. The publish
job attaches `install.sh` and `jacu.rb` and leaves the GitHub Release
**published** (drafts make `/releases/latest` 404). The Homebrew tap
never receives a bot push on `main`: sync opens a PR, `verify` checks
the formula against the signed checksums, and the owner merges.

Install:

```bash
brew install jacu-dev/jacu/jacu
brew upgrade jacu
```

```bash
curl -fsSL -o /tmp/jacu-install.sh \
  https://github.com/jacu-dev/jacu-harness/releases/latest/download/install.sh
bash /tmp/jacu-install.sh
```

There is no `curl | sh` instruction. Draft GitHub Releases are not
installable. The first public release is `v0.2.0`. Tag pushes remain an
owner keystroke (`actor == ecouto`). See [release.md](release.md).

Cloud VMs should prefer a verified release binary (only `github.com`
egress). Build-from-source is the fallback when the module proxy is
reachable; the `go` directive is the floor that compiles with
`GOTOOLCHAIN=local`. Cursor VMs use `.cursor/install.sh`, a thin wrapper
around `scripts/cloud-install.sh`.

See [install.md](install.md) and `SECURITY.md`.
