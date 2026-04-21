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

URL="https://github.com/${REPO}/releases/download/${LATEST}/apod_${OS}_${ARCH}.tar.gz"

echo "Downloading apod ${LATEST} for ${OS}/${ARCH}..."
TMP=$(mktemp -d)
curl -fsSL "$URL" -o "${TMP}/apod.tar.gz"
tar -xzf "${TMP}/apod.tar.gz" -C "$TMP"

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
    curl -fsSL "https://raw.githubusercontent.com/${REPO}/master/drivers/${f}" -o "/etc/apod/drivers/${f}" 2>/dev/null && echo "  ✓ ${f}" || echo "  ✗ ${f}"
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
echo "Done! Start the server:"
echo "  apod server --acme-email you@example.com"
echo ""
echo "Or set up as a service:"
echo "  cat > /etc/systemd/system/apod.service << 'EOF'"
echo "  [Unit]"
echo "  Description=apod server"
echo "  After=docker.service"
echo "  Requires=docker.service"
echo "  [Service]"
echo "  ExecStart=/usr/local/bin/apod server --acme-email you@example.com"
echo "  Restart=always"
echo "  [Install]"
echo "  WantedBy=multi-user.target"
echo "  EOF"
echo "  systemctl enable --now apod"
