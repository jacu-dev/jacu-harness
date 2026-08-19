#!/usr/bin/env bash
# Scripted cloud/VM bootstrap matrix. Proves the Cursor wrapper, from-source
# install, and that a failed release fetch names the unreachable host.
set -euo pipefail
cd "$(dirname "$0")/.."

root="$(pwd)"
test_root="$(mktemp -d)"
trap 'rm -rf "$test_root"' EXIT

if [ ! -x "$root/.cursor/install.sh" ]; then
  echo "cloud-install-eval: .cursor/install.sh is missing or not executable" >&2
  exit 1
fi
if ! grep -q 'scripts/cloud-install.sh' "$root/.cursor/install.sh"; then
  echo "cloud-install-eval: .cursor/install.sh must wrap scripts/cloud-install.sh" >&2
  exit 1
fi

prefix="$test_root/prefix"
JACU_INSTALL_PREFIX="$prefix" bash "$root/.cursor/install.sh" --from-source --prefix "$prefix"
if [ ! -f "$prefix/jacu" ] || [ -L "$prefix/jacu" ]; then
  echo "cloud-install-eval: from-source did not install a regular-file jacu" >&2
  exit 1
fi
if [ ! -L "$prefix/jacu-mcp" ] || [ "$(readlink "$prefix/jacu-mcp")" != jacu ]; then
  echo "cloud-install-eval: missing jacu-mcp compatibility symlink" >&2
  exit 1
fi
if ! "$prefix/jacu" version >/dev/null; then
  echo "cloud-install-eval: installed binary did not run" >&2
  exit 1
fi

fakebin="$test_root/bin"
mkdir -p "$fakebin"
printf '#!/bin/sh\n[ "$1" = verify-blob ]\n' >"$fakebin/cosign"
chmod 0755 "$fakebin/cosign"
fail_log="$test_root/fail.log"
if PATH="$fakebin:$PATH" JACU_RELEASE_BASE_URL="https://jacu-unreachable.example/v0.0.0" bash "$root/scripts/cloud-install.sh" --version v0.0.0 --prefix "$test_root/blocked" 2>"$fail_log"; then
  echo "cloud-install-eval: release mode succeeded against an unreachable host" >&2
  exit 1
fi
if ! grep -q 'unreachable host: jacu-unreachable.example' "$fail_log"; then
  echo "cloud-install-eval: failure must name the unreachable host" >&2
  cat "$fail_log" >&2
  exit 1
fi

echo "cloud-install-eval: OK"
