SHELL := bash

.PHONY: help build test lint install clean

# Detect OS
ifeq ($(OS),Windows_NT)
    BINARY_EXT := .exe
    RM := del /Q
    RMDIR := rmdir /S /Q
    MKDIR := mkdir
    SEP := \\
else
    BINARY_EXT :=
    RM := rm -f
    RMDIR := rm -rf
    MKDIR := mkdir -p
    SEP := /
endif

# Binary name
BINARY_NAME := muster$(BINARY_EXT)

# Version information
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
ifeq ($(OS),Windows_NT)
    BUILD_DATE := $(shell powershell -Command "Get-Date -Format 'yyyy-MM-ddTHH:mm:ssZ'")
else
    BUILD_DATE := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
endif

# Build flags
LDFLAGS := -s -w -X github.com/abenz1267/muster/cmd.version=$(VERSION) -X github.com/abenz1267/muster/cmd.commit=$(COMMIT) -X github.com/abenz1267/muster/cmd.date=$(BUILD_DATE)

help: ## Show this help message
	@echo "Available targets:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-15s %s\n", $$1, $$2}'

build: ## Build the binary
	@$(MKDIR) dist
	CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o dist/$(BINARY_NAME) .

test: ## Run tests with race detector
	go test -v -race ./...

lint: ## Run linter
	golangci-lint run --timeout=5m

install: ## Install the binary
	CGO_ENABLED=0 go install -ldflags "$(LDFLAGS)" .

clean: ## Remove build artifacts
	@$(RMDIR) dist 2>/dev/null || true
	@$(RMDIR) coverage 2>/dev/null || true
