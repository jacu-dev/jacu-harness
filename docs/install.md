# Installation

One command. Review it, then run it.

```bash
go install github.com/jacu-dev/jacu-harness/cmd/jacu@v0.2.0
```

That is the recommended install: Go's module checksum database, no remote
shell. The MCP command is `jacu serve`. Then:

```bash
jacu init --host cursor
```

There is no `curl | sh` installer.

## Verified release binary

If you want the signed GitHub Release instead of compiling, download the
installer, review it, then run it. One line, no pipe into a shell:

```bash
curl -fsSL -o /tmp/jacu-install.sh https://raw.githubusercontent.com/jacu-dev/jacu-harness/main/scripts/install.sh && bash /tmp/jacu-install.sh
```

Omit `--version` to install the latest public release. Pin with
`--version v0.2.0`. The script verifies Sigstore + sha256 before writing
`~/.local/bin/jacu`, and creates a symlink `jacu-mcp`. `--dry-run` and
`--rollback` are supported. Offline assets: `JACU_RELEASE_DIR`.

Requires `curl`, `cosign`, `tar` and `shasum`.

## Build from a checkout

```bash
git switch main
git pull --ff-only
sha=$(git rev-parse HEAD)
mkdir -p "$HOME/.local/bin"
go build -trimpath -ldflags "-X main.Version=$sha" \
  -o "$HOME/.local/bin/jacu" ./cmd/jacu
jacu version
```

Cloud and Cursor VMs: `bash .cursor/install.sh` in this repository, or
`bash scripts/cloud-install.sh --from-source`. After a tagged release,
prefer `bash .cursor/install.sh --version v0.2.0` (github.com egress only).
See [cursor-cloud.md](cursor-cloud.md).

See [distribution.md](distribution.md) and [release.md](release.md).
