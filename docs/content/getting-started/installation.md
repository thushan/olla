---
title: Installation Guide - Olla High-Performance LLM Proxy
description: Install Olla on Linux, macOS, or Windows using Docker, Go, or pre-built binaries. Complete setup guide for the high-performance LLM proxy and load balancer.
keywords: olla installation, llm proxy install, docker proxy, go install, binary download, linux install, macos install, windows install
---

# Installation

Get Olla running on your system with these installation options.

## Requirements

- **Operating System**: Linux, macOS, or Windows
- **CPU**: 2-4 Cores minimum
- **Memory**: Minimum 512MB RAM
- **Network**: Access to supported LLM endpoints you want to proxy

## Installation Methods

=== "Download Binary (Recommended)"

    You can use our script to install or update Olla easily:

    ```bash
    # Linux/macOS
    bash <(curl -s https://raw.githubusercontent.com/thushan/olla/main/install.sh)
    ```

    Alternatively, download pre-built binaries from the [releases page](https://github.com/thushan/olla/releases).

=== "Docker"

    Run Olla in a container:

    ```bash
    # Pull the image
    docker pull ghcr.io/thushan/olla:latest

    # Run with pretty terminal output 
    # for locally installed lmstudio, ollama or vllm
    docker run -t \
        --name olla \
        -p 40114:40114 \
        ghcr.io/thushan/olla:latest
    ```

=== "Go Install"

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
    make build-release
    # run freshly built olla!
    bin/olla --version
    ```

    The binary will be available at `./bin/olla`.


## Verification

Verify your installation works correctly:

```bash
# Check version
olla --version

# Run with default config (if available)
olla --config config.yaml

# Check health endpoint
curl http://localhost:40114/internal/health
```

## Next Steps

- [Quick Start Guide](quickstart.md) - Get your first proxy running
- [Configuration Reference](../configuration/reference.md) - Understand all configuration options
- [Architecture Overview](../development/architecture.md) - Learn how Olla works

## Troubleshooting

### Common Issues

**Command not found**
: Make sure `$GOPATH/bin` is in your `PATH` when using `go install`

**Permission denied**
: On Linux/macOS, ensure the binary has execute permissions: `chmod +x olla`

**Port already in use**
: Change the port in your configuration file or use `OLLA_SERVER_PORT` environment variable.

**Config file not found**
: Specify the config file path with `--config /path/to/config.yaml`

For more help, check the [troubleshooting guide](../development/contributing.md#troubleshooting) or [open an issue](https://github.com/thushan/olla/issues).