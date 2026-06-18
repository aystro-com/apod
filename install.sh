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

# Get latest release tag (fall back to pre-releases if no stable release)
LATEST=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" 2>/dev/null | grep '"tag_name"' | cut -d'"' -f4)

if [ -z "$LATEST" ]; then
  LATEST=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases" | grep '"tag_name"' | head -1 | cut -d'"' -f4)
fi

if [ -z "$LATEST" ]; then
  echo "Failed to fetch latest release"
  exit 1
fi

ASSET="apod_${OS}_${ARCH}.tar.gz"
URL="https://github.com/${REPO}/releases/download/${LATEST}/${ASSET}"
CHECKSUMS_URL="https://github.com/${REPO}/releases/download/${LATEST}/checksums.txt"

echo "Downloading apod ${LATEST} for ${OS}/${ARCH}..."
TMP=$(mktemp -d)
curl -fsSL "$URL" -o "${TMP}/${ASSET}"

# Verify the download against the release checksums.txt before installing.
echo "Verifying checksum..."
if curl -fsSL "$CHECKSUMS_URL" -o "${TMP}/checksums.txt" 2>/dev/null; then
  EXPECTED=$(grep " ${ASSET}\$" "${TMP}/checksums.txt" | awk '{print $1}')
  if [ -z "$EXPECTED" ]; then
    echo "ERROR: ${ASSET} not listed in checksums.txt — refusing to install."
    rm -rf "$TMP"; exit 1
  fi
  if command -v sha256sum >/dev/null 2>&1; then
    ACTUAL=$(sha256sum "${TMP}/${ASSET}" | awk '{print $1}')
  elif command -v shasum >/dev/null 2>&1; then
    ACTUAL=$(shasum -a 256 "${TMP}/${ASSET}" | awk '{print $1}')
  else
    echo "ERROR: no sha256sum/shasum available to verify download — refusing to install."
    rm -rf "$TMP"; exit 1
  fi
  if [ "$EXPECTED" != "$ACTUAL" ]; then
    echo "ERROR: checksum mismatch for ${ASSET}"
    echo "  expected: $EXPECTED"
    echo "  actual:   $ACTUAL"
    rm -rf "$TMP"; exit 1
  fi
  echo "  ✓ checksum verified"
else
  echo "ERROR: could not download checksums.txt — refusing to install unverified binary."
  rm -rf "$TMP"; exit 1
fi

tar -xzf "${TMP}/${ASSET}" -C "$TMP"

echo "Installing to ${INSTALL_DIR}..."
mv "${TMP}/${BINARY}" "${INSTALL_DIR}/${BINARY}" 2>/dev/null || sudo mv "${TMP}/${BINARY}" "${INSTALL_DIR}/${BINARY}"
chmod +x "${INSTALL_DIR}/${BINARY}" 2>/dev/null || sudo chmod +x "${INSTALL_DIR}/${BINARY}"
rm -rf "$TMP"

echo "apod ${LATEST} installed successfully"

# Create required directories
mkdir -p /etc/apod/drivers /var/lib/apod /etc/apod/traefik/dynamic

# Download built-in drivers (no server needed)
echo ""
echo "Downloading drivers..."
DRIVERS_URL="https://api.github.com/repos/${REPO}/contents/drivers"
DRIVER_FILES=$(curl -fsSL "$DRIVERS_URL" 2>/dev/null | grep '"name"' | grep '.yaml"' | cut -d'"' -f4)

if [ -n "$DRIVER_FILES" ]; then
  for f in $DRIVER_FILES; do
    # Pin to the released tag (not the mutable master branch) so drivers match
    # the installed binary and can't shift under us between fetches.
    curl -fsSL "https://raw.githubusercontent.com/${REPO}/${LATEST}/drivers/${f}" -o "/etc/apod/drivers/${f}" 2>/dev/null && echo "  ✓ ${f}" || echo "  ✗ ${f}"
  done
else
  echo "  Could not fetch driver list, skipping"
fi

# Check for Docker
if ! command -v docker >/dev/null 2>&1; then
  echo ""
  echo "⚠  Docker not found. Install it:"
  echo "   curl -fsSL https://get.docker.com | sh"
fi

# Check for Docker Compose
if ! docker compose version >/dev/null 2>&1; then
  echo ""
  echo "⚠  Docker Compose not found. Install it:"
  echo "   apt install docker-compose-plugin"
  echo "   # or"
  echo "   mkdir -p /usr/local/lib/docker/cli-plugins"
  echo "   curl -SL https://github.com/docker/compose/releases/latest/download/docker-compose-linux-$(uname -m) -o /usr/local/lib/docker/cli-plugins/docker-compose"
  echo "   chmod +x /usr/local/lib/docker/cli-plugins/docker-compose"
fi

echo ""
echo "apod ${LATEST} installed."

# Flow straight into setup. `apod init` is interactive, so read from the
# terminal explicitly — when this script is run via `curl ... | sh`, stdin is
# the piped script, not the TTY. Skip auto-init when Docker is missing or there
# is no terminal (e.g. CI); the user can run `apod init` later.
if ! command -v docker >/dev/null 2>&1; then
  echo "Install Docker (above), then finish setup with:  apod init"
elif [ -r /dev/tty ]; then
  echo "Finishing setup..."
  echo ""
  apod init < /dev/tty
else
  echo "Run 'apod init' to finish setup (SSL email, system service, optional web UI)."
fi
