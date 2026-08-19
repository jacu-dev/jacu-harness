#!/usr/bin/env bash
# Drives the built binary the way a host does: real process, real stdio, real
# JSON-RPC. Separate from verify.sh because it builds and spawns; keep verify
# fast enough to run on every save.
#
# No -e: the summary below has to be written whether the suite passed or not,
# and a failing run is exactly when the measured numbers are worth reading.
set -uo pipefail
cd "$(dirname "$0")/.."

log=$(mktemp)
trap 'rm -f "$log"' EXIT

go test -tags e2e -count=1 "$@" ./test/e2e/... 2>&1 | tee "$log"
status=${PIPESTATUS[0]}

# The suite reports the MCP surface size, cold start, and operation percentiles through
# the test log (with -v). Lift them into the Actions job summary so the numbers
# land on the pull request instead of inside a log nobody opens.
if [ -n "${GITHUB_STEP_SUMMARY:-}" ]; then
  {
    echo "### jacu e2e"
    grep -oE '(MCP surface|cold start|operation [a-z_]+): .*' "$log" | sed 's/^/- /' || echo "- (no measurements in the log; rerun with -v)"
  } >>"$GITHUB_STEP_SUMMARY"
fi

if [ "$status" -ne 0 ]; then
  exit "$status"
fi
echo "e2e: OK"
