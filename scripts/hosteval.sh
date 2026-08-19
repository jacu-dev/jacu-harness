#!/usr/bin/env bash
# Host routing eval — drives real coding CLIs and judges by jacu's own
# telemetry stream, never by scraping host transcripts.
#
# This is NOT a CI job and must never become one: it needs host credentials and
# it spends money per run. It is a local command whose output is pasted into
# scripts/host-smoke/README.md as the evidence sheet.
#
#   scripts/hosteval.sh                          # every host, every case
#   scripts/hosteval.sh --host codex             # one host
#   scripts/hosteval.sh --case 4.1-inspect       # one case
#   scripts/hosteval.sh --host codex --case 4.1-inspect
#
# Live hosts also need JACU_HOSTEVAL_PROJECT_ID. The catalogue assert below
# does not: a filter that matches nothing would make every routing case look
# like "no tools called", which is a silently passing 4.4.
set -euo pipefail

hosts=""
cases=""
while [ $# -gt 0 ]; do
  case "$1" in
    --host) hosts="${2:?--host needs a value}"; shift 2 ;;
    --case) cases="${2:?--case needs a value}"; shift 2 ;;
    -h|--help) sed -n '2,20p' "$0"; exit 0 ;;
    *) echo "unknown flag: $1" >&2; exit 2 ;;
  esac
done

# Catalogue fidelity is a machine assert and does not need a live host.
go test -count=1 ./test/hosteval/ -run 'TestCatalogue|TestTruncating|TestEmptyHost'

if [ -z "${JACU_HOSTEVAL_PROJECT_ID:-}" ]; then
  echo "hosteval: catalogue assert OK; set JACU_HOSTEVAL_PROJECT_ID to drive live hosts"
  exit 0
fi

export JACU_HOSTEVAL=1
export JACU_HOSTEVAL_HOSTS="$hosts"
export JACU_HOSTEVAL_CASES="$cases"

go test -tags=hosteval -count=1 -v -timeout 60m ./test/hosteval/ -run TestMatrix
