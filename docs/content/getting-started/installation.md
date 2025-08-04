# Installation

Get Olla running on your system with these installation options.

## Requirements

- **Go**: Version 1.24 or later
- **Operating System**: Linux, macOS, or Windows
- **Memory**: Minimum 512MB RAM
- **Network**: Access to LLM endpoints you want to proxy

## Installation Methods

=== "Go Install (Recommended)"

    Install the latest stable version directly from the Go module:

    ```bash
    go install github.com/thushan/olla@latest
    ```

    Verify the installation:

    ```bash
    olla --version
    ```

=== "From Source"

    Build from source for the latest features:

    ```bash
    git clone https://github.com/thushan/olla.git
    cd olla
    make build
    ```

    The binary will be available at `./bin/olla`.

=== "Docker"

    Run Olla in a container:

    ```bash
    # Pull the image
    docker pull thushan/olla:latest

    # Run with your config
    docker run -d \
      --name olla \
      -p 8080:8080 \
      -v $(pwd)/config.yaml:/app/config.yaml \
      thushan/olla:latest
    ```

=== "Binary Releases"

    Download pre-built binaries from the [releases page](https://github.com/thushan/olla/releases):

    ```bash
    # Linux/macOS
    curl -LO https://github.com/thushan/olla/releases/latest/download/olla-linux-amd64
    chmod +x olla-linux-amd64
    sudo mv olla-linux-amd64 /usr/local/bin/olla

    # Windows (PowerShell)
    Invoke-WebRequest -Uri "https://github.com/thushan/olla/releases/latest/download/olla-windows-amd64.exe" -OutFile "olla.exe"
    ```

## Development Setup

For development and contributing:

```bash
git clone https://github.com/thushan/olla.git
cd olla

# Install dependencies and run tests
make ready

# Start development server with auto-reload
make dev
```

### Development Commands

| Command | Description |
|---------|-------------|
| `make ready` | Run before commit (test + lint + fmt) |
| `make dev` | Development mode with auto-reload |
| `make test` | Run all tests |
| `make bench` | Run benchmarks |
| `make build` | Build production binary |

## Verification

Verify your installation works correctly:

```bash
# Check version
olla --version

# Run with default config (if available)
olla --config config.yaml

# Check health endpoint
curl http://localhost:8080/internal/health
```

## Next Steps

- [Quick Start Guide](quickstart.md) - Get your first proxy running
- [Configuration Reference](../config/reference.md) - Understand all configuration options
- [Architecture Overview](../architecture/overview.md) - Learn how Olla works

## Troubleshooting

### Common Issues

**Command not found**
: Make sure `$GOPATH/bin` is in your `PATH` when using `go install`

**Permission denied**
: On Linux/macOS, ensure the binary has execute permissions: `chmod +x olla`

**Port already in use**
: Change the port in your configuration file or use `--port` flag

**Config file not found**
: Specify the config file path with `--config /path/to/config.yaml`

For more help, check the [troubleshooting guide](../development/contributing.md#troubleshooting) or [open an issue](https://github.com/thushan/olla/issues).