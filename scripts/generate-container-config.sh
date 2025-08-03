#!/bin/bash
# creates a Container configuration file from the base configuration.

# Update endpoints to use host.docker.internal for local development
sed 's|- url: "http://localhost:|- url: "http://host.docker.internal:|g; s|- url: "http://127\.0\.0\.1:|- url: "http://host.docker.internal:|g' config/config.yaml > config/docker.yaml

# update the lmstudio port from 11234 to 1234 (default)
sed -i 's|http://host.docker.internal:11234|http://host.docker.internal:1234|g' config/docker.yaml

# update the host: "localhost" to be 0.0.0.0 for Docker compatibility
sed -i 's|host: "localhost"|host: "0.0.0.0"|g' config/docker.yaml

# update the header  from '# Olla Configuration (default)' to '# Olla Configuration (docker)'
sed -i 's|# Olla Configuration (default)|# Olla Configuration (docker)|g' config/docker.yaml