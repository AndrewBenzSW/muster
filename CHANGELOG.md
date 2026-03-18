# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.2.0] - 2026-03-18

### Added

- **Config System**: Two-layer YAML configuration (user at `~/.config/muster/config.yml`, project at `.muster/config.yml` + `.local.yml` overrides) with 5-step resolution algorithm for determining tool/provider/model triples per pipeline step
- **Model Tier Resolution**: Support for `muster-fast`, `muster-standard`, `muster-deep` tier references that resolve to tool-specific concrete model names (e.g., `muster-deep` → `claude-opus-4` for claude, `qwen3:235b` for opencode)
- **Deep Merge**: Field-level deep merge for project config overrides where local values override base values but missing fields fall through, and lists replace entirely rather than append
- **Template System**: go:embed and text/template-based system for embedding workflow skill prompts (.md.tmpl files) with conditional blocks and variable substitution via PromptContext
- **Prompt Staging**: Automatic staging of resolved templates to ephemeral temp directories (`$TMPDIR/muster-prompts-*`) in Claude Code plugin directory format (`skills/roadmap-*/SKILL.md`)
- **Stale Temp Directory Cleanup**: Best-effort cleanup of abandoned temp directories older than 24 hours on startup
- **muster code Command**: Launch Claude Code or OpenCode with workflow skills automatically staged and loaded via `--plugin-dir` flag
- **Config Flags**: `--tool` flag to override resolved tool selection on any command
- **Staging Flags**: `--no-plugin` flag to skip staging and launch bare tool, `--keep-staged` flag to preserve temp directory and print path for inspection
- **Workflow Skills**: Three core workflow skills embedded as templates: roadmap-plan-feature (research, synthesis, planning), roadmap-execute-plan (phased implementation), roadmap-review-implementation (adversarial review and remediation)
- **gopkg.in/yaml.v3**: Promoted from indirect to direct dependency for YAML configuration parsing

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
