#!/usr/bin/env bash
# Smoke MCP mais barato que existe (herança 10): pipe um initialize no binário
# e asserte EXATAMENTE uma linha JSON no stdout. Pega quebra de protocolo e
# qualquer log vazando no stdout que teste unitário não vê.
set -euo pipefail
cd "$(dirname "$0")/.."

BIN="${1:-./bin/jacu-smoke}"
if [ ! -x "$BIN" ]; then
  go build -o "$BIN" ./cmd/jacu
fi

REQ='{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2026-07-28","capabilities":{},"clientInfo":{"name":"smoke","version":"0"}}}'

# stdin precisa ficar aberto até a resposta chegar: EOF imediato derruba o
# transporte STDIO antes de ele escrever a linha. Envia, segura o pipe, e lê
# o stdout linha a linha saindo na primeira.
OUT="$( { printf '%s\n' "$REQ"; sleep 5; } | timeout 15 "$BIN" serve 2>/dev/null | head -n 1 || true )"

if [ -z "$OUT" ]; then
  echo "mcp-smoke: nenhuma linha no stdout (esperada exatamente 1)" >&2
  exit 1
fi
case "$OUT" in
  *'"result"'*'"protocolVersion"'*) ;;
  *) echo "mcp-smoke: resposta de initialize inválida" >&2; printf '%s\n' "$OUT" >&2; exit 1 ;;
esac

# Não pode haver uma segunda linha (log vazando no canal do protocolo).
SECOND="$( { printf '%s\n' "$REQ"; sleep 5; } | timeout 15 "$BIN" serve 2>/dev/null | sed -n '2p' || true )"
if [ -n "$SECOND" ]; then
  echo "mcp-smoke: mais de 1 linha no stdout — log vazando no protocolo" >&2
  printf '%s\n' "$SECOND" >&2
  exit 1
fi
echo "mcp-smoke: OK"
