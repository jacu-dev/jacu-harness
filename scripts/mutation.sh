#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

readonly -a allowed_packages=(
  "internal/capability/missioncompile"
  "internal/runtime"
  "internal/runstate"
)

packages=("${allowed_packages[@]}")
if [ "$#" -gt 0 ]; then
  packages=("$@")
fi
for package in "${packages[@]}"; do
  valid=false
  for allowed in "${allowed_packages[@]}"; do
    if [ "$package" = "$allowed" ]; then
      valid=true
      break
    fi
  done
  if [ "$valid" != true ]; then
    echo "mutation: package is outside the allowlist: $package" >&2
    exit 2
  fi
done

echo "mutation: baseline packages: ${packages[*]}"
for package in "${packages[@]}"; do
  go test -count=1 "./$package"
done

mutation_root=$(mktemp -d "${TMPDIR:-/tmp}/jacu-mutation.XXXXXX")
mutation_log="$mutation_root/mutant.log"
trap 'rm -rf "$mutation_root"' EXIT

git archive --format=tar HEAD | tar -x -C "$mutation_root"
target="$mutation_root/internal/runstate/state.go"
if [ "$(grep -F -c 'len("run_")+16' "$target")" -ne 1 ]; then
  echo "mutation: expected exactly one run-id length target" >&2
  exit 1
fi

perl -0pi -e 's/len\("run_"\)\+16/len("run_")+15/' "$target"
if (cd "$mutation_root" && go test -count=1 ./internal/runstate) >"$mutation_log" 2>&1; then
  echo "mutation: mutant survived (run-id length off-by-one)" >&2
  exit 1
fi
if ! grep -q "TestLoadValidatesRunIDAndStoredIdentity" "$mutation_log"; then
  echo "mutation: runstate test failure was unrelated to the targeted mutant" >&2
  sed -n '1,120p' "$mutation_log" >&2
  exit 1
fi

echo "mutation: killed real mutant in runstate.ValidRunID"
echo "mutation: PASS (1 killed mutant)"
