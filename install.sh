#!/bin/sh
# Installs siding: the binary of the latest release for this machine, checked against the
# release's SHA256SUMS, into ~/.local/bin. Run it again to update.
#
#   curl -fsSL https://raw.githubusercontent.com/tweinmann/siding/main/install.sh | sh
#
# SIDING_VERSION=v0.5.0         install that release instead of the latest
# SIDING_INSTALL_DIR=<dir>      install there instead of ~/.local/bin
# SIDING_DOWNLOAD_URL=<url>     download the release files from there (a mirror, or a test)
set -eu

repo="https://github.com/tweinmann/siding"

fail() {
  echo "siding install: $*" >&2
  exit 1
}

# The platform, as the release names its binaries. SIDING_UNAME_S and SIDING_UNAME_M exist for
# the test only.
os="${SIDING_UNAME_S:-$(uname -s)}"
arch="${SIDING_UNAME_M:-$(uname -m)}"
case "$os" in
  Darwin) os=darwin ;;
  Linux) os=linux ;;
  *) fail "no siding for $os; there are binaries for macOS and Linux" ;;
esac
case "$arch" in
  arm64 | aarch64) arch=arm64 ;;
  x86_64 | amd64) arch=amd64 ;;
  *) fail "no siding for $arch; there are binaries for arm64 and amd64 (x86_64)" ;;
esac
asset="siding-$os-$arch"

if [ -n "${SIDING_DOWNLOAD_URL:-}" ]; then
  url="$SIDING_DOWNLOAD_URL"
elif [ -n "${SIDING_VERSION:-}" ]; then
  url="$repo/releases/download/$SIDING_VERSION"
else
  url="$repo/releases/latest/download"
fi
dir="${SIDING_INSTALL_DIR:-$HOME/.local/bin}"

if command -v curl >/dev/null 2>&1; then
  fetch() { curl -fsSL -o "$2" "$1"; }
elif command -v wget >/dev/null 2>&1; then
  fetch() { wget -q -O "$2" "$1"; }
else
  fail "neither curl nor wget is installed"
fi
if command -v sha256sum >/dev/null 2>&1; then
  sum() { sha256sum "$1" | cut -d ' ' -f 1; }
elif command -v shasum >/dev/null 2>&1; then
  sum() { shasum -a 256 "$1" | cut -d ' ' -f 1; }
else
  fail "neither sha256sum nor shasum is installed, so the download cannot be checked"
fi

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT INT TERM

echo "Downloading $asset from $url"
fetch "$url/SHA256SUMS" "$tmp/SHA256SUMS" ||
  fail "found no release at $url; check SIDING_VERSION, or see $repo/releases"
fetch "$url/$asset" "$tmp/$asset" || fail "could not download $url/$asset"

want="$(awk -v f="$asset" '$2 == f || $2 == "*" f { print $1 }' "$tmp/SHA256SUMS")"
[ -n "$want" ] || fail "SHA256SUMS of this release has no line for $asset; nothing was installed"
got="$(sum "$tmp/$asset")"
[ "$got" = "$want" ] || fail "checksum mismatch for $asset (expected $want, got $got); nothing was installed"

mkdir -p "$dir" || fail "cannot create $dir; choose another with SIDING_INSTALL_DIR"
chmod 755 "$tmp/$asset"
# mv replaces the file instead of writing into it, so a running siding is not disturbed.
mv -f "$tmp/$asset" "$dir/siding" || fail "cannot write to $dir; choose another with SIDING_INSTALL_DIR"

echo "Installed $("$dir/siding" version) to $dir/siding"
case ":$PATH:" in
  *":$dir:"*) ;;
  *)
    echo
    echo "$dir is not on your PATH. Add it, e.g. in ~/.zshrc or ~/.bashrc:"
    echo "  export PATH=\"$dir:\$PATH\""
    ;;
esac
echo
echo "Next: siding setup, then follow \"Get started\" in $repo#get-started"
