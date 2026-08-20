#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."

version="v0.0.0-release-test"
test_root="$(mktemp -d)"
prefix="$test_root/prefix"
release="$test_root/release"
fakebin="$test_root/bin"
trap 'rm -rf "$test_root"' EXIT

if grep -q -- '-R jacu-dev/jacu ' scripts/install.sh; then
  echo "release test: install.sh still targets the retired jacu-dev/jacu repo" >&2
  exit 1
fi
if ! grep -q 'jacu-dev/jacu-harness' scripts/install.sh; then
  echo "release test: install.sh must fetch from jacu-dev/jacu-harness" >&2
  exit 1
fi
if grep -Ehn '^\s*(curl|wget).+\|[[:space:]]*(ba)?sh' README.md docs/install.md docs/distribution.md scripts/install.sh; then
  echo "release test: living install docs must not teach curl|sh" >&2
  exit 1
fi
if ! grep -q 'brew install jacu-dev/jacu/jacu' README.md || ! grep -q 'brew install jacu-dev/jacu/jacu' docs/install.md; then
  echo "release test: living install docs must teach brew install jacu-dev/jacu/jacu" >&2
  exit 1
fi
if ! grep -q 'brew upgrade jacu' README.md || ! grep -q 'brew upgrade jacu' docs/install.md; then
  echo "release test: living install docs must teach brew upgrade jacu after the tap" >&2
  exit 1
fi
if ! grep -qx 'brew install jacu' README.md || ! grep -qx 'brew install jacu' docs/install.md; then
  echo "release test: living install docs must teach the short brew install jacu" >&2
  exit 1
fi
if ! grep -q '^brews:' .goreleaser.yaml || ! grep -q 'homebrew-jacu' .goreleaser.yaml; then
  echo "release test: GoReleaser must generate the Homebrew tap formula" >&2
  exit 1
fi
if ! grep -q 'scripts/install.sh' .goreleaser.yaml; then
  echo "release test: GoReleaser must attach scripts/install.sh to the release" >&2
  exit 1
fi
# The workflow delegates asset collection to a script so it can be tested; the
# behavioural proof is the layout matrix at the end of this file. Here we only
# assert the wiring, because a workflow that stops calling the script would make
# that matrix meaningless.
if ! grep -q 'scripts/collect-release-assets.sh' .github/workflows/release.yml; then
  echo "release test: release workflow must collect assets via scripts/collect-release-assets.sh" >&2
  exit 1
fi
if ! grep -q 'exclude-drafts' scripts/install.sh; then
  echo "release test: install.sh must ignore draft GitHub Releases" >&2
  exit 1
fi
if ! grep -q 'scripts/install\.sh' scripts/collect-release-assets.sh \
  || ! grep -q 'install\.sh' scripts/collect-release-assets.sh; then
  echo "release test: asset collection must publish install.sh as a release asset" >&2
  exit 1
fi
if ! grep -q -- '--draft=false' .github/workflows/release.yml; then
  echo "release test: release workflow must publish a non-draft GitHub Release" >&2
  exit 1
fi
if [ ! -x scripts/install-smoke.sh ]; then
  echo "release test: scripts/install-smoke.sh must exist and be executable" >&2
  exit 1
fi
if grep -Ehn '^\s*(curl|wget).+\|[[:space:]]*(ba)?sh' scripts/install-smoke.sh; then
  echo "release test: install-smoke must not use curl|sh" >&2
  exit 1
fi
if ! grep -q 'releases/latest/download/install.sh' scripts/install-smoke.sh; then
  echo "release test: install-smoke must fetch the published installer" >&2
  exit 1
fi
if grep -q 'scripts/install-smoke.sh' .github/workflows/ci.yml .github/workflows/verify.yml; then
  echo "release test: install-smoke must stay off the pull-request gate" >&2
  exit 1
fi
if ! grep -q 'scripts/install-smoke.sh' .github/workflows/weekly.yml; then
  echo "release test: weekly workflow must run the published-release install smoke" >&2
  exit 1
fi
if ! grep -q -- '-fuzztime=3m' .github/workflows/weekly.yml; then
  echo "release test: weekly fuzz must run 3m per target" >&2
  exit 1
fi
if ! grep -q 'macos-latest' .github/workflows/weekly.yml; then
  echo "release test: weekly workflow must run go test on macOS" >&2
  exit 1
fi
if grep -Ehn 'runs-on:.*(larger|xl|4-core|8-core|windows-)' .github/workflows/weekly.yml; then
  echo "release test: weekly workflow must stay on free standard runners" >&2
  exit 1
fi

formula="Formula/jacu.rb"
if [ ! -f "$formula" ]; then
  echo "release test: missing $formula" >&2
  exit 1
fi
for needle in \
  'class Jacu < Formula' \
  'license "MIT"' \
  'bin.install "jacu"'
do
  if ! grep -Fq "$needle" "$formula"; then
    echo "release test: $formula is missing: $needle" >&2
    exit 1
  fi
done

# The formula's own `version` is the single source of truth here. Asserting a
# literal number would mean editing this test on every release and would pass
# happily on a formula whose urls disagree with its version — which is the
# defect that actually ships a broken `brew install`.
formula_version="$(sed -n 's/^  version "\([^"]*\)".*/\1/p' "$formula" | head -n 1)"
if [ -z "$formula_version" ]; then
  echo "release test: $formula does not declare a version" >&2
  exit 1
fi
if ! printf '%s' "$formula_version" | grep -qE '^[0-9]+\.[0-9]+\.[0-9]+([-.][0-9A-Za-z.-]+)?$'; then
  echo "release test: $formula version is not semver: $formula_version" >&2
  exit 1
fi
for platform in darwin_amd64 darwin_arm64 linux_amd64 linux_arm64; do
  asset="jacu_${formula_version}_${platform}.tar.gz"
  if ! grep -Fq "$asset" "$formula"; then
    echo "release test: $formula is missing the $formula_version asset: $asset" >&2
    exit 1
  fi
done
# Every url must carry the declared version. One stale url is a 404 install,
# and it survives any check that only asks whether the right version appears
# *somewhere* in the file.
url_versions="$(grep -oE 'releases/download/v[^/]+/' "$formula" | sed 's|releases/download/v||; s|/$||' | sort -u)"
distinct="$(printf '%s\n' "$url_versions" | grep -c . || true)"
if [ "$distinct" -ne 1 ] || [ "$url_versions" != "$formula_version" ]; then
  echo "release test: $formula declares version $formula_version but its urls use:" >&2
  printf '%s\n' "$url_versions" | sed 's/^/  /' >&2
  exit 1
fi
if [ "$(grep -cE 'sha256 "[a-f0-9]{64}"' "$formula" || true)" -lt 4 ]; then
  echo "release test: $formula must pin sha256 for every platform tarball" >&2
  exit 1
fi
if [ "$(grep -oE 'sha256 "[a-f0-9]{64}"' "$formula" | sort -u | wc -l | tr -d ' ')" -lt 4 ]; then
  echo "release test: $formula reuses a sha256 across platforms" >&2
  exit 1
fi

bash scripts/release-verify.sh "$version"
if bash scripts/install.sh --dry-run --version "$version" --prefix "$prefix" >"$test_root/dry-run.log"; then
  :
else
  echo "release test: installer dry-run failed" >&2
  exit 1
fi
if [ -e "$prefix" ]; then
  echo "release test: dry-run wrote into the destination" >&2
  exit 1
fi
if grep -q 'jacu-mcp' "$test_root/dry-run.log"; then
  echo "release test: dry-run still mentions the retired jacu-mcp alias" >&2
  exit 1
fi

mkdir -p "$release" "$fakebin" "$test_root/stage"
printf '#!/bin/sh\nold\n' >"$test_root/stage/jacu"
chmod 0755 "$test_root/stage/jacu"
case "$(uname -s)" in
  Darwin) release_os=darwin ;;
  Linux) release_os=linux ;;
  *) echo "release test: unsupported operating system" >&2; exit 1 ;;
esac
case "$(uname -m)" in
  x86_64|amd64) release_arch=amd64 ;;
  arm64|aarch64) release_arch=arm64 ;;
  *) echo "release test: unsupported architecture" >&2; exit 1 ;;
esac
asset="jacu_${version#v}_${release_os}_${release_arch}.tar.gz"
tar -czf "$release/$asset" -C "$test_root/stage" jacu
(cd "$release" && shasum -a 256 "$asset" > checksums.txt)
: >"$release/checksums.txt.sigstore.json"
printf '#!/bin/sh\n[ "$1" = verify-blob ]\n' >"$fakebin/cosign"
chmod 0755 "$fakebin/cosign"

assert_installed() {
  dest="$1"
  want="$2"
  if [ ! -f "$dest/jacu" ] || [ -L "$dest/jacu" ]; then
    echo "release test: $dest/jacu is not a regular-file binary" >&2
    exit 1
  fi
  if [ "$(cat "$dest/jacu")" != "$want" ]; then
    echo "release test: $dest/jacu content mismatch" >&2
    exit 1
  fi
}

# A jacu-mcp left over from an older install is not ours to delete. Installing
# over it must neither recreate it nor remove it: the user's file stays exactly
# as it was, and jacu is installed beside it.
legacy_prefix="$test_root/legacy-prefix"
mkdir -p "$legacy_prefix"
printf '#!/bin/sh\nlegacy-alias\n' >"$legacy_prefix/jacu-mcp"
chmod 0755 "$legacy_prefix/jacu-mcp"
PATH="$fakebin:$PATH" JACU_RELEASE_DIR="$release" bash scripts/install.sh --version "$version" --prefix "$legacy_prefix"
assert_installed "$legacy_prefix" $'#!/bin/sh\nold'
if [ ! -f "$legacy_prefix/jacu-mcp" ] || [ -L "$legacy_prefix/jacu-mcp" ]; then
  echo "release test: install must leave a pre-existing jacu-mcp untouched" >&2
  exit 1
fi
if [ "$(cat "$legacy_prefix/jacu-mcp")" != "$(printf '#!/bin/sh\nlegacy-alias')" ]; then
  echo "release test: install modified a pre-existing jacu-mcp" >&2
  exit 1
fi

PATH="$fakebin:$PATH" JACU_RELEASE_BASE_URL="file://$release" bash scripts/install.sh --version "$version" --prefix "$prefix"
assert_installed "$prefix" $'#!/bin/sh\nold'

latest_prefix="$test_root/latest-prefix"
PATH="$fakebin:$PATH" JACU_RELEASE_DIR="$release" JACU_LATEST_TAG="$version" bash scripts/install.sh --prefix "$latest_prefix"
assert_installed "$latest_prefix" $'#!/bin/sh\nold'
if PATH="$fakebin:$PATH" JACU_RELEASE_DIR="$release" bash scripts/install.sh --prefix "$test_root/need-version"; then
  echo "release test: omitted --version was accepted with JACU_RELEASE_DIR" >&2
  exit 1
fi

printf '#!/bin/sh\nnew\n' >"$test_root/stage/jacu"
tar -czf "$release/$asset" -C "$test_root/stage" jacu
(cd "$release" && shasum -a 256 "$asset" > checksums.txt)
PATH="$fakebin:$PATH" JACU_RELEASE_DIR="$release" bash scripts/install.sh --version "$version" --prefix "$prefix"
assert_installed "$prefix" $'#!/bin/sh\nnew'
PATH="$fakebin:$PATH" JACU_RELEASE_DIR="$release" bash scripts/install.sh --rollback --prefix "$prefix"
assert_installed "$prefix" $'#!/bin/sh\nold'

printf '#!/bin/sh\ntampered\n' >"$test_root/stage/jacu"
tar -czf "$release/$asset" -C "$test_root/stage" jacu
if PATH="$fakebin:$PATH" JACU_RELEASE_DIR="$release" bash scripts/install.sh --version "$version" --prefix "$prefix"; then
  echo "release test: tampered archive was accepted" >&2
  exit 1
fi
assert_installed "$prefix" $'#!/bin/sh\nold'

bad_host_log="$test_root/bad-host.log"
if PATH="$fakebin:$PATH" JACU_RELEASE_BASE_URL="https://jacu-unreachable.example/v0.0.0" bash scripts/install.sh --version "$version" --prefix "$test_root/bad-host" 2>"$bad_host_log"; then
  echo "release test: unreachable host was accepted" >&2
  exit 1
fi
if ! grep -q 'unreachable host: jacu-unreachable.example' "$bad_host_log"; then
  echo "release test: failure must name the unreachable host" >&2
  cat "$bad_host_log" >&2
  exit 1
fi

bash scripts/install.sh --dry-run --rollback --prefix "$prefix" >"$test_root/rollback-dry-run.log"

printf '#!/bin/sh\nexit 1\n' >"$fakebin/gh"
printf '%s\n' '#!/bin/sh' \
  'if printf "%s\n" "$*" | grep -q releases/latest; then' \
  '  echo "curl: HTTP 404" >&2' \
  '  exit 22' \
  'fi' \
  'if printf "%s\n" "$*" | grep -q "/releases"; then' \
  '  printf "%s\n" "[]"' \
  '  exit 0' \
  'fi' \
  'echo "unexpected curl: $*" >&2' \
  'exit 1' >"$fakebin/curl"
chmod 0755 "$fakebin/gh" "$fakebin/curl"
no_latest_log="$test_root/no-latest.log"
if PATH="$fakebin:$PATH" bash scripts/install.sh --prefix "$test_root/no-latest" 2>"$no_latest_log"; then
  echo "release test: latest resolution succeeded with no published release" >&2
  exit 1
fi
if ! grep -q 'no published GitHub Release' "$no_latest_log"; then
  echo "release test: missing-release failure must say no published GitHub Release" >&2
  cat "$no_latest_log" >&2
  exit 1
fi

# Asset collection, across every GoReleaser formula layout. Until v0.3.0 this
# logic was inline in release.yml, so no test ran it and a path mismatch failed
# the owner's tag instead of a pull request.
collect_fixture() {
  fixture_dist="$1"
  formula_rel="$2"
  mkdir -p "$fixture_dist"
  printf 'tarball\n' >"$fixture_dist/jacu_0.0.0_darwin_arm64.tar.gz"
  printf 'sums\n' >"$fixture_dist/checksums.txt"
  printf 'bundle\n' >"$fixture_dist/checksums.txt.sigstore.json"
  mkdir -p "$fixture_dist/$(dirname "$formula_rel")"
  printf 'class Jacu < Formula\n' >"$fixture_dist/$formula_rel"
  # A per-platform build directory must not be uploaded.
  mkdir -p "$fixture_dist/jacu_darwin_arm64"
  printf 'binary\n' >"$fixture_dist/jacu_darwin_arm64/jacu"
}

for layout in jacu.rb Formula/jacu.rb homebrew/Formula/jacu.rb; do
  collect_root="$test_root/collect-$(echo "$layout" | tr / -)"
  collect_fixture "$collect_root/dist" "$layout"
  if ! bash scripts/collect-release-assets.sh "$collect_root/dist" "$collect_root/out" >/dev/null 2>&1; then
    echo "release test: asset collection failed for formula layout $layout" >&2
    exit 1
  fi
  for required in jacu.rb install.sh checksums.txt checksums.txt.sigstore.json; do
    if [ ! -f "$collect_root/out/$required" ]; then
      echo "release test: layout $layout did not produce $required" >&2
      exit 1
    fi
  done
  if [ -e "$collect_root/out/jacu" ]; then
    echo "release test: layout $layout uploaded a per-platform build directory" >&2
    exit 1
  fi
done

missing_root="$test_root/collect-missing"
mkdir -p "$missing_root/dist"
printf 'sums\n' >"$missing_root/dist/checksums.txt"
if bash scripts/collect-release-assets.sh "$missing_root/dist" "$missing_root/out" >/dev/null 2>&1; then
  echo "release test: asset collection passed without a Homebrew formula" >&2
  exit 1
fi

echo "release-test: OK"
