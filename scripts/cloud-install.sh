#!/usr/bin/env bash
# Generic cloud/VM bootstrap for JACU. The product is a stdio MCP server: it
# must be installed on the machine that holds the repository it governs.
# Hosts (Claude Code, Codex, Cursor, OpenCode, or any stdio client) then
# register `jacu serve`. This script never talks to a host API.
#
#   cloud-install.sh                  build from this checkout (default)
#   cloud-install.sh --from-source    same
#   cloud-install.sh --version vX.Y.Z verified release install (github.com)
set -euo pipefail

from_source=true
version=""
prefix="${JACU_INSTALL_PREFIX:-}"

usage() {
  cat >&2 <<'EOF'
usage: cloud-install.sh [--from-source] [--version vX.Y.Z] [--prefix DIR]
EOF
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --from-source)
      from_source=true
      shift
      ;;
    --version)
      [ "$#" -ge 2 ] || { usage; exit 2; }
      version="$2"
      from_source=false
      shift 2
      ;;
    --prefix)
      [ "$#" -ge 2 ] || { usage; exit 2; }
      prefix="$2"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "cloud-install.sh: unknown option $1" >&2
      usage
      exit 2
      ;;
  esac
done

root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$root"


install_binary() {
  src="$1"
  if [ -n "$prefix" ]; then
    mkdir -p "$prefix"
    install -m 0755 "$src" "$prefix/jacu"
    echo "$prefix/jacu"
    return
  fi
  install_dir="/usr/local/bin"
  if [ -w "$install_dir" ]; then
    install -m 0755 "$src" "$install_dir/jacu"
  elif sudo -n true 2>/dev/null; then
    sudo install -m 0755 "$src" "$install_dir/jacu"
  else
    install_dir="${HOME}/.local/bin"
    mkdir -p "$install_dir"
    install -m 0755 "$src" "$install_dir/jacu"
    echo "WARN: installed jacu to $install_dir; ensure it is on PATH" >&2
  fi
  echo "$install_dir/jacu"
}

if [ "$from_source" = false ]; then
  args=(--version "$version")
  if [ -n "$prefix" ]; then
    args+=(--prefix "$prefix")
  fi
  bash "$root/scripts/install.sh" "${args[@]}"
  command -v jacu >/dev/null 2>&1 && jacu doctor || true
  exit 0
fi

go mod download
built_version="$(git describe --tags --always --dirty 2>/dev/null || echo dev)"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
CGO_ENABLED=0 go build -trimpath -buildvcs=false \
  -ldflags "-s -w -X main.Version=${built_version}" \
  -o "$tmp/jacu" ./cmd/jacu
installed="$(install_binary "$tmp/jacu")"
if command -v jacu >/dev/null 2>&1; then
  jacu doctor
else
  "$installed" doctor
fi
