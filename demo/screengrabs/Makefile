INSTALL_PATH ?= ../../website/src/components/screengrabs
all: build-all install

# Write to /website/static/screengrab/
install: 
	@echo "Installing screengrabs to $(INSTALL_PATH)"
	@mkdir -p $(INSTALL_PATH)
	@mv -f artifacts/* $(INSTALL_PATH)

build-all:
	@./build-all.sh demo-stacks.txt
