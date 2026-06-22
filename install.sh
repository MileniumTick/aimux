#!/usr/bin/env bash
set -euo pipefail

# aimux — Install script
# Usage: curl -fsSL https://raw.githubusercontent.com/MileniumTick/aimux/main/install.sh | bash
#
# Downloads the latest aimux release from GitHub, verifies checksums,
# and installs to /usr/local/bin (default) or $PREFIX/bin.

: "${PREFIX:=/usr/local}"
: "${VERSION:=latest}"
: "${TMPDIR:=/tmp}"

REPO="MileniumTick/aimux"
BINARY="aimux"

# --- helpers ---------------------------------------------------------------
die() { echo >&2 "[ERROR] $*"; exit 1; }
info() { echo "  * $*"; }
ok()   { echo "  ✓ $*"; }

cleanup() { rm -rf "$WORKDIR"; }
trap cleanup EXIT

# --- platform detection ----------------------------------------------------
OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m)"

case "$OS" in
  linux)   ;;
  darwin)  ;;
  *) die "unsupported OS: $OS (only linux/darwin)" ;;
esac

case "$ARCH" in
  x86_64|amd64) ARCH="amd64" ;;
  aarch64|arm64) ARCH="arm64" ;;
  *) die "unsupported arch: $ARCH (only amd64/arm64)" ;;
esac

# --- resolve version -------------------------------------------------------
WORKDIR="$(mktemp -d "$TMPDIR/aimux-install.XXXXXX")"

if [ "$VERSION" = "latest" ]; then
  info "fetching latest release from $REPO..."
  VERSION="$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" \
    | grep '"tag_name":' \
    | sed 's/.*"tag_name": *"v\([^"]*\)".*/\1/')"
  [ -n "$VERSION" ] || die "could not determine latest version"
  ok "latest version: v$VERSION"
else
  VERSION="${VERSION#v}"
fi

# --- paths -----------------------------------------------------------------
ARCHIVE="aimux_${VERSION}_${OS}_${ARCH}.tar.gz"
ARCHIVE_URL="https://github.com/$REPO/releases/download/v${VERSION}/$ARCHIVE"
CHECKSUMS_URL="https://github.com/$REPO/releases/download/v${VERSION}/checksums.txt"

# --- download --------------------------------------------------------------
info "downloading $ARCHIVE ..."
curl -fsSL -o "$WORKDIR/$ARCHIVE" "$ARCHIVE_URL" || die "download failed"
ok "archive downloaded"

info "downloading checksums.txt ..."
curl -fsSL -o "$WORKDIR/checksums.txt" "$CHECKSUMS_URL" || die "checksums download failed"
ok "checksums downloaded"

# --- verify checksum -------------------------------------------------------
info "verifying SHA256 checksum ..."
EXPECTED="$(grep "$ARCHIVE" "$WORKDIR/checksums.txt" | awk '{print $1}')"
[ -n "$EXPECTED" ] || die "checksum entry not found for $ARCHIVE"

COMPUTED="$(sha256sum "$WORKDIR/$ARCHIVE" | awk '{print $1}')"
if [ "$EXPECTED" != "$COMPUTED" ]; then
  die "checksum mismatch:
  expected: $EXPECTED
  computed: $COMPUTED"
fi
ok "checksum verified"

# --- extract & install -----------------------------------------------------
info "extracting ..."
tar -xzf "$WORKDIR/$ARCHIVE" -C "$WORKDIR" || die "extraction failed"

INSTALL_DIR="$PREFIX/bin"
mkdir -p "$INSTALL_DIR" || die "cannot create $INSTALL_DIR"

info "installing to $INSTALL_DIR/$BINARY ..."
install -m 755 "$WORKDIR/$BINARY" "$INSTALL_DIR/$BINARY" || die "install failed"
ok "installed $BINARY v$VERSION to $INSTALL_DIR/$BINARY"

# --- verify ----------------------------------------------------------------
if command -v "$BINARY" &>/dev/null; then
  "$BINARY" version
  echo ""
  echo "  aimux v$VERSION installed successfully!"
  echo "  Run 'aimux' to start the TUI."
else
  info "make sure $INSTALL_DIR is in your PATH, then run 'aimux'"
fi
