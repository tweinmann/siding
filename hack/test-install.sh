#!/usr/bin/env bash
# Tests install.sh against a release built with just dist and served on a loopback port: an
# install, an update, a broken checksum, an install directory missing from the PATH, and an
# unsupported platform. Needs no Docker and no network; leaves nothing behind.
set -euo pipefail
source "$(dirname "$0")/lib.sh"
[[ "${SIDING_DEVCONTAINER:-}" == 1 ]] || die "run this inside the siding devcontainer (SIDING_DEVCONTAINER is not set)"

cd "$(dirname "$0")/.."
work="$(mktemp -d)"
server=""
cleanup() {
  [[ -n "$server" ]] && kill "$server" 2>/dev/null
  rm -rf "$work"
}
trap cleanup EXIT

# A release of this platform's binary only: the script downloads nothing else.
case "$(uname -m)" in aarch64 | arm64) arch=arm64 ;; *) arch=amd64 ;; esac
asset="siding-linux-$arch"
mkdir -p "$work/release"
CGO_ENABLED=0 go build -trimpath -ldflags "-X main.version=v0.0.0-test" -o "$work/release/$asset" ./cmd/siding
(cd "$work/release" && sha256sum "$asset" >SHA256SUMS)

port="$(python3 -c 'import socket; s = socket.socket(); s.bind(("127.0.0.1", 0)); print(s.getsockname()[1])')"
python3 -m http.server "$port" --bind 127.0.0.1 --directory "$work/release" >/dev/null 2>&1 &
server=$!
retry 20 curl -fs -o /dev/null "http://127.0.0.1:$port/SHA256SUMS" || die "the test server did not start"

failed=0
check() { # check <name> <condition…>
  local name="$1"
  shift
  if "$@"; then echo "ok    $name"; else echo "FAIL  $name"; failed=1; fi
}
install() { # install <dir> [VAR=value…]: runs install.sh, output in $work/out
  local dir="$1"
  shift
  env SIDING_DOWNLOAD_URL="http://127.0.0.1:$port" SIDING_INSTALL_DIR="$dir" "$@" sh install.sh >"$work/out" 2>&1
}
has() { grep -qF -- "$1" "$work/out"; }

bin="$work/bin"
check "install" install "$bin" PATH="$bin:$PATH"
check "  the binary runs" bash -c "'$bin/siding' version | grep -q v0.0.0-test"
check "  says what comes next" has "Next: siding setup"
check "  no PATH hint when on the PATH" bash -c "! grep -q 'is not on your PATH' '$work/out'"

check "update" install "$bin" PATH="$bin:$PATH"
check "  still runs" bash -c "'$bin/siding' version >/dev/null"

check "not on the PATH" install "$work/other" PATH="/usr/bin:/bin"
check "  gives the export line" has "export PATH=\"$work/other:\$PATH\""

cp "$work/release/SHA256SUMS" "$work/sums"
echo "0000000000000000000000000000000000000000000000000000000000000000  $asset" >"$work/release/SHA256SUMS"
check "a checksum mismatch fails" bash -c "! env SIDING_DOWNLOAD_URL=http://127.0.0.1:$port SIDING_INSTALL_DIR='$work/bad' sh install.sh >'$work/out' 2>&1"
check "  installs nothing" test ! -e "$work/bad/siding"
check "  names the mismatch" has "checksum mismatch"
cp "$work/sums" "$work/release/SHA256SUMS"

check "an unsupported platform fails" bash -c "! env SIDING_UNAME_S=MINGW64_NT SIDING_DOWNLOAD_URL=http://127.0.0.1:$port SIDING_INSTALL_DIR='$work/win' sh install.sh >'$work/out' 2>&1"
check "  names the supported ones" has "there are binaries for macOS and Linux"
check "a 32-bit system fails" bash -c "! env SIDING_UNAME_M=i686 SIDING_DOWNLOAD_URL=http://127.0.0.1:$port SIDING_INSTALL_DIR='$work/x86' sh install.sh >'$work/out' 2>&1"
check "no release fails" bash -c "! env SIDING_DOWNLOAD_URL=http://127.0.0.1:$port/nope SIDING_INSTALL_DIR='$work/none' sh install.sh >'$work/out' 2>&1"
check "  says no release was found" has "found no release"

[[ "$failed" == 0 ]] || die "install.sh failed its test"
echo "test-install: ok"
