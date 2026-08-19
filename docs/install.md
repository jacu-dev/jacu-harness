# Installation

Pick one. There is no `curl | sh` installer.

## Homebrew

```bash
brew install --formula https://raw.githubusercontent.com/jacu-dev/jacu-harness/main/Formula/jacu.rb
```

That is the one-command path on macOS and Linuxbrew. To follow updates from
this repository instead of a raw URL:

```bash
brew tap jacu-dev/harness https://github.com/jacu-dev/jacu-harness
brew install jacu
```

`brew` fetches the signed GitHub Release tarball for your OS/arch and
installs `jacu` plus the compatibility symlink `jacu-mcp`.

## curl

Download the installer, review it, then run it. One extra file, no pipe
into a shell:

```bash
curl -fsSL -o /tmp/jacu-install.sh \
  https://raw.githubusercontent.com/jacu-dev/jacu-harness/main/scripts/install.sh
bash /tmp/jacu-install.sh
```

After a published release you can also take the copy attached to the
release (same script, pinned to that tag's tree):

```bash
curl -fsSL -o /tmp/jacu-install.sh \
  https://github.com/jacu-dev/jacu-harness/releases/latest/download/install.sh
bash /tmp/jacu-install.sh
```

Omit `--version` to install the latest **published** release. Drafts are
ignored. Pin with `--version v0.2.0`. The script verifies Sigstore + sha256
before writing `~/.local/bin/jacu`, and creates a symlink `jacu-mcp`.
`--dry-run` and `--rollback` are supported. Offline assets:
`JACU_RELEASE_DIR`.

Requires `curl`, `cosign`, `tar` and `shasum`.

## go install

```bash
go install github.com/jacu-dev/jacu-harness/cmd/jacu@v0.2.0
```

Go's module checksum database, no remote shell. Compiles on the machine.
Then:

```bash
jacu init --host cursor
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

Cloud and Cursor VMs: `bash .cursor/install.sh` in this repository, or
`bash scripts/cloud-install.sh --from-source`. After a tagged release,
prefer `bash .cursor/install.sh --version v0.2.0` (github.com egress only).
See [cursor-cloud.md](cursor-cloud.md).

See [distribution.md](distribution.md) and [release.md](release.md).
