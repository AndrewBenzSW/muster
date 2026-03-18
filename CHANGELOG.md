# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.1.0]

### Added

- **CLI Framework**: Cobra-based command-line interface with root command and global flags (`--format`, `--verbose`)
- **Version Command**: `muster version` displays version metadata, commit hash, build date, Go version, and platform
- **Output Formatting**: Automatic TTY detection with JSON mode (non-interactive) and table mode (interactive), overridable via `--format` flag
- **Embedded Asset System**: go:embed infrastructure for prompt templates and Docker assets with placeholder files for validation
- **Cross-Platform Build System**: Makefile with targets for build, test, lint, install, and clean on Linux, macOS, and Windows
- **GoReleaser Configuration**: Automated cross-platform binary releases for Linux (amd64, arm64), macOS (amd64, arm64), and Windows (amd64)
- **CI/CD Pipeline**: GitHub Actions workflows for testing on 3-OS matrix (ubuntu-latest, macos-latest, windows-latest) with fail-fast disabled
- **Linting**: golangci-lint configuration with gofmt, govet, staticcheck, errcheck, gosec, and gocyclo
- **Testing Infrastructure**: testify assertions, table-driven test patterns, golden file test helpers, and embed smoke tests
- **Static Binaries**: CGO_ENABLED=0 for portable binaries with no C dependencies
- **Cross-Platform Path Handling**: All path construction uses filepath.Join(), temp directories use os.MkdirTemp(), TTY detection via golang.org/x/term
- **Line Ending Normalization**: .gitattributes enforces LF for all source files, templates, and scripts
