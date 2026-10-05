#!/bin/sh
# Installs the skillgild CLI / MCP server on macOS or Linux from GitHub Releases.
#
#   curl -fsSL https://raw.githubusercontent.com/SkillGild/skillgild/main/scripts/install-cli.sh | sh
#
# Options (environment variables):
#   SKILLGILD_VERSION      release to install, e.g. v0.2.0 (default: latest)
#   SKILLGILD_INSTALL_DIR  directory for the binary (default: ~/.local/bin, or /usr/local/bin as root)
#   SKILLGILD_REPO         GitHub repository that publishes releases (default: SkillGild/skillgild)
#
# The archive's SHA-256 is checked against the release's checksums.txt before anything
# is installed. Nothing else is written outside the install directory.
set -eu

REPO="${SKILLGILD_REPO:-SkillGild/skillgild}"
VERSION="${SKILLGILD_VERSION:-latest}"

say() { printf '%s\n' "$*" >&2; }
fail() { say "install-cli: $*"; exit 1; }
need() { command -v "$1" >/dev/null 2>&1 || fail "$1 is required"; }

need uname
need tar
if command -v curl >/dev/null 2>&1; then
  fetch() { curl -fsSL "$1" -o "$2"; }
  fetch_text() { curl -fsSL "$1"; }
elif command -v wget >/dev/null 2>&1; then
  fetch() { wget -qO "$2" "$1"; }
  fetch_text() { wget -qO- "$1"; }
else
  fail "curl or wget is required"
fi

os=$(uname -s | tr '[:upper:]' '[:lower:]')
case "$os" in
  linux) os=linux ;;
  darwin) os=darwin ;;
  mingw*|msys*|cygwin*) fail "use scripts/install-cli.ps1 from PowerShell on Windows" ;;
  *) fail "unsupported operating system: $os" ;;
esac
arch=$(uname -m)
case "$arch" in
  x86_64|amd64) arch=amd64 ;;
  aarch64|arm64) arch=arm64 ;;
  *) fail "unsupported CPU architecture: $arch" ;;
esac

if [ "$VERSION" = "latest" ]; then
  VERSION=$(fetch_text "https://api.github.com/repos/$REPO/releases/latest" | sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p' | head -n 1)
  [ -n "$VERSION" ] || fail "could not find the latest release of $REPO"
fi
case "$VERSION" in v*) ;; *) VERSION="v$VERSION" ;; esac
plain=${VERSION#v}

if [ -n "${SKILLGILD_INSTALL_DIR:-}" ]; then
  install_dir="$SKILLGILD_INSTALL_DIR"
elif [ "$(id -u)" = "0" ]; then
  install_dir=/usr/local/bin
else
  install_dir="$HOME/.local/bin"
fi

archive="skillgild_${plain}_${os}_${arch}.tar.gz"
base="https://github.com/$REPO/releases/download/$VERSION"
tmp=$(mktemp -d 2>/dev/null || mktemp -d -t skillgild)
trap 'rm -rf "$tmp"' EXIT

say "Downloading skillgild $VERSION for $os/$arch"
fetch "$base/$archive" "$tmp/$archive" || fail "no release asset $archive at $base"
fetch "$base/checksums.txt" "$tmp/checksums.txt" || fail "release $VERSION has no checksums.txt"

expected=$(grep " $archive\$" "$tmp/checksums.txt" | awk '{print $1}')
[ -n "$expected" ] || fail "$archive is not listed in checksums.txt"
if command -v sha256sum >/dev/null 2>&1; then
  actual=$(sha256sum "$tmp/$archive" | awk '{print $1}')
elif command -v shasum >/dev/null 2>&1; then
  actual=$(shasum -a 256 "$tmp/$archive" | awk '{print $1}')
else
  fail "sha256sum or shasum is required to verify the download"
fi
[ "$expected" = "$actual" ] || fail "checksum mismatch for $archive; refusing to install"

tar -xzf "$tmp/$archive" -C "$tmp" skillgild
mkdir -p "$install_dir"
install -m 0755 "$tmp/skillgild" "$install_dir/skillgild" 2>/dev/null || {
  cp "$tmp/skillgild" "$install_dir/skillgild" && chmod 0755 "$install_dir/skillgild"
}

say "Installed $("$install_dir/skillgild" version) to $install_dir/skillgild"
case ":${PATH}:" in
  *":$install_dir:"*) ;;
  *)
    say ""
    say "$install_dir is not on your PATH. Add it, for example:"
    say "  export PATH=\"$install_dir:\$PATH\""
    ;;
esac
say ""
say "Next: skillgild login"
say "Then register the MCP server with your agent, e.g.:"
say "  claude mcp add --scope user skillgild -- $install_dir/skillgild mcp"
