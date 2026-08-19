#!/usr/bin/env bash
# Thin Cursor VM wrapper around the generic cloud bootstrap.
set -euo pipefail
root="$(cd "$(dirname "$0")/.." && pwd)"
exec bash "$root/scripts/cloud-install.sh" "$@"
