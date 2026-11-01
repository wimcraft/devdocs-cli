#!/bin/sh
# Install the devdocs binary from GitHub Releases:
#
#   curl -fsSL https://raw.githubusercontent.com/wimcraft/devdocs-cli/main/install.sh | sh
#
# DEVDOCS_VERSION pins a release tag such as v0.1.0 (default: latest).
# DEVDOCS_INSTALL_DIR sets the target directory (default: ~/.local/bin).
set -eu

fail() {
  echo "devdocs install: $*" >&2
  exit 1
}

sha256() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1"
  elif command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$1"
  else
    fail "sha256sum or shasum is required to verify the download"
  fi
}

# All work happens in main so a truncated download cannot run a partial script.
main() {
  repo=wimcraft/devdocs-cli
  dir=${DEVDOCS_INSTALL_DIR:-$HOME/.local/bin}

  case $(uname -s) in
    Darwin) os=darwin ;;
    Linux) os=linux ;;
    *) fail "unsupported OS $(uname -s); devdocs supports macOS and Linux" ;;
  esac
  case $(uname -m) in
    x86_64 | amd64) arch=amd64 ;;
    arm64 | aarch64) arch=arm64 ;;
    *) fail "unsupported architecture $(uname -m)" ;;
  esac

  if [ -n "${DEVDOCS_VERSION:-}" ]; then
    url=https://github.com/$repo/releases/download/$DEVDOCS_VERSION
  else
    url=https://github.com/$repo/releases/latest/download
  fi
  asset=devdocs_${os}_$arch

  mkdir -p "$dir"
  # Inside the target directory so the final mv is an atomic rename.
  tmp=$(mktemp -d "$dir/.devdocs-install.XXXXXX")
  trap 'rm -rf "$tmp"' EXIT

  curl -fsSL "$url/checksums.txt" -o "$tmp/checksums.txt" || fail "cannot download $url/checksums.txt"
  curl -fsSL "$url/$asset" -o "$tmp/$asset" || fail "cannot download $url/$asset"

  expected=$(awk -v name="$asset" '$2 == name { print $1 }' "$tmp/checksums.txt")
  actual=$(sha256 "$tmp/$asset" | awk '{ print $1 }')
  [ -n "$expected" ] && [ "$expected" = "$actual" ] || fail "checksum mismatch for $asset"

  chmod 755 "$tmp/$asset"
  mv -f "$tmp/$asset" "$dir/devdocs"

  "$dir/devdocs" --version
  echo "Installed to $dir/devdocs"
  case ":$PATH:" in
    *":$dir:"*) ;;
    *) echo "Add $dir to your PATH to run devdocs." ;;
  esac
}

main
