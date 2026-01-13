#!/bin/sh
# MEOW installer script
# Usage: curl -fsSL https://raw.githubusercontent.com/akatz-ai/meow-machine/main/install.sh | sh

set -e

REPO="akatz-ai/meow-machine"
BINARY="meow"
INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"

# Detect OS (Linux/macOS only)
OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
case "$OS" in
  darwin) OS="darwin" ;;
  linux) OS="linux" ;;
  *)
    echo "Error: Unsupported operating system: $OS"
    echo "meow currently supports Linux and macOS only"
    exit 1
    ;;
esac

# Detect architecture
ARCH="$(uname -m)"
case "$ARCH" in
  x86_64|amd64) ARCH="amd64" ;;
  aarch64|arm64) ARCH="arm64" ;;
  *)
    echo "Error: Unsupported architecture: $ARCH"
    exit 1
    ;;
esac

# Get latest version from GitHub API
get_latest_version() {
  curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" |
    grep '"tag_name":' |
    sed -E 's/.*"v([^"]+)".*/\1/'
}

VERSION="${VERSION:-$(get_latest_version)}"
if [ -z "$VERSION" ]; then
  echo "Error: Could not determine latest version"
  exit 1
fi

echo "Installing meow v${VERSION} for ${OS}/${ARCH}..."

# Construct download URL
DOWNLOAD_URL="https://github.com/${REPO}/releases/download/v${VERSION}/${BINARY}_${VERSION}_${OS}_${ARCH}.tar.gz"

# Create temp directory
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

# Download and extract
echo "Downloading from ${DOWNLOAD_URL}..."
cd "$TMP_DIR"

if ! curl -fsSL -o "archive.tar.gz" "$DOWNLOAD_URL"; then
  echo "Error: Failed to download. Check if version v${VERSION} exists."
  exit 1
fi

tar -xzf "archive.tar.gz"

# Install binary
if [ -w "$INSTALL_DIR" ]; then
  mv "$BINARY" "$INSTALL_DIR/"
else
  echo "Installing to ${INSTALL_DIR} (requires sudo)..."
  sudo mv "$BINARY" "$INSTALL_DIR/"
fi

# Verify installation
if command -v "$BINARY" > /dev/null 2>&1; then
  echo ""
  echo "Successfully installed meow v${VERSION}!"
  echo ""
  "$BINARY" --version
else
  echo ""
  echo "Installed to ${INSTALL_DIR}/${BINARY}"
  echo "Make sure ${INSTALL_DIR} is in your PATH"
fi
