---
title: "Configuration File Management"
description: "How Olla resolves configuration files, override strategies for native and Docker deployments, and common file management pitfalls."
keywords: ["olla configuration", "config management", "docker config", "config.local.yaml", "configuration files"]
---

# Configuration File Management

This guide covers how to manage configuration files for production workloads.

For specific configuration values and settings, see the [Configuration Reference](../reference.md). 

For security settings, see [Security Best Practices](security.md).

## Resolution Order

By default, Olla starts with sane defaults and resolves configuration in this priority order (highest to lowest):

1. Command-line flags (`--config` or `-c`)
2. Environment variables 
      - `OLLA_CONFIG_FILE` environment variable for the configuration file
      - `OLLA_*` for specific configuration elements
3. Configuration files (first found):   
      - `config/config.local.yaml`
      - `config/config.yaml`
      - `config.yaml`
      - `default.yaml`

Higher-priority sources override lower-priority ones. This means `config.local.yaml` always takes precedence over `config.yaml`.

## The config.local.yaml Pattern

The core strategy for managing configuration is to never modify `config.yaml` directly. Instead, create a copy of `config.yaml` as `config.local.yaml`.

Once you've created a copy, keep settings that you wish to override and remove others to keep a lean overridden configuration file.

```bash
cp config/config.yaml config/config.local.yaml
vi config/config.local.yaml  # modify the settings you need here
```

This works because:

- `config.local.yaml` has higher resolution priority than `config.yaml`
- You can override the settings you need (Eg. Endpoint or Proxy Engine) and remove others
- Your changes survive updates (upgrades won't overwrite your file)
- The file is in `.gitignore`, so it won't be committed accidentally if you're developing locally
- You can see new settings in the `config.yaml` when updating and migrate to use them 

## Native Deployment

For native deployments (binary or source builds), create a minimal `config.local.yaml` containing only what you need to change:

```bash
# Create a minimal override file
cat > config/config.local.yaml << 'EOF'
discovery:
  type: "static"
  static:
    endpoints:
      - url: "http://localhost:11434"
        name: "local-ollama"
        type: "ollama"
        priority: 100
        preserve_path: true
        model_url: "/v1/models"
        health_check_url: "/"
        check_interval: 2s
        check_timeout: 1s
EOF
```

If you prefer to start from the full reference, copy and trim:

```bash
cp config/config.yaml config/config.local.yaml
vi config/config.local.yaml  # remove everything you don't need to override
```

## Docker Deployment

The Docker image ships with default configuration at `/app/config/config.yaml`. You have three approaches depending on how much you need to customise.

### Mount a Local Override (Recommended)

Create a minimal `config.local.yaml` and mount it into the container:

```bash
docker run -v $(pwd)/config.local.yaml:/app/config/config.local.yaml \
    -p 40114:40114 \
    ghcr.io/thushan/olla:latest
```

The container's default `config.yaml` remains intact; your mounted file takes priority.

### Extract and Customise the Full Config

If you need to see every available setting before deciding what to override:

```bash
# Extract the image's default config
docker run --rm ghcr.io/thushan/olla:latest \
    cat /app/config/config.yaml > config.local.yaml

# Edit, then mount
docker run -v $(pwd)/config.local.yaml:/app/config/config.local.yaml \
    -p 40114:40114 \
    ghcr.io/thushan/olla:latest
```

### Extract the Entire Config Directory

For full control including provider profiles:

```bash
docker create --name olla-tmp ghcr.io/thushan/olla:latest
docker cp olla-tmp:/app/config ./config
docker rm olla-tmp

# Create your local override
cp config/config.yaml config/config.local.yaml
vi config/config.local.yaml

# Mount the entire directory
docker run -v $(pwd)/config:/app/config \
    -p 40114:40114 \
    ghcr.io/thushan/olla:latest
```

## Environment Variables

Environment variables override config file values, useful for simple settings or CI/CD pipelines.

```bash
# Pattern
OLLA_<SECTION>_<KEY>=value

# Examples
OLLA_SERVER_PORT=8080
OLLA_PROXY_ENGINE=olla
OLLA_LOG_LEVEL=debug
```

**When to use environment variables:**

- Simple scalar overrides (host, port, log level)
- CI/CD deployments with varying settings per environment
- Container orchestration (Kubernetes, Docker Swarm)

**When to use config files instead:**

- Complex or nested structures (endpoints, profiles, discovery blocks)
- Anything that benefits from version control or peer review

## Configuration Validation

Olla validates configuration on startup. Check logs for validation errors:

```bash
olla --config config.local.yaml
# {"level":"INFO","msg":"Loaded configuration","config":"config/config.local.yaml"}
```

Common validation errors: missing required fields, invalid duration formats (must end in `s`, `m`, `h`), invalid URLs, or conflicting settings.

## Common Pitfalls

### Modifying config.yaml Directly

Updates and upgrades overwrite `config.yaml`. Always use `config.local.yaml` for your changes.

### Mounting a File as a Directory in Docker

Mount the specific file, not the directory path:

```bash
# Correct -- mounts the file
-v $(pwd)/config.local.yaml:/app/config/config.local.yaml

# Wrong -- overwrites the entire config directory
-v $(pwd)/config.local.yaml:/app/config/
```

### Binding to localhost in Docker

Containers need `0.0.0.0` to accept connections from outside the container. Using `localhost` or `127.0.0.1` means only processes inside the container can connect.

### Docker host.docker.internal on Linux

`host.docker.internal` may not resolve on Linux. Use `--add-host=host.docker.internal:host-gateway` or the host machine's actual IP address in your endpoint URLs.

## Next Steps

- [Configuration Reference](../reference.md) - Complete configuration options
- [Security Best Practices](security.md) - Network binding, rate limiting, request size limits
- [Performance Tuning](performance.md) - Optimise for your workload
- [Monitoring Guide](monitoring.md) - Track system health
