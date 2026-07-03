INSTALL_PATH ?= ../../website/static/casts/screengrabs
ATMOS := ../../build/atmos

all: build-all install

# Write to website/static/casts/screengrabs/
install:
	@echo "Installing screengrab casts to $(INSTALL_PATH)"
	@mkdir -p $(INSTALL_PATH)
	@cp -a artifacts/* $(INSTALL_PATH)

# Build the working-tree atmos so recordings always reflect local changes.
atmos-binary:
	@echo "Building atmos from the working tree..."
	@cd ../.. && go build -o build/atmos .

# Install recording dependencies via the Atmos toolchain (cross-platform,
# no Homebrew or containers required). Versions are pinned in the repo-root
# .tool-versions (bat and tree appear in recorded output; terraform is needed
# by demo commands), so casts are reproducible across machines.
deps: atmos-binary
	@cd ../.. && ./build/atmos toolchain install

# Recording works natively on any OS: casts are captured without a PTY, and
# Atmos renders correct TrueColor output and layout width via the
# ATMOS_FORCE_COLOR and COLUMNS environment variables set by the grabber.
build-all: deps
	@eval "$$($(ATMOS) toolchain env --format=bash)" && \
		PATH="$$(cd ../../build && pwd):$$PATH" go run . demo-stacks.txt
	@echo "Validating screengrab casts..."
	@! grep -r "atmos: not found" artifacts/ || (echo "ERROR: Found 'atmos: not found' in screengrab casts" && exit 1)
	@echo "✓ All screengrab casts validated successfully"
