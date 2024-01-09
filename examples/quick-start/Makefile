export DOCKER_ORG ?= cloudposse
export DOCKER_IMAGE ?= $(DOCKER_ORG)/atmos
export DOCKER_TAG ?= latest
export DOCKER_IMAGE_NAME ?= $(DOCKER_IMAGE):$(DOCKER_TAG)
export APP_NAME = atmos

# Default install path, if lacking permissions, ~/.local/bin will be used instead
export INSTALL_PATH ?= /usr/local/bin

-include $(shell curl -sSL -o .build-harness "https://cloudposse.tools/build-harness"; echo .build-harness)

.DEFAULT_GOAL := all

## Initialize build-harness, install deps, build docker container, install wrapper script and run shell
all: init deps build install run
	@exit 0

## Install dependencies (if any)
deps:
	@exit 0

## Build docker image
build:
	@make --no-print-directory docker/build

## Install wrapper script from geodesic container
install:
	@docker run --rm --env APP_NAME --env DOCKER_IMAGE --env DOCKER_TAG --env INSTALL_PATH $(DOCKER_IMAGE_NAME) | bash -s $(DOCKER_TAG)

## Start the geodesic shell by calling wrapper script
run:
	@$(APP_NAME)
