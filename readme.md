# Olla -  The Sherpa proxy for Ollama nodes

![Olla](https://raw.githubusercontent.com/ollama/olla/main/docs/images/logo.png)

Just as a mountain guide leads climbers safely over crevasses and up rugged peaks, Olla navigates your AI requests across multiple Ollama nodes—always choosing the healthiest, highest-priority endpoint, then smoothly falling back when conditions change.

- **Priority-aware routing**: Configure node priorities (workstation first, laptop second) and let Olla guide each request to the best available server at the time of query.
- **Health-checked failover**: Continuous probes ensure you never get stranded. Olla automatically reroutes around offline or overloaded nodes.
- **Plugin-driven**: Extend load-balancing, auth, rate-limiting or metrics via Go's plugin system—keep the core lean and flexible.
- **Docker-ready**: Launch via Docker Compose on any machine, hot-reload your config and hit the trail without friction.

Climb higher with confidence—let Olla shepherd your Ollama workloads.

## Quick Start

```bash
# Install dependencies
make deps

# Run with default settings
make run

# Run with debug logging
make run-debug

# Build optimised binary
make build

# Show version information
make version
./bin/olla --version
```

## Build System

Olla automatically embeds version information at build time:

```bash
make build         # Standard build with version info
make build-release # Optimised static build for releases
make version       # Show what version info will be embedded
make version-built # Show version from built binary
```

Version information is automatically detected from Git tags and commit hashes.

## Logging

Olla features dual-output structured logging:

- **Terminal**: Coloured, human-readable logs with theme support
- **File**: Clean JSON logs in `./logs/app.log` with automatic rotation
- **TTY Detection**: Colours automatically disabled when piped or redirected

## Configuration

Configure via environment variables:

- `OLLA_LOG_LEVEL`: `debug`, `info`, `warn`, `error` (default: `info`)
- `OLLA_FILE_OUTPUT`: `true`/`false` (default: `true`)
- `OLLA_THEME`: `default`, `dark`, `light` (default: `default`)
- `OLLA_LOG_DIR`: Log directory (default: `./logs`)
- `OLLA_MAX_SIZE`: Max log file size in MB (default: `100`)
- `OLLA_MAX_BACKUPS`: Max backup files to keep (default: `5`)
- `OLLA_MAX_AGE`: Max age of log files in days (default: `30`)
