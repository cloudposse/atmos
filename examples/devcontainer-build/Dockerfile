# Custom Geodesic devcontainer with Atmos pre-installed
# This demonstrates using a custom Dockerfile with devcontainers

ARG GEODESIC_VERSION=latest@sha256:5c6edc24b2a71a462fdd4d04debb80216e9f0168340ae235715e31971214781f
FROM cloudposse/geodesic:${GEODESIC_VERSION}

# Install Atmos using the official install script.
# Pinned to a specific commit of this repo's own website/static/install.sh
# (served at atmos.tools/install.sh) to satisfy Scorecard Pinned-Dependencies.
# ATMOS_VERSION pins the installed Atmos release itself; override at build
# time (--build-arg ATMOS_VERSION=vX.Y.Z) to bump it.
ARG ATMOS_VERSION=1.227.0
RUN export ATMOS_VERSION="${ATMOS_VERSION}" && \
    curl -fsSL https://raw.githubusercontent.com/cloudposse/atmos/d81f1d09ec8aa2ae4b9cc1304d84cf4cc0a53199/website/static/install.sh | bash && \
    atmos version

# Set working directory
WORKDIR /workspace

# Default command
CMD ["/bin/bash", "-l"]
