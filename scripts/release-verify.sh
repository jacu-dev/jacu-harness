#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."
version="${1:-}"
if [[ ! "$version" =~ ^v[0-9]+\.[0-9]+\.[0-9]+([-.][0-9A-Za-z.-]+)?$ ]]; then
  echo "release-verify: expected semver tag vX.Y.Z" >&2
  exit 2
fi

out="$(mktemp -d)"
trap 'rm -rf "$out"' EXIT
targets=("darwin/amd64" "darwin/arm64" "linux/amd64" "linux/arm64")
for target in "${targets[@]}"; do
  IFS=/ read -r goos goarch <<<"$target"
  name="${goos}_${goarch}"
  first="$out/${name}.one"
  second="$out/${name}.two"
  for output in "$first" "$second"; do
    CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" go build \
      -trimpath -buildvcs=false -ldflags "-s -w -X main.Version=$version" \
      -o "$output" ./cmd/jacu
  done
  if ! cmp -s "$first" "$second"; then
    echo "release-verify: non-reproducible build for $target" >&2
    exit 1
  fi
  echo "release-verify: $target $(shasum -a 256 "$first" | awk '{print $1}')"
done

reported="$(go run -ldflags "-X main.Version=$version" ./cmd/jacu version)"
if [ "$reported" != "jacu $version" ]; then
  echo "release-verify: version output $reported; want jacu $version" >&2
  exit 1
fi
echo "release-verify: OK"
