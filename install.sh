#!/bin/sh
set -e

REPO="aystro-com/apod"
BINARY="apod"
INSTALL_DIR="/usr/local/bin"

# Detect architecture
ARCH=$(uname -m)
case "$ARCH" in
  x86_64)  ARCH="amd64" ;;
  aarch64) ARCH="arm64" ;;
  arm64)   ARCH="arm64" ;;
  *)       echo "Unsupported architecture: $ARCH"; exit 1 ;;
esac

OS="linux"

# Get latest release tag
LATEST=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name"' | cut -d'"' -f4)

if [ -z "$LATEST" ]; then
  echo "Failed to fetch latest release"
  exit 1
fi

URL="https://github.com/${REPO}/releases/download/${LATEST}/apod_${OS}_${ARCH}.tar.gz"

echo "Downloading apod ${LATEST} for ${OS}/${ARCH}..."
TMP=$(mktemp -d)
curl -fsSL "$URL" -o "${TMP}/apod.tar.gz"
tar -xzf "${TMP}/apod.tar.gz" -C "$TMP"

echo "Installing to ${INSTALL_DIR}..."
sudo mv "${TMP}/${BINARY}" "${INSTALL_DIR}/${BINARY}"
sudo chmod +x "${INSTALL_DIR}/${BINARY}"
rm -rf "$TMP"

echo "apod ${LATEST} installed successfully"
