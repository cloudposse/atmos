# Use a base image with platform specification
FROM --platform=$BUILDPLATFORM debian:bookworm-slim

# Define the arguments for Atmos version and platforms
ARG TARGETPLATFORM
ARG BUILDPLATFORM
ARG ATMOS_VERSION

SHELL ["/bin/bash", "-c"]

# Check if ATMOS_VERSION is set
RUN if [ -z "$ATMOS_VERSION" ]; then echo "ERROR: ATMOS_VERSION argument must be set" && exit 1; fi

# Update the package list and install curl and git
RUN apt-get update && apt-get install -y curl git

# Install the Cloud Posse Debian repository
RUN curl -1sLf 'https://dl.cloudsmith.io/public/cloudposse/packages/cfg/setup/bash.deb.sh' | bash

# Install OpenTofu
RUN curl -1sSLf 'https://get.opentofu.org/install-opentofu.sh' | bash -s -- --root-method none --install-method deb

# Install Kustomize binary (required by Helmfile)
RUN curl -1sSLf "https://raw.githubusercontent.com/kubernetes-sigs/kustomize/master/hack/install_kustomize.sh"  | bash -s -- /usr/local/bin

# Install toolchain used with Atmos
RUN apt-get -y install terraform kubectl helmfile helm

# Install the helm-diff plugin required by Helmfile
RUN helm plugin install https://github.com/databus23/helm-diff

# Install Atmos from the GitHub Release
RUN case ${TARGETPLATFORM} in \
        "linux/amd64") OS=linux; ARCH=amd64 ;; \
        "linux/arm64") OS=linux; ARCH=arm64 ;; \
        *) echo "Unsupported platform: ${TARGETPLATFORM}" && exit 1 ;; \
    esac && \
    ATMOS_VERSION=${ATMOS_VERSION#v} && \
    echo "Downloading Atmos v${ATMOS_VERSION} for ${OS}/${ARCH}" && \
    curl -1sSLf "https://github.com/cloudposse/atmos/releases/download/v${ATMOS_VERSION}/atmos_${ATMOS_VERSION}_${OS}_${ARCH}" -o /usr/local/bin/atmos && \
    chmod +x /usr/local/bin/atmos
