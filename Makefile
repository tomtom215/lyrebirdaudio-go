# LyreBirdAudio-Go Makefile
# Production-grade build system

.PHONY: help build build-all install clean test test-race test-coverage test-integration bench lint fmt vet check coverage-html

# Default target
.DEFAULT_GOAL := help

# Build variables
BINARY_NAME := lyrebird
STREAM_BINARY := lyrebird-stream

BUILD_DIR := bin
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_TIME := $(shell date -u '+%Y-%m-%d_%H:%M:%S')

# Go build flags
LDFLAGS := -ldflags "-X main.Version=$(VERSION) -X main.Commit=$(COMMIT) -X main.BuildTime=$(BUILD_TIME) -s -w"
GO_BUILD_FLAGS := -trimpath $(LDFLAGS)

# Cross-compilation targets
PLATFORMS := linux/amd64 linux/arm64 linux/arm/7 linux/arm/6

# Tools
GOLANGCI_LINT := $(shell command -v golangci-lint 2>/dev/null)
GOSEC := $(shell command -v gosec 2>/dev/null)

## help: Display this help message
help:
	@echo "LyreBirdAudio-Go Build System"
	@echo ""
	@echo "Usage:"
	@echo "  make <target>"
	@echo ""
	@echo "Targets:"
	@awk '/^## [a-zA-Z_-]+:/ { split($$0, a, ": "); target=substr(a[1], 4); desc=a[2]; printf "  \033[36m%-20s\033[0m %s\n", target, desc }' $(MAKEFILE_LIST)

## build: Build all binaries for current platform
build:
	@echo "==> Building binaries (version: $(VERSION))..."
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 go build $(GO_BUILD_FLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/lyrebird
	CGO_ENABLED=0 go build $(GO_BUILD_FLAGS) -o $(BUILD_DIR)/$(STREAM_BINARY) ./cmd/lyrebird-stream
	@echo "==> Build complete: $(BUILD_DIR)/"
	@ls -lh $(BUILD_DIR)/

## build-all: Cross-compile for all supported platforms
build-all:
	@echo "==> Cross-compiling for all platforms..."
	@mkdir -p $(BUILD_DIR)
	@for platform in $(PLATFORMS); do \
		GOOS=$${platform%/*} GOARCH=$${platform#*/}; \
		GOARM=""; \
		if [ "$$GOARCH" = "arm/7" ]; then GOARCH=arm; GOARM=7; fi; \
		if [ "$$GOARCH" = "arm/6" ]; then GOARCH=arm; GOARM=6; fi; \
		output_name=$(BUILD_DIR)/$(BINARY_NAME)-$${GOOS}-$${GOARCH}$${GOARM:+v$$GOARM}; \
		echo "  Building $$output_name..."; \
		CGO_ENABLED=0 GOOS=$$GOOS GOARCH=$$GOARCH GOARM=$$GOARM \
			go build $(GO_BUILD_FLAGS) -o $$output_name ./cmd/lyrebird || exit 1; \
	done
	@echo "==> Cross-compilation complete"
	@ls -lh $(BUILD_DIR)/

## install: Install binaries to /usr/local/bin (requires sudo)
install: build
	@echo "==> Installing binaries to /usr/local/bin..."
	install -m 755 $(BUILD_DIR)/$(BINARY_NAME) /usr/local/bin/
	install -m 755 $(BUILD_DIR)/$(STREAM_BINARY) /usr/local/bin/
	@echo "==> Installation complete"

## clean: Remove build artifacts
clean:
	@echo "==> Cleaning build artifacts..."
	rm -rf $(BUILD_DIR)
	rm -f coverage.out coverage.html
	go clean -cache -testcache
	@echo "==> Clean complete"

## test: Run all unit tests
test:
	@echo "==> Running unit tests..."
	go test -v -race -timeout 30s ./...

## test-race: Run tests with race detector
test-race:
	@echo "==> Running tests with race detector..."
	go test -race -timeout 30s ./...

## test-coverage: Run tests with coverage report
test-coverage:
	@echo "==> Running tests with coverage..."
	go test -race -coverprofile=coverage.out -covermode=atomic ./...
	go tool cover -func=coverage.out
	@echo ""
	@echo "==> Coverage summary:"
	@go tool cover -func=coverage.out | grep total | awk '{print "Total coverage: " $$3}'

## test-integration: Run integration tests (requires USB hardware)
test-integration:
	@echo "==> Running integration tests..."
	go test -v -race -tags=integration -timeout 5m ./...

## bench: Run benchmarks
bench:
	@echo "==> Running benchmarks..."
	go test -bench=. -benchmem -run=^$$ ./...

## coverage-html: Generate HTML coverage report
coverage-html: test-coverage
	@echo "==> Generating HTML coverage report..."
	go tool cover -html=coverage.out -o coverage.html
	@echo "==> Coverage report: coverage.html"

## lint: Run linters
lint:
ifndef GOLANGCI_LINT
	@echo "==> golangci-lint not installed. Installing..."
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
endif
	@echo "==> Running linters..."
	golangci-lint run --timeout 5m ./...

## fmt: Format code
fmt:
	@echo "==> Formatting code..."
	go fmt ./...
	@echo "==> Format complete"

## vet: Run go vet
vet:
	@echo "==> Running go vet..."
	go vet ./...

## check: Run all checks (fmt, vet, lint, test)
check: fmt vet lint test
	@echo "==> All checks passed ✓"

## sec: Run security scanner
sec:
ifndef GOSEC
	@echo "==> gosec not installed. Installing..."
	go install github.com/securego/gosec/v2/cmd/gosec@latest
endif
	@echo "==> Running security scanner..."
	gosec -quiet ./...

## deps: Download and verify dependencies
deps:
	@echo "==> Downloading dependencies..."
	go mod download
	go mod verify
	@echo "==> Dependencies verified"

## tidy: Tidy go.mod and go.sum
tidy:
	@echo "==> Tidying dependencies..."
	go mod tidy -v

## update-deps: Update all dependencies
update-deps:
	@echo "==> Updating dependencies..."
	go get -u ./...
	go mod tidy -v
	@echo "==> Dependencies updated"

## ci: Run all CI checks (used in GitHub Actions)
ci: deps check test-race test-coverage sec
	@echo "==> All CI checks passed ✓"

## dev: Install development tools
dev:
	@echo "==> Installing development tools..."
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	go install github.com/securego/gosec/v2/cmd/gosec@latest
	go install golang.org/x/tools/cmd/goimports@latest
	@echo "==> Development tools installed"

## version: Display version information
version:
	@echo "Version:    $(VERSION)"
	@echo "Commit:     $(COMMIT)"
	@echo "Build Time: $(BUILD_TIME)"
	@echo "Go Version: $(shell go version)"
