#!/bin/bash
set -euo pipefail

GREEN='\033[0;32m'
DIM='\033[2m'
RESET='\033[0m'

echo ""
echo "OpenBoot Installer"
echo "=================="
echo ""

sleep 0.3
echo -e "${GREEN}✓${RESET} Xcode Command Line Tools ready"

sleep 0.3
echo -e "${GREEN}✓${RESET} Homebrew ready"

sleep 0.2
echo ""
echo -e "${DIM}Detected: darwin/arm64${RESET}"
sleep 0.3
echo "Downloading OpenBoot v0.20.0..."
sleep 0.8
echo -e "${GREEN}✓${RESET} Downloaded to ~/.openboot/bin/openboot"
sleep 0.2
echo -e "${GREEN}✓${RESET} Checksum verified"
sleep 0.2
echo -e "${GREEN}✓${RESET} Added to PATH"
echo ""

sleep 0.5

exec /tmp/openboot-demo --dry-run
