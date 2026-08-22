# Installation

Pick one. There is no `curl | sh` installer.

## Homebrew

```bash
brew install jacu-dev/jacu/jacu
```

That first command auto-taps
[`jacu-dev/homebrew-jacu`](https://github.com/jacu-dev/homebrew-jacu)
and installs the formula. After the tap is present, the short name
works:

```bash
brew upgrade jacu
brew install jacu
```

GoReleaser writes `jacu.rb` into the GitHub Release; the tap syncs that
file. `brew` then fetches the signed tarball for your OS/arch and
installs `jacu`.

`brew install jacu` without the tap only works for homebrew-core
formulae. JACU is not there yet, so the first install must name the
tap.

Same-repo fallback (this tree's `Formula/jacu.rb`):

```bash
brew install --formula https://raw.githubusercontent.com/jacu-dev/jacu-harness/main/Formula/jacu.rb
```

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
ignored. Pin a specific release with `--version vX.Y.Z`. The script verifies Sigstore + sha256
before writing `~/.local/bin/jacu`.
`--dry-run` and `--rollback` are supported. Offline assets:
`JACU_RELEASE_DIR`.

Requires `curl`, `cosign` **2.5.0 or newer**, `tar` and `shasum`.

The releases are signed with a Sigstore bundle v0.3, which carries the
certificate under `verificationMaterial.certificate`. cosign only learned to
read that in 2.5.0; an older build looks for `x509CertificateChain`, finds
nothing, and aborts with `bundle does not contain cert for verification,
please provide public key` — which says nothing about the version. The
installer checks the version up front and says so instead.

## go install

```bash
go install github.com/jacu-dev/jacu-harness/cmd/jacu@latest
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
prefer `bash .cursor/install.sh --version vX.Y.Z` (github.com egress only).
See [cursor-cloud.md](cursor-cloud.md).

See [distribution.md](distribution.md) and [release.md](release.md).
