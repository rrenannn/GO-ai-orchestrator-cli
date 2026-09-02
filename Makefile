BINARY   := bin/maestro
PACKAGE  := ./cmd/maestro
VERSION  ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS  := -X main.version=$(VERSION)

.PHONY: all build test cover vet fmt install tidy clean

all: fmt vet test build

build:
	go build -ldflags "$(LDFLAGS)" -o $(BINARY) $(PACKAGE)

test:
	go test ./...

cover:
	go test -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out | tail -1

vet:
	go vet ./...

fmt:
	gofmt -l -w .

install:
	go install -ldflags "$(LDFLAGS)" $(PACKAGE)

tidy:
	go mod tidy

clean:
	rm -rf bin coverage.out
