#!/usr/bin/env bash
# The lane a prose-only pull request runs instead of scripts/verify.sh.
#
# It skips what only code can break: the race detector, the release and install
# rehearsals, and the linters that read Go. It does NOT skip the checks that
# prose alone can break, because in this repository nearly every document is
# read by a test — living docs (cmd/jacu/rename_test.go), the skill catalogue
# (internal/mcpadapter/skills_test.go), the provenance allowlist, the memory
# bridge, and `sdd lint --all` over every SDD and its lock.
#
# Running the whole test binary set, rather than a hand-picked list, is
# deliberate: a document check added tomorrow is covered without editing this
# file.
set -euo pipefail
cd "$(dirname "$0")/.."

git diff --check
go test ./...
go run ./cmd/jacu sdd lint --all
echo "verify-prose: OK"
