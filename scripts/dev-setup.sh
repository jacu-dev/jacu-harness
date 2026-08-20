#!/usr/bin/env bash
# Dev-environment bootstrap for agent VMs (Claude Code on the web, Cursor cloud
# agents) and for any clean Linux checkout.
#
# It reads no vault, decrypts nothing, and needs no credential. Everything it
# touches is either a public toolchain or this checkout. That is the whole
# design: a cloud session never holds a secret, so there is nothing to leak.
#
#   dev-setup.sh --phase image      persists to disk; runs once per snapshot
#   dev-setup.sh --phase session    runs every session; cheap and idempotent
#   dev-setup.sh --phase all        both (default)
#   dev-setup.sh --if-remote        no-op unless this is an agent VM
#
# The two phases exist because both platforms have the same shape: a build step
# that is snapshotted (Claude's setup script, Cursor's `install`) and a start
# step that runs per session (SessionStart hook, Cursor's `start`). The image
# phase must never exit non-zero: on Claude Code a failing setup script means
# the session does not start at all, so failures are recorded and surfaced by
# the session phase instead of aborting.
set -uo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$root"

phase=all
if_remote=false
while [ "$#" -gt 0 ]; do
  case "$1" in
    --phase)
      [ "$#" -ge 2 ] || { echo "dev-setup.sh: --phase needs a value" >&2; exit 2; }
      phase="$2"
      shift 2
      ;;
    --if-remote)
      if_remote=true
      shift
      ;;
    -h|--help)
      sed -n '2,18p' "$0" >&2
      exit 0
      ;;
    *)
      echo "dev-setup.sh: unknown option $1" >&2
      exit 2
      ;;
  esac
done

case "$phase" in
  image|session|all) ;;
  *) echo "dev-setup.sh: --phase must be image, session, or all" >&2; exit 2 ;;
esac

# An agent VM is the negative case: anything that is not this developer's Mac.
# CLAUDE_CODE_REMOTE and CURSOR_AGENT are explicit confirmations, but the guard
# has to hold on a platform that ships neither, so Darwin is the real test.
is_remote() {
  [ "${CLAUDE_CODE_REMOTE:-}" = true ] && return 0
  [ -n "${CURSOR_AGENT:-}" ] && return 0
  [ "$(uname -s)" != Darwin ]
}

if [ "$if_remote" = true ] && ! is_remote; then
  exit 0
fi

state_dir="${XDG_STATE_HOME:-$HOME/.local/state}/jacu-dev-setup"
mkdir -p "$state_dir" 2>/dev/null || true
stamp="$state_dir/image-version"
failures="$state_dir/image-failures"
self_hash="$(shasum -a 256 "$0" 2>/dev/null | cut -d' ' -f1)"

note_failure() {
  echo "$1" >>"$failures"
  echo "dev-setup: $1" >&2
}

phase_image() {
  : >"$failures"
  echo "dev-setup: image phase"

  if command -v go >/dev/null 2>&1; then
    go mod download || note_failure "go mod download failed; check network allowlist for proxy.golang.org"
  else
    note_failure "go toolchain missing; install Go and re-run scripts/dev-setup.sh --phase image"
  fi

  # Install the jacu binary from this checkout. Cloud sessions register
  # `jacu serve` themselves; this script never touches a host config.
  if [ -x "$root/scripts/cloud-install.sh" ]; then
    bash "$root/scripts/cloud-install.sh" --from-source \
      || note_failure "cloud-install.sh failed; the MCP server will be unavailable this session"
  else
    note_failure "scripts/cloud-install.sh missing"
  fi

  echo "$self_hash" >"$stamp" 2>/dev/null || true
  echo "dev-setup: image phase done"
  # Never propagate failure: see the header.
  return 0
}

phase_session() {
  echo "dev-setup: session phase"

  # The Claude Code environment cache is a filesystem snapshot that survives
  # edits to this file, so a session can boot on a stale image phase. Compare
  # hashes and self-heal, otherwise "I edited dev-setup.sh and nothing changed"
  # costs an afternoon.
  if [ ! -f "$stamp" ] || [ "$(cat "$stamp" 2>/dev/null)" != "$self_hash" ]; then
    echo "dev-setup: image phase is stale or missing, re-running it"
    phase_image
  fi

  if [ -s "$failures" ]; then
    echo "dev-setup: the image phase reported problems:"
    sed 's/^/  - /' "$failures"
  fi

  if command -v jacu >/dev/null 2>&1; then
    jacu doctor || true
  elif [ -x "${JACU_INSTALL_PREFIX:-$HOME/.local/bin}/jacu" ]; then
    "${JACU_INSTALL_PREFIX:-$HOME/.local/bin}/jacu" doctor || true
  else
    echo "dev-setup: jacu is not on PATH; run scripts/dev-setup.sh --phase image" >&2
  fi

  echo "dev-setup: ready — build with 'go build ./cmd/jacu', verify with 'scripts/verify.sh'"
  return 0
}

case "$phase" in
  image)   phase_image ;;
  session) phase_session ;;
  all)     phase_image; phase_session ;;
esac
