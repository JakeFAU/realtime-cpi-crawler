.PHONY: all fmt vet lint test build

all: fmt vet lint test build

fmt:
	go fmt ./...

vet:
	go vet ./...

lint:
	golangci-lint run

test:
	go test ./...

build:
	go build ./...
