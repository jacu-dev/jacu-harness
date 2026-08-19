#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
gofmt -l . | grep -v '^$' && { echo "gofmt: arquivos acima não formatados"; exit 1; } || true
go vet ./...
go test -race ./...
go test -race ./internal/capability/cleanexit
go build ./...
git diff --check
go run ./cmd/jacu sdd lint --all
echo "verify: OK"
