.DEFAULT_GOAL := build

build:
	variant2 export binary $(CURDIR) opsctl

shim:
	variant2 generate shim ./
	sed -i s/variant/variant2/g cli
	mv cli opsctl

test:
	variant2 test
