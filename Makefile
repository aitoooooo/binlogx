.PHONY: build install cross-compile test clean help

# Variables
VERSION := $(shell git describe --tags --always 2>/dev/null || echo "dev")
BUILD_TIME := $(shell date -u '+%Y-%m-%d %H:%M:%S')
GIT_COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
GIT_BRANCH := $(shell git rev-parse --abbrev-ref HEAD 2>/dev/null || echo "unknown")

LDFLAGS := -ldflags "-X 'github.com/aitoooooo/binlogx/pkg/version.Version=$(VERSION)' \
	-X 'github.com/aitoooooo/binlogx/pkg/version.BuildTime=$(BUILD_TIME)' \
	-X 'github.com/aitoooooo/binlogx/pkg/version.GitCommit=$(GIT_COMMIT)' \
	-X 'github.com/aitoooooo/binlogx/pkg/version.GitBranch=$(GIT_BRANCH)'"

build:
	@echo "Building binlogx..."
	go build -o bin/binlogx $(LDFLAGS) .

install: build
	@echo "Installing binlogx to ~/go/bin..."
	cp bin/binlogx ~/go/bin/binlogx
	@echo "Installation complete. Binary at ~/go/bin/binlogx"

cross-compile:
	@echo "Cross-compiling for multiple platforms..."
	mkdir -p bin
	GOOS=linux GOARCH=amd64 go build -o bin/binlogx-linux-amd64 $(LDFLAGS) .
	GOOS=linux GOARCH=arm64 go build -o bin/binlogx-linux-arm64 $(LDFLAGS) .
	GOOS=darwin GOARCH=amd64 go build -o bin/binlogx-darwin-amd64 $(LDFLAGS) .
	GOOS=darwin GOARCH=arm64 go build -o bin/binlogx-darwin-arm64 $(LDFLAGS) .
	GOOS=windows GOARCH=amd64 go build -o bin/binlogx-windows-amd64.exe $(LDFLAGS) .
	@echo "Cross-compilation complete"

test:
	@echo "Running tests..."
	go test -v -race -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

clean:
	@echo "Cleaning..."
	rm -rf bin/ coverage.out coverage.html

help:
	@echo "Available targets:"
	@echo "  build           - Build binlogx binary"
	@echo "  install         - Install to ~/go/bin"
	@echo "  cross-compile   - Cross-compile for 5 platforms"
	@echo "  test            - Run tests with coverage"
	@echo "  clean           - Remove build artifacts"
	@echo "  help            - Show this help message"
