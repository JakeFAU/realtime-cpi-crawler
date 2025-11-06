BINARY_NAME=webcrawler

.PHONY: all build test lint clean tidy

all: build

build:
	go build -o build/${BINARY_NAME} main.go

test:
	go test ./...

lint:
	golangci-lint run

clean:
	rm -f build/${BINARY_NAME}

tidy:
	go mod tidy
