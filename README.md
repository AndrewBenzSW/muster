# muster

A CLI that consolidates AI-assisted development workflows into a single tool. It manages the full lifecycle of roadmap items — from planning through implementation, review, and release — using AI coding agents orchestrated through configurable pipelines.

muster replaces a fragmented system of shell scripts, Claude Code skills, Docker configs, and lifecycle hooks with one coherent interface: one tool, one install, one workflow.

## Why "muster"?

The name maps naturally to every phase of the workflow:

- **"Muster" as a verb** — gather, assemble, dispatch. The workflow gathers roadmap items, assembles a plan, and dispatches agents to execute it.
- **"Pass muster"** — the verify and review gates are literally about whether work passes muster.
- **Military muster** — roll call before a mission. Fits the batched, sortie-like execution model where items are selected, checked, and sent out.

**"Muster in"** and **"muster out"** are real terms: *muster in* means to enlist into active service, *muster out* means to discharge upon completion. Items enter the pipeline with `muster in` and leave it with `muster out`.

## Commands

```
muster in [slug]          Full development pipeline for a roadmap item
muster in --next          Advance one step at a time
muster out [slug]         Post-PR lifecycle: monitor CI, ensure merge, cleanup
muster plan [slug]        Standalone research and planning
muster code               Launch an interactive coding agent with workflow skills
muster code --yolo        ...in a sandboxed container
muster status             Show all roadmap items and their state
muster add <description>  AI-assisted item creation
muster sync               Reconcile README and roadmap data
muster down [slug]        Tear down containers
muster init               Set up a project for muster
muster doctor             Health check project configuration
muster update             Self-update from GitHub Releases
```

## Pipeline

`muster in` walks a roadmap item through a configurable pipeline:

**worktree** → **setup** → **plan** → **execute** → **verify** → **review** → **prepare** → **finish**

Each step can use a different AI tool, provider, and model — configured per-project with per-user overrides. Run the full pipeline at once, advance one step at a time with `--next`, or process all eligible items in batch with `--all`.

## Configuration

muster uses a two-layer config system:

- **User config** (`~/.config/muster/config.yml`) — defines available tools, providers, model tiers, and defaults.
- **Project config** (`.muster/config.yml`) — pipeline step overrides, merge strategy, lifecycle hooks. Container settings live in `.muster/dev-agent/config.yml`. Both support `.local.yml` overrides that are gitignored.

## Prerequisites

- Go 1.23 or later
- Docker Desktop (for `muster code --yolo` and `muster down` commands)
  - Linux/macOS: Docker Engine with Docker Compose v2+ plugin
  - Windows: Docker Desktop with WSL2 backend and file sharing enabled for project directories

## Building

```bash
# Build the binary (creates dist/muster or dist/muster.exe)
make build

# Run tests
make test

# Install to GOPATH/bin
make install

# See all available targets
make help
```

## Usage

Currently implemented commands:

```bash
# Display version information
./dist/muster version

# Launch an interactive coding agent with workflow skills
./dist/muster code

# Launch in a sandboxed Docker container with isolated environment
./dist/muster code --yolo

# Stop and remove Docker containers
./dist/muster down              # Stop all containers for current project
./dist/muster down my-feature   # Stop containers for specific slug
./dist/muster down --orphans    # Stop orphaned containers (not in_progress)
./dist/muster down --all        # Explicitly stop all containers

# Show help for any command
./dist/muster code --help
./dist/muster down --help
```

The `muster code` command launches Claude Code with a set of skills that understand the muster workflow. It uses the resolved configuration from your user and project configs to select the appropriate tool, provider, and model.

With the `--yolo` flag, it runs the agent in a Docker container with full orchestration:
- Authentication collection (AWS Bedrock, Claude Max, API keys, local providers)
- Workspace detection (worktree vs main repo)
- Docker Compose generation with proper volume mounts
- Container lifecycle management

The `muster down` command manages Docker containers created by muster:
- Without arguments: stops all containers for the current project
- With `--orphans`: stops containers whose slugs are no longer `in_progress` in the roadmap
- Respects the 1-hour grace period for new containers

Other commands listed above (`muster in`, `muster out`, `muster plan`, etc.) are deferred to future phases.

## Makefile Targets

Run `make help` to see all available targets:

- `build` - Build the binary with version metadata
- `test` - Run tests with race detector
- `lint` - Run golangci-lint (requires golangci-lint to be installed)
- `install` - Install binary to GOPATH
- `clean` - Remove build artifacts

## Status

Phase 2 (docker-orchestration) is complete, implementing full Docker container orchestration:

- **Configuration system** with 5-step resolution chain
- **Prompt template** loading and rendering with template variables
- **`muster code`** command that launches interactive coding agents
- **`muster code --yolo`** for sandboxed Docker container execution with:
  - Authentication collection (Bedrock, Max, API keys, local providers)
  - Workspace detection and mounting (worktree + main repo)
  - Docker Compose generation with auth/volume/network configuration
  - Container lifecycle management
- **`muster down`** command for container cleanup with orphan detection
- **Model tier support** for multi-tier tool configurations
- **Cross-platform testing** on Linux, macOS, and Windows

See [docs/design.md](docs/design.md) for the full design document and [CHANGELOG.md](CHANGELOG.md) for detailed phase progression.
