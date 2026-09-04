#!/usr/bin/env bash
# ==============================================================================
# ⚡ Flagura CLI Installer
# One-line installer for Flagura high-performance feature flag platform CLI.
# Usage:
#   curl -sSL https://flagura.dev/install.sh | bash
# ==============================================================================

set -euo pipefail

# ANSI color codes
BOLD="\033[1m"
GREEN="\033[32m"
CYAN="\033[36m"
YELLOW="\033[33m"
RED="\033[31m"
RESET="\033[0m"

log_info() {
    echo -e "${CYAN}==>${RESET} ${BOLD}$1${RESET}"
}

log_success() {
    echo -e "${GREEN}✓${RESET} ${BOLD}$1${RESET}"
}

log_warn() {
    echo -e "${YELLOW}Warning:${RESET} $1"
}

log_error() {
    echo -e "${RED}Error:${RESET} $1" >&2
}

echo -e "${BOLD}"
echo "    ______                               "
echo "   / ____/___ _____ ___  ______ _____ _ "
echo "  / /_  / __ \`/ __ \`/ / / / __ \`/ __ \`/"
echo " / __/ / /_/ / /_/ / /_/ / /_/ / /_/ /  "
echo "/_/    \__,_/\__, /\__,_/\__,_/\__,_/   "
echo "            /____/                      "
echo -e "${RESET}"
echo -e "  ⚡ Installing Flagura Developer CLI..."
echo ""

# 1. Detect Operating System
OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
case "$OS" in
    darwin)
        OS="darwin"
        ;;
    linux)
        OS="linux"
        ;;
    *)
        log_error "Unsupported operating system: $OS"
        echo "   Flagura CLI currently provides precompiled binaries for macOS and Linux."
        echo "   For Windows, please install via Go:"
        echo "     go install github.com/dhawalhost/flagura/cmd/cli@latest"
        exit 1
        ;;
esac

# 2. Detect CPU Architecture
ARCH="$(uname -m)"
case "$ARCH" in
    x86_64|amd64)
        ARCH="amd64"
        ;;
    arm64|aarch64)
        ARCH="arm64"
        ;;
    *)
        log_error "Unsupported CPU architecture: $ARCH"
        echo "   Please install using the Go toolchain:"
        echo "     go install github.com/dhawalhost/flagura/cmd/cli@latest"
        exit 1
        ;;
esac

# 3. Resolve Target Version
VERSION="${FLAGURA_VERSION:-}"
if [ -z "$VERSION" ]; then
    log_info "Detecting latest release tag..."
    LATEST_TAG=$(curl -sSL -H "Accept: application/vnd.github.v3+json" "https://api.github.com/repos/dhawalhost/flagura/releases/latest" 2>/dev/null | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/' || true)
    if [ -n "$LATEST_TAG" ]; then
        VERSION="$LATEST_TAG"
    else
        VERSION="v1.6.0"
    fi
fi

# Ensure version has 'v' prefix
if [[ ! "$VERSION" =~ ^v ]]; then
    VERSION="v${VERSION}"
fi
CLEAN_VERSION="${VERSION#v}"

log_info "Targeting Flagura CLI ${VERSION} (${OS}/${ARCH})"

# 4. Prepare Temporary Directory
TMP_DIR="$(mktemp -d 2>/dev/null || mktemp -d -t 'flagura-install')"
cleanup() {
    rm -rf "$TMP_DIR"
}
trap cleanup EXIT

# 5. Download Release Archive
log_info "Fetching Flagura binary..."

# Candidates for GoReleaser naming format
TAR_CANDIDATE_1="flagura_v${CLEAN_VERSION}_${OS}_${ARCH}.tar.gz"
TAR_CANDIDATE_2="flagura_${CLEAN_VERSION}_${OS}_${ARCH}.tar.gz"
TAR_CANDIDATE_3="flagura_${VERSION}_${OS}_${ARCH}.tar.gz"

DOWNLOADED=false
for TAR_NAME in "$TAR_CANDIDATE_1" "$TAR_CANDIDATE_2" "$TAR_CANDIDATE_3"; do
    DOWNLOAD_URL="https://github.com/dhawalhost/flagura/releases/download/${VERSION}/${TAR_NAME}"
    HTTP_CODE=$(curl -sSL -w "%{http_code}" -o "$TMP_DIR/$TAR_NAME" "$DOWNLOAD_URL" 2>/dev/null || echo "000")
    if [ "$HTTP_CODE" = "200" ]; then
        if tar -xzf "$TMP_DIR/$TAR_NAME" -C "$TMP_DIR" 2>/dev/null; then
            DOWNLOADED=true
            break
        fi
    fi
done

# Fallback to Go build if prebuilt release tarball is not yet available
if [ "$DOWNLOADED" = false ]; then
    if command -v go >/dev/null 2>&1; then
        log_warn "Prebuilt GitHub binary release not found for ${VERSION}."
        log_info "Building Flagura CLI from source via local Go toolchain..."
        GOBIN="$TMP_DIR" go install -ldflags="-s -w -X main.version=${VERSION}" github.com/dhawalhost/flagura/cmd/cli@latest
        if [ -f "$TMP_DIR/cli" ]; then
            mv "$TMP_DIR/cli" "$TMP_DIR/flagura"
            DOWNLOADED=true
        fi
    else
        log_error "Failed to download Flagura binary from GitHub Releases for version ${VERSION}."
        echo "   URL tried: https://github.com/dhawalhost/flagura/releases/download/${VERSION}/${TAR_CANDIDATE_1}"
        echo "   Please check https://github.com/dhawalhost/flagura/releases or install via Go:"
        echo "     go install github.com/dhawalhost/flagura/cmd/cli@latest"
        exit 1
    fi
fi

# Verify extracted or built binary exists
if [ ! -f "$TMP_DIR/flagura" ]; then
    log_error "Flagura binary was not found in release archive."
    exit 1
fi
chmod +x "$TMP_DIR/flagura"

# 6. Determine Target Installation Directory
INSTALL_DIR=""
SUDO=""

if [ -n "${FLAGURA_INSTALL_DIR:-}" ]; then
    INSTALL_DIR="$FLAGURA_INSTALL_DIR"
    mkdir -p "$INSTALL_DIR"
elif [ -w "/usr/local/bin" ]; then
    INSTALL_DIR="/usr/local/bin"
elif command -v sudo >/dev/null 2>&1 && sudo -n true 2>/dev/null; then
    INSTALL_DIR="/usr/local/bin"
    SUDO="sudo"
elif mkdir -p "$HOME/.local/bin" 2>/dev/null && [ -d "$HOME/.local/bin" ]; then
    INSTALL_DIR="$HOME/.local/bin"
else
    INSTALL_DIR="/usr/local/bin"
    SUDO="sudo"
fi

log_info "Installing flagura binary to ${INSTALL_DIR}..."

if [ -n "$SUDO" ]; then
    echo "   (Root privileges required to install to ${INSTALL_DIR})"
    $SUDO cp "$TMP_DIR/flagura" "$INSTALL_DIR/flagura"
    $SUDO chmod 755 "$INSTALL_DIR/flagura"
else
    cp "$TMP_DIR/flagura" "$INSTALL_DIR/flagura"
    chmod 755 "$INSTALL_DIR/flagura"
fi

# 7. Verification & Setup Guidance
echo ""
log_success "Flagura CLI installed successfully to ${INSTALL_DIR}/flagura!"
echo ""

# Check if INSTALL_DIR is in PATH
if ! echo ":$PATH:" | grep -q ":$INSTALL_DIR:"; then
    log_warn "${INSTALL_DIR} is not in your current PATH environment variable."
    echo "   Add the following line to your shell profile (~/.zshrc or ~/.bashrc):"
    echo -e "   ${BOLD}export PATH=\"${INSTALL_DIR}:\$PATH\"${RESET}"
    echo ""
fi

# Run version check
INSTALLED_VERSION=$("$INSTALL_DIR/flagura" --version 2>/dev/null || echo "Flagura CLI")
echo -e "   Installed: ${GREEN}${INSTALLED_VERSION}${RESET}"
echo ""
echo -e "${BOLD}Quickstart Commands:${RESET}"
echo -e "  1. Check CLI options:        ${CYAN}flagura --help${RESET}"
echo -e "  2. Connect to Flagura:       ${CYAN}export FLAGURA_API_KEY=\"flg_live_your_key\"${RESET}"
echo -e "  3. List feature flags:       ${CYAN}flagura list${RESET}"
echo -e "  4. Adjust rollout percent:   ${CYAN}flagura rollout ai-smart-search 50% --env=production${RESET}"
echo ""
