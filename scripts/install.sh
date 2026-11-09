#!/usr/bin/env bash
# ==============================================================================
# ShadowStar Server Installation Script
# Automated One-Click Deployment for Ubuntu / Debian / CentOS / AlmaLinux
# ==============================================================================

set -e

RED='\033[0;31m'
GREEN='\033[0;32m'
CYAN='\033[0;36m'
YELLOW='\033[1;33m'
PURPLE='\033[0;35m'
NC='\033[0m' # No Color

echo -e "${PURPLE}"
cat << "EOF"
   _____ _                 _                 _____ _               
  / ____| |               | |               / ____| |              
 | (___ | |__   __ _  __| | _____      _| (___ | |_ __ _ _ __  
  \___ \| '_ \ / _` |/ _` |/ _ \ \ /\ / / \___ \| __/ _` | '__| 
  ____) | | | | (_| | (_| | (_) \ V  V /  ____) | || (_| | |    
 |_____/|_| |_|\__,_|\__,_|\___/ \_/\_/  |_____/ \__\__,_|_|    
               Next-Gen Anti-DPI Proxy Server
EOF
echo -e "${NC}"

# Check root privileges
if [ "$EUID" -ne 0 ]; then
    echo -e "${RED}[Error] Please run this script as root (sudo ./install.sh)${NC}"
    exit 1
fi

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

echo -e "${CYAN}[*] Detected architecture:${NC} $ARCH ($BIN_ARCH)"

# Install necessary utilities
echo -e "${CYAN}[*] Installing dependencies (curl, openssl, qrencode)...${NC}"
if [ -x "$(command -v apt-get)" ]; then
    apt-get update -qq && apt-get install -y -qq curl openssl qrencode ufw > /dev/null 2>&1 || true
elif [ -x "$(command -v yum)" ]; then
    yum install -y curl openssl qrencode firewalld > /dev/null 2>&1 || true
fi

# Detect Public IP
SERVER_IP=$(curl -s4 --connect-timeout 5 https://api.ipify.org || curl -s4 --connect-timeout 5 https://ifconfig.me || echo "YOUR_SERVER_IP")
echo -e "${CYAN}[*] Detected Public IP:${NC} $SERVER_IP"

# Configuration Prompts
echo ""
echo -e "${YELLOW}--- Configuration Setup ---${NC}"
read -rp "Enter Listen Port [default: 443]: " LISTEN_PORT
LISTEN_PORT=${LISTEN_PORT:-443}

read -rp "Enter Camouflage SNI [default: www.microsoft.com]: " SNI_HOST
SNI_HOST=${SNI_HOST:-www.microsoft.com}

read -rp "Enter Fallback Decoy Target [default: www.microsoft.com:443]: " FALLBACK_TARGET
FALLBACK_TARGET=${FALLBACK_TARGET:-www.microsoft.com:443}

# Generate 32-byte cryptographic Master Key (hex encoded)
MASTER_KEY=$(openssl rand -hex 32)

echo -e "${GREEN}[+] Generated High-Entropy Master Key:${NC} $MASTER_KEY"

# Create installation directories
INSTALL_DIR="/usr/local/bin"
CONFIG_DIR="/etc/shadowstar"
mkdir -p "$CONFIG_DIR"

# Install binary
echo -e "${CYAN}[*] Installing ShadowStar Server binary...${NC}"
LOCAL_BIN="$(dirname "$0")/../bin/shadowstar-server-$BIN_ARCH"

if [ -f "$LOCAL_BIN" ]; then
    cp "$LOCAL_BIN" "$INSTALL_DIR/shadowstar-server"
else
    # Fallback to local build or repo download
    echo -e "${YELLOW}[!] Local prebuilt binary not found, checking system...${NC}"
    if [ -f "./shadowstar-server-$BIN_ARCH" ]; then
        cp "./shadowstar-server-$BIN_ARCH" "$INSTALL_DIR/shadowstar-server"
    else
        echo -e "${CYAN}[*] Compiling binary locally via Go...${NC}"
        if command -v go >/dev/null 2>&1; then
            go build -ldflags="-s -w" -o "$INSTALL_DIR/shadowstar-server" ../cmd/shadowstar-server
        else
            echo -e "${RED}[Error] Please place shadowstar-server-$BIN_ARCH in this directory.${NC}"
            exit 1
        fi
    fi
fi

chmod +x "$INSTALL_DIR/shadowstar-server"

# Write Server Config
CONFIG_FILE="$CONFIG_DIR/config.json"
cat > "$CONFIG_FILE" << EOF
{
  "listen_addr": ":$LISTEN_PORT",
  "master_key": "$MASTER_KEY",
  "fallback_target": "$FALLBACK_TARGET",
  "sni": "$SNI_HOST",
  "verbose": false
}
EOF

chmod 600 "$CONFIG_FILE"
echo -e "${GREEN}[+] Config saved to $CONFIG_FILE${NC}"

# Enable BBR and TCP Kernel Tuning
echo -e "${CYAN}[*] Applying Linux TCP BBR and high-concurrency optimizations...${NC}"
cat > /etc/sysctl.d/99-shadowstar.conf << EOF
fs.file-max = 1048576
net.core.default_qdisc = fq
net.ipv4.tcp_congestion_control = bbr
net.ipv4.tcp_fastopen = 3
net.core.rmem_max = 67108864
net.core.wmem_max = 67108864
net.ipv4.tcp_rmem = 4096 87380 33554432
net.ipv4.tcp_wmem = 4096 65536 33554432
EOF
sysctl --system > /dev/null 2>&1 || true

# Create Systemd Service
echo -e "${CYAN}[*] Creating systemd service (shadowstar.service)...${NC}"
cat > /etc/systemd/system/shadowstar.service << EOF
[Unit]
Description=ShadowStar Next-Gen Anti-DPI Proxy Server
After=network.target

[Service]
Type=simple
User=root
LimitNOFILE=1048576
ExecStart=$INSTALL_DIR/shadowstar-server -c $CONFIG_FILE
Restart=always
RestartSec=3

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable shadowstar.service
systemctl restart shadowstar.service

# Configure Firewall
if command -v ufw >/dev/null 2>&1; then
    ufw allow "$LISTEN_PORT"/tcp >/dev/null 2>&1 || true
elif command -v firewall-cmd >/dev/null 2>&1; then
    firewall-cmd --add-port="$LISTEN_PORT"/tcp --permanent >/dev/null 2>&1 || true
    firewall-cmd --reload >/dev/null 2>&1 || true
fi

# Install Global Management CLI tool
cat > /usr/local/bin/shadowstar << 'EOF'
#!/usr/bin/env bash
CONFIG="/etc/shadowstar/config.json"
case "$1" in
    status)
        systemctl status shadowstar.service
        ;;
    restart)
        systemctl restart shadowstar.service
        echo "ShadowStar restarted."
        ;;
    stop)
        systemctl stop shadowstar.service
        echo "ShadowStar stopped."
        ;;
    start)
        systemctl start shadowstar.service
        echo "ShadowStar started."
        ;;
    log|logs)
        journalctl -u shadowstar.service -f
        ;;
    link|url|show)
        cat /etc/shadowstar/client_link.txt
        ;;
    update)
        /etc/shadowstar/update.sh
        ;;
    *)
        echo "Usage: shadowstar {status|start|stop|restart|logs|link|update}"
        ;;
esac
EOF
chmod +x /usr/local/bin/shadowstar

# Copy update script to /etc/shadowstar/update.sh
if [ -f "$(dirname "$0")/update.sh" ]; then
    cp "$(dirname "$0")/update.sh" /etc/shadowstar/update.sh
    chmod +x /etc/shadowstar/update.sh
fi

# Generate Shareable shadowstar:// Link
CLIENT_PAYLOAD=$(cat << EOF
{
  "id": "shadowstar-$(date +%s)",
  "name": "ShadowStar-$SERVER_IP",
  "server_addr": "$SERVER_IP:$LISTEN_PORT",
  "master_key": "$MASTER_KEY",
  "sni": "$SNI_HOST",
  "enable_chunker": true,
  "padding_len": 64
}
EOF
)

BASE64_URL=$(echo -n "$CLIENT_PAYLOAD" | base64 -w 0 | tr '+/' '-_' | tr -d '=')
SHADOWSTAR_URI="shadowstar://${BASE64_URL}#ShadowStar-${SERVER_IP}"

echo "$SHADOWSTAR_URI" > /etc/shadowstar/client_link.txt

# Final Display
echo ""
echo -e "${GREEN}================================================================${NC}"
echo -e "${GREEN}    ⭐ SHADOWSTAR SERVER SUCCESSFULLY INSTALLED & RUNNING!      ${NC}"
echo -e "${GREEN}================================================================${NC}"
echo -e " ${CYAN}Server IP:${NC}           $SERVER_IP"
echo -e " ${CYAN}Port:${NC}                $LISTEN_PORT (TCP)"
echo -e " ${CYAN}Master Key:${NC}          $MASTER_KEY"
echo -e " ${CYAN}Camouflage SNI:${NC}      $SNI_HOST"
echo -e " ${CYAN}Fallback Decoy:${NC}      $FALLBACK_TARGET"
echo -e " ${CYAN}Anti-DPI Shield:${NC}     TLS 1.3 Reality + Dynamic Morphing + Active Probe Shield"
echo ""
echo -e "${YELLOW}>>> YOUR SHADOWSTAR CLIENT IMPORT LINK <<<${NC}"
echo -e "${CYAN}${SHADOWSTAR_URI}${NC}"
echo ""

if command -v qrencode >/dev/null 2>&1; then
    echo -e "${PURPLE}QR Code for Client Import:${NC}"
    qrencode -t ANSIUTF8 "$SHADOWSTAR_URI" || true
fi

echo -e "${GREEN}================================================================${NC}"
echo -e " Quick Server Management Commands:"
echo -e "   ${YELLOW}shadowstar status${NC}   - Check service status"
echo -e "   ${YELLOW}shadowstar logs${NC}     - View real-time logs"
echo -e "   ${YELLOW}shadowstar restart${NC}  - Restart service"
echo -e "   ${YELLOW}shadowstar link${NC}     - View client connection link"
echo -e "   ${YELLOW}shadowstar update${NC}   - Update to latest version"
echo -e "${GREEN}================================================================${NC}"
EOF
