# Use a base image with platform specification
FROM --platform=$BUILDPLATFORM debian:bookworm-slim

# Define the arguments for Atmos version and platforms
ARG TARGETPLATFORM
ARG BUILDPLATFORM
ARG ATMOS_VERSION

# Check if ATMOS_VERSION is set
RUN if [ -z "$ATMOS_VERSION" ]; then echo "ERROR: ATMOS_VERSION argument must be set" && exit 1; fi

# Set SHELL to use bash and enable pipefail
SHELL ["/bin/bash", "-eo", "pipefail", "-c"]

RUN set -ex; \
    # Update the package list
    apt-get update; \
    # Install curl and git
    apt-get -y install  --no-install-recommends curl git ca-certificates; \
    # Install the Cloud Posse Debian repository
    curl -1sLf 'https://dl.cloudsmith.io/public/cloudposse/packages/cfg/setup/bash.deb.sh' | bash -x; \
    # Install OpenTofu
    curl -1sSLf 'https://get.opentofu.org/install-opentofu.sh' | bash -s -- --root-method none --install-method deb; \
    # Install Kustomize binary (required by Helmfile)
    curl -1sSLf "https://raw.githubusercontent.com/kubernetes-sigs/kustomize/master/hack/install_kustomize.sh" | bash -s -- /usr/local/bin; \
    # Install toolchain used with Atmos (pin Helm 3.x for consistency)
    apt-get -y install --no-install-recommends terraform kubectl helmfile helm=3.16.4-1; \
    # Install the helm-diff plugin required by Helmfile (pin to v3.9.x for Helm 3 compatibility)
    helm plugin install https://github.com/databus23/helm-diff --version v3.9.14; \
    # Clean up the package lists to keep the image clean
    rm -rf /var/lib/apt/lists/*

# Install Atmos from the GitHub Release
RUN case ${TARGETPLATFORM} in \
        "linux/amd64") OS=linux; ARCH=amd64 ;; \
        "linux/arm64") OS=linux; ARCH=arm64 ;; \
        *) echo "Unsupported platform: ${TARGETPLATFORM}" && exit 1 ;; \
    esac && \
    echo "Downloading Atmos v${ATMOS_VERSION#v} for ${OS}/${ARCH}" && \
    curl -1sSLf "https://github.com/cloudposse/atmos/releases/download/v${ATMOS_VERSION#v}/atmos_${ATMOS_VERSION#v}_${OS}_${ARCH}" -o /usr/local/bin/atmos && \
    chmod +x /usr/local/bin/atmos
