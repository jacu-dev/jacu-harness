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

# Approve this repository's .mcp.json for the session's user.
#
# A cloned repository cannot approve its own MCP servers: since Claude Code
# v2.1.196, `enabledMcpjsonServers` committed to .claude/settings.json is ignored
# in an untrusted folder, and the server sits at "Pending approval" forever. In a
# cloud VM every checkout is untrusted and nobody is there to click the trust
# dialog, so a committed .mcp.json alone would never connect.
#
# Approvals from ~/.claude/settings.json do apply in an untrusted folder, and the
# image phase runs before Claude Code starts. Writing the approval there is the
# only path that makes `jacu serve` reachable in a cloud session.
#
# Deliberately narrow: enables the `jacu` server by name, never
# enableAllProjectMcpServers, so an unrelated .mcp.json in some other checkout
# does not get auto-approved as a side effect.
approve_project_mcp() {
  if ! is_remote; then
    return 0
  fi
  settings="$HOME/.claude/settings.json"
  mkdir -p "$(dirname "$settings")" 2>/dev/null || {
    note_failure "could not create $(dirname "$settings"); jacu will need manual approval"
    return 0
  }
  if ! command -v python3 >/dev/null 2>&1; then
    note_failure "python3 missing; cannot approve the jacu MCP server automatically"
    return 0
  fi
  SETTINGS_PATH="$settings" python3 - <<'PY' || note_failure "could not write the MCP approval to ~/.claude/settings.json"
import json, os, pathlib

path = pathlib.Path(os.environ["SETTINGS_PATH"])
data = {}
if path.exists():
    try:
        data = json.loads(path.read_text() or "{}")
    except json.JSONDecodeError:
        # Never clobber a settings file we cannot parse.
        raise SystemExit("dev-setup: ~/.claude/settings.json is not valid JSON; leaving it alone")
if not isinstance(data, dict):
    raise SystemExit("dev-setup: ~/.claude/settings.json is not a JSON object; leaving it alone")

enabled = data.get("enabledMcpjsonServers")
if not isinstance(enabled, list):
    enabled = []
if "jacu" not in enabled:
    enabled.append("jacu")
data["enabledMcpjsonServers"] = enabled

path.write_text(json.dumps(data, indent=2) + "\n")
print("dev-setup: approved the jacu MCP server in", path)
PY
}

# Cursor Cloud launches stdio servers from ~/.cursor/mcp.json. A leftover
# `jacu-mcp` command (retired in 0.3.0) becomes spawn ENOENT, so the image
# phase rewrites that host pack to `jacu serve` without dropping siblings.
repair_cursor_mcp() {
  if ! is_remote; then
    return 0
  fi
  config="$HOME/.cursor/mcp.json"
  mkdir -p "$(dirname "$config")" 2>/dev/null || {
    note_failure "could not create $(dirname "$config"); Cursor will keep launching whatever host pack it already has"
    return 0
  }
  if ! command -v python3 >/dev/null 2>&1; then
    note_failure "python3 missing; cannot repair ~/.cursor/mcp.json"
    return 0
  fi
  CONFIG_PATH="$config" python3 - <<'PY' || note_failure "could not repair ~/.cursor/mcp.json"
import json, os, pathlib

path = pathlib.Path(os.environ["CONFIG_PATH"])
canonical = {"command": "jacu", "args": ["serve"]}
data = {}
if path.exists():
    try:
        data = json.loads(path.read_text() or "{}")
    except json.JSONDecodeError:
        raise SystemExit("dev-setup: ~/.cursor/mcp.json is not valid JSON; leaving it alone")
if not isinstance(data, dict):
    raise SystemExit("dev-setup: ~/.cursor/mcp.json is not a JSON object; leaving it alone")

servers = data.get("mcpServers")
if not isinstance(servers, dict):
    servers = {}
    data["mcpServers"] = servers

def launches_serve(entry):
    return isinstance(entry, dict) and entry.get("command") == "jacu" and entry.get("args") == ["serve"]

def is_retired(entry):
    if not isinstance(entry, dict):
        return False
    command = entry.get("command")
    if command == "jacu-mcp":
        return True
    if isinstance(command, list) and command and command[0] == "jacu-mcp":
        return True
    return False

changed = False
if "jacu-mcp" in servers:
    current = servers.get("jacu")
    if current is not None and not launches_serve(current) and not is_retired(current):
        raise SystemExit("dev-setup: ~/.cursor/mcp.json already registers a different jacu server; leaving it alone")
    del servers["jacu-mcp"]
    changed = True

current = servers.get("jacu")
if current is None or is_retired(current):
    servers["jacu"] = canonical
    changed = True
elif not launches_serve(current):
    raise SystemExit("dev-setup: ~/.cursor/mcp.json already registers a different jacu server; leaving it alone")

if changed or not path.exists():
    path.write_text(json.dumps(data, indent=2) + "\n")
    print("dev-setup: wrote jacu serve into", path)
PY
}

phase_image() {
  : >"$failures"
  echo "dev-setup: image phase"

  if command -v go >/dev/null 2>&1; then
    go mod download || note_failure "go mod download failed; check network allowlist for proxy.golang.org"
  else
    note_failure "go toolchain missing; install Go and re-run scripts/dev-setup.sh --phase image"
  fi

  # Install the jacu binary from this checkout.
  if [ -x "$root/scripts/cloud-install.sh" ]; then
    bash "$root/scripts/cloud-install.sh" --from-source \
      || note_failure "cloud-install.sh failed; the MCP server will be unavailable this session"
  else
    note_failure "scripts/cloud-install.sh missing"
  fi

  approve_project_mcp
  repair_cursor_mcp

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

  # The MCP server is the product. A session where `jacu` is not on PATH gets a
  # host that silently has no jacu tools, so say so in the session's own output
  # rather than letting the agent discover it by the tools being absent.
  if command -v jacu >/dev/null 2>&1; then
    jacu doctor || true
  else
    fallback="${JACU_INSTALL_PREFIX:-$HOME/.local/bin}/jacu"
    if [ -x "$fallback" ]; then
      "$fallback" doctor || true
      echo "dev-setup: jacu is installed at $fallback but not on PATH." >&2
      echo "dev-setup: the host launches \`jacu serve\` by name, so add that directory to PATH." >&2
    else
      echo "dev-setup: jacu is not installed; the jacu MCP tools will be unavailable." >&2
      echo "dev-setup: re-run scripts/dev-setup.sh --phase image and read the errors above." >&2
    fi
  fi

  if is_remote; then
    approved=no
    if [ -f "$HOME/.claude/settings.json" ] \
      && grep -q '"jacu"' "$HOME/.claude/settings.json" 2>/dev/null; then
      approved=yes
    fi
    echo "dev-setup: project MCP approval in ~/.claude/settings.json: $approved"
    if [ "$approved" = no ]; then
      echo "dev-setup: .mcp.json will sit at 'Pending approval' — a cloned repo cannot approve itself." >&2
    fi
    cursor_pack=no
    if [ -f "$HOME/.cursor/mcp.json" ] \
      && python3 -c 'import json,sys; s=json.load(open(sys.argv[1]))["mcpServers"]["jacu"]; assert s["command"]=="jacu" and s["args"]==["serve"]' \
        "$HOME/.cursor/mcp.json" 2>/dev/null; then
      cursor_pack=yes
    fi
    echo "dev-setup: Cursor host pack in ~/.cursor/mcp.json: $cursor_pack"
  fi

  echo "dev-setup: ready — build with 'go build ./cmd/jacu', verify with 'scripts/verify.sh'"
  return 0
}

case "$phase" in
  image)   phase_image ;;
  session) phase_session ;;
  all)     phase_image; phase_session ;;
esac
