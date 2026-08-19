#!/usr/bin/env bash
set -euo pipefail

version=""
prefix="${JACU_INSTALL_PREFIX:-${HOME}/.local/bin}"
dry_run=false
rollback=false

usage() {
  cat >&2 <<'EOF'
usage: install.sh --version vX.Y.Z [--prefix DIR] [--dry-run]
       install.sh --rollback [--prefix DIR] [--dry-run]
EOF
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --version)
      [ "$#" -ge 2 ] || { usage; exit 2; }
      version="$2"
      shift 2
      ;;
    --prefix)
      [ "$#" -ge 2 ] || { usage; exit 2; }
      prefix="$2"
      shift 2
      ;;
    --dry-run)
      dry_run=true
      shift
      ;;
    --rollback)
      rollback=true
      shift
      ;;
    -h|--help)
      usage >&1
      exit 0
      ;;
    *)
      echo "install.sh: unknown option $1" >&2
      usage
      exit 2
      ;;
  esac
done

target="$prefix/jacu"
previous="$prefix/jacu.previous"

# Hosts that still launch the old binary name keep working after the rename.
install_legacy_alias() {
  dest_dir="$1"
  alias_path="$dest_dir/jacu"
  previous_path="$dest_dir/jacu.previous"
  if [ -e "$alias_path" ] && [ ! -L "$alias_path" ]; then
    if [ -d "$alias_path" ]; then
      echo "install.sh: refusing to replace directory $alias_path" >&2
      return 1
    fi
    # Pre-rename hosts only have a regular-file jacu. Seed the existing
    # rollback artifact so --rollback is not left without jacu.previous.
    if [ ! -e "$previous_path" ] || [ -L "$previous_path" ]; then
      install -m 0755 "$alias_path" "$previous_path"
    fi
    rm -f "$alias_path"
  fi
  ln -sfn jacu "$alias_path"
}

if [ "$rollback" = true ]; then
  if [ "$dry_run" = true ]; then
    echo "dry-run: restore $previous to $target"
    exit 0
  fi
  if [ ! -f "$previous" ] || [ -L "$previous" ]; then
    echo "install.sh: no safe previous binary at $previous" >&2
    exit 1
  fi
  if [ -L "$target" ]; then
    echo "install.sh: refusing to replace symlink $target" >&2
    exit 1
  fi
  mkdir -p "$prefix"
  install -m 0755 "$previous" "$target"
  install_legacy_alias "$prefix"
  echo "restored $target from $previous"
  exit 0
fi

if [[ ! "$version" =~ ^v[0-9]+\.[0-9]+\.[0-9]+([-.][0-9A-Za-z.-]+)?$ ]]; then
  echo "install.sh: --version must be a semver tag vX.Y.Z" >&2
  exit 2
fi

case "$(uname -s)" in
  Darwin) os_name=darwin ;;
  Linux) os_name=linux ;;
  *) echo "install.sh: unsupported operating system $(uname -s)" >&2; exit 1 ;;
esac
case "$(uname -m)" in
  x86_64|amd64) arch_name=amd64 ;;
  arm64|aarch64) arch_name=arm64 ;;
  *) echo "install.sh: unsupported architecture $(uname -m)" >&2; exit 1 ;;
esac

asset="jacu_${version#v}_${os_name}_${arch_name}.tar.gz"
base_url="${JACU_RELEASE_BASE_URL:-https://github.com/jacu-dev/jacu-harness/releases/download/${version}}"
bundle="checksums.txt.sigstore.json"
identity_regexp="${JACU_COSIGN_IDENTITY_REGEXP:-^https://github.com/jacu-dev/jacu-harness/.github/workflows/release.yml@refs/tags/v.*$}"

if [ "$dry_run" = true ]; then
  echo "dry-run: download $base_url/$asset"
  echo "dry-run: verify $bundle and sha256 for $asset"
  echo "dry-run: backup $target to $previous and install into $prefix"
  echo "dry-run: link $prefix/jacu -> jacu"
  exit 0
fi

for command_name in curl cosign shasum tar install; do
  command -v "$command_name" >/dev/null || {
    echo "install.sh: required command missing: $command_name" >&2
    exit 1
  }
done
if [ -L "$target" ]; then
  echo "install.sh: refusing to replace symlink $target" >&2
  exit 1
fi

download_dir="$(mktemp -d)"
trap 'rm -rf "$download_dir"' EXIT
# The repository is private, so the anonymous GitHub endpoint answers 404 for
# every asset. Trying it first cost three 404s per install and printed a
# "download failed" line before the path that actually works. gh goes first
# whenever it is available and the caller did not override the base URL;
# curl stays as the fallback for a custom or public mirror.
fetch() {
  file_name="$1"
  if [ -z "${JACU_RELEASE_BASE_URL:-}" ] && command -v gh >/dev/null 2>&1; then
    if gh release download "$version" -R jacu-dev/jacu -p "$file_name" -D "$download_dir" 2>/dev/null; then
      return 0
    fi
    echo "install.sh: gh could not fetch $file_name; falling back to a direct download" >&2
  fi
  if curl --fail --location --silent --show-error "$base_url/$file_name" -o "$download_dir/$file_name"; then
    return 0
  fi
  echo "install.sh: download failed for $file_name (private repo? run '"'"'gh auth login'"'"' or set JACU_RELEASE_BASE_URL)" >&2
  exit 1
}
fetch checksums.txt
fetch "$bundle"
fetch "$asset"

cosign verify-blob \
  --bundle "$download_dir/$bundle" \
  --certificate-identity-regexp "$identity_regexp" \
  --certificate-oidc-issuer "https://token.actions.githubusercontent.com" \
  "$download_dir/checksums.txt" >/dev/null

expected="$(awk -v wanted="$asset" '{candidate=$2; sub(/^\*/, "", candidate); if (candidate == wanted) print $1}' "$download_dir/checksums.txt")"
actual="$(shasum -a 256 "$download_dir/$asset" | awk '{print $1}')"
if [ -z "$expected" ] || [ "$expected" != "$actual" ]; then
  echo "install.sh: checksum verification failed for $asset" >&2
  exit 1
fi

extract_dir="$download_dir/extract"
mkdir "$extract_dir"
tar -xzf "$download_dir/$asset" -C "$extract_dir"
binary="$extract_dir/jacu"
if [ ! -f "$binary" ] || [ -L "$binary" ]; then
  echo "install.sh: archive does not contain a safe jacu binary" >&2
  exit 1
fi

mkdir -p "$prefix"
if [ -f "$target" ]; then
  install -m 0755 "$target" "$previous"
fi
install -m 0755 "$binary" "$target"
install_legacy_alias "$prefix"
echo "installed $target from $version"
