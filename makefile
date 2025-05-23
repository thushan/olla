# Version variables for build-time injection
PKG := github.com/thushan/olla/internal/version
VERSION := "v0.0.1"
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
DATE := $(shell date +%Y-%m-%dT%H:%M:%S%z)
USER := $(shell git config user.name 2>/dev/null || whoami)

# Build flags
LDFLAGS := -ldflags "\
	-X '$(PKG).Version=$(VERSION)' \
	-X '$(PKG).Commit=$(COMMIT)' \
	-X '$(PKG).Date=$(DATE)' \
	-X '$(PKG).User=$(USER)'"

.PHONY: run clean build test version

# Build the application with version info
build:
	@echo "Building olla $(VERSION)..."
	@go build $(LDFLAGS) -o bin/olla .

# Build release version (optimised)
build-release:
	@echo "Building olla $(VERSION) for release..."
	@CGO_ENABLED=0 go build $(LDFLAGS) -a -installsuffix cgo -o bin/olla .

# Run the application
run:
	@go run $(LDFLAGS) .

# Run with debug logging
run-debug:
	@OLLA_LOG_LEVEL=debug go run $(LDFLAGS) .

# Show version information that would be embedded
version:
	@echo "Version: $(VERSION)"
	@echo "Commit:  $(COMMIT)"
	@echo "Date:    $(DATE)"
	@echo "User:    $(USER)"

# Show embedded version from built binary
version-built: build
	@./bin/olla --version 2>/dev/null || echo "Add --version flag to main.go to see embedded version"

# Clean build artifacts and logs
clean:
	@rm -rf bin/ logs/

# Download dependencies
deps:
	@go mod download && go mod tidy

# Format code
fmt:
	@go fmt ./...

# Run tests
test:
	@go test -v ./...

# Development build (no optimisations)
dev:
	@echo "Building development version..."
	@go build $(LDFLAGS) -gcflags="all=-N -l" -o bin/olla-dev .

# Show help
help:
	@echo "Available targets:"
	@echo "  build         - Build optimised binary with version info"
	@echo "  build-release - Build release binary (static, optimised)"
	@echo "  run           - Run with version info"
	@echo "  run-debug     - Run with debug logging"
	@echo "  version       - Show version info that will be embedded"
	@echo "  version-built - Show version from built binary"
	@echo "  dev           - Build development binary (with debug symbols)"
	@echo "  test          - Run tests"
	@echo "  clean         - Clean build artifacts and logs"
	@echo "  deps          - Download and tidy dependencies"
	@echo "  fmt           - Format code"
	@echo "  help          - Show this help"