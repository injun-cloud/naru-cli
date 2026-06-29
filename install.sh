#!/bin/sh
# Install the latest naru CLI.
#   curl -fsSL https://raw.githubusercontent.com/injun-cloud/naru-cli/main/install.sh | sh
# Override the install dir with NARU_INSTALL_DIR (default ~/.local/bin, no sudo).
# All logic is wrapped in main() so a truncated download can't run partially.
set -e

main() {
  repo="injun-cloud/naru-cli"
  bin="naru"
  dir="${NARU_INSTALL_DIR:-$HOME/.local/bin}"

  os=$(uname -s)
  arch=$(uname -m)
  case "$os" in
    Linux) os=linux ;;
    Darwin) os=darwin ;;
    *) echo "unsupported OS: $os — download from https://github.com/$repo/releases" >&2; exit 1 ;;
  esac
  case "$arch" in
    x86_64 | amd64) arch=amd64 ;;
    aarch64 | arm64) arch=arm64 ;;
    *) echo "unsupported arch: $arch — download from https://github.com/$repo/releases" >&2; exit 1 ;;
  esac

  asset="${bin}_${os}_${arch}.tar.gz"
  base="https://github.com/$repo/releases/latest/download"
  tmp=$(mktemp -d)
  trap 'rm -rf "$tmp"' EXIT

  echo "downloading $asset"
  curl -fsSL "$base/$asset" -o "$tmp/$asset"
  curl -fsSL "$base/checksums.txt" -o "$tmp/checksums.txt"

  want=$(awk -v a="$asset" '$2 == a {print $1}' "$tmp/checksums.txt")
  [ -n "$want" ] || { echo "no checksum entry for $asset" >&2; exit 1; }
  if command -v sha256sum >/dev/null 2>&1; then
    got=$(sha256sum "$tmp/$asset" | awk '{print $1}')
  else
    got=$(shasum -a 256 "$tmp/$asset" | awk '{print $1}')
  fi
  [ "$got" = "$want" ] || { echo "checksum mismatch for $asset" >&2; exit 1; }

  tar -xzf "$tmp/$asset" -C "$tmp"
  mkdir -p "$dir"
  if [ -w "$dir" ]; then
    mv "$tmp/$bin" "$dir/$bin"
  else
    echo "writing to $dir requires sudo"
    sudo mv "$tmp/$bin" "$dir/$bin"
  fi
  echo "installed $bin to $dir/$bin"

  case ":$PATH:" in
    *":$dir:"*) ;;
    *) echo; echo "note: $dir is not on your PATH — add it:"; echo "  export PATH=\"$dir:\$PATH\"" ;;
  esac
  "$dir/$bin" --version 2>/dev/null || true
}

main "$@"
