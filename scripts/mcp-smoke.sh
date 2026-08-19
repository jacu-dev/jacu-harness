#!/usr/bin/env bash
# Cheapest MCP smoke (heritage 10): pipe one initialize into the binary and
# assert EXACTLY one JSON line on stdout. Catches protocol breaks and any
# log leaking on stdout that a unit test would miss.
set -euo pipefail
cd "$(dirname "$0")/.."

BIN="${1:-./bin/jacu-smoke}"
if [ ! -x "$BIN" ]; then
  go build -o "$BIN" ./cmd/jacu
fi

REQ='{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2026-07-28","capabilities":{},"clientInfo":{"name":"smoke","version":"0"}}}'

# stdin must stay open until the response arrives: an immediate EOF tears
# down the STDIO transport before it writes the line. Send, hold the pipe,
# and read stdout line by line, leaving on the first.
OUT="$( { printf '%s\n' "$REQ"; sleep 5; } | timeout 15 "$BIN" serve 2>/dev/null | head -n 1 || true )"

if [ -z "$OUT" ]; then
  echo "mcp-smoke: no line on stdout (expected exactly 1)" >&2
  exit 1
fi
case "$OUT" in
  *'"result"'*'"protocolVersion"'*) ;;
  *) echo "mcp-smoke: invalid initialize response" >&2; printf '%s\n' "$OUT" >&2; exit 1 ;;
esac

# A second line means a log leaked onto the protocol channel.
SECOND="$( { printf '%s\n' "$REQ"; sleep 5; } | timeout 15 "$BIN" serve 2>/dev/null | sed -n '2p' || true )"
if [ -n "$SECOND" ]; then
  echo "mcp-smoke: more than 1 line on stdout — log leaking onto the protocol" >&2
  printf '%s\n' "$SECOND" >&2
  exit 1
fi
echo "mcp-smoke: OK"
