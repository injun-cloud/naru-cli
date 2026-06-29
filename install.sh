#!/bin/sh
# Install the latest naru CLI binary.
#   curl -fsSL https://raw.githubusercontent.com/injun-cloud/naru-cli/main/install.sh | sh
# Override the install dir with NARU_INSTALL_DIR (default /usr/local/bin).
set -e

repo="injun-cloud/naru-cli"
bin="naru"
dir="${NARU_INSTALL_DIR:-/usr/local/bin}"

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

url="https://github.com/$repo/releases/latest/download/${bin}_${os}_${arch}.tar.gz"
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

echo "downloading $url"
curl -fsSL "$url" | tar -xz -C "$tmp"

if [ -w "$dir" ]; then
  mv "$tmp/$bin" "$dir/$bin"
else
  echo "writing to $dir requires sudo"
  sudo mv "$tmp/$bin" "$dir/$bin"
fi

echo "installed $bin to $dir/$bin"
"$dir/$bin" --version || true
