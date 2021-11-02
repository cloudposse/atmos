SHELL := /bin/bash
GOOS=darwin
#GOOS=linux
GOARCH=amd64

# List of targets the `readme` target should call before generating the readme
export README_DEPS ?= docs/targets.md

-include $(shell curl -sSL -o .build-harness "https://git.io/build-harness"; echo .build-harness)

## Lint terraform code
lint:
	$(SELF) terraform/install terraform/get-modules terraform/get-plugins terraform/lint terraform/validate

build:
	env GOOS=${GOOS} GOARCH=${GOARCH} go build -o build/atmos

deps:
	go mod download
