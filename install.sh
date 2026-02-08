#!/bin/bash
# LogBuddy One-Line Installer
# Usage: curl -fsSL https://logbuddy.dev/install | sudo bash
set -e

REPO="karloscodes/logbuddy"
BINARY_NAME="logbuddy"
INSTALL_PATH="/usr/local/bin/${BINARY_NAME}"

echo "LogBuddy Installer"
echo "=================="
echo ""

# Check root
if [ "$(id -u)" -ne 0 ]; then
  echo "Error: This script must be run as root (use sudo)"
  exit 1
fi

# Detect architecture
ARCH=$(uname -m)
case "$ARCH" in
  x86_64)  ARCH="amd64" ;;
  aarch64) ARCH="arm64" ;;
  arm64)   ARCH="arm64" ;;
  *)
    echo "Error: Unsupported architecture: $ARCH"
    exit 1
    ;;
esac

OS=$(uname -s | tr '[:upper:]' '[:lower:]')
if [ "$OS" != "linux" ]; then
  echo "Error: This installer is for Linux only (detected: $OS)"
  exit 1
fi

ASSET_NAME="${BINARY_NAME}-${OS}-${ARCH}"

echo "Detected: ${OS}/${ARCH}"
echo "Downloading latest release..."

# Get latest release URL from GitHub
DOWNLOAD_URL=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" | \
  grep "browser_download_url.*${ASSET_NAME}" | \
  head -1 | \
  cut -d '"' -f 4)

if [ -z "$DOWNLOAD_URL" ]; then
  echo "Error: Could not find release for ${ASSET_NAME}"
  exit 1
fi

# Download and install
curl -fsSL "$DOWNLOAD_URL" -o "$INSTALL_PATH"
chmod +x "$INSTALL_PATH"

echo ""
echo "Installed ${BINARY_NAME} to ${INSTALL_PATH}"
echo ""
echo "Run 'logbuddy install' to set up LogBuddy on this server."
echo ""

# Auto-run install if interactive
if [ -t 0 ]; then
  read -p "Run installation now? [Y/n] " -n 1 -r
  echo
  if [[ ! $REPLY =~ ^[Nn]$ ]]; then
    exec "$INSTALL_PATH" install
  fi
fi
