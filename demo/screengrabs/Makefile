INSTALL_PATH ?= ../../website/src/components/Screengrabs
all: build-all install

# Write to website/src/components/Screengrabs/
install:
	@echo "Installing screengrabs to $(INSTALL_PATH)"
	@mkdir -p $(INSTALL_PATH)
	@cp -a artifacts/* $(INSTALL_PATH)

build-all:
	@./build-all.sh demo-stacks.txt

# Generate screengrabs using Docker (cross-platform, recommended)
docker-build:
	@echo "Generating screengrabs using Podman (Linux container)..."
	@podman run --rm -t -v $$(pwd)/../..:/workspace -w /workspace/demo/screengrabs \
		-e ATMOS_TELEMETRY=false \
		-e ATMOS_COLOR=true \
		ubuntu:22.04 bash -c ' \
		set -e && \
		apt-get update -qq && \
		DEBIAN_FRONTEND=noninteractive apt-get install -y -qq aha util-linux make git curl jq && \
		echo "Installing latest atmos release..." && \
		VERSION=$$(curl -s https://api.github.com/repos/cloudposse/atmos/releases/latest | jq -r .tag_name) && \
		echo "Installing atmos $$VERSION" && \
		ARCH=$$(uname -m | sed "s/aarch64/arm64/;s/x86_64/amd64/") && \
		curl -sSL -o /usr/local/bin/atmos "https://github.com/cloudposse/atmos/releases/download/$$VERSION/atmos_$${VERSION#v}_linux_$$ARCH" && \
		chmod +x /usr/local/bin/atmos && \
		atmos version && \
		make build-all && \
		echo "Validating screengrabs..." && \
		! grep -r "atmos: not found" artifacts/ && \
		! grep -r "COMMAND_EXIT_CODE" artifacts/ \
		'
	@echo "Screengrabs generated and validated successfully in artifacts/"

# Docker build and install in one step
docker-all: docker-build install
