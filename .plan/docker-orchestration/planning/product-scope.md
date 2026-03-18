# Product Scope: Docker Container Orchestration

*Product Planner — 2026-03-18*

## Overview

This feature implements Docker-based sandboxed execution for AI coding sessions in the muster CLI (Phase 2 of the overall roadmap). It enables users to run coding agents in isolated containers with controlled network access, dynamic authentication injection, and lifecycle management. The feature bridges user configuration (tools, providers, models) with runtime container orchestration.

## User Stories

### Story 1: Sandboxed Interactive Coding Session

**As a** developer using muster
**I want to** spawn an interactive Claude Code session in a sandboxed Docker container
**So that** AI agents can't access my network or filesystem beyond what I explicitly allow

**Acceptance Criteria:**
- `muster code --yolo` launches Claude Code in a container with the current worktree mounted
- Container has network access restricted to an allowlist of domains (provider APIs, npm registry, etc.)
- Container cannot access host filesystem beyond the mounted worktree and read-only credentials
- Skills are available inside the container without manual configuration
- Container persists across commands until explicitly torn down with `muster down`

**Trace to Requirements:** synthesis.md MUST §Container Orchestration, §Network Isolation, devcontainer-patterns.md §Container Lifecycle Management

---

### Story 2: Multi-Provider Authentication

**As a** developer with access to multiple AI providers
**I want to** configure which provider/model each pipeline step uses
**So that** I can use Anthropic's hosted Opus for planning, a local Qwen for execution, and OpenRouter for review

**Acceptance Criteria:**
- User config (`~/.config/muster/config.yml`) defines available tools, providers, and model tiers
- Project config (`.muster/config.yml`) specifies tool+provider+model per pipeline step
- Container startup injects all required authentication (AWS credentials, API keys, credential files) based on which providers are referenced by any pipeline step
- Auth validation happens at container startup with clear error messages for missing credentials
- `muster doctor --config` shows the resolved (tool, provider, model) triple for each step with source layer

**Trace to Requirements:** synthesis.md MUST §Provider Authentication, §Config Resolution, config-system.md §Config Resolution Flow, §Provider Auth Detection Patterns

---

### Story 3: Container Lifecycle Management

**As a** developer running a multi-step pipeline
**I want** the container to persist across all pipeline steps
**So that** execution is fast and state (installed packages, git history) is preserved between steps

**Acceptance Criteria:**
- Container boots once at pipeline start (before the first step)
- Each step executes via `docker compose exec` with the appropriate tool+model
- Container switches tools (Claude Code vs OpenCode) via different exec commands, not container restarts
- Container stays running after pipeline completes (user inspects it for debugging)
- `muster down <slug>` explicitly tears down containers by slug
- Failed steps leave containers running so users can debug them

**Trace to Requirements:** synthesis.md MUST §Container Lifecycle, devcontainer-patterns.md §Container Restart vs Exec, design.md Phase 2 deliverables

---

### Story 4: Container Discovery and Cleanup

**As a** developer managing multiple work items in parallel
**I want to** see which containers are running and clean them up by project or slug
**So that** I can manage Docker resource usage and avoid orphaned containers

**Acceptance Criteria:**
- All containers created by muster have Docker labels: `muster.managed=true`, `muster.project=<name>`, `muster.slug=<slug>`
- `muster status` shows container state (running/stopped) alongside each roadmap item
- `muster down` without args tears down containers for the current project
- `muster down <slug>` tears down containers for a specific work item
- `muster down --all` tears down all containers for the current project
- `muster down --orphans` finds containers whose slug is no longer in the roadmap and tears them down
- `muster down --project <name>` targets a different project

**Trace to Requirements:** synthesis.md MUST §Container Orchestration (label discovery), devcontainer-patterns.md §Container Labeling and Discovery, design.md §Docker Integration

---

### Story 5: Dynamic Compose File Generation

**As a** developer with custom volume mounts and environment variables
**I want to** configure my container environment per-project
**So that** containers have access to databases, services, or tools specific to each project

**Acceptance Criteria:**
- Base compose file is embedded in the muster binary
- Container config (`.muster/dev-agent/config.yml`) specifies allowed domains, env vars, volumes, networks
- `.muster/dev-agent/config.local.yml` (gitignored) provides per-user overrides
- Muster generates a single merged compose file at `~/.cache/muster/{project}/docker-compose.yml`
- Compose file includes: auth volumes/env vars (detected from config), workspace mounts (from worktree), proxy config (merged allowlist), user's custom volumes/env/networks
- Compose file is validated with `docker compose config` before container startup

**Trace to Requirements:** synthesis.md MUST §Compose File Generation, §Volume Mounting, compose-generation.md §Override Merging Strategy, devcontainer-patterns.md §Multiple Compose Files

---

### Story 6: Local Inference Server Support

**As a** developer running LM Studio or Ollama locally
**I want** containers to reach my local inference server
**So that** I can use local models without exposing them to the internet or paying for API calls

**Acceptance Criteria:**
- User config can specify `provider: local` with `base_url: http://localhost:1234`
- Muster rewrites `localhost` URLs to `host.docker.internal` for container context
- Proxy allowlist and firewall rules permit access to `host.docker.internal`
- Container startup validates local provider is reachable (HTTP GET with 2s timeout)
- Clear error message if local server is not running, suggesting user start it

**Trace to Requirements:** synthesis.md MUST §Provider-Specific Auth Patterns (Local providers), config-system.md §Local Provider Container Networking, devcontainer-patterns.md §Local Inference Server Patterns

---

## MVP Scope

### In Scope for Phase 2

**Container orchestration core:**
- Docker Compose CLI integration for lifecycle (up, down, exec)
- Docker SDK for container queries (list, inspect, labels)
- Single merged compose file generation with manual merge semantics
- Container labeling for discovery (`muster.managed`, `muster.project`, `muster.slug`)

**Authentication system:**
- Per-provider detection: Bedrock (AWS config), Max (credentials.json), API keys (env vars), local (URL rewriting)
- Upfront auth scanning across all pipeline steps
- Validation at container startup with fail-fast errors
- File-based credential mounting (read-only) over environment variables

**Configuration:**
- Two-layer config: user config for tools/providers/models, project config for workflow and sandbox
- `.local.yml` overrides for both layers (deep merge, gitignored)
- Step resolution via fallback chain: step-specific > pipeline.defaults > user.default
- Model tier resolution (`muster-fast` → `haiku`, `muster-deep` → `opus`)

**Commands:**
- `muster code --yolo`: launch sandboxed interactive session
- `muster down [slug]`: tear down containers (with `--all`, `--orphans`, `--project` flags)
- `muster status`: show container state per item

**Network isolation:**
- Squid proxy with merged domain allowlist
- Base allowlist includes provider domains
- User/project allowlists merged from config
- `NO_PROXY` includes localhost and `host.docker.internal`

**Testing:**
- Unit tests for compose generation (YAML structure, no Docker required)
- Table-driven tests for auth detection (mock filesystem/env)
- Integration tests for container lifecycle (guarded by `testing.Short()`)

### Out of Scope for Phase 2

**Deferred to future phases:**
- Auth hot reload (`muster reload-auth`) — requires `muster down` and restart for now
- Image management (pre-flight checks, programmatic pull) — rely on Docker's default pull behavior
- Config environment variable expansion (`${VAR}`) — keep config literal
- Additional firewall layer (iptables/nftables) — proxy is the sole isolation mechanism
- Compose validation with compose-go library — use `docker compose config` instead
- Container naming override — use Compose's default naming

**Never planned:**
- Docker SDK for orchestration (start/stop/restart) — use Compose CLI
- compose-go library dependency — too heavy, use YAML marshaling
- text/template for compose generation — use YAML marshaling
- Custom error types across packages — use fmt.Errorf wrapping
- Sentinel errors — check error messages where needed

---

## Feature Prioritization

### Priority 1: Core Container Orchestration (Blocking)
- Compose file generation with auth/workspace/proxy overrides
- Container start/stop/exec via Compose CLI
- Label-based discovery via Docker SDK
- Must work before any AI invocation in containers

### Priority 2: Authentication System (Blocking)
- Per-provider auth detection and injection
- Validation at startup
- Clear error messages for missing credentials
- Must work before any provider can be used

### Priority 3: Configuration System (Blocking)
- User + project config parsing
- Step resolution (tool + provider + model)
- `.local.yml` override merging
- Must work before any step can determine what to run

### Priority 4: Container Lifecycle (High Value)
- Persistent containers across steps
- Cleanup commands (`muster down` variants)
- Orphan detection
- High user-facing value, impacts resource usage

### Priority 5: Network Isolation (High Value)
- Proxy allowlist merging
- Domain restriction enforcement
- Local provider URL rewriting
- Core security feature

### Priority 6: Developer Experience (Medium Value)
- `muster doctor --config` resolution tracing
- Local provider reachability checks
- Compose validation before startup
- Helps diagnose issues, not blocking

---

## Success Metrics

**Functional correctness:**
- All unit tests pass on Linux, macOS, Windows
- Integration tests pass with Docker installed
- `muster code --yolo` successfully launches container with skills
- `muster down` cleanly tears down containers
- Auth injection works for all provider types (Bedrock, Max, Anthropic, OpenRouter, local)

**User experience:**
- Container startup completes in <10 seconds (after image pull)
- Auth validation errors provide actionable fix instructions
- `muster status` shows accurate container state
- Orphan cleanup finds and removes stale containers

**Security:**
- Containers cannot reach arbitrary internet domains
- Credential files mounted read-only
- Network allowlist enforced via proxy
- Local provider access restricted to host gateway

---

## Dependencies

**External tools (hard requirements):**
- Docker Compose CLI (v2+) — all container operations
- Docker daemon running — container execution
- Git — worktree detection for volume mounts

**External tools (soft requirements):**
- Claude Code or OpenCode — at least one tool must be installed for AI invocation
- AWS CLI — only for Bedrock provider credential pre-resolution
- GitHub CLI / GitLab CLI — only for PR/MR creation in finish step

**Go libraries:**
- `github.com/docker/docker/client` — Docker SDK for queries
- `gopkg.in/yaml.v3` — YAML marshaling
- `github.com/spf13/cobra` — CLI framework
- `github.com/stretchr/testify` — test assertions
- Standard library: `os/exec`, `text/template`, `filepath`

---

## Open Questions (to be resolved during implementation)

1. **Image build vs pre-built**: Should muster build the container image at first use, or should images be pre-built and pulled from a registry? Embedding a Dockerfile suggests build-at-use, but pre-built images would be faster. **Decision needed in architecture phase.**

2. **Parallel execution locks**: How should `muster in --all` claim work items to enable parallel execution? Filesystem locks with stale PID detection are specified, but cross-platform PID checking needs careful implementation. **Decision needed in Phase 6.**

3. **Windows Docker Desktop compatibility**: Does the compose generation work correctly on Windows with Docker Desktop's VM layer? File path translation and volume mount syntax may differ. **Needs verification testing in Phase 2.**

4. **Compose validation performance**: Running `docker compose config` adds ~1s latency. Is this acceptable for all invocations, or should it be behind a flag? **Decision needed during implementation.**

---

## Relationship to Overall Roadmap

This feature is **Phase 2** of the muster CLI roadmap:
- **Phase 0** (completed): Project bootstrap, version command, CI
- **Phase 1** (completed): `muster code` with prompt staging (local only)
- **Phase 2** (this feature): Docker orchestration, `muster code --yolo`, `muster down`
- **Phase 3** (next): Roadmap management, status command
- **Phase 4+**: Pipeline implementation, git operations, full workflow

Phase 2 deliverables enable:
- Sandboxed interactive sessions (`muster code --yolo`)
- Container lifecycle management (`muster down`)
- Foundation for pipeline execution in containers (Phase 6)

All container infrastructure built in Phase 2 will be reused by the pipeline in Phase 6.
