#!/bin/sh
# Install or update smoothcast from the latest GitHub release — a single static
# binary, any Linux distribution, amd64 or arm64. No root needed.
#
#   curl -fsSL https://raw.githubusercontent.com/tihonove/smoothcast/main/install.sh | sh
#
# Installs into ~/.local/bin, or /usr/local/bin when run as root; override with
# PREFIX=/some/prefix. Needs ffmpeg at runtime. On Ubuntu prefer the apt
# repository (https://github.com/tihonove/apt): it updates with the system.
set -eu

repo=tihonove/smoothcast

die() { echo "smoothcast install: $*" >&2; exit 1; }

case $(uname -s) in
    Linux) ;;
    *) die "only Linux builds are published" ;;
esac
case $(uname -m) in
    x86_64 | amd64) arch=amd64 ;;
    aarch64 | arm64) arch=arm64 ;;
    *) die "no build for $(uname -m)" ;;
esac

if [ -n "${PREFIX:-}" ]; then
    bindir=$PREFIX/bin
elif [ "$(id -u)" -eq 0 ]; then
    bindir=/usr/local/bin
else
    bindir=$HOME/.local/bin
fi

if command -v curl >/dev/null; then
    fetch() { curl -fsSL -o "$2" "$1"; }
elif command -v wget >/dev/null; then
    fetch() { wget -qO "$2" "$1"; }
else
    die "needs curl or wget"
fi

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

base=https://github.com/$repo/releases/latest/download
bin=smoothcast-linux-$arch
fetch "$base/$bin" "$tmp/$bin" || die "download failed: $base/$bin"
fetch "$base/SHA256SUMS" "$tmp/SHA256SUMS" || die "download failed: $base/SHA256SUMS"
(cd "$tmp" && grep " $bin\$" SHA256SUMS | sha256sum -c >/dev/null 2>&1) || die "checksum mismatch for $bin"

mkdir -p "$bindir"
install -m 755 "$tmp/$bin" "$bindir/smoothcast"
echo "Installed $("$bindir/smoothcast" -version) to $bindir/smoothcast"

case ":$PATH:" in
    *":$bindir:"*) ;;
    *) echo "Note: $bindir is not in PATH" ;;
esac
command -v ffmpeg >/dev/null || echo "Note: smoothcast needs ffmpeg — install it with your package manager"
