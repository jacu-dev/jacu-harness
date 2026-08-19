# Installation

JACU is distributed as signed GitHub Release assets and as `go install`.
There is no `curl | sh` installer. Download, review `scripts/install.sh`,
then run it.

The first public release is `v0.2.0`. Until that tag exists, build from
source or use `go install`.

## Verified release (recommended)

Requires `curl`, `cosign`, `tar` and `shasum`. Review the script, then:

```bash
curl -fsSL -o install.sh \
  https://raw.githubusercontent.com/jacu-dev/jacu-harness/main/scripts/install.sh
less install.sh
bash install.sh --version v0.2.0
```

The script downloads the tarball, `checksums.txt` and the Sigstore bundle
from GitHub Releases, verifies them, installs `~/.local/bin/jacu`, and
creates a symlink `jacu-mcp` for hosts that still launch the retired name.
`--dry-run` and `--rollback` are supported. Offline assets: set
`JACU_RELEASE_DIR` to a directory that already contains the three files.

## go install

```bash
go install github.com/jacu-dev/jacu-harness/cmd/jacu@latest
```

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

Cloud and Cursor VMs: `bash scripts/cloud-install.sh` (from-source default)
or `bash .cursor/install.sh` in this repository. Prefer
`cloud-install.sh --version v0.2.0` once the tag exists (github.com egress
only). See [cursor-cloud.md](cursor-cloud.md).

The MCP command is always `jacu serve`.

## Per-host registration

```bash
jacu init --host cursor
jacu init --host claude-code --config "$HOME/.claude.json"
jacu init --host claude-desktop --repo /path/to/repo --config "$HOME/Library/Application Support/Claude/claude_desktop_config.json"
```

`--host` is required. Skills are copied into the host skills directory.
Without `--config`, `init` prints the host pack and the exact target path
and does not edit that file. `--json` prints only a machine-readable
result. `jacu doctor --emit <host>` prints the pack alone.

See [distribution.md](distribution.md) and [release.md](release.md).
