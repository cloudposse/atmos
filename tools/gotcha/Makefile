# Standard Go project Makefile
.PHONY: build install test lint clean

build:
	go build -o gotcha .

install:
	go install .

test: build
	./gotcha ./...

lint:
	golangci-lint run ./...

clean:
	rm -rf build/ gotcha