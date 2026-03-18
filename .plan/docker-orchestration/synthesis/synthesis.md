# Docker Container Orchestration: Requirements Synthesis

*Synthesized: 2026-03-18*
*Sources: config-system.md, project-structure.md, docker-sdk.md, compose-generation.md, devcontainer-patterns.md, design.md*

---

## Executive Summary

The research confirms a clear architectural pattern: use `docker compose` CLI for orchestration while leveraging the Docker Go SDK exclusively for container queries and inspection. The five research outputs converge on key decisions: YAML marshaling with `gopkg.in/yaml.v3` for compose generation (avoiding heavy dependencies), file-based credential mounting over environment variables (industry standard for security), and Docker labels as the universal discovery mechanism.

The config system must scan all pipeline steps upfront to collect auth requirements for both tools (Claude and OpenCode) since the container hosts both and stays running across steps. Provider auth detection follows consistent patterns from the abenz-workflow reference implementation: Bedrock via AWS config inspection, Max via credential file mounting, API keys via environment pass-through, and local providers via URL rewriting to `host.docker.internal`.

Two critical implementation decisions emerge: (1) generate a single merged compose file programmatically rather than relying on compose's override semantics, and (2) implement auth validation at startup (fail fast) rather than mid-execution. The most important open question is handling localhost URL rewriting for local inference servers — the proxy allowlist and firewall rules must be updated to permit host gateway access.

## Requirements

### MUST Have

**Container Orchestration (docker-sdk.md, devcontainer-patterns.md)**
- Use `docker compose` CLI for all lifecycle operations (up, down, start, stop, exec) via `os/exec.Command` — do not reimplement orchestration logic with Docker SDK
- Use Docker Go SDK (`github.com/docker/docker/client`) exclusively for container queries: `ContainerList()` with label filters, `ContainerInspect()` for state inspection
- Always initialize SDK client with `client.WithAPIVersionNegotiation()` to avoid daemon version mismatches
- Support container discovery via Docker labels: `muster.managed=true`, `muster.project={name}`, `muster.slug={slug}` (devcontainer-patterns.md §Container Labeling and Discovery)

**Compose File Generation (compose-generation.md, config-system.md)**
- Use YAML marshaling with `gopkg.in/yaml.v3` (already in dependencies) for programmatic compose generation — define minimal Go structs covering only fields actually used
- Load embedded base `docker-compose.yml` via `go:embed`, apply overrides programmatically, write single merged file to `~/.cache/muster/{project}/docker-compose.yml`
- Implement Docker Compose merge semantics manually: single-value fields replace, map fields merge by key, list fields append with deduplication (compose-generation.md §Override Merging Strategy)
- Label all containers at service level (not under `deploy:`) with project/slug metadata for query-based discovery

**Provider Authentication (config-system.md §Provider Auth Detection, devcontainer-patterns.md §Authentication Injection)**
- Scan all pipeline steps before container startup to collect required auth for both Claude and OpenCode (multi-tool container needs all auth injected upfront)
- Implement per-provider detection functions returning structured config or error: `DetectBedrockAuth()`, `DetectMaxAuth()`, `DetectAPIKeyAuth()`, `DetectLocalProvider()`
- Follow security best practice: mount credentials as read-only files over environment variables where possible (AWS `~/.aws`, Max `~/.claude/.credentials.json`)
- Validate all required provider auth exists at startup — fail early with actionable error messages before invoking any step

**Provider-Specific Auth Patterns (config-system.md §Provider Auth Detection Patterns)**
- **Bedrock**: Detect `CLAUDE_CODE_USE_BEDROCK` in `~/.claude/settings.json`, mount `~/.aws` read-only, export `AWS_PROFILE`/`AWS_REGION`/`CLAUDE_CODE_USE_BEDROCK=1`, pre-resolve credentials via `aws configure export-credentials`
- **Max**: Mount `~/.claude/.credentials.json` read-only, use static settings template at `dot-claude/settings.max.json`
- **Anthropic/OpenRouter**: Pass through API key via environment variable named in `api_key_env` config field
- **Local providers**: Rewrite `localhost` URLs to `host.docker.internal` for container context, update proxy allowlist and firewall rules to permit host gateway access

**Config Resolution (config-system.md §Config Resolution Flow)**
- Implement two-layer config architecture: user config (`~/.config/muster/config.yml`) for tools/providers/models, project config (`.muster/config.yml` + `.muster/dev-agent/config.yml`) for workflow and sandbox
- Support `.local.yml` overrides (gitignored) with deep merge: local values override at field level, lists are replaced not appended
- Resolve step configuration via fallback chain: step-specific > `pipeline.defaults` > `user.default`
- Support model tier resolution: `muster-{tier}` prefix looks up tier in `user.tools[tool].models[tier]`, otherwise use literal model name

**Volume Mounting (devcontainer-patterns.md §Volume Mount Strategies)**
- Mount git worktrees as bind mounts: shared `.git` directory read-only from main repo, worktree directory read-write
- Support credential mounts: `~/.aws`, `~/.ssh`, `~/.gitconfig` all read-only
- Allow user-defined volumes from `.muster/dev-agent/config.yml` with relative path resolution from workspace root
- Use `os.MkdirAll()` for directory creation and `filepath.Join()` for all path operations (cross-platform)

**Container Lifecycle (devcontainer-patterns.md §Container Lifecycle Management, docker-sdk.md §When to Use Docker Compose CLI)**
- Generate all compose overrides (auth, workspace, proxy domains, labels) before invoking `docker compose`
- Start services: `docker compose -f {composed.yml} up -d`
- Execute commands: `docker compose exec {service} {tool} {args}` — switch tools via different exec commands, not container restarts
- Query containers: SDK `ContainerList()` with label filters for `muster down`, `muster status`
- Teardown: `docker compose -p {project} down` — leave containers running on failure for debugging

**Network Isolation (devcontainer-patterns.md §Network Isolation)**
- Implement proxy-based allowlisting: Squid proxy container with merged domain list from base + project config + provider-specific domains
- Default proxy allowlist includes provider domains (anthropic.com, aws.amazon.com, etc.) merged with user's `.muster/dev-agent/config.yml` `allowed_domains`
- Set `HTTP_PROXY`/`HTTPS_PROXY` environment variables in agent container, `NO_PROXY` includes localhost and `host.docker.internal`
- Use Docker Compose internal networks to isolate agent from direct internet access

**Error Handling (project-structure.md §Error Handling, design.md §Error Handling & Logging)**
- Use `fmt.Errorf("context: %w", err)` for all error wrapping — no custom error types unless needed across packages
- Validate prerequisites (auth, config, tools) before starting work — fail early with clear messages
- Write structured logs to `.muster/work/{slug}/pipeline.log` with timestamps, step names, durations, exit codes, stderr
- Leave containers running on failure for debugging — explicit `muster down` is cleanup command

### SHOULD Have

**Testing Coverage (project-structure.md §Testing Approach)**
- Unit tests for compose generation: assert YAML structure without requiring Docker daemon
- Table-driven tests for auth detection: mock filesystem and environment variables
- Integration tests for container lifecycle guarded by `if testing.Short() { t.Skip() }`
- Golden file tests for any generated YAML output with `-update` flag to regenerate

**Config Package Organization (config-system.md §For Config Package Implementation)**
- Split into focused files: `user.go`, `project.go`, `devagent.go`, `resolve.go`, `auth.go`
- Provide resolution tracing for `muster doctor --config`: track which config layer provided each value
- Return all validation errors at once (collect them), don't fail on first error
- Format traced resolution as table: step name | tool | provider | model | source layer

**Compose Package Structure (compose-generation.md §Implementation Pattern)**
- Define minimal structs covering only used fields: `ComposeFile`, `Service`, `Volume`, `Network`
- Implement helper functions: `mergeComposeFiles()`, `mergeService()`, `appendUnique()` for list deduplication
- Write temp files to `~/.cache/muster/{project}/` (must be under `$HOME` for Docker Desktop VM compatibility on macOS)
- Expose compose generation as `GenerateComposeFile(config Config) ([]byte, error)` with structured input

**Docker Client Wrapper (docker-sdk.md §Recommendations)**
- Create `internal/docker/client.go` with `NewClient()` initializing SDK with version negotiation
- Implement helper methods: `ListMusterContainers(project, slug)`, `IsRunning(containerID)`
- Wrap compose CLI operations: `ComposeUp()`, `ComposeDown()`, `ComposeExec()` using `exec.CommandContext()`
- Always set `cmd.Stdout = os.Stdout` and `cmd.Stderr = os.Stderr` for compose commands to stream output

**Multi-File Layering (devcontainer-patterns.md §Multiple Compose Files)**
- Support optional user override: `.muster/dev-agent/docker-compose.override.yml` (gitignored)
- Layer compose files in order: base (embedded) → auth overrides (generated) → workspace config (generated) → user override (if exists)
- Write each layer to temp directory: `auth-override.yml`, `workspace-override.yml`, merge into single composed file

**Parallel Execution Support (design.md §`muster in --all`)**
- Implement filesystem locks with stale PID detection: `os.OpenFile` with `os.O_CREATE|os.O_EXCL`
- Cross-platform PID checking: `os.FindProcess` + signal 0 on Unix, `syscall.OpenProcess` on Windows behind build tags
- Claim locks per slug to enable parallel `--all` execution without conflicts

### NICE TO HAVE

**Enhanced Error Diagnostics (config-system.md §Open Questions)**
- Track YAML line numbers using `yaml.v3` decoder for validation error messages showing file path + line number
- Implement `muster doctor --models` to validate provider/model combinations (warn about mismatches like local provider with cloud-only model)
- Provide credential reachability checks: for local providers, HTTP GET with 2-second timeout to validate inference server is running

**Compose Validation (compose-generation.md §Open Questions)**
- Run `docker compose config` on generated compose file in tests to validate syntax
- Use compose-go's validator in test mode only (don't add as runtime dependency)
- Compare generated output against golden files for regression detection

**Image Management (docker-sdk.md §Image Operations)**
- Pre-flight image existence check via SDK `ImageInspect()` before starting container
- Optional programmatic image pull with progress reporting via `ImagePull()` reader
- Cache validation checks to avoid repeated network requests per session

**Config Hot Reload (config-system.md §Open Questions §API key rotation)**
- Future: `muster reload-auth` command to regenerate overrides and recreate container without full teardown
- Document as known limitation for Phase 2: auth changes require `muster down` and restart

**Network Firewall Layer (devcontainer-patterns.md §Network Isolation, devcontainer-patterns.md §Open Questions)**
- Additional egress filtering via iptables/nftables in container entrypoint (blocks accidental allowlist bypass)
- Platform detection: Linux prefers nftables with iptables fallback; Mac/Windows Docker Desktop relies solely on proxy (no firewall available)
- Whitelist only proxy + DNS in firewall rules

### SHOULD NOT Include

**Docker SDK for Orchestration (docker-sdk.md §Docker Compose CLI vs SDK)**
- Do NOT reimplement container lifecycle (start/stop/restart) with Docker SDK — compose handles service dependencies, health checks, and ordering that we shouldn't duplicate
- Do NOT use SDK for exec operations — `docker compose exec` is simpler and handles TTY/stdin attachment properly

**compose-go Library (compose-generation.md §Approach 1 Verdict)**
- Do NOT add `github.com/compose-spec/compose-go/v2` as dependency — it's heavy (extensive dependency tree), optimized for parsing not generation, and overkill for this use case
- Do NOT rely on compose-go's merge semantics — implement simple merge logic with YAML marshaling for full control

**text/template for Compose (compose-generation.md §Approach 3 Verdict)**
- Do NOT use Go templates for YAML generation — whitespace sensitivity combined with template syntax creates maintenance burden and obscures structure
- Reserve `text/template` for prompt staging where it excels — structured data like compose files should use YAML marshaling

**Custom Error Types (project-structure.md §Error Handling Patterns)**
- Do NOT create custom error types unless multiple packages need to handle specific error cases
- Do NOT implement sentinel errors — wrap with `fmt.Errorf` and check error messages where needed

**Environment Variable Expansion (config-system.md §Open Questions §Config env var expansion)**
- Do NOT support `${VAR}` expansion in config values — keep config literal for Phase 2
- Users can use `.local.yml` overrides or shell scripts to generate dynamic config if needed
- Can add in future phase if demand exists

**Container Naming Override (devcontainer-patterns.md §Open Questions)**
- Do NOT override Compose's default container naming (`{project}_{service}_{replica}`)
- Label-based discovery works regardless of naming scheme — custom names create edge cases and lose replica support

## Key Decisions

### Decision: Hybrid Docker SDK + Compose CLI Architecture
**Rationale**: Docker Compose CLI excels at multi-container orchestration with declarative YAML (service dependencies, health checks, network creation), while the Docker SDK provides type-safe programmatic querying and label filtering. Using both leverages their strengths: compose for orchestration, SDK for queries. (docker-sdk.md §Key Findings, devcontainer-patterns.md §Container Lifecycle Management)

**Alternatives considered**: SDK-only (would require reimplementing orchestration logic), compose CLI-only (would require parsing `docker ps` output as strings)

### Decision: YAML Marshaling Over compose-go or Templates
**Rationale**: `gopkg.in/yaml.v3` is already in dependencies, provides full control over merge logic, and is lightweight with no external dependencies. compose-go adds significant complexity (learning curve, breaking changes between versions) for features we don't need. Templates obscure structure and are fragile with YAML whitespace. (compose-generation.md §Chosen Approach, config-system.md §Recommendations)

**Alternatives considered**: compose-go library (too heavy, load-focused not generation-focused), text/template (whitespace-fragile, harder to maintain)

### Decision: Single Merged Compose File
**Rationale**: Generate one composed YAML and write to `~/.cache/muster/{project}/docker-compose.yml` rather than passing multiple `-f` flags to compose. Simpler to debug, explicit merge semantics we control, single source of truth for what's running. (compose-generation.md §Implementation Pattern, devcontainer-patterns.md §Layered Composition)

**Alternatives considered**: Multiple compose files with `-f` flags (relies on compose's merge semantics which can be surprising), dynamic override generation per-command (more complex state management)

### Decision: File-Based Credential Mounting Over Environment Variables
**Rationale**: Industry standard across all production dev container tools (GitHub Codespaces, Coder, VSCode). Environment variables leak into logs, process listings, and child processes. Files provide proper permission boundaries via filesystem ACLs and read-only mounts. (devcontainer-patterns.md §Authentication Injection, config-system.md §Provider Auth Detection)

**Alternatives considered**: Environment variables (easier to implement but less secure), Docker Secrets (more secure but adds complexity and not supported in Docker Compose v2 without Swarm mode)

### Decision: Scan All Steps for Auth Requirements Upfront
**Rationale**: The multi-tool container (both Claude and OpenCode installed) stays running across pipeline steps. Different steps may use different tool+provider combinations. All required auth must be injected at container startup since we can't dynamically mount credentials mid-execution. (config-system.md §Multi-Tool Container Design, devcontainer-patterns.md §Container Restart vs Exec)

**Alternatives considered**: Per-step auth injection (would require restarting container between steps, slower and loses state), assume single tool per pipeline (limits flexibility)

### Decision: Validate Auth at Startup, Fail Fast
**Rationale**: Better UX to fail before starting 5-minute execution than mid-execution. Validation collects all missing auth requirements and presents them together with actionable fix instructions. Also powers `muster doctor` health checks. (config-system.md §For Docker Auth Detection, design.md §Error Handling)

**Alternatives considered**: Validate per-step (wastes time if later step fails auth), silently skip steps with missing auth (confusing behavior)

### Decision: Container-Level Labels for Discovery
**Rationale**: Universal pattern across all dev container platforms. Docker native filtering via `docker ps --filter label=key=value` is fast and type-safe. Labels persist with containers across restarts. Enables powerful queries: all managed containers, containers by project, containers by slug, orphaned containers. (devcontainer-patterns.md §Container Labeling and Discovery, docker-sdk.md §Container Labels in Compose Files)

**Alternatives considered**: Container name patterns (fragile, no semantic meaning), external state file (requires sync with Docker state)

### Decision: Localhost URL Rewriting for Local Providers
**Rationale**: Containers can't reach `localhost` on the host. Docker provides `host.docker.internal` hostname that resolves to host gateway IP. Rewriting `base_url: http://localhost:1234` to `http://host.docker.internal:1234` enables containers to reach LM Studio/Ollama/vLLM running on host. Requires updating proxy allowlist and firewall rules to permit gateway. (config-system.md §Local Provider Container Networking, devcontainer-patterns.md §Local Inference Server Patterns)

**Alternatives considered**: Host networking mode (breaks isolation), manual user config (poor UX), bridge network with extra_hosts (same end result, more verbose)

## Resolved Questions

### Resolved: Missing user config → Require config, error with message
Require `~/.config/muster/config.yml` to exist. Error with a helpful message pointing to `muster init` or manual config creation. Don't assume defaults since we can't know what providers the user has access to.

### Resolved: Local provider reachability → Validate at startup with 2s timeout
HTTP GET with 2-second timeout at container startup to validate local inference servers are running. Also include in `muster doctor` checks. Catches the common mistake of forgetting to start LM Studio/Ollama.

### Resolved: Environment variable expansion → Not in Phase 2
Keep config values literal. No `${VAR}` expansion. Avoids complexity and injection risks. Users use `.local.yml` overrides for per-machine values.

### Resolved: Undefined provider handling → Collect all errors at load time
Validate during config resolution before starting container. Collect all errors (undefined providers, missing auth, invalid tiers) and present together with file path + line number. Better UX than failing on first error.

### Resolved: Container persistence → Keep running across steps
Persistent containers across all pipeline steps. Faster execution, preserves installed packages and state. Explicit `muster down` for cleanup. Matches design doc's exec-based step invocation.

### Resolved: Firewall implementation → Proxy only, no firewall layer
Use Squid proxy as the sole network isolation mechanism across all platforms. No iptables/nftables layer. Simpler implementation with one consistent security model. Proxy is the primary isolation mechanism.

### Resolved: Compose validation → Validate at runtime before starting
Run `docker compose config` on the generated compose file before starting containers. Small latency cost (~1s) but catches generation bugs before they cause cryptic failures.

### Resolved: Auth priority → Environment variable wins
When both env var and credential file exist, environment variable takes precedence (explicit user intent). Matches AWS CLI and most tool conventions. Document clearly.

## References

### Research Files
- `config-system.md` — Config architecture, auth detection patterns, resolution algorithm, two-layer design
- `project-structure.md` — Go patterns, package organization, testing approach, error handling conventions
- `docker-sdk.md` — SDK vs CLI decision, container lifecycle APIs, label filtering, auth handling
- `compose-generation.md` — YAML marshaling approach, merge semantics, struct definitions
- `devcontainer-patterns.md` — Industry patterns, volume mounts, network isolation, lifecycle hooks

### Design Document
- `docs/design.md` (lines 84-192) — Configuration section
- `docs/design.md` (lines 194-237) — Architecture overview
- `docs/design.md` (lines 339-359) — Docker integration and multi-tool container design

### Reference Implementation
- `dev-agent/run.sh` (lines 132-229) — Auth detection (Bedrock vs Max)
- `dev-agent/run.sh` (lines 235-367) — Compose override generation
- `dev-agent/docker-compose.yml` — Base compose structure
- `dev-agent/dot-claude/settings.json` — Bedrock container settings template
- `dev-agent/dot-claude/settings.max.json` — Max container settings template

### External Resources
- Docker Compose Specification: https://github.com/compose-spec/compose-spec
- Docker SDK for Go: https://pkg.go.dev/github.com/docker/docker/client
- gopkg.in/yaml.v3: https://pkg.go.dev/gopkg.in/yaml.v3
- Devcontainer Specification: https://containers.dev/implementors/spec/
