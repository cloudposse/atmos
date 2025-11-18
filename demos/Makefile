# Makefile for Atmos VHS Demo Videos
SHELL := /bin/bash

.PHONY: help
help:
	@echo "Atmos Demo Videos - Available targets:"
	@echo ""
	@echo "  make install       Install dependencies (VHS, gifsicle, etc.)"
	@echo "  make director      Build director tool"
	@echo "  make new NAME=...  Create new scene"
	@echo "  make validate      Validate all scenes"
	@echo "  make render        Render all scenes (incremental)"
	@echo "  make catalog       List all scenes"
	@echo "  make clean         Remove generated files"

.PHONY: install
install:
	@echo "Installing dependencies..."
	brew bundle --file=Brewfile

.PHONY: director
director:
	@echo "Building director tool..."
	@cd ../tools/director && go build -o ../../build/director .
	@test -x ../build/director || (echo "Error: Failed to build director" && exit 1)
	@echo "Director built successfully: ../build/director"

.PHONY: new
new: director
	@if [ -z "$(NAME)" ]; then \
		echo "Error: NAME is required. Usage: make new NAME=my-scene"; \
		exit 1; \
	fi
	@../build/director new $(NAME)

.PHONY: validate
validate: director
	@../build/director validate

.PHONY: render
render: director
	@../build/director render $(SCENE)

.PHONY: catalog
catalog: director
	@../build/director catalog

.PHONY: clean
clean:
	@echo "Cleaning generated files..."
	@rm -rf .cache
	@echo "Cleaned: .cache/"
