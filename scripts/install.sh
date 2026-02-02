#!/bin/bash
set -euo pipefail

VERSION="${OPENBOOT_VERSION:-latest}"
REPO="openbootdotdev/openboot"
BINARY_NAME="openboot"
INSTALL_DIR="${OPENBOOT_INSTALL_DIR:-/tmp}"

detect_arch() {
    local arch
    arch=$(uname -m)
    case "$arch" in
        x86_64)  echo "amd64" ;;
        arm64)   echo "arm64" ;;
        aarch64) echo "arm64" ;;
        *)       echo "unsupported: $arch" >&2; exit 1 ;;
    esac
}

detect_os() {
    local os
    os=$(uname -s | tr '[:upper:]' '[:lower:]')
    case "$os" in
        darwin) echo "darwin" ;;
        linux)  echo "linux" ;;
        *)      echo "unsupported: $os" >&2; exit 1 ;;
    esac
}

get_download_url() {
    local os="$1"
    local arch="$2"
    
    if [[ "$VERSION" == "latest" ]]; then
        echo "https://github.com/${REPO}/releases/latest/download/${BINARY_NAME}-${os}-${arch}"
    else
        echo "https://github.com/${REPO}/releases/download/${VERSION}/${BINARY_NAME}-${os}-${arch}"
    fi
}

main() {
    echo ""
    echo "OpenBoot Installer"
    echo "=================="
    echo ""

    local os arch url binary_path
    os=$(detect_os)
    arch=$(detect_arch)
    url=$(get_download_url "$os" "$arch")
    binary_path="${INSTALL_DIR}/${BINARY_NAME}"

    echo "Detected: ${os}/${arch}"
    echo "Downloading OpenBoot..."
    
    if ! curl -fsSL "$url" -o "$binary_path"; then
        echo ""
        echo "Error: Failed to download OpenBoot"
        echo "URL: $url"
        echo ""
        echo "If this is a new installation, releases may not be available yet."
        echo "Please check: https://github.com/${REPO}/releases"
        exit 1
    fi

    chmod +x "$binary_path"
    
    echo "Running OpenBoot..."
    echo ""
    
    exec "$binary_path" "$@"
}

main "$@"
