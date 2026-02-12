PKG := github.com/thushan/olla/internal/version
RUNTIME := Go v$(shell go version | awk '{print $$3}' | sed 's/go//')
VERSION := "v0.0.1"
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
DATE := $(shell date +%Y-%m-%dT%H:%M:%S%z)
USER := $(shell git config user.name 2>/dev/null || whoami)
TOOL := "make"

# Tool versions (pinned)
GOLANGCI_LINT_VERSION := v1.64.8
BETTERALIGN_VERSION := v0.8.2

LDFLAGS := -ldflags "\
	-X '$(PKG).Version=$(VERSION)' \
	-X '$(PKG).Runtime=$(RUNTIME)' \
	-X '$(PKG).Commit=$(COMMIT)' \
	-X '$(PKG).Date=$(DATE)' \
	-X '$(PKG).Tool=$(TOOL)' \
	-X '$(PKG).User=$(USER)'"

.PHONY: run clean build test test-verbose test-short test-race test-cover bench version install-deps check-deps

# Build the application with version info
build:
	@echo "Building olla $(VERSION)..."
	@go build $(LDFLAGS) -o bin/olla .

# Build release version (optimised)
build-release:
	@echo "Building olla $(VERSION) for release..."
	@mkdir -p bin
	@CGO_ENABLED=0 go build $(LDFLAGS) -a -installsuffix cgo -o bin/olla$(shell go env GOEXE) .

# Platform-specific validation (used by CI - builds both archs for current OS but runs only AMD64)
validate-linux:
	@echo "Building and testing Linux binaries..."
	@mkdir -p bin
	@CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o bin/olla-linux-amd64 .
	@CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build $(LDFLAGS) -o bin/olla-linux-arm64 .
	@echo "Testing Linux AMD64 binary..."
	@bin/olla-linux-amd64 --version > /dev/null && echo "Linux AMD64: OK"
	@echo "Testing Linux ARM64 binary..."
	@if bin/olla-linux-arm64 --version > /dev/null 2>&1; then \
		echo "Linux ARM64: OK (tested via QEMU)"; \
	else \
		echo "Linux ARM64: Build OK (runtime test failed/skipped)"; \
	fi
	@rm -f bin/olla-linux-*

validate-darwin:
	@echo "Building and testing macOS binaries..."
	@mkdir -p bin
	@CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) -o bin/olla-darwin-amd64 .
	@CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -o bin/olla-darwin-arm64 .
	@echo "Testing macOS binary..."
	@if [ "$$(uname -m)" = "arm64" ]; then \
		bin/olla-darwin-arm64 --version > /dev/null && echo "Darwin ARM64: OK"; \
		echo "AMD64 build: OK (compile-only)"; \
	else \
		bin/olla-darwin-amd64 --version > /dev/null && echo "Darwin AMD64: OK"; \
		echo "ARM64 build: OK (compile-only)"; \
	fi
	@rm -f bin/olla-darwin-*

validate-windows:
	@echo "Building and testing Windows binaries..."
	@mkdir -p bin
	@CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build $(LDFLAGS) -o bin/olla-windows-amd64.exe .
	@CGO_ENABLED=0 GOOS=windows GOARCH=arm64 go build $(LDFLAGS) -o bin/olla-windows-arm64.exe .
	@echo "Testing Windows AMD64 binary..."
	@bin/olla-windows-amd64.exe --version > /dev/null && echo "Windows AMD64: OK"
	@echo "ARM64 build: OK (compile-only)"
	@rm -f bin/olla-windows-*

# Validate all platforms (for local testing or releases - builds all 6 targets)
#
# Platform Support Matrix:
#   OS       | AMD64 | ARM64 |
#   ---------|-------|-------|
#   Linux    |  ✓    |  ✓    |
#   macOS    |  ✓    |  ✓    |  (ARM64 = Apple Silicon M1/M2/M3)
#   Windows  |  ✓    |  ✓    |  (ARM64 = Windows on ARM devices)
#
# This target builds all combinations but does NOT test execution
# Use the OS-specific targets (validate-linux, etc) to test execution
validate-all-platforms:
	@echo "Building all platforms (6 targets)..."
	@mkdir -p bin
	@echo "Linux AMD64..."
	@CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o bin/olla-linux-amd64 .
	@echo "Linux ARM64..."
	@CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build $(LDFLAGS) -o bin/olla-linux-arm64 .
	@echo "macOS AMD64 (Intel)..."
	@CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) -o bin/olla-darwin-amd64 .
	@echo "macOS ARM64 (Apple Silicon)..."
	@CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -o bin/olla-darwin-arm64 .
	@echo "Windows AMD64..."
	@CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build $(LDFLAGS) -o bin/olla-windows-amd64.exe .
	@echo "Windows ARM64..."
	@CGO_ENABLED=0 GOOS=windows GOARCH=arm64 go build $(LDFLAGS) -o bin/olla-windows-arm64.exe .
	@echo "All platforms built successfully (6 targets)"
	@rm -f bin/olla-*

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
	@go fmt ./...
	@echo "Running go fmt...Done!"

# Run linter
lint:
	@echo "Running golangci-lint..."
	@if command -v golangci-lint > /dev/null 2>&1; then \
		INSTALLED=$$(golangci-lint --version 2>/dev/null | grep -oE 'v[0-9]+\.[0-9]+\.[0-9]+' | head -1); \
		if [ "$$INSTALLED" = "$(GOLANGCI_LINT_VERSION)" ]; then \
			printf "  Version: %s \033[32m(verified)\033[0m\n" "$$INSTALLED"; \
		else \
			printf "  Version: %s [require: %s \033[31m(pinned)\033[0m]\n" "$$INSTALLED" "$(GOLANGCI_LINT_VERSION)"; \
		fi; \
		golangci-lint run --fix; \
	else \
		echo "golangci-lint not installed. Run 'make install-deps' or 'make check-deps' for more info."; \
	fi
	@echo "Running golangci-lint...Done!"

# Run betteralign
align:
	@echo "Running better-align..."
	@if command -v betteralign > /dev/null 2>&1; then \
		INSTALLED=$$(go version -m "$$(go env GOPATH)/bin/betteralign$$(go env GOEXE)" 2>/dev/null | grep -E '^\s+mod' | awk '{print $$3}'); \
		if [ "$$INSTALLED" = "$(BETTERALIGN_VERSION)" ]; then \
			printf "  Version: %s \033[32m(verified)\033[0m\n" "$$INSTALLED"; \
		else \
			printf "  Version: %s [require: %s \033[31m(pinned)\033[0m]\n" "$$INSTALLED" "$(BETTERALIGN_VERSION)"; \
		fi; \
		betteralign -apply ./...; \
	else \
		echo "betteralign not installed. Run 'make install-deps' or 'make check-deps' for more info."; \
	fi
	@echo "Running better-align...Done!"

# Install dependencies at pinned versions
install-deps:
	@echo "Installing dependencies..."
	@echo "  Installing golangci-lint $(GOLANGCI_LINT_VERSION)..."
	@go install github.com/golangci/golangci-lint/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)
	@echo "  Installing betteralign $(BETTERALIGN_VERSION)..."
	@go install github.com/dkorunic/betteralign/cmd/betteralign@$(BETTERALIGN_VERSION)
	@echo "Dependencies installed successfully!"

# Check installed tool versions against requirements
check-deps:
	@echo "Checking dependencies..."
	@if command -v golangci-lint > /dev/null 2>&1; then \
		INSTALLED=$$(golangci-lint --version 2>/dev/null | grep -oE 'v[0-9]+\.[0-9]+\.[0-9]+' | head -1); \
		if [ "$$INSTALLED" = "$(GOLANGCI_LINT_VERSION)" ]; then \
			printf "  golangci-lint: %s \033[32m(verified)\033[0m\n" "$$INSTALLED"; \
		else \
			printf "  golangci-lint: %s [require: %s \033[31m(pinned)\033[0m]\n" "$$INSTALLED" "$(GOLANGCI_LINT_VERSION)"; \
		fi \
	else \
		printf "  golangci-lint: not installed [require: %s \033[31m(pinned)\033[0m]\n" "$(GOLANGCI_LINT_VERSION)"; \
	fi
	@if command -v betteralign > /dev/null 2>&1; then \
		INSTALLED=$$(go version -m "$$(go env GOPATH)/bin/betteralign$$(go env GOEXE)" 2>/dev/null | grep -E '^\s+mod' | awk '{print $$3}'); \
		if [ "$$INSTALLED" = "$(BETTERALIGN_VERSION)" ]; then \
			printf "  betteralign: %s \033[32m(verified)\033[0m\n" "$$INSTALLED"; \
		else \
			printf "  betteralign: %s [require: %s \033[31m(pinned)\033[0m]\n" "$$INSTALLED" "$(BETTERALIGN_VERSION)"; \
		fi \
	else \
		printf "  betteralign: not installed [require: %s \033[31m(pinned)\033[0m]\n" "$(BETTERALIGN_VERSION)"; \
	fi

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
	@echo "  validate-linux  - Build and test Linux binaries (AMD64 + ARM64)"
	@echo "  validate-darwin - Build and test macOS binaries (Intel + Apple Silicon)"
	@echo "  validate-windows- Build and test Windows binaries (AMD64 + ARM64)"
	@echo "  validate-all-platforms - Build all 6 platform combinations (local testing)"
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
	@echo "  install-deps    - Install dependencies at pinned versions"
	@echo "  check-deps      - Check installed tool versions against requirements"
	@echo "  ci              - Run full CI pipeline locally"
	@echo "  help            - Show this help"