# Custom Geodesic devcontainer with Atmos pre-installed
# This demonstrates using a custom Dockerfile with devcontainers

ARG GEODESIC_VERSION=latest
FROM cloudposse/geodesic:${GEODESIC_VERSION}

# Install Atmos
ARG ATMOS_VERSION=latest
ARG TARGETARCH

# Download and install Atmos binary
RUN set -eux; \
    ATMOS_INSTALL_PATH=/usr/local/bin/atmos; \
    if [ "${ATMOS_VERSION}" = "latest" ]; then \
      DOWNLOAD_URL="https://github.com/cloudposse/atmos/releases/latest/download/atmos_$(uname -s)_${TARGETARCH}"; \
    else \
      DOWNLOAD_URL="https://github.com/cloudposse/atmos/releases/download/${ATMOS_VERSION}/atmos_$(uname -s)_${TARGETARCH}"; \
    fi; \
    curl -fsSL "${DOWNLOAD_URL}" -o "${ATMOS_INSTALL_PATH}"; \
    chmod +x "${ATMOS_INSTALL_PATH}"; \
    atmos version

# Set working directory
WORKDIR /workspace

# Default command
CMD ["/bin/bash", "-l"]
