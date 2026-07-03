INSTALL_PATH ?= ../../website/src/components/Screengrabs
all: build-all install

# Write to website/src/components/Screengrabs/
install:
	@echo "Installing screengrabs to $(INSTALL_PATH)"
	@mkdir -p $(INSTALL_PATH)
	@cp -a artifacts/* $(INSTALL_PATH)

build-all:
	@go run . demo-stacks.txt
	@echo "Validating screengrabs..."
	@! grep -r "atmos: not found" artifacts/ || (echo "ERROR: Found 'atmos: not found' in screengrabs" && exit 1)
	@echo "✓ All screengrabs validated successfully"

# Generate screengrabs using Docker (cross-platform, recommended)
docker-build:
	@echo "Generating screengrabs using Podman (Linux container)..."
	@podman run --rm -v $$(pwd)/../..:/workspace -w /workspace/demo/screengrabs \
		--memory=4g \
		-e ATMOS_TELEMETRY=false \
		-e ATMOS_FORCE_COLOR=true \
		-e FORCE_COLOR=1 \
		ubuntu:22.04 bash -c ' \
		set -e && \
		apt-get update -qq && \
		DEBIAN_FRONTEND=noninteractive apt-get install -y -qq make wget tar tree bat && \
		ln -sf /usr/bin/batcat /usr/local/bin/bat && \
		echo "Installing Go 1.26..." && \
		ARCH=$$(uname -m | sed "s/x86_64/amd64/; s/aarch64/arm64/") && \
		wget -q https://go.dev/dl/go1.26.0.linux-$$ARCH.tar.gz && \
		tar -C /usr/local -xzf go1.26.0.linux-$$ARCH.tar.gz && \
		export PATH=/usr/local/go/bin:$$PATH && \
		go version && \
		echo "Building atmos from source..." && \
		cd /workspace && \
		GOMEMLIMIT=3GiB go build -p 2 -o /usr/local/bin/atmos . && \
		atmos version && \
		atmos toolchain install hashicorp/terraform && \
		eval "$$(atmos toolchain env --format=bash)" && \
		cd /workspace/demo/screengrabs && \
		make build-all \
		'
	@echo "Screengrabs generated and validated successfully in artifacts/"

# Docker build and install in one step
docker-all: docker-build install
