#!/usr/bin/env bash
# ==============================================================================
# ShadowStar Server Zero-Downtime Updater
# ==============================================================================

set -e

RED='\033[0;31m'
GREEN='\033[0;32m'
CYAN='\033[0;36m'
YELLOW='\033[1;33m'
NC='\033[0m'

echo -e "${CYAN}[*] Starting ShadowStar Server Update...${NC}"

if [ "$EUID" -ne 0 ]; then
    echo -e "${RED}[Error] Please run as root (sudo ./update.sh)${NC}"
    exit 1
fi

INSTALL_BIN="/usr/local/bin/shadowstar-server"
BACKUP_BIN="/usr/local/bin/shadowstar-server.bak"

# Detect Architecture
ARCH=$(uname -m)
case "$ARCH" in
    x86_64)
        BIN_ARCH="linux-amd64"
        ;;
    aarch64|arm64)
        BIN_ARCH="linux-arm64"
        ;;
    *)
        echo -e "${RED}[Error] Unsupported architecture: $ARCH${NC}"
        exit 1
        ;;
esac

# 1. Backup existing binary
if [ -f "$INSTALL_BIN" ]; then
    cp "$INSTALL_BIN" "$BACKUP_BIN"
    echo -e "${GREEN}[+] Created backup: $BACKUP_BIN${NC}"
fi

# 2. Source new binary
NEW_BIN_SOURCE=""
SCRIPT_DIR="$(dirname "$0")"

if [ -f "$SCRIPT_DIR/../bin/shadowstar-server-$BIN_ARCH" ]; then
    NEW_BIN_SOURCE="$SCRIPT_DIR/../bin/shadowstar-server-$BIN_ARCH"
elif [ -f "$SCRIPT_DIR/shadowstar-server-$BIN_ARCH" ]; then
    NEW_BIN_SOURCE="$SCRIPT_DIR/shadowstar-server-$BIN_ARCH"
elif [ -f "./shadowstar-server-$BIN_ARCH" ]; then
    NEW_BIN_SOURCE="./shadowstar-server-$BIN_ARCH"
fi

if [ -n "$NEW_BIN_SOURCE" ]; then
    echo -e "${CYAN}[*] Applying update from $NEW_BIN_SOURCE...${NC}"
    cp "$NEW_BIN_SOURCE" "$INSTALL_BIN.new"
    chmod +x "$INSTALL_BIN.new"
else
    # Remote update check (if git repository / URL is configured)
    echo -e "${CYAN}[*] Fetching latest release...${NC}"
    # Example remote URL endpoint
    DOWNLOAD_URL="https://github.com/shadowstar/shadowstar/releases/latest/download/shadowstar-server-$BIN_ARCH"
    if curl -sLf "$DOWNLOAD_URL" -o "$INSTALL_BIN.new"; then
        chmod +x "$INSTALL_BIN.new"
    else
        echo -e "${RED}[Error] Failed to download or find new binary. Update aborted.${NC}"
        exit 1
    fi
fi

# 3. Test integrity of new binary
echo -e "${CYAN}[*] Verifying new binary integrity...${NC}"
if "$INSTALL_BIN.new" -version > /dev/null 2>&1; then
    mv "$INSTALL_BIN.new" "$INSTALL_BIN"
    echo -e "${GREEN}[+] Binary verification passed!${NC}"
else
    echo -e "${RED}[Error] Verification failed! Rolling back to backup...${NC}"
    rm -f "$INSTALL_BIN.new"
    if [ -f "$BACKUP_BIN" ]; then
        mv "$BACKUP_BIN" "$INSTALL_BIN"
    fi
    exit 1
fi

# 4. Graceful restart
echo -e "${CYAN}[*] Restarting shadowstar.service...${NC}"
systemctl restart shadowstar.service

if systemctl is-active --quiet shadowstar.service; then
    echo -e "${GREEN}================================================================${NC}"
    echo -e "${GREEN}  ⭐ ShadowStar Server updated and restarted successfully!      ${NC}"
    echo -e "${GREEN}================================================================${NC}"
    "$INSTALL_BIN" -version
else
    echo -e "${RED}[Error] Service failed to restart. Check: journalctl -u shadowstar.service${NC}"
    if [ -f "$BACKUP_BIN" ]; then
        echo -e "${YELLOW}[!] Restoring backup binary...${NC}"
        mv "$BACKUP_BIN" "$INSTALL_BIN"
        systemctl restart shadowstar.service
    fi
    exit 1
fi
