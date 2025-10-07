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
		ubuntu:22.04 bash -c \
		"apt-get update && \
		DEBIAN_FRONTEND=noninteractive apt-get install -y aha util-linux make git && \
		make build-all"
	@echo "Screengrabs generated successfully in artifacts/"

# Docker build and install in one step
docker-all: docker-build install
