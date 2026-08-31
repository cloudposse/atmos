# Custom Geodesic devcontainer with Atmos pre-installed
# This demonstrates using a custom Dockerfile with devcontainers

ARG GEODESIC_VERSION=latest@sha256:5c6edc24b2a71a462fdd4d04debb80216e9f0168340ae235715e31971214781f
FROM cloudposse/geodesic:${GEODESIC_VERSION}

# Install Atmos using the official install script
# This handles version detection and platform detection automatically.
# Pinned to a specific commit of this repo's own website/static/install.sh
# (served at atmos.tools/install.sh) to satisfy Scorecard Pinned-Dependencies.
RUN curl -fsSL https://raw.githubusercontent.com/cloudposse/atmos/d81f1d09ec8aa2ae4b9cc1304d84cf4cc0a53199/website/static/install.sh | bash && \
    atmos version

# Set working directory
WORKDIR /workspace

# Default command
CMD ["/bin/bash", "-l"]
