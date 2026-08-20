#!/usr/bin/env bash
# Assemble the files the release job uploads to the GitHub Release.
#
#   collect-release-assets.sh [dist-dir] [output-dir]
#
# Defaults: dist, release-assets.
#
# This lives in a script rather than inline in .github/workflows/release.yml so
# scripts/release-test.sh can exercise it. The inline version shipped a defect
# that no test could catch: it looked for the Homebrew formula at dist/jacu.rb
# and dist/Formula/jacu.rb, while GoReleaser v2 writes dist/homebrew/Formula/
# jacu.rb. The mismatch stayed invisible because the brews block landed after
# v0.2.0 was cut, so the first release to run this path was v0.3.0 — it failed
# there, on the owner's tag, which is the worst place to discover it.
set -euo pipefail

dist="${1:-dist}"
out="${2:-release-assets}"
root="$(cd "$(dirname "$0")/.." && pwd)"

if [ ! -d "$dist" ]; then
  echo "collect-release-assets: no such directory: $dist" >&2
  exit 1
fi

mkdir -p "$out"

# Top level only: GoReleaser puts the tarballs, checksums and sigstore bundle
# there, and the per-platform build directories underneath must not be uploaded.
find "$dist" -maxdepth 1 -type f -exec cp {} "$out"/ \;

if [ ! -f "$root/scripts/install.sh" ]; then
  echo "collect-release-assets: scripts/install.sh is missing" >&2
  exit 1
fi
cp "$root/scripts/install.sh" "$out/install.sh"

# Find the formula rather than guessing its path, so a GoReleaser layout change
# does not break the release again.
formula="$(find "$dist" -type f -name jacu.rb -print | sort | head -n 1)"
if [ -z "$formula" ]; then
  echo "collect-release-assets: GoReleaser did not write the Homebrew formula" >&2
  find "$dist" -name '*.rb' >&2 || true
  exit 1
fi
cp "$formula" "$out/jacu.rb"

# The tap and the install script both depend on these three. A release missing
# any of them is broken in a way that only surfaces at install time.
for required in checksums.txt checksums.txt.sigstore.json install.sh jacu.rb; do
  if [ ! -f "$out/$required" ]; then
    echo "collect-release-assets: missing required asset: $required" >&2
    exit 1
  fi
done

ls -la "$out"
