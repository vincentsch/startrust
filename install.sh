#!/bin/sh
# Usage: curl -fsSL https://raw.githubusercontent.com/vincentsch/startrust/main/install.sh | sh
set -eu

fail() { printf '%s\n' "StarTrust: $*" >&2; exit 1; }
work=
staged=
cleanup() {
    [ -z "$staged" ] || rm -f "$staged"
    [ -z "$work" ] || rm -rf "$work"
    return 0
}
trap cleanup 0
trap 'exit 1' HUP INT TERM

os=$(uname -s)
case "$os" in
    Linux) os=linux ;;
    Darwin) os=darwin ;;
    MINGW*|MSYS*|CYGWIN*) os=windows ;;
    *) fail "Unsupported operating system: $os" ;;
esac
arch=$(uname -m)
case "$arch" in
    x86_64|amd64|AMD64) arch=amd64 ;;
    aarch64|arm64|ARM64) arch=arm64 ;;
    *) fail "Unsupported architecture: $arch (need amd64 or arm64)" ;;
esac
asset=startrust-$os-$arch
binary=startrust
if [ "$os" = windows ]; then asset=$asset.exe; binary=$binary.exe; fi
base=${STARTRUST_DOWNLOAD_BASE:-https://github.com/vincentsch/startrust}
if [ -n "${STARTRUST_VERSION:-}" ]; then
    url=${base%/}/releases/download/$STARTRUST_VERSION
else
    url=${base%/}/releases/latest/download
fi
command -v curl >/dev/null 2>&1 || fail 'Cannot download the release without curl. Install curl and try again'
work=$(mktemp -d) || fail 'Cannot create temporary download directory'
curl -fsSL "$url/$asset" -o "$work/$asset" || fail "Could not download $url/$asset. Check your connection and that this release exists"
curl -fsSL "$url/checksums.txt" -o "$work/checksums.txt" || fail "Could not download $url/checksums.txt, so the download cannot be checked"
expected=$(awk -v name="$asset" '$2 == name {print $1}' "$work/checksums.txt")
[ -n "$expected" ] || fail "checksums.txt has no line for $asset"
[ "${#expected}" -eq 64 ] || fail "Invalid checksum entry for $asset"
case "$expected" in *[!0-9a-fA-F]*) fail "Invalid checksum entry for $asset" ;; esac
if command -v sha256sum >/dev/null 2>&1; then
    digest=$(sha256sum "$work/$asset") || fail 'SHA-256 calculation failed'
elif command -v shasum >/dev/null 2>&1; then
    digest=$(shasum -a 256 "$work/$asset") || fail 'SHA-256 calculation failed'
else
    fail 'Need sha256sum or shasum to check the download'
fi
actual=${digest%% *}
[ "$actual" = "$expected" ] || fail "Checksum mismatch for $asset: the download does not match checksums.txt, so nothing was installed"

install_dir=${INSTALL_DIR:-$HOME/.local/bin}
mkdir -p "$install_dir" || fail 'Cannot create installation directory; choose a writable INSTALL_DIR'
# Stage beside the destination so replacement is a rename on the same filesystem.
staged=$(mktemp "$install_dir/.startrust.XXXXXX") || fail 'Cannot write to the installation directory; choose a writable INSTALL_DIR'
cp "$work/$asset" "$staged" || fail "Cannot copy the binary into $install_dir"
chmod 755 "$staged" || fail 'Cannot make downloaded binary executable'
mv -f "$staged" "$install_dir/$binary" || fail 'Cannot replace installed executable (stop StarTrust if it is running)'
staged=
if [ -n "${STARTRUST_VERSION:-}" ]; then
    printf 'Installed StarTrust %s to %s\n' "$STARTRUST_VERSION" "$install_dir/$binary"
else
    printf 'Installed the latest StarTrust release to %s\n' "$install_dir/$binary"
fi
case ":${PATH:-}:" in
    *":$install_dir:"*) ;;
    *) printf 'Add the installation directory to PATH:\n  export PATH="%s:$PATH"\n' "$install_dir" ;;
esac
printf 'Try it: startrust check junegunn/fzf\n'
