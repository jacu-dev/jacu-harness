#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."

assert_i4() {
  local leaks
  leaks="$(grep -RIn --include='*.go' --exclude='*_test.go' --exclude-dir=gitx --exclude-dir=testgit \
    -E 'exec\.Command(Context)?\([^)]*"git"' cmd internal || true)"
  if [[ -n "$leaks" ]]; then
    echo "I4: git exec outside internal/gitx:"
    echo "$leaks"
    return 1
  fi
}

# I6: non-test Go files may not grow past the current maximum. Decrease only.
I6_MAX_FILE_BYTES=26722

assert_i6() {
  local over
  over="$(find cmd internal -name '*.go' ! -name '*_test.go' -printf '%s %p\n' | awk -v max="$I6_MAX_FILE_BYTES" '$1 > max { print }')"
  if [[ -n "$over" ]]; then
    echo "I6: non-test file exceeds ${I6_MAX_FILE_BYTES} bytes:"
    echo "$over"
    return 1
  fi
}

seed_i6() {
  local seed="internal/capability/projectinspect/i6_seed_probe.go"
  trap 'rm -f "$seed"' RETURN
  { printf 'package projectinspect\n'; dd if=/dev/zero bs=1 count=$((I6_MAX_FILE_BYTES)) status=none; } >"$seed"
  if assert_i6 >/dev/null; then
    echo "I6 seed was not detected"
    return 1
  fi
}

assert_i5() {
  local leaks
  leaks="$(grep -RIn --include='*.go' --exclude='*_test.go' --exclude='tool.go' --exclude-dir=mcpadapter \
    'github.com/modelcontextprotocol/go-sdk' cmd internal || true)"
  if [[ -n "$leaks" ]]; then
    echo "I5: MCP SDK outside mcpadapter and tool.go:"
    echo "$leaks"
    return 1
  fi
}

seed_i4() {
  local seed="internal/capability/projectinspect/i4_seed_probe.go"
  trap 'rm -f "$seed"' RETURN
  printf '%s\n' 'package projectinspect' 'import "os/exec"' 'var _ = exec.Command("git", "--version")' >"$seed"
  if assert_i4 >/dev/null; then
    echo "I4 seed was not detected"
    return 1
  fi
}

seed_i5() {
  local seed="internal/capability/projectinspect/i5_seed_probe.go"
  trap 'rm -f "$seed"' RETURN
  printf '%s\n' 'package projectinspect' 'import _ "github.com/modelcontextprotocol/go-sdk/mcp"' >"$seed"
  if assert_i5 >/dev/null; then
    echo "I5 seed was not detected"
    return 1
  fi
}

gofmt -l . | grep -v '^$' && { echo "gofmt: files above are not formatted"; exit 1; } || true
go vet ./...
go test -race ./...
go test -race ./internal/capability/cleanexit
go build ./...
bash scripts/release-test.sh
bash scripts/cloud-install-eval.sh
git diff --check
go run ./cmd/jacu sdd lint --all
assert_i4
assert_i5
assert_i6
seed_i4
seed_i5
seed_i6
echo "verify: OK"
