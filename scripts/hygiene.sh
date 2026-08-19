#!/usr/bin/env bash
# Runs the phase-closing checklist of docs/hygiene.md by machine.
#
# Every item here used to be a human typing four commands at the end of a phase
# and pasting the output into a report. A checklist a machine can run is a
# checklist that actually runs.
set -uo pipefail
cd "$(dirname "$0")/.."

status=0
fail() {
  printf 'hygiene: %s\n' "$1" >&2
  status=1
}

# 1. Dependencies stay lean: `go mod tidy` must be a no-op.
if tidy_diff=$(go mod tidy -diff 2>&1); then
  :
else
  fail "go mod tidy -diff found drift — proposal (no files were changed):"
  printf '%s\n' "$tidy_diff" >&2
fi

# 2. No unannotated TODO/FIXME. docs/hygiene.md accepts them only with an issue
#    attached, so TODO(#12) passes and a bare TODO does not.
if bare_todos=$(grep -rnE '(TODO|FIXME)([^(]|$)' --include='*.go' . 2>/dev/null); then
  fail "TODO/FIXME without an annotated issue (use TODO(#123)):"
  printf '%s\n' "$bare_todos" >&2
fi

# 3. No build artifact ever gets committed — the lesson of the dist/ left behind
#    in jacu-code. These are the .gitignore patterns, checked against what is
#    actually tracked rather than against what happens to be in the working tree.
if tracked=$(git ls-files -- 'bin/**' 'dist/**' '*.test' 'coverage.out' '*.prof' | head -20); then
  if [ -n "$tracked" ]; then
    fail "build artifact tracked by git:"
    printf '%s\n' "$tracked" >&2
  fi
fi

# 4. No tracked file is an executable binary. Catches the artifact that slips in
#    under a name .gitignore never predicted.
while IFS= read -r file; do
  [ -f "$file" ] || continue
  case "$(head -c 4 "$file" | od -An -tx1 | tr -d ' \n')" in
    7f454c46 | cffaedfe | cefaedfe | feedfacf | feedface)
      fail "binary tracked by git: $file"
      ;;
  esac
done < <(git ls-files)

if [ "$status" -eq 0 ]; then
  echo "hygiene: OK"
fi
exit "$status"
