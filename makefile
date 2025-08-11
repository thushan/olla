PKG := github.com/thushan/olla/internal/version
RUNTIME := Go v$(shell go version | awk '{print $$3}' | sed 's/go//')
VERSION := "v0.0.1"
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
DATE := $(shell date +%Y-%m-%dT%H:%M:%S%z)
USER := $(shell git config user.name 2>/dev/null || whoami)
TOOL := "make"

LDFLAGS := -ldflags "\
	-X '$(PKG).Version=$(VERSION)' \
	-X '$(PKG).Runtime=$(RUNTIME)' \
	-X '$(PKG).Commit=$(COMMIT)' \
	-X '$(PKG).Date=$(DATE)' \
	-X '$(PKG).Tool=$(TOOL)' \
	-X '$(PKG).User=$(USER)'"

.PHONY: run clean build test test-verbose test-short test-race test-cover bench version

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

# Run tests
test:
	@echo "Running tests..."
	@go test ./...

# Run tests with verbose output
test-verbose:
	@echo "Running tests with verbose output..."
	@go test -v ./...

# Run tests with short flag
test-short:
	@echo "Running short tests..."
	@go test -short ./...

# Run tests with race detection
test-race:
	@echo "Running tests with race detection..."
	@go test -race -short ./...

# Run tests with coverage
test-cover:
	@echo "Running tests with coverage..."
	@go test -cover ./...

# Run stress tests (comprehensive testing)
test-stress:
	@echo "Running comprehensive stress tests..."
	@go test -v ./... -run "Stress"

# Run tests with coverage profile and generate HTML report
test-cover-html:
	@echo "Running tests with coverage and generating HTML report..."
	@go test -coverprofile=coverage.out ./...
	@go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

# Run benchmark tests
bench:
	@echo "Running benchmarks..."
	@go test -bench=. -benchmem ./...

# Run repository benchmarks
bench-repo:
	@echo "Running repository benchmarks..."
	@go test -bench=BenchmarkRepository -benchmem ./internal/adapter/discovery/

# Run balancer benchmarks
bench-balancer:
	@echo "Running balancer benchmarks..."
	@go test -bench=BenchmarkFactory -benchmem ./internal/adapter/balancer/
	@go test -bench=BenchmarkPriority -benchmem ./internal/adapter/balancer/
	@go test -bench=BenchmarkRoundRobin -benchmem ./internal/adapter/balancer/
	@go test -bench=BenchmarkLeastConnections -benchmem ./internal/adapter/balancer/

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
	@rm -rf bin/ build/ dist/ logs/ coverage.out coverage.html

# Download dependencies
deps:
	@go mod download && go mod tidy

ready-tools: fmt lint align
	@echo -e "\033[32mCode is clean for tests!\033[0m"

# Make code ready for commit (test, test-race, fmt, lint, align)
ready: test-short test-race fmt lint align
	@echo -e "\033[32mCode is ready for commit!\033[0m"

# Build binaries only (no archives) to ./build directory
build-local:
	@echo "Building local binaries to ./build/..."
	@goreleaser build --snapshot --clean --single-target --output ./build/
	@echo "Binary created: ./build/olla$(shell go env GOEXE)"

# Build full snapshot release (with archives) to ./dist directory
build-snapshot:
	@echo "Building full snapshot release to ./dist/..."
	@goreleaser release --snapshot --clean
	@echo "Release artifacts in ./dist/"

# Deprecated: use build-local or build-snapshot instead
ready-local: build-snapshot

# Build and test Docker image locally
docker-build:
	@echo "Building Docker image locally..."
	@goreleaser release --snapshot --clean --skip=publish,announce,sign,sbom
	@echo "Docker images:"
	@docker images | grep olla | head -5

# Run Docker image with local config
docker-run:
	@echo "Running Docker image with local config..."
	@docker run --rm -it \
		-p 40114:40114 \
		-v "$(shell pwd)/config/config.local.yaml:/config/config.yaml:ro" \
		-e OLLA_CONFIG_FILE=/config/config.yaml \
		ghcr.io/thushan/olla:latest

# Test full release locally (binaries + docker + archives)
release-test:
	@echo "Testing full release locally..."
	@goreleaser release --snapshot --clean
	@echo "Release artifacts in ./dist/"
	@echo "Docker images:"
	@docker images | grep olla | head -5

# Test goreleaser configuration
goreleaser-check:
	@echo "Checking goreleaser configuration..."
	@goreleaser check

# Format code
fmt:
	@echo "Running go fmt..."
	@go fmt . ./internal/... ./pkg/... 2>/dev/null || true
	@echo "Running go fmt...Done!"

# Run linter
lint:
	@echo "Running golangci-lint..."
	@which golangci-lint > /dev/null && golangci-lint run --fix || echo "golangci-lint not installed, skipping..."
	@echo "Running golangci-lint...Done!"

# Run betteralign
align:
	@echo "Running better-align..."
	@which betteralign > /dev/null && betteralign -apply ./... || echo "betteralign not installed, skipping..."	
	@echo "Running better-align...Done!"

# Development build (no optimisations)
dev:
	@echo "Building development version..."
	@go build $(LDFLAGS) -gcflags="all=-N -l" -o bin/olla-dev .

# Run full CI pipeline locally
ci: deps fmt lint test-race test-cover build
	@echo "CI pipeline completed successfully!"

# Docker compose up with local config
docker-compose:
	@echo "Starting with docker-compose..."
	@docker-compose up

# Show help
help:
	@echo "Available targets:"
	@echo "  build           - Build optimised binary with version info"
	@echo "  build-release   - Build release binary (static, optimised)"
	@echo "  run             - Run with version info"
	@echo "  run-debug       - Run with debug logging"
	@echo "  test            - Run all tests"
	@echo "  test-verbose    - Run tests with verbose output"
	@echo "  test-short      - Run short tests only"
	@echo "  test-race       - Run tests with race detection"
	@echo "  test-cover      - Run tests with coverage"
	@echo "  test-stress     - Run comprehensive stress tests (may take 30+ seconds)"
	@echo "  test-cover-html - Run tests with coverage and generate HTML report"
	@echo "  bench           - Run all benchmarks"
	@echo "  bench-repo      - Run repository benchmarks"
	@echo "  bench-balancer  - Run balancer benchmarks"
	@echo "  version         - Show version info that will be embedded"
	@echo "  version-built   - Show version from built binary"
	@echo "  dev             - Build development binary (with debug symbols)"
	@echo "  clean           - Clean build artifacts and logs"
	@echo "  deps            - Download and tidy dependencies"
	@echo "  ready     		 - Make code ready for commit (test, fmt, lint, align)"
	@echo "  ready-tools     - Check code is ready with tools (fmt, lint, align)"
	@echo "  build-local     - Build binary only to ./build/ (fast, for testing)"
	@echo "  build-snapshot  - Build full release to ./dist/ (archives, checksums, etc)"
	@echo "  docker-build    - Build Docker images locally"
	@echo "  docker-run      - Run Docker image with local config"
	@echo "  docker-compose  - Run with docker-compose"
	@echo "  release-test    - Test full release (binaries + docker + archives)"
	@echo "  goreleaser-check- Check goreleaser configuration"
	@echo "  fmt             - Format code"
	@echo "  lint            - Run linter (requires golangci-lint)"
	@echo "  align           - Run alignment checker (requires betteralign)"
	@echo "  ci              - Run full CI pipeline locally"
	@echo "  help            - Show this help"