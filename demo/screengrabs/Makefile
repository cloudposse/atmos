MIGRATION_TARGETS := all build-all install docker-build docker-all clean

.DEFAULT_GOAL := help

.PHONY: help $(MIGRATION_TARGETS)
help:
	@echo "The demo/screengrabs Makefile is no longer used."
	@echo ""
	@echo "Use Atmos custom commands from the repository root instead:"
	@echo "  make all          -> atmos screengrabs all"
	@echo "  make build-all    -> atmos screengrabs build --all"
	@echo "  make install      -> atmos screengrabs install"
	@echo "  make docker-build -> atmos screengrabs docker-build"
	@echo "  make docker-all   -> atmos screengrabs docker-all"
	@echo "  make clean        -> (no longer needed / see atmos screengrabs --help)"
	@echo ""
	@exit 1

$(MIGRATION_TARGETS): help

%: help
