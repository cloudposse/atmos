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
	@podman run --rm -v $$(pwd)/../..:/workspace -w /workspace/demo/screengrabs \
		-e ATMOS_TELEMETRY=false \
		-e ATMOS_FORCE_COLOR=true \
		-e FORCE_COLOR=1 \
		ubuntu:22.04 bash -c ' \
		set -e && \
		apt-get update -qq && \
		DEBIAN_FRONTEND=noninteractive apt-get install -y -qq aha make golang-go && \
		echo "Building atmos from source..." && \
		cd /workspace && \
		go build -o /usr/local/bin/atmos . && \
		atmos version && \
		cd /workspace/demo/screengrabs && \
		make build-all && \
		echo "Validating screengrabs..." && \
		! grep -r "atmos: not found" artifacts/ && \
		! grep -r "COMMAND_EXIT_CODE" artifacts/ \
		'
	@echo "Screengrabs generated and validated successfully in artifacts/"

# Docker build and install in one step
docker-all: docker-build install
