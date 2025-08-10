---
title: Development Setup - Configure Your Olla Development Environment
description: Complete guide to setting up your development environment for Olla. Prerequisites, tools, and configuration.
keywords: olla setup, development environment, go development, make commands
---

# Development Setup

This guide covers setting up a complete development environment for Olla.

## Prerequisites

### Required

- **Go 1.24+**: [Download Go](https://golang.org/dl/)
- **Git**: For version control
- **Make**: Build automation

### Recommended Tools

- **[golangci-lint](https://golangci-lint.run/usage/install/)**: Linting
- **[betteralign](https://github.com/dkorunic/betteralign)**: Struct alignment optimisation
- **[air](https://github.com/cosmtrek/air)**: Hot reload for development
- **Docker**: For testing with real backends

## Initial Setup

### 1. Clone Repository

```bash
git clone https://github.com/thushan/olla.git
cd olla
```

### 2. Install Dependencies

```bash
# Install Go dependencies
make deps

# Install development tools
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
go install github.com/dkorunic/betteralign@latest
go install github.com/cosmtrek/air@latest
```

### 3. Verify Setup

```bash
# Check Go version
go version

# Run tests to verify environment
make test

# Build development binary
make dev
```

## Development Workflow

### Build Commands

| Command | Description |
|---------|-------------|
| `make dev` | Build development binary with debug symbols |
| `make build` | Build production binary |
| `make build-release` | Build optimised release binary |
| `make clean` | Clean build artifacts |

### Testing Commands

| Command | Description |
|---------|-------------|
| `make test` | Run all tests |
| `make test-race` | Run tests with race detection |
| `make test-cover` | Generate coverage report |
| `make bench` | Run benchmarks |

### Code Quality

| Command | Description |
|---------|-------------|
| `make fmt` | Format code with gofmt |
| `make lint` | Run golangci-lint |
| `make vet` | Run go vet |
| `make ready` | Run all checks (fmt, lint, test) |

## Configuration

### Development Config

Create `config/config.local.yaml` for local development:

```yaml
server:
  host: localhost
  port: 40114
  request_logging: true  # Enable for debugging

proxy:
  engine: sherpa  # Simpler for debugging

discovery:
  static:
    endpoints:
      - url: "http://localhost:11434"
        name: "local-ollama"
        type: "ollama"
        priority: 100

logging:
  level: debug
  format: text  # Easier to read than JSON
```

### Environment Variables

```bash
# Development environment
export OLLA_LOG_LEVEL=debug
export OLLA_SERVER_REQUEST_LOGGING=true

# Use local config
export OLLA_CONFIG_FILE=config/config.local.yaml
```

## Hot Reload Setup

### Using Air

Create `.air.toml` in project root:

```toml
root = "."
testdata_dir = "testdata"
tmp_dir = "tmp"

[build]
  args_bin = ["--config", "config/config.local.yaml"]
  bin = "./tmp/main"
  cmd = "go build -o ./tmp/main ."
  delay = 1000
  exclude_dir = ["assets", "tmp", "vendor", "testdata", "docs"]
  exclude_file = []
  exclude_regex = ["_test.go"]
  exclude_unchanged = false
  follow_symlink = false
  full_bin = ""
  include_dir = []
  include_ext = ["go", "tpl", "tmpl", "html", "yaml"]
  include_file = []
  kill_delay = "0s"
  log = "build-errors.log"
  poll = false
  poll_interval = 0
  rerun = false
  rerun_delay = 500
  send_interrupt = false
  stop_on_error = false

[color]
  app = ""
  build = "yellow"
  main = "magenta"
  runner = "green"
  watcher = "cyan"

[log]
  main_only = false
  time = false

[misc]
  clean_on_exit = false

[screen]
  clear_on_rebuild = false
  keep_scroll = true
```

Run with hot reload:

```bash
air
```

### Using Make

Alternative using make:

```bash
# Watches for changes and rebuilds
make dev-watch
```

## IDE Configuration

### VS Code

`.vscode/settings.json`:

```json
{
  "go.lintTool": "golangci-lint",
  "go.lintFlags": [
    "--fast"
  ],
  "go.testFlags": ["-v"],
  "go.testTimeout": "10s",
  "go.buildTags": "",
  "editor.formatOnSave": true,
  "[go]": {
    "editor.codeActionsOnSave": {
      "source.organizeImports": true
    }
  }
}
```

`.vscode/launch.json`:

```json
{
  "version": "0.2.0",
  "configurations": [
    {
      "name": "Debug Olla",
      "type": "go",
      "request": "launch",
      "mode": "debug",
      "program": "${workspaceFolder}",
      "args": ["--config", "config/config.local.yaml"],
      "env": {
        "OLLA_LOG_LEVEL": "debug"
      }
    }
  ]
}
```

### GoLand/IntelliJ

1. Set Go SDK to 1.24+
2. Enable Go modules
3. Configure run configuration with `--config config/config.local.yaml`
4. Enable golangci-lint file watcher

## Testing Setup

### Local Test Backends

Start test backends for development:

```bash
# Ollama
docker run -d --name ollama-test \
  -p 11434:11434 \
  ollama/ollama

# Pull test model
docker exec ollama-test ollama pull llama3.2

# LM Studio mock
docker run -d --name lmstudio-mock \
  -p 1234:1234 \
  your-mock-image
```

### Running Tests

```bash
# Unit tests only
go test ./internal/...

# Integration tests
go test ./test/...

# Specific package with verbose output
go test -v ./internal/adapter/proxy/...

# With coverage
go test -cover -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

## Debugging

### Enable Debug Logging

```yaml
logging:
  level: debug
  format: text
```

### Using Delve

```bash
# Install delve
go install github.com/go-delve/delve/cmd/dlv@latest

# Debug the application
dlv debug -- --config config/config.local.yaml

# Debug a test
dlv test ./internal/adapter/proxy
```

### Performance Profiling

```bash
# CPU profiling
go test -cpuprofile cpu.prof -bench .
go tool pprof cpu.prof

# Memory profiling
go test -memprofile mem.prof -bench .
go tool pprof mem.prof
```

## Git Hooks

### Pre-commit Hook

Create `.git/hooks/pre-commit`:

```bash
#!/bin/sh
make ready
```

Make it executable:

```bash
chmod +x .git/hooks/pre-commit
```

## Common Issues

### Port Already in Use

```bash
# Find process using port
lsof -i :40114

# Kill process
kill -9 <PID>
```

### Module Dependencies

```bash
# Update dependencies
go mod tidy

# Download dependencies
go mod download

# Verify dependencies
go mod verify
```

### Build Errors

```bash
# Clean and rebuild
make clean
make deps
make build
```

## Next Steps

- Review [Contributing Guide](contributing.md) for code standards
- Check [Testing Guide](testing.md) for testing patterns
- See [Benchmarking](benchmarking.md) for performance testing