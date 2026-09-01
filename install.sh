#!/bin/bash
# Atlas Agent installer for macOS and Linux
set -e

OS=$(uname -s)
ARCH=$(uname -m)

case "$OS" in
  Linux)  PLATFORM="linux" ;;
  Darwin) PLATFORM="darwin" ;;
  *) echo "Unsupported OS: $OS" >&2 ; exit 1 ;;
esac

case "$ARCH" in
  x86_64|amd64) ARCH="x64" ;;
  arm64|aarch64) ARCH="arm64" ;;
  *) echo "Unsupported arch: $ARCH" >&2 ; exit 1 ;;
esac

REPO="Omerfaruk-aydn/Atlas-Agent"
BIN_NAME="atlas-agent-${PLATFORM}-${ARCH}"
INSTALL_DIR="${ATLAS_AGENT_INSTALL_DIR:-$HOME/.atlas-agent/bin}"

mkdir -p "$INSTALL_DIR"

URL="https://github.com/${REPO}/releases/latest/download/${BIN_NAME}"
echo "Downloading $BIN_NAME..."
curl -fsSL "$URL" -o "$INSTALL_DIR/atlas-agent"
chmod +x "$INSTALL_DIR/atlas-agent"

# Best-effort PATH update (idempotent)
case ":$PATH:" in
  *":$INSTALL_DIR:"*) ;;
  *)
    for rc in "$HOME/.zshrc" "$HOME/.bashrc" "$HOME/.profile"; do
      [ -f "$rc" ] || continue
      grep -F "$INSTALL_DIR" "$rc" >/dev/null 2>&1 || \
        echo "" >> "$rc" && echo "# Atlas Agent" >> "$rc" && echo "export PATH=\"\$PATH:$INSTALL_DIR\"" >> "$rc"
    done
    ;;
esac

echo ""
echo "✓ Installed: $INSTALL_DIR/atlas-agent"
echo "→ Restart your terminal (or: source ~/.zshrc / source ~/.bashrc)"
echo "→ Then run: atlas-agent --version"
