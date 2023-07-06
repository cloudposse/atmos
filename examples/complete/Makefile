export DOCKER_ORG ?= cloudposse
export DOCKER_IMAGE ?= $(DOCKER_ORG)/atmos
export DOCKER_TAG ?= latest
export DOCKER_IMAGE_NAME ?= $(DOCKER_IMAGE):$(DOCKER_TAG)
export APP_NAME = atmos
GEODESIC_INSTALL_PATH ?= /usr/local/bin
export INSTALL_PATH ?= $(GEODESIC_INSTALL_PATH)
export SCRIPT = $(INSTALL_PATH)/$(APP_NAME)
export ADR_DOCS_DIR = docs/adr
export ADR_DOCS_README = $(ADR_DOCS_DIR)/README.md
BUILD_HARNESS_EXTENSIONS_PATH := $(CURDIR)/.build-harness-extensions

-include $(shell curl -sSL -o .build-harness "https://cloudposse.tools/build-harness"; echo .build-harness)

.DEFAULT_GOAL := default

## Initialize build-harness, install deps, build docker container, install wrapper script and run shell
all: init deps build install run
	@exit 0

## Install dependencies (if any)
deps:
	@exit 0

## Build docker image
build:
	@make --no-print-directory docker/build

## Push docker image to registry
push:
	@$(call fail,Refusing to push $(DOCKER_IMAGE_NAME) to docker hub)

## Install wrapper script from geodesic container
install:
	@docker run --rm $(DOCKER_IMAGE_NAME) | bash -s $(DOCKER_TAG) || (echo "Try: sudo make install"; exit 1)

## Start the geodesic shell by calling wrapper script
run:
	$(SCRIPT)
