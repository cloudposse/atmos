FROM golang:1.26-bookworm
ENV DEBIAN_FRONTEND=noninteractive
RUN apt-get update && apt-get install -y --no-install-recommends \
      ca-certificates curl build-essential git pkg-config libudev-dev \
    && rm -rf /var/lib/apt/lists/*

# Install a released Atmos; it is used to build Atmos from source via `atmos build`.
ARG ATMOS_VERSION=1.233.0
RUN arch="$(dpkg --print-architecture)" \
    && curl -fsSL -o /usr/local/bin/atmos \
       "https://github.com/cloudposse/atmos/releases/download/v${ATMOS_VERSION}/atmos_${ATMOS_VERSION}_linux_${arch}" \
    && chmod +x /usr/local/bin/atmos

# scanner contract: the checkout lives inside the image, at /src
COPY . /src
WORKDIR /src
ENV ATMOS_DEV_SHELL=1

# Fetch all Go modules (including the mage build tool) now; the audit runs without network access.
RUN atmos build deps

# Build the Atmos binary from source, to ./build/atmos.
RUN atmos build
ENV PATH="/src/build:${PATH}"

# Compile the tests (warming the Go build cache) so they can be run offline; do not fail the image build.
RUN (go test -count=1 -run '^$' ./pkg/... ./internal/... || echo "WARNING: some test packages failed to compile")
