#!/usr/bin/env sh
# mcrcon — single-command installer for Linux/macOS.
#
# Downloads the release build matching OS/arch from GitHub, verifies the
# SHA-256 checksum, and installs it into a bin directory.
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/mcrcon/mcrcon/main/install.sh | sh
#   sh install.sh --version v1.3.0 --bin-dir "$HOME/.local/bin"
#
# Env-var equivalents: MCRCON_VERSION, MCRCON_BIN_DIR.
set -eu

REPO="mcrcon/mcrcon"
LATEST_URL="https://github.com/${REPO}/releases/latest"
API_URL="https://api.github.com/repos/${REPO}/releases/latest"

VERSION="${MCRCON_VERSION:-}"
BIN_DIR="${MCRCON_BIN_DIR:-}"

say() { printf '%s\n' "$*"; }
die() { printf 'install.sh: %s\n' "$*" >&2; exit 1; }

usage() {
	cat <<EOF
mcrcon installer

Usage:
  sh install.sh [options]

Options:
  --version <tag>   install a specific release (default: latest)
                    example: --version v1.3.0
  --bin-dir <dir>   install directory (default: /usr/local/bin as root,
                    \$HOME/.local/bin otherwise)
  -h, --help        show this help

Env: MCRCON_VERSION, MCRCON_BIN_DIR (same as the flags above).
EOF
}

while [ "$#" -gt 0 ]; do
	case "$1" in
		--version)
			[ "$#" -ge 2 ] || die "--version needs a value"
			VERSION="$2"
			shift 2
			;;
		--bin-dir)
			[ "$#" -ge 2 ] || die "--bin-dir needs a value"
			BIN_DIR="$2"
			shift 2
			;;
		-h | --help)
			usage
			exit 0
			;;
		*)
			die "unknown argument: $1 (see --help)"
			;;
	esac
done

# --- download tool ---
if command -v curl >/dev/null 2>&1; then
	DL="curl -fsSL"
else
	command -v wget >/dev/null 2>&1 || die "need curl or wget to download"
	DL="wget -qO-"
fi

fetch() { # $1 = url, $2 = local path
	if [ "$DL" = "curl -fsSL" ]; then
		curl -fsSL -o "$2" "$1"
	else
		wget -q -O "$2" "$1"
	fi
}

# --- detect platform ---
OS=$(uname -s 2>/dev/null || echo unknown)
case "$OS" in
	Linux) OS=linux ;;
	Darwin) OS=darwin ;;
	*) die "unsupported OS: $OS (supported: linux, darwin)" ;;
esac

MACHINE=$(uname -m 2>/dev/null || echo unknown)
case "$MACHINE" in
	x86_64 | amd64) ARCH=amd64 ;;
	aarch64 | arm64) ARCH=arm64 ;;
	*) die "unsupported architecture: $MACHINE (supported: amd64, arm64)" ;;
esac

# --- resolve version ---
if [ -z "$VERSION" ]; then
	VERSION=$(fetch "$API_URL" - 2>/dev/null | sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -n 1)
	[ -n "$VERSION" ] || die "could not determine the latest version from $LATEST_URL"
fi
case "$VERSION" in
	v*) ;;
	*) VERSION="v$VERSION" ;;
esac
VER_NO_V=${VERSION#v}

# --- pick install location ---
if [ -z "$BIN_DIR" ]; then
	if [ "$(id -u 2>/dev/null || echo 0)" = 0 ] || [ -w /usr/local/bin ]; then
		BIN_DIR=/usr/local/bin
	else
		BIN_DIR="$HOME/.local/bin"
	fi
fi
mkdir -p "$BIN_DIR"

# --- download + verify + install ---
ARCHIVE="mcrcon_${VER_NO_V}_${OS}_${ARCH}.tar.gz"
URL="https://github.com/${REPO}/releases/download/${VERSION}/${ARCHIVE}"
TMPDIR_=$(mktemp -d)
trap 'rm -rf "$TMPDIR_"' EXIT INT TERM

say "Downloading ${URL}"
fetch "$URL" "$TMPDIR_/$ARCHIVE" || die "download failed (does $VERSION exist for $OS/$ARCH?)"

say "Verifying SHA-256 checksum"
fetch "https://github.com/${REPO}/releases/download/${VERSION}/checksums.txt" "$TMPDIR_/checksums.txt" || die "checksum download failed"
EXPECTED=$(grep -F "  ${ARCHIVE}" "$TMPDIR_/checksums.txt" | awk '{print $1}' | head -n 1)
[ -n "$EXPECTED" ] || die "no checksum entry for ${ARCHIVE}"
COMPUTED=$(sha256sum "$TMPDIR_/$ARCHIVE" 2>/dev/null | awk '{print $1}' || shasum -a 256 "$TMPDIR_/$ARCHIVE" | awk '{print $1}')
[ "$COMPUTED" = "$EXPECTED" ] || die "checksum mismatch: got ${COMPUTED}, want ${EXPECTED}"

say "Extracting and installing to ${BIN_DIR}"
tar -xzf "$TMPDIR_/$ARCHIVE" -C "$TMPDIR_" mcrcon || die "unexpected archive layout"
install -m 0755 "$TMPDIR_/mcrcon" "$BIN_DIR/mcrcon" || die "install failed (is ${BIN_DIR} writable?)"

VER_OUT=$("$BIN_DIR/mcrcon" -v 2>/dev/null || true)
say "Installed: ${VER_OUT:-mcrcon}"
case ":$PATH:" in
	*":$BIN_DIR:"*) : ;;
	*) say "Add ${BIN_DIR} to your PATH:"; say "  export PATH=\"${BIN_DIR}:\$PATH\"" ;;
esac