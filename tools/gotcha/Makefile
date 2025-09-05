# Standard Go project Makefile
.PHONY: build install test clean

build:
	go build -o build/gotcha .

install:
	go install .

test:
	go test ./...

clean:
	rm -rf build/