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

    !!! warning "No dashboard with `go install`"
        A `go install` binary embeds only a placeholder `dist/`, so `/internal/ui/` serves a
        `503 dashboard not built` response instead of the admin dashboard. Use a release binary
        or the Docker image if you want the dashboard, or run `make build-web` before `make build`
        when building from source. See [Admin Dashboard](../configuration/dashboard.md#building-the-frontend).

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

## Command-line Flags

Olla exposes a deliberately small set of CLI flags. Configuration belongs in the YAML file or environment variables (see [Configuration Reference](../configuration/reference.md)).

| Flag | Description |
|---|---|
| `--version` | Print version, build commit, and Go runtime, then exit. Equivalent to `OLLA_SHOW_VERSION=true`. |
| `--profile` | Start a pprof profiling server on `localhost:19841` (visit `/debug/pprof/`). Equivalent to `OLLA_ENABLE_PROFILER=true`. Note: this is the profiling flag, not a port flag. There is no `-p`/`--port` flag; set the port in your config or via `OLLA_SERVER_PORT`. |
| `-c`, `--config <path>` | Path to a YAML config file. Falls back to `OLLA_CONFIG_FILE`, then the built-in defaults. |

`-h` / `--help` is provided automatically by Go's flag package and prints the list above. Running `olla` without arguments uses the YAML config from `-c`/`--config` if set, then `OLLA_CONFIG_FILE`, otherwise the built-in defaults.

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
: Change the port in your YAML config file (`server.port`) or via the `OLLA_SERVER_PORT` environment variable. There is no `--port` CLI flag.

**Config file not found**
: Specify the config file path with `--config /path/to/config.yaml`

For more help, check the [troubleshooting guide](../development/contributing.md#troubleshooting) or [open an issue](https://github.com/thushan/olla/issues).