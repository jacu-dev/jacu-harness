#!/usr/bin/env bash
# Smoke the published GitHub Release the way a stranger would: download
# install.sh to a file, then run it into a throwaway prefix. This is not a
# pull-request gate and must not become one.
set -euo pipefail

installer_url="${JACU_SMOKE_INSTALLER_URL:-https://github.com/jacu-dev/jacu-harness/releases/latest/download/install.sh}"

for command_name in curl cosign tar shasum; do
  command -v "$command_name" >/dev/null || {
    echo "install-smoke: required command missing: $command_name" >&2
    exit 1
  }
done

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

curl -fsSL -o "$work/install.sh" "$installer_url"
if [ ! -s "$work/install.sh" ] || ! grep -q '^#!/usr/bin/env bash' "$work/install.sh"; then
  echo "install-smoke: downloaded installer is not the jacu install script" >&2
  exit 1
fi

bash "$work/install.sh" --prefix "$work/bin"
if [ ! -x "$work/bin/jacu" ] || [ -L "$work/bin/jacu" ]; then
  echo "install-smoke: prefix does not contain a regular jacu binary" >&2
  exit 1
fi
if [ ! -L "$work/bin/jacu-mcp" ] || [ "$(readlink "$work/bin/jacu-mcp")" != jacu ]; then
  echo "install-smoke: missing jacu-mcp compatibility symlink" >&2
  exit 1
fi

version="$("$work/bin/jacu" version)"
case "$version" in
  jacu\ [0-9]*) ;;
  *)
    echo "install-smoke: unexpected version output: $version" >&2
    exit 1
    ;;
esac
echo "install-smoke: OK ($version)"
