#!/usr/bin/env bash
# test-geodesic-prebuilt.sh
# Quick test script: builds Atmos on host, then tests in Geodesic
#
# Usage: ./scripts/test-geodesic-prebuilt.sh <path-to-infrastructure>
# Example: ./scripts/test-geodesic-prebuilt.sh ~/Dev/cloudposse/infra/infra-live

set -e

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

# Usage check
if [[ $# -lt 1 ]]; then
    echo -e "${RED}Error: Missing required argument${NC}"
    echo ""
    echo "Usage: $0 <path-to-infrastructure>"
    echo ""
    echo "Example:"
    echo "  $0 ~/Dev/cloudposse/infra/infra-live"
    echo ""
    echo "This script builds Atmos for Linux and launches a Geodesic container"
    echo "with the pre-built binary mounted, along with your infrastructure directory."
    exit 1
fi

# Configuration
ATMOS_SOURCE_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
INFRA_LIVE_DIR="$1"
GEODESIC_IMAGE="${GEODESIC_IMAGE:-cloudposse/geodesic:latest-debian}"
CONTAINER_NAME="geodesic-atmos-test"

echo -e "${BLUE}=== Quick Geodesic Testing (Pre-built Binary) ===${NC}"
echo ""

# Detect container runtime (Docker or Podman) - check if actually running
CONTAINER_CMD=""

# Try Docker first
if command -v docker &> /dev/null && docker info &> /dev/null; then
    CONTAINER_CMD="docker"
    echo -e "${GREEN}✓ Using Docker${NC}"
# Try Podman if Docker isn't working
elif command -v podman &> /dev/null && podman info &> /dev/null; then
    CONTAINER_CMD="podman"
    echo -e "${GREEN}✓ Using Podman${NC}"
else
    echo -e "${RED}Error: Neither Docker nor Podman found or running${NC}"
    exit 1
fi
echo ""

# Verify directories
if [[ ! -d "${ATMOS_SOURCE_DIR}" ]]; then
    echo -e "${RED}Error: Atmos source directory not found: ${ATMOS_SOURCE_DIR}${NC}"
    exit 1
fi

if [[ ! -d "${INFRA_LIVE_DIR}" ]]; then
    echo -e "${RED}Error: Infrastructure directory not found: ${INFRA_LIVE_DIR}${NC}"
    echo -e "${YELLOW}Please provide a valid path to your infrastructure directory${NC}"
    exit 1
fi

echo -e "${GREEN}✓ Atmos source: ${ATMOS_SOURCE_DIR}${NC}"
echo -e "${GREEN}✓ Infrastructure: ${INFRA_LIVE_DIR}${NC}"
echo ""

# Build Atmos on host
echo -e "${BLUE}Building Atmos on host...${NC}"
cd "${ATMOS_SOURCE_DIR}"

if ! command -v go &> /dev/null; then
    echo -e "${RED}Error: Go is not installed on host${NC}"
    exit 1
fi

echo -e "${YELLOW}Go version: $(go version)${NC}"

# Build for Linux (Geodesic runs Linux containers)
mkdir -p "${ATMOS_SOURCE_DIR}/build"

# Delete old binary to ensure fresh build
if [[ -f "${ATMOS_SOURCE_DIR}/build/atmos-linux" ]]; then
    echo -e "${BLUE}Removing old binary...${NC}"
    rm -f "${ATMOS_SOURCE_DIR}/build/atmos-linux"
fi

# Detect host architecture to determine target
HOST_ARCH=$(uname -m)
if [[ "${HOST_ARCH}" == "arm64" ]] || [[ "${HOST_ARCH}" == "aarch64" ]]; then
    echo -e "${BLUE}Cross-compiling for Linux ARM64...${NC}"
    GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -a -o "${ATMOS_SOURCE_DIR}/build/atmos-linux" .
    PLATFORM_FLAG="--platform=linux/arm64"
else
    echo -e "${BLUE}Cross-compiling for Linux AMD64...${NC}"
    GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -a -o "${ATMOS_SOURCE_DIR}/build/atmos-linux" .
    PLATFORM_FLAG="--platform=linux/amd64"
fi

if [[ ! -f "${ATMOS_SOURCE_DIR}/build/atmos-linux" ]]; then
    echo -e "${RED}Error: Failed to build atmos binary${NC}"
    exit 1
fi

echo -e "${GREEN}✓ Atmos built: ${ATMOS_SOURCE_DIR}/build/atmos-linux${NC}"

# Output checksum for verification
if command -v md5sum &> /dev/null; then
    CHECKSUM=$(md5sum "${ATMOS_SOURCE_DIR}/build/atmos-linux" | awk '{print $1}')
elif command -v md5 &> /dev/null; then
    CHECKSUM=$(md5 -q "${ATMOS_SOURCE_DIR}/build/atmos-linux")
else
    CHECKSUM="(checksum command not available)"
fi
echo -e "${YELLOW}Binary MD5: ${CHECKSUM}${NC}"
echo ""

# Ensure mount directories exist
echo -e "${BLUE}Ensuring mount directories exist...${NC}"
mkdir -p "${HOME}/.aws"
mkdir -p "${HOME}/.config/atmos"
mkdir -p "${HOME}/.local/share/atmos/keyring"
mkdir -p "${HOME}/.cache/atmos"
echo -e "${GREEN}✓ Mount directories ready${NC}"
echo ""

# Clean up existing container (stop if running, then remove)
if ${CONTAINER_CMD} ps --format '{{.Names}}' | grep -q "^${CONTAINER_NAME}$"; then
    echo -e "${YELLOW}Stopping running container: ${CONTAINER_NAME}${NC}"
    ${CONTAINER_CMD} stop "${CONTAINER_NAME}" >/dev/null 2>&1 || true
fi
if ${CONTAINER_CMD} ps -a --format '{{.Names}}' | grep -q "^${CONTAINER_NAME}$"; then
    echo -e "${YELLOW}Removing existing container: ${CONTAINER_NAME}${NC}"
    ${CONTAINER_CMD} rm -f "${CONTAINER_NAME}" >/dev/null 2>&1 || true
fi

echo -e "${BLUE}Launching Geodesic container...${NC}"
echo -e "${YELLOW}Container runtime: ${CONTAINER_CMD}${NC}"
echo -e "${YELLOW}Image: ${GEODESIC_IMAGE}${NC}"
echo -e "${YELLOW}Host binary MD5: ${CHECKSUM}${NC}"
echo ""

# Launch Geodesic with pre-built binary mounted
# Mount to standard XDG paths that Atmos expects (HOME=/root in container)
# Mount infra-live to /workspace (Geodesic convention)
# Mount AWS files including Atmos-managed credentials
# Mount cache directory for SSO token caching
# Use platform flag to ensure correct architecture
# Use :Z instead of :z to force relabel and avoid caching issues
${CONTAINER_CMD} run -it --rm \
    ${PLATFORM_FLAG} \
    --name "${CONTAINER_NAME}" \
    -e TERM="${TERM:-xterm-256color}" \
    -e ATMOS_LOGS_LEVEL=Debug \
    -e ATMOS_XDG_CONFIG_HOME=/root/.config \
    -e ATMOS_XDG_DATA_HOME=/root/.local/share \
    -e ATMOS_XDG_CACHE_HOME=/root/.cache \
    -v "${ATMOS_SOURCE_DIR}/build/atmos-linux:/usr/local/bin/atmos:Z" \
    -v "${INFRA_LIVE_DIR}:/workspace:cached,z" \
    -v "${HOME}/.aws:/root/.aws:cached,z" \
    -v "${HOME}/.config/atmos:/root/.config/atmos:cached,z" \
    -v "${HOME}/.local/share/atmos:/root/.local/share/atmos:cached,z" \
    -v "${HOME}/.cache/atmos:/root/.cache/atmos:cached,z" \
    "${GEODESIC_IMAGE}"

echo ""
echo -e "${GREEN}Geodesic session ended${NC}"
