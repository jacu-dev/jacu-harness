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

# Cursor cloud agents read .cursor/environment.json, not install.sh. Both entry
# points have to stay wired to the same bootstrap, or a cloud session silently
# boots without jacu while the eval still passes on the unused wrapper.
if [ ! -f "$root/.cursor/environment.json" ]; then
  echo "cloud-install-eval: .cursor/environment.json is missing" >&2
  exit 1
fi
for key in install start; do
  if ! grep -q "\"$key\"" "$root/.cursor/environment.json"; then
    echo "cloud-install-eval: .cursor/environment.json must define \"$key\"" >&2
    exit 1
  fi
done
if ! grep -q 'scripts/dev-setup.sh' "$root/.cursor/environment.json"; then
  echo "cloud-install-eval: .cursor/environment.json must call scripts/dev-setup.sh" >&2
  exit 1
fi
if ! python3 -c 'import json,sys; json.load(open(sys.argv[1]))' "$root/.cursor/environment.json" 2>/dev/null; then
  echo "cloud-install-eval: .cursor/environment.json is not valid JSON" >&2
  exit 1
fi
if [ ! -f "$root/scripts/dev-setup.sh" ]; then
  echo "cloud-install-eval: scripts/dev-setup.sh is missing" >&2
  exit 1
fi

# The image phase must not abort a session: on Claude Code a non-zero setup
# script means the VM never starts. Prove it survives a broken toolchain.
broken_bin="$test_root/broken"
mkdir -p "$broken_bin"
printf '#!/bin/sh\nexit 1\n' >"$broken_bin/go"
chmod 0755 "$broken_bin/go"
if ! PATH="$broken_bin:$PATH" JACU_INSTALL_PREFIX="$test_root/degraded" \
    bash "$root/scripts/dev-setup.sh" --phase image >/dev/null 2>&1; then
  echo "cloud-install-eval: dev-setup.sh --phase image must exit 0 even when a tool fails" >&2
  exit 1
fi

# --if-remote has to be inert on the developer's Mac, or committing
# .claude/settings.json would change every local session.
if [ "$(uname -s)" = Darwin ]; then
  out="$(CLAUDE_CODE_REMOTE= CURSOR_AGENT= bash "$root/scripts/dev-setup.sh" --phase session --if-remote 2>&1)"
  if [ -n "$out" ]; then
    echo "cloud-install-eval: --if-remote must be a no-op on Darwin, got: $out" >&2
    exit 1
  fi
fi

prefix="$test_root/prefix"
JACU_INSTALL_PREFIX="$prefix" bash "$root/.cursor/install.sh" --from-source --prefix "$prefix"
if [ ! -f "$prefix/jacu" ] || [ -L "$prefix/jacu" ]; then
  echo "cloud-install-eval: from-source did not install a regular-file jacu" >&2
  exit 1
fi
if [ -e "$prefix/jacu-mcp" ] || [ -L "$prefix/jacu-mcp" ]; then
  echo "cloud-install-eval: install created a retired jacu-mcp entry" >&2
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
