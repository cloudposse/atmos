.PHONY: build watch package install clean

build:
	npm run compile

watch:
	npm run watch

package: build
	npx @vscode/vsce package

install: package
	code --install-extension atmos-vscode-*.vsix

clean:
	rm -rf out node_modules *.vsix
