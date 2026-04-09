#!/bin/sh
# deadcode installer — download the latest release binary from GitHub.
#
# Usage:
#   curl -sSL https://raw.githubusercontent.com/sglyon/deadcode/main/install.sh | sh
#
#   # Or specify a version and/or install directory:
#   curl -sSL https://raw.githubusercontent.com/sglyon/deadcode/main/install.sh | VERSION=v0.7.0 sh
#   curl -sSL https://raw.githubusercontent.com/sglyon/deadcode/main/install.sh | INSTALL_DIR=$HOME/.local/bin sh
#
# Detected variables:
#   VERSION      — release tag to install (default: latest)
#   INSTALL_DIR  — where to put the binary (default: /usr/local/bin)
#   GITHUB_REPO  — owner/repo (default: sglyon/deadcode)

set -e

GITHUB_REPO="${GITHUB_REPO:-sglyon/deadcode}"
INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"
BINARY_NAME="deadcode"

# Detect OS
OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
case "$OS" in
  linux)  OS="linux" ;;
  darwin) OS="darwin" ;;
  *)
    echo "error: unsupported OS: $OS" >&2
    echo "       download manually from https://github.com/$GITHUB_REPO/releases" >&2
    exit 1
    ;;
esac

# Detect architecture
ARCH="$(uname -m)"
case "$ARCH" in
  x86_64|amd64)  ARCH="x86_64" ;;
  arm64|aarch64) ARCH="arm64" ;;
  *)
    echo "error: unsupported architecture: $ARCH" >&2
    echo "       download manually from https://github.com/$GITHUB_REPO/releases" >&2
    exit 1
    ;;
esac

# Resolve version (latest if not specified)
if [ -z "$VERSION" ]; then
  VERSION="$(curl -sSL "https://api.github.com/repos/$GITHUB_REPO/releases/latest" \
    | grep '"tag_name"' \
    | head -1 \
    | sed 's/.*"tag_name": *"//;s/".*//')"
  if [ -z "$VERSION" ]; then
    echo "error: could not determine latest version from GitHub API" >&2
    echo "       set VERSION=vX.Y.Z explicitly and retry" >&2
    exit 1
  fi
fi

# Strip leading 'v' for the archive name (goreleaser uses bare version)
BARE_VERSION="${VERSION#v}"

# Construct download URL
ARCHIVE="${BINARY_NAME}_${BARE_VERSION}_${OS}_${ARCH}.tar.gz"
URL="https://github.com/$GITHUB_REPO/releases/download/$VERSION/$ARCHIVE"

echo "deadcode installer"
echo "  version:  $VERSION"
echo "  os/arch:  ${OS}/${ARCH}"
echo "  archive:  $ARCHIVE"
echo "  install:  $INSTALL_DIR/$BINARY_NAME"
echo ""

# Download and extract
TMPDIR="$(mktemp -d)"
trap 'rm -rf "$TMPDIR"' EXIT

echo "downloading $URL ..."
if ! curl -sSL --fail -o "$TMPDIR/$ARCHIVE" "$URL"; then
  echo "" >&2
  echo "error: download failed. Check the version and OS/arch:" >&2
  echo "       $URL" >&2
  echo "       available releases: https://github.com/$GITHUB_REPO/releases" >&2
  exit 1
fi

echo "extracting ..."
tar -xzf "$TMPDIR/$ARCHIVE" -C "$TMPDIR"

# Install
if [ ! -f "$TMPDIR/$BINARY_NAME" ]; then
  echo "error: $BINARY_NAME not found in archive" >&2
  exit 1
fi

mkdir -p "$INSTALL_DIR"
if [ -w "$INSTALL_DIR" ]; then
  mv "$TMPDIR/$BINARY_NAME" "$INSTALL_DIR/$BINARY_NAME"
else
  echo "installing to $INSTALL_DIR requires elevated permissions ..."
  sudo mv "$TMPDIR/$BINARY_NAME" "$INSTALL_DIR/$BINARY_NAME"
fi
chmod +x "$INSTALL_DIR/$BINARY_NAME"

echo ""
echo "installed $BINARY_NAME $VERSION to $INSTALL_DIR/$BINARY_NAME"
echo ""
echo "verify:"
echo "  $BINARY_NAME --version"
echo "  $BINARY_NAME doctor"
