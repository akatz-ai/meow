# MEOW Stack Makefile
# Build automation for meow-machine

BINARY := meow
MODULE := github.com/meow-stack/meow-machine
MAIN := ./cmd/meow

# Version from git
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_TIME := $(shell date -u '+%Y-%m-%dT%H:%M:%SZ')

# Go settings
GOFLAGS := -trimpath
LDFLAGS := -ldflags "-s -w \
	-X $(MODULE)/cmd/meow/cmd.Version=$(VERSION)"

# Output directory
BIN_DIR := bin

# Default target
.DEFAULT_GOAL := build

# Ensure bin directory exists
$(BIN_DIR):
	@mkdir -p $(BIN_DIR)

## build: Build the binary to bin/meow
.PHONY: build
build: $(BIN_DIR)
	go build $(GOFLAGS) $(LDFLAGS) -o $(BIN_DIR)/$(BINARY) $(MAIN)

## install: Install meow to GOPATH/bin
.PHONY: install
install:
	go install $(GOFLAGS) $(LDFLAGS) $(MAIN)

## test: Run all tests
.PHONY: test
test:
	go test -race -v ./...

## test-short: Run short tests (skip slow/integration tests)
.PHONY: test-short
test-short:
	go test -short -race ./...

## test-cover: Run tests with coverage report
.PHONY: test-cover
test-cover:
	go test -race -coverprofile=coverage.out -covermode=atomic ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

## lint: Run golangci-lint
.PHONY: lint
lint:
	@which golangci-lint > /dev/null || (echo "golangci-lint not installed" && exit 1)
	golangci-lint run ./...

## fmt: Format code
.PHONY: fmt
fmt:
	go fmt ./...
	@echo "Code formatted"

## vet: Run go vet
.PHONY: vet
vet:
	go vet ./...

## tidy: Tidy dependencies
.PHONY: tidy
tidy:
	go mod tidy

## generate: Run go generate
.PHONY: generate
generate:
	go generate ./...

## clean: Remove build artifacts
.PHONY: clean
clean:
	rm -rf $(BIN_DIR)
	rm -f coverage.out coverage.html

## run: Build and run meow with arguments
.PHONY: run
run: build
	./$(BIN_DIR)/$(BINARY) $(ARGS)

## version: Show version info
.PHONY: version
version:
	@echo "Version:    $(VERSION)"
	@echo "Commit:     $(COMMIT)"
	@echo "Build Time: $(BUILD_TIME)"

## check: Run all checks (fmt, vet, test)
.PHONY: check
check: fmt vet test
	@echo "All checks passed"

## help: Show this help
.PHONY: help
help:
	@echo "MEOW Stack - Build targets:"
	@echo ""
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/^## /  /'
