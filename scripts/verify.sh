#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
gofmt -l . | grep -v '^$' && { echo "gofmt: files above are not formatted"; exit 1; } || true
go vet ./...
go test -race ./...
go test -race ./internal/capability/cleanexit
go build ./...
bash scripts/release-test.sh
bash scripts/cloud-install-eval.sh
git diff --check
go run ./cmd/jacu sdd lint --all
echo "verify: OK"
