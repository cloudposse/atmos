# Standard Go project Makefile
.PHONY: build install test lint clean

build:
	go build -o build/gotcha .

install:
	go install .

test:
	go test ./...

lint:
	golangci-lint run ./...

clean:
	rm -rf build/