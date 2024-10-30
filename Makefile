TEST ?= $$(go list ./... | grep -v 'vendor')
SHELL := /bin/bash
#GOOS=darwin
#GOOS=linux
#GOARCH=amd64
VERSION=test

# List of targets the `readme` target should call before generating the readme
export README_DEPS ?= docs/targets.md

-include $(shell curl -sSL -o .build-harness "https://cloudposse.tools/build-harness"; echo .build-harness)

## Lint terraform code
lint:
	$(SELF) terraform/install terraform/get-modules terraform/get-plugins terraform/lint terraform/validate

get:
	go get

build: get
	env $(if $(GOOS),GOOS=$(GOOS)) $(if $(GOARCH),GOARCH=$(GOARCH)) go build -o build/atmos -v -ldflags "-X 'github.com/cloudposse/atmos/pkg/version.Version=${VERSION}'"

version: build
	chmod +x ./build/atmos
	./build/atmos version

deps:
	go mod download

# Run acceptance tests
testacc: get
	go test $(TEST) -v $(TESTARGS) -timeout 2m

.PHONY: lint get build deps version testacc
