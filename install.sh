#!/usr/bin/env sh
# install.sh — download and install the latest adbx release
# Usage: curl -fsSL https://github.com/imvaskii/adbx/releases/latest/download/install.sh | sh
set -e

REPO="imvaskii/adbx"
BINARY="adbx"

# ---------------------------------------------------------------------------
# Detect platform
# ---------------------------------------------------------------------------
OS=$(uname -s)
ARCH=$(uname -m)

case "$OS" in
  Darwin) PLATFORM="darwin" ;;
  Linux)  PLATFORM="linux"  ;;
  *)
    echo "error: unsupported OS: $OS" >&2
    exit 1
    ;;
esac

case "$ARCH" in
  x86_64)        ARCH_SUFFIX="amd64" ;;
  arm64|aarch64) ARCH_SUFFIX="arm64" ;;
  *)
    echo "error: unsupported architecture: $ARCH" >&2
    exit 1
    ;;
esac

if [ "$PLATFORM" = "linux" ] && [ "$ARCH_SUFFIX" = "arm64" ]; then
  echo "error: no pre-built binary for linux/arm64." >&2
  echo "  Install from source: go install github.com/imvaskii/adbx@latest" >&2
  exit 1
fi

ASSET="${BINARY}-${PLATFORM}-${ARCH_SUFFIX}"

# ---------------------------------------------------------------------------
# Resolve download URL from GitHub Releases API
# ---------------------------------------------------------------------------
RELEASES_URL="https://api.github.com/repos/${REPO}/releases/latest"

if command -v curl >/dev/null 2>&1; then
  RELEASE_JSON=$(curl -fsSL "$RELEASES_URL")
elif command -v wget >/dev/null 2>&1; then
  RELEASE_JSON=$(wget -qO- "$RELEASES_URL")
else
  echo "error: curl or wget is required" >&2
  exit 1
fi

# Extract the download URL for the matching asset
DOWNLOAD_URL=$(printf '%s' "$RELEASE_JSON" \
  | grep '"browser_download_url"' \
  | grep "/${ASSET}\"" \
  | sed 's/.*"browser_download_url": *"\([^"]*\)".*/\1/' \
  | head -1)

if [ -z "$DOWNLOAD_URL" ]; then
  echo "error: could not find release asset '${ASSET}'" >&2
  echo "  Check: https://github.com/${REPO}/releases/latest" >&2
  exit 1
fi

# ---------------------------------------------------------------------------
# Find a user-writable install directory that is already in PATH
# ---------------------------------------------------------------------------
find_install_dir() {
  for candidate in "$HOME/.local/bin" "$HOME/bin" "$HOME/.bin"; do
    case ":$PATH:" in
      *":$candidate:"*)
        printf '%s' "$candidate"
        return
        ;;
    esac
  done
  printf ''
}

INSTALL_DIR=$(find_install_dir)
WARN_PATH=0

if [ -z "$INSTALL_DIR" ]; then
  INSTALL_DIR="$HOME/.local/bin"
  WARN_PATH=1
fi

mkdir -p "$INSTALL_DIR"

# ---------------------------------------------------------------------------
# Download, strip quarantine (macOS), install
# ---------------------------------------------------------------------------
TMP=$(mktemp)
# Ensure temp file is cleaned up on exit
trap 'rm -f "$TMP"' EXIT

printf 'Downloading %s ...\n' "$ASSET"

if command -v curl >/dev/null 2>&1; then
  curl -fsSL "$DOWNLOAD_URL" -o "$TMP"
else
  wget -qO "$TMP" "$DOWNLOAD_URL"
fi

if [ "$PLATFORM" = "darwin" ]; then
  xattr -d com.apple.quarantine "$TMP" 2>/dev/null || true
fi

chmod +x "$TMP"
mv "$TMP" "${INSTALL_DIR}/${BINARY}"

printf 'Installed %s -> %s/%s\n' "$BINARY" "$INSTALL_DIR" "$BINARY"

if [ "$WARN_PATH" = "1" ]; then
  printf '\n'
  printf '  warning: %s is not in your PATH\n' "$INSTALL_DIR"
  printf '  Add it to your shell profile:\n'
  printf '    export PATH="$HOME/.local/bin:$PATH"\n'
fi
