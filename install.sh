#!/usr/bin/env sh
set -e

REPO="radhe-singh/flowtest"
BINARY="flowtest"
INSTALL_DIR="${FLOWTEST_INSTALL_DIR:-}"

# Colors (only when stdout is a terminal)
if [ -t 1 ]; then
  BOLD="\033[1m"
  GREEN="\033[0;32m"
  RED="\033[0;31m"
  YELLOW="\033[1;33m"
  RESET="\033[0m"
else
  BOLD="" GREEN="" RED="" YELLOW="" RESET=""
fi

info()    { printf "${BOLD}%s${RESET}\n" "$1"; }
success() { printf "${GREEN}✓${RESET} %s\n" "$1"; }
warn()    { printf "${YELLOW}!${RESET} %s\n" "$1"; }
fatal()   { printf "${RED}✗${RESET} %s\n" "$1" >&2; exit 1; }

# ── Detect OS ────────────────────────────────────────────────────────────────

OS="$(uname -s 2>/dev/null | tr '[:upper:]' '[:lower:]')"
case "$OS" in
  linux)  OS="linux"  ;;
  darwin) OS="darwin" ;;
  *)      fatal "Unsupported OS: $OS. Download manually from https://github.com/${REPO}/releases" ;;
esac

ARCH="$(uname -m 2>/dev/null)"
case "$ARCH" in
  x86_64)          ARCH="amd64" ;;
  aarch64 | arm64) ARCH="arm64" ;;
  *)               fatal "Unsupported architecture: $ARCH. Download manually from https://github.com/${REPO}/releases" ;;
esac

# ── Pick install directory ────────────────────────────────────────────────────

if [ -z "$INSTALL_DIR" ]; then
  if [ -w "/usr/local/bin" ] 2>/dev/null; then
    INSTALL_DIR="/usr/local/bin"
  elif echo "$PATH" | grep -q "$HOME/.local/bin"; then
    INSTALL_DIR="$HOME/.local/bin"
    mkdir -p "$INSTALL_DIR"
  else
    INSTALL_DIR="$HOME/.local/bin"
    mkdir -p "$INSTALL_DIR"
    warn "Adding $INSTALL_DIR to PATH. Add this to your shell profile:"
    warn "  export PATH=\"\$HOME/.local/bin:\$PATH\""
  fi
fi

# ── Check for required tools ──────────────────────────────────────────────────

need() {
  command -v "$1" >/dev/null 2>&1 || fatal "$1 is required but not installed"
}

if command -v curl >/dev/null 2>&1; then
  DOWNLOADER="curl"
elif command -v wget >/dev/null 2>&1; then
  DOWNLOADER="wget"
else
  fatal "curl or wget is required to download FlowTest"
fi

need tar

# ── Resolve latest version ────────────────────────────────────────────────────

info "Fetching latest FlowTest release..."

if [ "$DOWNLOADER" = "curl" ]; then
  VERSION=$(curl -sSfL "https://api.github.com/repos/${REPO}/releases/latest" \
    | grep '"tag_name"' \
    | sed -E 's/.*"tag_name": *"([^"]+)".*/\1/')
else
  VERSION=$(wget -qO- "https://api.github.com/repos/${REPO}/releases/latest" \
    | grep '"tag_name"' \
    | sed -E 's/.*"tag_name": *"([^"]+)".*/\1/')
fi

[ -z "$VERSION" ] && fatal "Could not determine latest version. Check https://github.com/${REPO}/releases"

# GoReleaser strips the leading 'v' from the archive filename but keeps it in the tag
VERSION_NUM="${VERSION#v}"
ARCHIVE="${BINARY}_${VERSION_NUM}_${OS}_${ARCH}.tar.gz"
URL="https://github.com/${REPO}/releases/download/${VERSION}/${ARCHIVE}"

success "Latest version: $VERSION"

# ── Download and install ──────────────────────────────────────────────────────

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT INT TERM

info "Downloading $ARCHIVE..."

if [ "$DOWNLOADER" = "curl" ]; then
  curl -sSfL "$URL" -o "$TMP/$ARCHIVE" || fatal "Download failed: $URL"
else
  wget -qO "$TMP/$ARCHIVE" "$URL" || fatal "Download failed: $URL"
fi

tar -xzf "$TMP/$ARCHIVE" -C "$TMP"

# Verify the binary extracted correctly
[ -f "$TMP/$BINARY" ] || fatal "Binary not found in archive. Please report this at https://github.com/${REPO}/issues"

chmod +x "$TMP/$BINARY"
mv "$TMP/$BINARY" "$INSTALL_DIR/$BINARY"

# ── Verify installation ───────────────────────────────────────────────────────

INSTALLED_VERSION="$("$INSTALL_DIR/$BINARY" --version 2>&1 | head -1)"
success "Installed: $INSTALL_DIR/$BINARY  ($INSTALLED_VERSION)"

# ── Next steps ────────────────────────────────────────────────────────────────

printf "\n${BOLD}Get started:${RESET}\n"
printf "  mkdir my-tests && cd my-tests\n"
printf "  $BINARY init\n"
printf "  $BINARY run flows/example.yaml\n"
printf "\n  Docs: https://github.com/${REPO}#readme\n\n"
