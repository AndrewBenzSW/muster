#!/bin/bash
set -euo pipefail

# Verify script for muster (Go project)
# This script runs the full build and test pipeline

echo "==> Checking for Go module..."
if [ ! -f "go.mod" ]; then
    echo "ERROR: go.mod not found. Run 'go mod init' to initialize the project."
    exit 1
fi

echo "==> Downloading Go dependencies..."
go mod download

echo "==> Building project..."
go build ./...

echo "==> Running tests..."
go test ./...

echo "==> All checks passed!"
