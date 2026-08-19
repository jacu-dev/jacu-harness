#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."

version="v0.0.0-release-test"
test_root="$(mktemp -d)"
prefix="$test_root/prefix"
release="$test_root/release"
fakebin="$test_root/bin"
trap 'rm -rf "$test_root"' EXIT

if grep -q -- '-R jacu-dev/jacu ' scripts/install.sh; then
  echo "release test: install.sh still targets the retired jacu-dev/jacu repo" >&2
  exit 1
fi
if ! grep -q 'jacu-dev/jacu-harness' scripts/install.sh; then
  echo "release test: install.sh must fetch from jacu-dev/jacu-harness" >&2
  exit 1
fi
if grep -Ehn '^\s*(curl|wget).+\|[[:space:]]*(ba)?sh' README.md docs/install.md docs/distribution.md scripts/install.sh; then
  echo "release test: living install docs must not teach curl|sh" >&2
  exit 1
fi

bash scripts/release-verify.sh "$version"
if bash scripts/install.sh --dry-run --version "$version" --prefix "$prefix" >"$test_root/dry-run.log"; then
  :
else
  echo "release test: installer dry-run failed" >&2
  exit 1
fi
if [ -e "$prefix" ]; then
  echo "release test: dry-run wrote into the destination" >&2
  exit 1
fi
if ! grep -q 'jacu-mcp -> jacu' "$test_root/dry-run.log"; then
  echo "release test: dry-run must describe the jacu-mcp compatibility symlink" >&2
  exit 1
fi

mkdir -p "$release" "$fakebin" "$test_root/stage"
printf '#!/bin/sh\nold\n' >"$test_root/stage/jacu"
chmod 0755 "$test_root/stage/jacu"
case "$(uname -s)" in
  Darwin) release_os=darwin ;;
  Linux) release_os=linux ;;
  *) echo "release test: unsupported operating system" >&2; exit 1 ;;
esac
case "$(uname -m)" in
  x86_64|amd64) release_arch=amd64 ;;
  arm64|aarch64) release_arch=arm64 ;;
  *) echo "release test: unsupported architecture" >&2; exit 1 ;;
esac
asset="jacu_${version#v}_${release_os}_${release_arch}.tar.gz"
tar -czf "$release/$asset" -C "$test_root/stage" jacu
(cd "$release" && shasum -a 256 "$asset" > checksums.txt)
: >"$release/checksums.txt.sigstore.json"
printf '#!/bin/sh\n[ "$1" = verify-blob ]\n' >"$fakebin/cosign"
chmod 0755 "$fakebin/cosign"

assert_installed() {
  dest="$1"
  want="$2"
  if [ ! -f "$dest/jacu" ] || [ -L "$dest/jacu" ]; then
    echo "release test: $dest/jacu is not a regular-file binary" >&2
    exit 1
  fi
  if [ "$(cat "$dest/jacu")" != "$want" ]; then
    echo "release test: $dest/jacu content mismatch" >&2
    exit 1
  fi
  if [ ! -L "$dest/jacu-mcp" ] || [ "$(readlink "$dest/jacu-mcp")" != jacu ]; then
    echo "release test: missing jacu-mcp -> jacu compatibility symlink" >&2
    exit 1
  fi
}

legacy_prefix="$test_root/legacy-prefix"
mkdir -p "$legacy_prefix"
printf '#!/bin/sh\nlegacy-alias\n' >"$legacy_prefix/jacu-mcp"
chmod 0755 "$legacy_prefix/jacu-mcp"
PATH="$fakebin:$PATH" JACU_RELEASE_DIR="$release" bash scripts/install.sh --version "$version" --prefix "$legacy_prefix"
assert_installed "$legacy_prefix" $'#!/bin/sh\nold'
if [ -e "$legacy_prefix/jacu-mcp" ] && [ ! -L "$legacy_prefix/jacu-mcp" ]; then
  echo "release test: regular-file jacu-mcp was not replaced by a symlink" >&2
  exit 1
fi

PATH="$fakebin:$PATH" JACU_RELEASE_BASE_URL="file://$release" bash scripts/install.sh --version "$version" --prefix "$prefix"
assert_installed "$prefix" $'#!/bin/sh\nold'

latest_prefix="$test_root/latest-prefix"
PATH="$fakebin:$PATH" JACU_RELEASE_DIR="$release" JACU_LATEST_TAG="$version" bash scripts/install.sh --prefix "$latest_prefix"
assert_installed "$latest_prefix" $'#!/bin/sh\nold'
if PATH="$fakebin:$PATH" JACU_RELEASE_DIR="$release" bash scripts/install.sh --prefix "$test_root/need-version"; then
  echo "release test: omitted --version was accepted with JACU_RELEASE_DIR" >&2
  exit 1
fi

printf '#!/bin/sh\nnew\n' >"$test_root/stage/jacu"
tar -czf "$release/$asset" -C "$test_root/stage" jacu
(cd "$release" && shasum -a 256 "$asset" > checksums.txt)
PATH="$fakebin:$PATH" JACU_RELEASE_DIR="$release" bash scripts/install.sh --version "$version" --prefix "$prefix"
assert_installed "$prefix" $'#!/bin/sh\nnew'
PATH="$fakebin:$PATH" JACU_RELEASE_DIR="$release" bash scripts/install.sh --rollback --prefix "$prefix"
assert_installed "$prefix" $'#!/bin/sh\nold'

printf '#!/bin/sh\ntampered\n' >"$test_root/stage/jacu"
tar -czf "$release/$asset" -C "$test_root/stage" jacu
if PATH="$fakebin:$PATH" JACU_RELEASE_DIR="$release" bash scripts/install.sh --version "$version" --prefix "$prefix"; then
  echo "release test: tampered archive was accepted" >&2
  exit 1
fi
assert_installed "$prefix" $'#!/bin/sh\nold'

bad_host_log="$test_root/bad-host.log"
if PATH="$fakebin:$PATH" JACU_RELEASE_BASE_URL="https://jacu-unreachable.example/v0.0.0" bash scripts/install.sh --version "$version" --prefix "$test_root/bad-host" 2>"$bad_host_log"; then
  echo "release test: unreachable host was accepted" >&2
  exit 1
fi
if ! grep -q 'unreachable host: jacu-unreachable.example' "$bad_host_log"; then
  echo "release test: failure must name the unreachable host" >&2
  cat "$bad_host_log" >&2
  exit 1
fi

bash scripts/install.sh --dry-run --rollback --prefix "$prefix" >"$test_root/rollback-dry-run.log"
echo "release-test: OK"
