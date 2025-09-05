# Makefile for toolchain experiment

BINARY=toolchain

.PHONY: all build test clean tidy

all: build

tidy:
	go mod tidy

build: tidy
	go build -o $(BINARY) .

test: tidy
	go test -v ./...

clean:
	rm -f $(BINARY)
