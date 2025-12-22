# Custom Geodesic devcontainer with Atmos pre-installed
# This demonstrates using a custom Dockerfile with devcontainers

ARG GEODESIC_VERSION=latest
FROM cloudposse/geodesic:${GEODESIC_VERSION}

# Install Atmos using the official install script
# This handles version detection and platform detection automatically
RUN curl -fsSL https://atmos.tools/install.sh | bash && \
    atmos version

# Set working directory
WORKDIR /workspace

# Default command
CMD ["/bin/bash", "-l"]
