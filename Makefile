.DEFAULT_GOAL := help

define MIGRATION_HELP
@echo "The root Makefile is no longer used for Atmos development tasks."
@echo ""
@echo "Use Atmos custom commands instead:"
@printf "  %-23s -> %s\n" "make deps" "atmos build deps"
@printf "  %-23s -> %s\n" "make build" "atmos build"
@printf "  %-23s -> %s\n" "make build-linux" "atmos build --target linux"
@printf "  %-23s -> %s\n" "make build-windows" "atmos build --target windows"
@printf "  %-23s -> %s\n" "make build-macos" "atmos build --target macos"
@printf "  %-23s -> %s\n" "make build-macos-intel" "atmos build --target macos-intel"
@printf "  %-23s -> %s\n" "make version-linux" "atmos build version --target linux"
@printf "  %-23s -> %s\n" "make version-windows" "atmos build version --target windows"
@printf "  %-23s -> %s\n" "make version-macos" "atmos build version --target macos"
@printf "  %-23s -> %s\n" "make lint" "atmos lint changed"
@printf "  %-23s -> %s\n" "make testacc" "atmos test acc"
@printf "  %-23s -> %s\n" "make testacc-cover" "atmos test acc-cover"
@printf "  %-23s -> %s\n" "make test-short" "atmos test short"
@printf "  %-23s -> %s\n" "make test-race" "atmos test race"
@printf "  %-23s -> %s\n" "make generate-mocks" "atmos test generate-mocks"
@printf "  %-23s -> %s\n" "make link-check" "atmos lint link-check"
@echo ""
@exit 1
endef

.PHONY: help FORCE
help:
	$(MIGRATION_HELP)

FORCE:

Makefile: ;

%: FORCE
	$(MIGRATION_HELP)
