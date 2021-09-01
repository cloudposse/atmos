SHELL := /bin/bash
GIT_SERVER 	:= github.com
ORG         := cloudposse
REPO        := atmos
BINARY 		:= atmos

# List of targets the `readme` target should call before generating the readme
export README_DEPS ?= docs/targets.md

-include $(shell curl -sSL -o .build-harness "https://git.io/build-harness"; echo .build-harness)


VERSION_REPO := $(GIT_SERVER)/$(ORG)/$(NAME)
# Make does not offer a recursive wildcard function, so here's one:
rwildcard=$(wildcard $1$2) $(foreach d,$(wildcard $1*),$(call rwildcard,$d/,$2))

BUILD_TARGET = build
MAIN_SRC_FILE=main.go
GO := GO111MODULE=on go
GO_NOMOD :=GO111MODULE=off go
GO_VERSION := $(shell $(GO) version | sed -e 's/^[^0-9.]*\([0-9.]*\).*/\1/')
GO_DEPENDENCIES := $(call rwildcard,pkg/,*.go) $(call rwildcard,*.go)

REV := $(shell git rev-parse --short HEAD 2> /dev/null || echo 'unknown')
ORG_REPO := $(ORG)/$(REPO)
ROOT_PACKAGE := $(GIT_SERVER)/$(ORG_REPO)

BRANCH     := $(shell git rev-parse --abbrev-ref HEAD 2> /dev/null  || echo 'unknown')
BUILD_DATE := $(shell date +%Y%m%d-%H:%M:%S)
CGO_ENABLED = 0

REPORTS_DIR=$(BUILD_TARGET)/reports

GOTEST := $(GO) test
# If available, use gotestsum which provides more comprehensive output
# This is used in the CI builds
ifneq (, $(shell which gotestsum 2> /dev/null))
GOTESTSUM_FORMAT ?= standard-quiet
GOTEST := GO111MODULE=on gotestsum --junitfile $(REPORTS_DIR)/integration.junit.xml --format $(GOTESTSUM_FORMAT) --
endif

NOW := $(shell date -u +'%Y-%m-%d %T')
# set dev version unless VERSION is explicitly set via environment
VERSION ?= $(shell echo "$$(git describe --abbrev=0 --tags 2>/dev/null)-dev+$(REV)" | sed 's/^v//')


BUILDFLAGS :=  -ldflags \
  " -X 'atmos/cmd.Version=$(VERSION)'\
	-X 'atmos/cmd.BuildTime=$(NOW)'"

ifdef DEBUG
BUILDFLAGS := -gcflags "all=-N -l" $(BUILDFLAGS)
endif

ifdef PARALLEL_BUILDS
BUILDFLAGS += -p $(PARALLEL_BUILDS)
GOTEST += -p $(PARALLEL_BUILDS)
else
# -p 4 seems to work well for people
GOTEST += -p 4
endif

TEST_PACKAGE ?= ./...
COVER_OUT:=$(REPORTS_DIR)/cover.out
COVERFLAGS=-coverprofile=$(COVER_OUT) --covermode=count --coverpkg=./...

.PHONY: list
list: ## List all make targets
	@$(MAKE) -pRrn : -f $(MAKEFILE_LIST) 2>/dev/null | awk -v RS= -F: '/^# File/,/^# Finished Make data base/ {if ($$1 !~ "^[#.]") {print $$1}}' | egrep -v -e '^[^[:alnum:]]' -e '^$@$$' | sort

.PHONY: help
.DEFAULT_GOAL := help
help:
	@grep -h -E '^[a-zA-Z0-9_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'

lint: ## Lint terraform code
	$(SELF) terraform/install terraform/get-modules terraform/get-plugins terraform/lint terraform/validate

all: build ## Build the binary
full: check ## Build and run the tests
check: build test ## Build and run the tests
get-test-deps: ## Install test dependencies
	$(GO_NOMOD) get github.com/axw/gocov/gocov
	$(GO_NOMOD) get -u gopkg.in/matm/v1/gocov-html

shell: ## runs docker-compose run getting into the shell faster.
	docker-compose run os_info_dump

print-version: ## Print version
	@echo $(VERSION)

build: $(GO_DEPENDENCIES) ## Build binary for current OS
	CGO_ENABLED=$(CGO_ENABLED) $(GO) $(BUILD_TARGET) $(BUILDFLAGS) -o build/$(BINARY) $(MAIN_SRC_FILE)

tidy-deps: ## Cleans up dependencies
	$(GO) mod tidy
	# mod tidy only takes compile dependencies into account, let's make sure we capture tooling dependencies as well
	@$(MAKE) install-generate-deps

.PHONY: make-reports-dir
make-reports-dir:
	mkdir -p $(REPORTS_DIR)

test: ## Run tests with the "unit" build tag
	KUBECONFIG=/cluster/connections/not/allowed CGO_ENABLED=$(CGO_ENABLED) $(GOTEST) --tags=unit -failfast -short ./... $(TEST_BUILDFLAGS)

test-coverage : make-reports-dir ## Run tests and coverage for all tests with the "unit" build tag
	CGO_ENABLED=$(CGO_ENABLED) $(GOTEST) --tags=unit $(COVERFLAGS) -failfast -short ./... $(TEST_BUILDFLAGS)

test-report: make-reports-dir get-test-deps test-coverage ## Create the test report
	@gocov convert $(COVER_OUT) | gocov report | tee $(REPORTS_DIR)/report.txt

test-report-html: make-reports-dir get-test-deps test-coverage ## Create the test report in HTML format
	@gocov convert $(COVER_OUT) | gocov-html > $(REPORTS_DIR)/cover.html && open $(REPORTS_DIR)/cover.html

test-verbose: make-reports-dir ## Run the unit tests in verbose mode
	CGO_ENABLED=$(CGO_ENABLED) $(GOTEST) -v $(COVERFLAGS) --tags=unit -failfast ./... $(TEST_BUILDFLAGS)

install: $(GO_DEPENDENCIES) ## Install the binary
	GOBIN=${GOPATH}/bin $(GO) install $(BUILDFLAGS) $(MAIN_SRC_FILE)

linux: ## Build for Linux
	CGO_ENABLED=$(CGO_ENABLED) GOOS=linux GOARCH=amd64 $(GO) $(BUILD_TARGET) $(BUILDFLAGS) -o build/$(BINARY)-linux-amd64 $(MAIN_SRC_FILE)
	chmod +x build/$(BINARY)-linux-amd64

arm: ## Build for ARM
	CGO_ENABLED=$(CGO_ENABLED) GOOS=linux GOARCH=arm $(GO) $(BUILD_TARGET) $(BUILDFLAGS) -o build/$(BINARY)-linux-arm $(MAIN_SRC_FILE)
	chmod +x build/$(BINARY)-linux-arm

win: ## Build for Windows
	CGO_ENABLED=$(CGO_ENABLED) GOOS=windows GOARCH=amd64 $(GO) $(BUILD_TARGET) $(BUILDFLAGS) -o build/$(BINARY)-windows-amd64.exe $(MAIN_SRC_FILE)
	chmod +x build/$(BINARY)-windows-amd64.exe

win32:
	CGO_ENABLED=$(CGO_ENABLED) GOOS=windows GOARCH=386 $(GO) $(BUILD_TARGET) $(BUILDFLAGS) -o build/$(BINARY)-windows-386.exe $(MAIN_SRC_FILE)
	chmod +x build/$(BINARY)-windows-386.exe

darwin: ## Build for OSX
	CGO_ENABLED=$(CGO_ENABLED) GOOS=darwin GOARCH=amd64 $(GO) $(BUILD_TARGET) $(BUILDFLAGS) -o build/$(BINARY)-darwin-amd64 $(MAIN_SRC_FILE)
	chmod +x build/$(BINARY)-darwin-amd64

fmt: ## Format the code
	$(eval FORMATTED = $(shell $(GO) fmt ./...))
	@if [ "$(FORMATTED)" == "" ]; \
      	then \
      	    echo "All Go files properly formatted"; \
      	else \
      		echo "Fixed formatting for: $(FORMATTED)"; \
      	fi

all: linux arm win win32 darwin
