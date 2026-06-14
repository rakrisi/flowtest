#!/usr/bin/env sh
set -e

REPO="rakrisi/flowtest"
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

# ── Pick install directory ──────────────────────────────────────────────────

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

# ── Check for required tools ────────────────────────────────────────────────

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

# ── Resolve latest version ──────────────────────────────────────────────────

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

# GoReleaser v2 strips the leading 'v' from the archive filename
VERSION_NUM="${VERSION#v}"
ARCHIVE="${BINARY}_${VERSION_NUM}_${OS}_${ARCH}.tar.gz"
URL="https://github.com/${REPO}/releases/download/${VERSION}/${ARCHIVE}"

success "Latest version: $VERSION"

# ── Download and install ────────────────────────────────────────────────────

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT INT TERM

info "Downloading $ARCHIVE..."

DOWNLOAD_FAILED=0
if [ "$DOWNLOADER" = "curl" ]; then
  if ! curl -sSfL "$URL" -o "$TMP/$ARCHIVE"; then
    warn "Download failed: $URL"
    DOWNLOAD_FAILED=1
  fi
else
  if ! wget -qO "$TMP/$ARCHIVE" "$URL"; then
    warn "Download failed: $URL"
    DOWNLOAD_FAILED=1
  fi
fi

if [ "$DOWNLOAD_FAILED" -eq 0 ]; then
  if tar -xzf "$TMP/$ARCHIVE" -C "$TMP"; then
    # The binary may be at the root OR inside a subdirectory
    # (goreleaser v1 wraps archives in a dir by default; v2 does not)
    BINARY_PATH=$(find "$TMP" -maxdepth 2 -name "$BINARY" -type f 2>/dev/null | head -1)
    if [ -n "$BINARY_PATH" ] && [ -f "$BINARY_PATH" ]; then
      chmod +x "$BINARY_PATH"
      mv "$BINARY_PATH" "$INSTALL_DIR/$BINARY"
      success "Installed $INSTALL_DIR/$BINARY  ($VERSION)"
      INSTALLED_FROM="release"
    else
      warn "Binary not found in archive after extraction"
      warn "Contents: $(tar -tzf "$TMP/$ARCHIVE" 2>/dev/null | head -20)"
      DOWNLOAD_FAILED=1
    fi
  else
    warn "Failed to extract archive: $TMP/$ARCHIVE"
    DOWNLOAD_FAILED=1
  fi
fi

# Fallback: try `go install` when release asset is missing
if [ "$DOWNLOAD_FAILED" -ne 0 ]; then
  warn "Falling back to 'go install' (requires Go >=1.21)"

  if ! command -v go >/dev/null 2>&1; then
    fatal "Download failed and 'go' is not installed. Please install Go or download a release from https://github.com/${REPO}/releases"
  fi

  # Prefer @latest which always points to the newest tagged release
  GO_INSTALL_TARGET="github.com/rakrisi/flowtest/cmd/flowtest@latest"

  warn "Running: go install $GO_INSTALL_TARGET"
  if go install "$GO_INSTALL_TARGET"; then
    # Locate the installed binary (go install puts it in $GOBIN or $GOPATH/bin)
    GOBIN="$(go env GOBIN 2>/dev/null)"
    if [ -n "$GOBIN" ] && [ -f "$GOBIN/$BINARY" ]; then
      mv "$GOBIN/$BINARY" "$INSTALL_DIR/$BINARY"
    else
      GOPATH="$(go env GOPATH 2>/dev/null)"
      if [ -n "$GOPATH" ] && [ -f "$GOPATH/bin/$BINARY" ]; then
        mv "$GOPATH/bin/$BINARY" "$INSTALL_DIR/$BINARY"
      elif command -v $BINARY >/dev/null 2>&1; then
        BIN_PATH="$(command -v $BINARY)"
        mv "$BIN_PATH" "$INSTALL_DIR/$BINARY"
      else
        fatal "Binary installed by 'go install' not found in \$GOBIN or \$GOPATH/bin"
      fi
    fi
    INSTALLED_FROM="go-install"
  else
    warn "go install failed; attempting git clone + build as fallback"
    if ! command -v git >/dev/null 2>&1; then
      fatal "go install failed and git is not available. Please build manually: git clone https://github.com/${REPO}.git && cd flowtest && go build -o flowtest ./cmd/flowtest/"
    fi

    CLONE_DIR="$(mktemp -d)"
    if git clone --depth 1 "https://github.com/${REPO}.git" "$CLONE_DIR"; then
      # Try to check out the tagged version for reproducibility
      (cd "$CLONE_DIR" && git fetch --tags --quiet 2>/dev/null && git checkout "$VERSION" >/dev/null 2>&1 || true)
      if (cd "$CLONE_DIR" && go build -o "$CLONE_DIR/$BINARY" ./cmd/flowtest/); then
        mv "$CLONE_DIR/$BINARY" "$INSTALL_DIR/$BINARY"
        rm -rf "$CLONE_DIR"
        INSTALLED_FROM="source"
      else
        rm -rf "$CLONE_DIR"
        fatal "go build from source failed. Please build manually: git clone https://github.com/${REPO}.git && cd flowtest && go build -o flowtest ./cmd/flowtest/"
      fi
    else
      rm -rf "$CLONE_DIR"
      fatal "git clone failed. Please download a release from https://github.com/${REPO}/releases"
    fi
  fi
fi

# ── Verify installation ─────────────────────────────────────────────────────

if [ -f "$INSTALL_DIR/$BINARY" ]; then
  INSTALLED_VERSION="$("$INSTALL_DIR/$BINARY" --version 2>&1 | head -1)"
  success "Installed: $INSTALL_DIR/$BINARY  ($INSTALLED_VERSION)"
else
  fatal "Installation completed but binary not found at $INSTALL_DIR/$BINARY"
fi

# ── Next steps ──────────────────────────────────────────────────────────────

printf "\n${BOLD}Get started:${RESET}\n"
printf "  mkdir my-tests && cd my-tests\n"
printf "  $BINARY init\n"
printf "  $BINARY run flows/example.yaml\n"
printf "\n  Docs: https://github.com/${REPO}#readme\n\n"
