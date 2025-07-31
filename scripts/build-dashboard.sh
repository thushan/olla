#!/bin/bash

# Build the embedded dashboard for Olla

set -e

echo "Building Olla Dashboard..."

# Change to dashboard directory
cd web/dashboard

# Install dependencies if needed
if [ ! -d "node_modules" ]; then
    echo "Installing dependencies..."
    bun install
fi

# Build the dashboard
echo "Building dashboard for production..."
bun run build

# Copy built files to embed location
echo "Copying built files to embed location..."
rm -rf ../../internal/app/handlers/dashboard/dist
cp -r dist ../../internal/app/handlers/dashboard/

echo "Dashboard build complete!"
echo "Now run 'make build' to compile Olla with the embedded dashboard."