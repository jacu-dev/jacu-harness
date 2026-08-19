#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."

version="v0.0.0-release-test"
test_root="$(mktemp -d)"
prefix="$test_root/prefix"
release="$test_root/release"
fakebin="$test_root/bin"
trap 'rm -rf "$test_root"' EXIT

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

legacy_prefix="$test_root/legacy-prefix"
mkdir -p "$legacy_prefix"
printf '#!/bin/sh\nlegacy-alias\n' >"$legacy_prefix/jacu"
chmod 0755 "$legacy_prefix/jacu"
PATH="$fakebin:$PATH" JACU_RELEASE_BASE_URL="file://$release" bash scripts/install.sh --version "$version" --prefix "$legacy_prefix"
if [ "$(cat "$legacy_prefix/jacu")" != '#!/bin/sh
old' ]; then
  echo "release test: legacy-alias install did not write the new jacu" >&2
  exit 1
fi
if [ ! -L "$legacy_prefix/jacu" ] || [ "$(readlink "$legacy_prefix/jacu")" != jacu ]; then
  echo "release test: regular-file jacu was not replaced by a symlink" >&2
  exit 1
fi
if [ "$(cat "$legacy_prefix/jacu.previous")" != '#!/bin/sh
legacy-alias' ]; then
  echo "release test: regular-file jacu was not preserved as jacu.previous" >&2
  exit 1
fi
PATH="$fakebin:$PATH" JACU_RELEASE_BASE_URL="file://$release" bash scripts/install.sh --rollback --prefix "$legacy_prefix"
if [ "$(cat "$legacy_prefix/jacu")" != '#!/bin/sh
legacy-alias' ]; then
  echo "release test: rollback did not restore the preserved alias bytes" >&2
  exit 1
fi
if [ ! -L "$legacy_prefix/jacu" ] || [ "$(readlink "$legacy_prefix/jacu")" != jacu ]; then
  echo "release test: rollback left jacu as a regular file" >&2
  exit 1
fi

PATH="$fakebin:$PATH" JACU_RELEASE_BASE_URL="file://$release" bash scripts/install.sh --version "$version" --prefix "$prefix"
if [ "$(cat "$prefix/jacu")" != '#!/bin/sh
old' ]; then
  echo "release test: initial install content mismatch" >&2
  exit 1
fi
if [ ! -L "$prefix/jacu" ] || [ "$(readlink "$prefix/jacu")" != jacu ]; then
  echo "release test: missing jacu compatibility symlink" >&2
  exit 1
fi

printf '#!/bin/sh\nnew\n' >"$test_root/stage/jacu"
tar -czf "$release/$asset" -C "$test_root/stage" jacu
(cd "$release" && shasum -a 256 "$asset" > checksums.txt)
PATH="$fakebin:$PATH" JACU_RELEASE_BASE_URL="file://$release" bash scripts/install.sh --version "$version" --prefix "$prefix"
PATH="$fakebin:$PATH" JACU_RELEASE_BASE_URL="file://$release" bash scripts/install.sh --rollback --prefix "$prefix"
if [ "$(cat "$prefix/jacu")" != '#!/bin/sh
old' ]; then
  echo "release test: rollback did not restore the previous binary" >&2
  exit 1
fi

printf '#!/bin/sh\ntampered\n' >"$test_root/stage/jacu"
tar -czf "$release/$asset" -C "$test_root/stage" jacu
if PATH="$fakebin:$PATH" JACU_RELEASE_BASE_URL="file://$release" bash scripts/install.sh --version "$version" --prefix "$prefix"; then
  echo "release test: tampered archive was accepted" >&2
  exit 1
fi
if [ "$(cat "$prefix/jacu")" != '#!/bin/sh
old' ]; then
  echo "release test: tamper changed the installed binary" >&2
  exit 1
fi

bash scripts/install.sh --dry-run --rollback --prefix "$prefix" >"$test_root/rollback-dry-run.log"
echo "release-test: OK"
