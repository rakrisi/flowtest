.PHONY: build test vet lint clean install run-integration

BINARY := flowtest
MODULE := github.com/radhe-singh/flowtest
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")

## build: Build the flowtest binary
build:
	go build -ldflags "-X main.version=$(VERSION)" -o $(BINARY) ./cmd/flowtest/

## test: Run unit tests
test:
	go test ./... -count=1

## test-verbose: Run unit tests with verbose output
test-verbose:
	go test ./... -v -count=1

## test-cover: Run tests with coverage report
test-cover:
	go test ./... -coverprofile=coverage.out
	go tool cover -func=coverage.out
	@rm -f coverage.out

## vet: Run go vet
vet:
	go vet ./...

## lint: Run golangci-lint (install: https://golangci-lint.run/usage/install/)
lint:
	@which golangci-lint > /dev/null 2>&1 || { echo "golangci-lint not installed. Run: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest"; exit 1; }
	golangci-lint run ./...

## check: Run vet + test (used by CI)
check: vet test

## install: Install flowtest to GOPATH/bin
install:
	go install ./cmd/flowtest/

## run-integration: Run integration tests (requires Docker)
run-integration:
	cd testproject && ./run-tests.sh

## clean: Remove build artifacts
clean:
	rm -f $(BINARY) coverage.out

## help: Show this help
help:
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/## //' | column -t -s ':'
