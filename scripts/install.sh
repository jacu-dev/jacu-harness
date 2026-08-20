#!/usr/bin/env bash
set -euo pipefail

version=""
prefix="${JACU_INSTALL_PREFIX:-${HOME}/.local/bin}"
dry_run=false
rollback=false
release_repo="jacu-dev/jacu-harness"

usage() {
  cat >&2 <<'EOF'
usage: install.sh [--version vX.Y.Z|latest] [--prefix DIR] [--dry-run]
       install.sh --rollback [--prefix DIR] [--dry-run]

Download a signed GitHub Release, verify it, then install jacu.
Omit --version (or pass latest) to install the newest published release.
Drafts are ignored. There is no curl|sh installer. Review this script,
then run it.

Offline assets: set JACU_RELEASE_DIR to a directory that already contains
the tarball, checksums.txt and checksums.txt.sigstore.json.
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

release_host() {
  case "$1" in
    https://*|http://*)
      host="${1#*://}"
      printf '%s\n' "${host%%/*}"
      ;;
    file://*)
      printf 'local-file\n'
      ;;
    *)
      printf '%s\n' "$1"
      ;;
  esac
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
  echo "restored $target from $previous"
  exit 0
fi

resolve_latest_version() {
  if [ -n "${JACU_LATEST_TAG:-}" ]; then
    printf '%s\n' "$JACU_LATEST_TAG"
    return 0
  fi
  # Drafts are not installable. `gh release view` would return one.
  if command -v gh >/dev/null 2>&1; then
    if tag="$(gh release list -R "$release_repo" --exclude-drafts --limit 1 --json tagName -q '.[0].tagName' 2>/dev/null)" && [ -n "$tag" ]; then
      printf '%s\n' "$tag"
      return 0
    fi
  fi
  api_url="https://api.github.com/repos/${release_repo}/releases/latest"
  if body="$(curl --fail --location --silent --show-error "$api_url")" && [ -n "$body" ]; then
    tag="$(printf '%s\n' "$body" | sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -n 1)"
    if [ -n "$tag" ]; then
      printf '%s\n' "$tag"
      return 0
    fi
    echo "install.sh: latest release response did not include tag_name" >&2
    exit 1
  fi
  echo "install.sh: no published GitHub Release (drafts are not installable)" >&2
  echo "install.sh: publish the release or pass --version vX.Y.Z" >&2
  exit 1
}

if [ -z "$version" ] || [ "$version" = latest ]; then
  if [ -n "${JACU_RELEASE_DIR:-}" ] && [ -z "${JACU_LATEST_TAG:-}" ]; then
    echo "install.sh: --version vX.Y.Z is required when JACU_RELEASE_DIR is set" >&2
    exit 2
  fi
  version="$(resolve_latest_version)"
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
base_url="${JACU_RELEASE_BASE_URL:-https://github.com/${release_repo}/releases/download/${version}}"
bundle="checksums.txt.sigstore.json"
identity_regexp="${JACU_COSIGN_IDENTITY_REGEXP:-^https://github.com/${release_repo}/.github/workflows/release.yml@refs/tags/v.*$}"

if [ "$dry_run" = true ]; then
  echo "dry-run: download $base_url/$asset"
  echo "dry-run: verify $bundle and sha256 for $asset"
  echo "dry-run: backup $target to $previous and install into $prefix"
  exit 0
fi

required_commands=(cosign shasum tar install)
if [ -z "${JACU_RELEASE_DIR:-}" ]; then
  required_commands+=(curl)
fi
for command_name in "${required_commands[@]}"; do
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

# Public GitHub Releases are fetched with curl. JACU_RELEASE_DIR is the
# offline path (assets already on disk). gh is a fallback for the same
# public repository when anonymous download is blocked.
fetch() {
  file_name="$1"
  if [ -n "${JACU_RELEASE_DIR:-}" ]; then
    if [ ! -f "$JACU_RELEASE_DIR/$file_name" ]; then
      echo "install.sh: local asset missing: $JACU_RELEASE_DIR/$file_name" >&2
      exit 1
    fi
    cp "$JACU_RELEASE_DIR/$file_name" "$download_dir/$file_name"
    return 0
  fi
  if curl --fail --location --silent --show-error "$base_url/$file_name" -o "$download_dir/$file_name"; then
    return 0
  fi
  if [ -z "${JACU_RELEASE_BASE_URL:-}" ] && command -v gh >/dev/null 2>&1; then
    if gh release download "$version" -R "$release_repo" -p "$file_name" -D "$download_dir"; then
      return 0
    fi
  fi
  echo "install.sh: download failed for $file_name (unreachable host: $(release_host "$base_url"))" >&2
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
if [ -f "$target" ] && [ ! -L "$target" ]; then
  install -m 0755 "$target" "$previous"
fi
install -m 0755 "$binary" "$target"
echo "installed $target from $version"
