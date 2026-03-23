# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.5.0] - 2026-03-23

### Added

- **`muster out` Command**: Post-PR lifecycle management — monitors CI checks on a PR/MR, optionally invokes AI to fix CI failures (up to 3 retries), waits for merge, pulls latest changes, and cleans up the worktree, branch, and roadmap entry
- **`internal/git/` Package**: Low-level git operations (`RunGit`, `CurrentBranch`, `PullLatest`, `DeleteBranch`, `RemoveWorktree`) with explicit directory parameters on every function for cross-platform safety
- **`internal/vcs/` Package**: Platform abstraction for GitHub (`gh`) and GitLab (`glab`) CLIs behind a common `VCS` interface with `CommandRunner` injection for testability — supports auth checking, PR status viewing, CI check listing, failed log retrieval, and PR merging
- **Merge Strategy Configuration**: `merge_strategy` field on `ProjectConfig` with `direct`, `github-pr`, and `gitlab-mr` values, defaulting to `github-pr`
- **AI-Assisted CI Fixing**: When CI fails, `muster out` fetches failure logs and invokes AI to analyze and push fixes, with `--no-fix` flag to skip and configurable retry limit (default 3)
- **Fault-Tolerant Cleanup**: All cleanup steps (pull latest, remove worktree, delete branch, update roadmap) are attempted regardless of individual failures, with errors collected and reported together
- **`testutil.InitGitRepo` Helper**: Shared test utility for initializing git repos with deterministic branch names (`git init -b main`) and initial commits

## [0.4.0] - 2026-03-19

### Added

- **Mock AI Tool Infrastructure**: Centralized testing mock in `internal/testutil/` with dual implementation strategy — in-process function variable replacement for fast unit tests and pre-compiled binary for integration tests validating full subprocess contract
- **`testutil.MockInvokeAI(response, error)`**: In-process mock that replaces `ai.InvokeAI` with configurable behavior, returns cleanup function for automatic restoration
- **`testutil.MockInvokeAIWithQueue(responses...)`**: Stateful mock supporting sequential responses for testing workflows with multiple AI invocations
- **`testutil.NewMockAITool(t, response)`**: Pre-compiled binary mock implementing full tool contract (--print, --plugin-dir, --model flags, skills/SKILL.md reading) with zero test-time compilation overhead via sync.Once initialization
- **Mock Tool Configuration**: Chainable methods `WithError(code, stderr)` and `WithDelay(duration)` for simulating AI failures, timeouts, and latency without actual delays
- **Test Fixtures**: Common response data in `internal/testutil/fixtures.go` including ValidRoadmapItemJSON, HighConfidenceMatch, InvalidJSON, and error messages to eliminate duplication across test files
- **Expanded Sync Test Coverage**: Five new fuzzy matching tests covering low confidence prompts, AI failure fallback, timeout handling, multiple matches, and invalid slug rejection

### Changed

- **AI Invocation Function**: Exported `ai.InvokeAI` as reassignable package variable (backed by private `invokeAI` function) to enable test replacement while preserving all existing calling code
- **Test Infrastructure Consolidation**: Removed three duplicate mock implementations (TestMain from `internal/ai/invoke_test.go`, `createMockAITool` from `cmd/add_test.go`, `createSyncMockAITool` from `cmd/sync_test.go`) in favor of centralized `testutil` helpers
- **Test Execution Performance**: Eliminated 500ms-1s per-test-file compilation overhead by pre-compiling mock binary once per test run
- **Code Command Tests**: Enabled four previously skipped tests verifying tool-not-found errors, --no-plugin behavior, --keep-staged directory preservation, and --tool flag overrides

## [0.3.0] - 2026-03-18

### Added

- **`muster status` Command**: Display all roadmap items in a table (SLUG/TITLE/PRIORITY/STATUS columns) or detailed view for a specific slug, with `--format json` for pipe-friendly JSON output and a friendly empty-state message
- **`muster add` Command**: Create roadmap items in batch mode via flags (`--title`, `--priority`, `--context`) or interactively with AI-assisted slug generation, priority suggestion, and context expansion using the fast model tier
- **`muster sync` Command**: Unidirectional sync from source to target roadmap file with exact slug matching, AI-assisted fuzzy matching (confidence threshold-based), `--dry-run` preview, `--delete` for removing unmatched target items, and `--yes` for automation
- **Roadmap Persistence**: `internal/roadmap/` package for loading and saving `.muster/roadmap.json` with backward-compatible fallback to `.roadmap.json`, automatic format detection (wrapper vs array JSON), parse-error isolation (no fallback on malformed primary), and post-load validation of required fields, unique slugs, and enum values
- **Interactive Picker**: Fuzzy-filterable item selector using `charmbracelet/huh` behind a testable `Picker` interface, reusable by future commands (`in`, `out`, `plan`)
- **Non-Interactive AI Invocation**: `internal/ai/invoke.go` helper for programmatic AI tool invocation with prompt staging and structured JSON response capture, distinct from the interactive `muster code` pattern
- **Slug Generation**: `GenerateSlug()` utility for converting titles to URL-safe kebab-case identifiers (max 40 chars, unicode-safe)

- **Docker Container Orchestration**: `muster code --yolo` boots a sandboxed Docker container with AI coding tools, network proxy isolation, and all required provider authentication pre-configured
- **Multi-Provider Auth Detection**: Automatic detection and injection of credentials for Bedrock (AWS SSO with credential pre-resolution), Max (credential file mounting), Anthropic/OpenRouter (API key passthrough), and local inference servers (localhost URL rewriting to `host.docker.internal`)
- **Dynamic Compose Generation**: Programmatic Docker Compose file generation from embedded base template with runtime overrides for auth, workspace mounts, proxy domains, and container labels — written atomically with validation via `docker compose config`
- **Container Lifecycle Management**: Start, stop, exec, and list containers via hybrid Docker SDK + Compose CLI architecture with label-based discovery (`muster.managed`, `muster.project`, `muster.slug`, `muster.created`)
- **`muster down` Command**: Tear down containers by slug, `--all` for entire project, or `--orphans` to find containers whose roadmap item is no longer in progress (with 1-hour age threshold to protect active sessions)
- **Dev-Agent Config**: Parse `.muster/dev-agent/config.yml` for container environment settings (allowed domains, environment variables, extra volumes, networks) with `.local.yml` override support
- **Docker Asset Extraction**: Embedded Docker assets (Dockerfiles, compose files, proxy configs) extracted to `~/.cache/muster/docker-assets/{hash}/` with atomic temp-dir-then-rename and filesystem lock for concurrent safety
- **Git Worktree Detection**: Automatic detection of git worktree vs main repo via `git rev-parse` for correct volume mounting (worktree read-write, shared `.git` read-only)
- **Docker Compose Version Check**: Validates Docker Compose v2+ is installed at startup with clear upgrade instructions for v1 users
- **Proxy-Based Network Isolation**: Squid proxy container with merged domain allowlist from embedded defaults, provider-specific domains, and project configuration

### Changed

- **Config Validation**: Validation now collects all errors (missing providers, invalid tiers, missing API keys) and presents them together with actionable fix instructions including example commands
- **Deep Merge Semantics**: Clarified and documented merge behavior for `.local.yml` overrides — `null` removes fields, empty maps/lists are valid replacement values, lists always replace entirely

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
