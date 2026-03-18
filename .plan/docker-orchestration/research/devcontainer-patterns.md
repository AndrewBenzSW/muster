# Dev Container Orchestration Patterns

*Researched: 2026-03-18*
*Scope: Survey of devcontainers, Coder, Gitpod, GitHub Codespaces, Devbox, and Docker Compose patterns for container orchestration, labeling, volume mounts, authentication injection, and network isolation.*

---

## Key Findings

### Industry Standard: Docker Labels for Container Discovery

All major dev container platforms use Docker labels as the primary mechanism for container discovery and lifecycle management:

- **Docker Compose** natively supports labels via the `labels:` service attribute
- **GitHub Codespaces** generates unique domain identifiers for session-level tracking
- **Coder** uses Terraform-based templates that provision labeled resources
- Labels enable filtering via `docker ps --filter label=key=value`
- Label matching supports both presence-only (`label=key`) and key-value matching (`label=key=value`)

### Composition Over Configuration

Modern dev tools favor **layered composition** of Docker Compose files:

- Base configuration with common settings
- Environment-specific overrides (dev/staging/prod)
- User-specific local overrides (gitignored)
- The `--file` flag chains multiple compose files with merge semantics

### Volume Mount Patterns

Three distinct patterns emerge across tools:

1. **Bind mounts for code**: Direct workspace folder mounting (read-write)
2. **Named volumes for persistence**: Database data, caches, build artifacts
3. **Read-only credential mounts**: SSH keys, cloud credentials, API tokens

### Authentication Injection: Files Over Environment

Despite environment variables being easier, **all production tools prefer file-based credential mounting**:

- Docker Secrets mount to `/run/secrets/<secret_name>` (filesystem permissions)
- AWS credentials via read-only `~/.aws` mount
- SSH keys via read-only `~/.ssh` mount
- API tokens via Docker Secrets or mounted credential files

**Rationale**: Environment variables leak into logs, process listings, and child processes. Files provide proper permission boundaries.

### Network Isolation: Default + Custom Networks

Docker Compose's default networking model is widely adopted:

- Each project gets an automatic `{project}_default` network
- Services discover each other via service names (DNS)
- Custom networks enable isolation between service groups
- External networks connect to manually-created bridges
- Internal services never expose host ports (container ports only)

### Lifecycle Hooks: Three Execution Points

The devcontainer spec defines lifecycle hooks that all major tools implement in some form:

- **onCreate**: Runs once during container creation (install dependencies)
- **onStart**: Runs every time container starts (start background services)
- **onAttach**: Runs when tool connects to container (setup environment)

---

## Detailed Analysis

### Container Labeling and Discovery

#### Docker Compose Label Patterns

Docker Compose applies labels at the service level:

```yaml
services:
  dev-agent:
    image: muster-dev-agent
    labels:
      com.example.managed: "true"
      com.example.project: "my-api"
      com.example.slug: "add-retry-logic"
```

Compose automatically adds system labels like `com.docker.compose.project`, `com.docker.compose.service`, and `com.docker.compose.version` for its own tracking.

**Discovery pattern:**
```bash
# Find all managed containers
docker ps --filter label=com.example.managed=true

# Find containers for a specific project
docker ps --filter label=com.example.project=my-api

# Find a specific workspace container
docker ps --filter label=com.example.slug=add-retry-logic
```

Multiple filters create an AND relationship — all filters must match.

#### Devcontainers Specification

The devcontainer spec stores metadata in **image labels** via `devcontainer.metadata` containing a JSON array. This enables:

- Storing configuration directly in the image
- Merging image metadata with `devcontainer.json` at runtime
- Caching configuration across rebuilds

Size constraints: Dockerfile labels have ~1.3MB total limits with 65k character line lengths.

#### Coder Workspace Management

Coder uses **Terraform-based templates** to provision infrastructure. Each workspace is tagged via cloud provider tags (AWS tags, Kubernetes labels) and tracked via a workspace ID. The `coder` CLI and web UI query these tags to display workspace status and enable lifecycle operations.

#### GitHub Codespaces Session Tracking

Codespaces generates unique domain identifiers like `mono-github-g95jq2w5wf7.github.dev` for each session. These serve as:

- Stable DNS endpoints for port forwarding
- Session identifiers for resource tracking
- Billing and lifecycle management hooks

### Volume Mount Strategies

#### Devcontainers: Mounts Property

The `mounts` property in `devcontainer.json` accepts Docker CLI `--mount` syntax:

```json
{
  "mounts": [
    "source=${localEnv:HOME}/.ssh,target=/home/node/.ssh,type=bind,readonly",
    "source=${localWorkspaceFolder},target=/workspace,type=bind"
  ]
}
```

Key patterns:

- **Environment variable substitution**: `${localEnv:VAR}` references host environment
- **Workspace-relative paths**: `${localWorkspaceFolder}` resolves to project root
- **Read-only flags**: Credentials are mounted readonly to prevent accidental modification
- **Type specification**: `type=bind` for host paths, `type=volume` for named volumes

#### VSCode Dev Containers

VSCode uses **workspace-relative bind mounts** by default:

- Primary workspace: `./:/workspace` (project root)
- Git repository access: `.git` always accessible for version control
- Monorepo support: Can mount subfolder as workspace while keeping `.git` accessible

Additional mounts via `devcontainer.json`:
```json
{
  "mounts": [
    "source=/local/data,target=/data,type=bind",
    "source=project-cache,target=/cache,type=volume"
  ]
}
```

**Platform limitation**: Mounting local filesystem not supported in GitHub Codespaces (remote Docker hosts can't access local files).

#### Docker Compose Volume Types

Three mount types, each with specific use cases:

**1. Bind mounts** — Direct host path mapping:
```yaml
services:
  dev:
    volumes:
      - /host/path:/container/path:ro
      - ./relative/path:/app
```

**2. Named volumes** — Docker-managed storage:
```yaml
services:
  dev:
    volumes:
      - postgres-data:/var/lib/postgresql/data

volumes:
  postgres-data:
```

**3. tmpfs mounts** — In-memory filesystem:
```yaml
services:
  dev:
    tmpfs:
      - /tmp
```

#### Git Worktree Mounting

For multi-worktree setups (like muster's use case), the pattern is:

1. **Mount the entire `.git` directory** from main repo (shared across worktrees)
2. **Mount the worktree directory** itself
3. Ensure both mounts use the same container user to avoid permission issues

Example for muster:
```yaml
services:
  dev-agent:
    volumes:
      - /Users/andrew.benz/work/muster/muster-main/.git:/workspace/.git:ro
      - /Users/andrew.benz/work/muster/muster-main/.worktrees/add-retry-logic:/workspace:rw
```

This allows git operations within the container while isolating code changes to the worktree.

### Authentication and Credential Injection

#### Docker Secrets (Compose Specification)

Docker's **preferred pattern** for sensitive data:

```yaml
services:
  dev:
    secrets:
      - aws_credentials
      - api_token

secrets:
  aws_credentials:
    file: ~/.aws/credentials
  api_token:
    environment: API_TOKEN
```

Secrets are mounted at `/run/secrets/<secret_name>` with:
- Filesystem-level permissions
- No exposure in `docker inspect` output
- No leakage to logs or process listings

**Development vs. Production**:
- Dev: Use `file:` to reference local credential files
- Production: Use `external: true` to reference orchestrator-managed secrets (Docker Swarm, Kubernetes)

#### Environment Variable Patterns

Despite being less secure, environment variables remain widely used for development:

**Direct assignment:**
```yaml
services:
  dev:
    environment:
      DEBUG: "true"
      DATABASE_URL: "postgres://localhost/dev"
```

**File-based injection:**
```yaml
services:
  dev:
    env_file:
      - .env
      - .env.local
```

**Shell pass-through:**
```yaml
services:
  dev:
    environment:
      - AWS_ACCESS_KEY_ID  # Value from host environment
```

**Docker documentation warning**: "Don't use environment variables to pass sensitive information, such as passwords, in to your containers." Use secrets instead.

#### Provider-Specific Patterns

**AWS Credentials**:
```yaml
services:
  dev:
    volumes:
      - ~/.aws:/root/.aws:ro
    environment:
      - AWS_PROFILE
      - AWS_REGION
      - AWS_SDK_LOAD_CONFIG=1
```

**SSH Keys**:
```yaml
services:
  dev:
    volumes:
      - ~/.ssh:/root/.ssh:ro
```

**Git Credentials**:
```yaml
services:
  dev:
    volumes:
      - ~/.gitconfig:/root/.gitconfig:ro
      - ~/.git-credentials:/root/.git-credentials:ro
```

**API Keys** (multiple patterns observed):

1. **Via Docker Secrets** (most secure):
```yaml
services:
  dev:
    secrets:
      - anthropic_api_key

secrets:
  anthropic_api_key:
    file: ~/.anthropic_api_key
```

2. **Via environment file** (common in dev):
```yaml
services:
  dev:
    env_file:
      - .env.local  # Contains ANTHROPIC_API_KEY=sk-...
```

3. **Via direct environment pass-through**:
```yaml
services:
  dev:
    environment:
      - ANTHROPIC_API_KEY  # Inherits from host shell
```

#### Devcontainers: remoteEnv Property

Devcontainer spec uses `remoteEnv` for runtime environment configuration:

```json
{
  "remoteEnv": {
    "PATH": "${containerEnv:PATH}:/custom/bin",
    "API_KEY": "${localEnv:MY_API_KEY}"
  }
}
```

- `${containerEnv:VAR}` references existing container variables
- `${localEnv:VAR}` pulls from host environment
- Applied at container start, not build time
- Enables credential injection without rebuilding

#### Local Inference Server Patterns

When containers need to access host-local services (LM Studio, Ollama, vLLM):

**Docker Desktop (Mac/Windows)**:
```yaml
services:
  dev:
    environment:
      - OLLAMA_BASE_URL=http://host.docker.internal:11434
```

**Linux**:
```yaml
services:
  dev:
    extra_hosts:
      - "host.docker.internal:host-gateway"
    environment:
      - OLLAMA_BASE_URL=http://host.docker.internal:11434
```

Docker's `host.docker.internal` hostname resolves to host gateway IP, enabling containers to reach services on the host.

### Network Configuration and Isolation

#### Docker Compose Default Networking

Every Compose project gets an automatic network:

```yaml
# Implicit — no configuration needed
services:
  web:
    # Automatically joins {project}_default network
  db:
    # Same network — can reach web via DNS name "web"
```

**Key behaviors**:
- Network named `{project}_default`
- All services join by default
- Service discovery via service names
- Isolated from other Compose projects
- Bridge driver for external connectivity

#### Custom Networks for Isolation

Create multiple networks to segment services:

```yaml
services:
  proxy:
    networks:
      - frontend

  app:
    networks:
      - frontend
      - backend

  db:
    networks:
      - backend

networks:
  frontend:
  backend:
```

**Result**: `proxy` can't reach `db` directly (no shared network). Only `app` bridges both networks.

#### External Networks

Connect to pre-existing networks:

```yaml
services:
  dev:
    networks:
      - shared-network

networks:
  shared-network:
    external: true
```

Useful for:
- Connecting to manually-created Docker networks
- Sharing networks between multiple Compose projects
- Integrating with existing container infrastructure

#### Port Exposure: Internal vs External

Devcontainer spec distinguishes between:

**Internal communication** (container-to-container):
```yaml
services:
  db:
    # No ports published — only accessible within Docker network
```

**External access** (host-to-container):
```yaml
services:
  web:
    ports:
      - "3000:3000"  # HOST_PORT:CONTAINER_PORT
```

The `forwardPorts` property in `devcontainer.json` tells IDEs which ports to proxy, separate from Docker's port publishing.

#### Network Proxy for Domain Allowlisting

No standard pattern exists across tools. Custom implementations typically use:

- **Squid proxy** with whitelist ACLs
- **Firewall rules** (iptables, nftables) blocking direct egress
- **Explicit HTTP_PROXY/HTTPS_PROXY** environment variables pointing to proxy
- **DNS filtering** (not recommended — easily bypassed)

Example pattern (observed in custom setups):
```yaml
services:
  proxy:
    image: squid-allowlist
    volumes:
      - ./allowed-domains.txt:/etc/squid/allowed-domains.txt:ro

  dev-agent:
    environment:
      - HTTP_PROXY=http://proxy:3128
      - HTTPS_PROXY=http://proxy:3128
    depends_on:
      - proxy
```

### Container Lifecycle Management

#### Docker Compose CLI Patterns

Core lifecycle commands:

```bash
# Start services (create if needed)
docker compose up -d

# Stop services (containers remain)
docker compose stop

# Remove stopped containers
docker compose down

# Execute in running container
docker compose exec service-name command

# View running services
docker compose ps

# View logs
docker compose logs -f service-name
```

**Label-based discovery** for multi-project management:

```bash
# List all containers managed by tool
docker ps --filter label=muster.managed=true

# Stop specific project
docker compose -f ~/.cache/muster/project-name/docker-compose.yml down

# Find orphaned containers (slug no longer in roadmap)
docker ps --filter label=muster.managed=true --format '{{.Label "muster.slug"}}'
```

#### Devcontainer CLI Lifecycle

Reference implementation provides:

```bash
# Create and start container
devcontainer up

# Execute command in container
devcontainer exec --workspace-folder /workspace -- command

# Run lifecycle hooks
devcontainer run-user-commands --workspace-folder /workspace

# Build image
devcontainer build

# Stop container (incomplete in reference impl)
devcontainer stop
```

#### Coder Workspace Lifecycle

Terraform-based provisioning with workspace states:

- **Creating**: Infrastructure provisioning in progress
- **Running**: Workspace active and accessible
- **Stopped**: Resources exist but not running (saves cost)
- **Deleting**: Cleanup in progress

Automated lifecycle features:
- **Start/stop schedules**: Auto-stop after inactivity
- **Workspace sharing**: Temporary access grants (beta)
- **Template versioning**: Upgrade workspaces to new template versions

#### Lifecycle Hooks: Execution Order

Based on devcontainer spec, implemented across tools:

**Container creation** (runs once):
1. `initializeCommand` — validates orchestrator access
2. Container created with mounts/env/user config applied
3. `onCreateCommand` — install dependencies, setup environment
4. `updateContentCommand` — pull latest code, update packages
5. `postCreateCommand` — final setup steps

**Container start** (every restart):
1. Container started
2. `postStartCommand` — start background services

**Tool attach** (when IDE/CLI connects):
1. `postAttachCommand` — setup shell environment, print welcome

**waitFor property**: Blocks execution until specified command completes (default: `updateContentCommand`).

#### Container Persistence Patterns

Three patterns observed:

**1. Ephemeral containers** (GitHub Codespaces default):
- Containers destroyed after inactivity timeout
- Named volumes persist data across recreations
- Fastest startup, lowest resource usage

**2. Persistent containers** (VSCode Dev Containers default):
- Containers stopped but not removed
- Faster restart (skip onCreate hooks)
- Preserves installed tools, cached data

**3. Hybrid** (Coder's approach):
- Infrastructure persists (VM/pod continues running)
- Container within infrastructure can be recreated
- Balances cost and startup speed

### Project-Based Organization

#### Docker Compose Project Naming

Compose uses project names for resource isolation:

```bash
# Set via flag (highest priority)
docker compose -p my-project up

# Set via environment variable
COMPOSE_PROJECT_NAME=my-project docker compose up

# Set in compose file
services:
  web:
    # ...

name: my-project  # Top-level attribute
```

**Naming constraints**: Must contain only lowercase letters, digits, dashes, underscores; must start with letter or digit.

**Resource naming**: All resources get prefixed: `{project}_{service}_{replica}` for containers, `{project}_{network}` for networks.

**Discovery**: Labels automatically added:
```yaml
labels:
  com.docker.compose.project: my-project
  com.docker.compose.service: web
```

#### Multiple Compose Files

Layered composition for environment management:

```bash
# Base + override
docker compose -f docker-compose.yml -f docker-compose.override.yml up

# Environment-specific
docker compose -f docker-compose.yml -f docker-compose.prod.yml up

# User-specific (gitignored)
docker compose -f docker-compose.yml -f docker-compose.local.yml up
```

**Merge semantics**:
- Later files override earlier files
- Arrays are replaced, not appended
- Objects are deep-merged
- List-based properties (like `ports`) use last-wins for conflicts

#### Compose Profiles

Selective service activation:

```yaml
services:
  core-api:
    # Always enabled (no profile)

  debug-tools:
    profiles:
      - debug

  local-db:
    profiles:
      - local

# Enable specific profiles
$ docker compose --profile debug --profile local up

# Enable all profiles
$ docker compose --profile "*" up
```

**Use cases**:
- Development vs production service sets
- Optional debugging tools
- Platform-specific services (local DB vs cloud DB)

### Devbox: Alternative Isolation Model

Devbox takes a **different approach** — isolation without containers:

**Package management** via Nix:
```json
{
  "packages": ["python@3.10", "nodejs@20", "postgresql@15"]
}
```

**Isolation mechanisms**:
- OS-level package isolation (not virtualization)
- Shell environment with specific package versions
- No Docker required for local development

**Container generation** when needed:
- Same `devbox.json` generates Dockerfile
- Same config works in devcontainer.json
- Enables local-first workflow with deploy-time containerization

**Credential handling**: Not covered in documentation — appears to rely on standard shell environment.

---

## Recommendations for Muster

### Container Labeling Scheme

Adopt the industry-standard pattern with muster-specific namespace:

```yaml
services:
  dev-agent:
    labels:
      muster.managed: "true"
      muster.project: "muster-main"  # From project directory name
      muster.slug: "docker-orchestration"  # From roadmap item
      muster.version: "0.1.0"  # CLI version for debugging
```

**Discovery commands**:
```bash
# All muster containers
docker ps --filter label=muster.managed=true

# Specific project
docker ps --filter label=muster.project=muster-main

# Specific workspace
docker ps --filter label=muster.slug=docker-orchestration

# Find orphaned containers
# (Cross-reference running slugs against .muster/roadmap.json)
docker ps --filter label=muster.managed=true --format '{{.Label "muster.slug"}}'
```

### Volume Mount Strategy

**For git worktrees**, mount both:

```yaml
services:
  dev-agent:
    volumes:
      # Shared .git directory (read-only)
      - ${MAIN_REPO}/.git:/workspace/.git:ro

      # Worktree directory (read-write)
      - ${WORKTREE_PATH}:/workspace:rw

      # Credential mounts (read-only)
      - ~/.aws:/home/node/.aws:ro
      - ~/.ssh:/home/node/.ssh:ro
      - ~/.gitconfig:/home/node/.gitconfig:ro
```

**Volume naming pattern**: Use project-scoped named volumes for persistence:
```yaml
volumes:
  muster-${PROJECT}-${SLUG}-cache:
    driver: local
```

### Authentication Injection

Implement **tiered auth detection**:

**Priority 1: Docker Secrets** (most secure, future-proof):
```yaml
secrets:
  anthropic_api_key:
    file: ~/.anthropic_api_key
  aws_credentials:
    file: ~/.aws/credentials
```

**Priority 2: File-based mounts** (current standard):
```yaml
volumes:
  - ~/.aws:/home/node/.aws:ro
  - ~/.config/muster/credentials.json:/home/node/.claude/.credentials.json:ro
```

**Priority 3: Environment pass-through** (least secure, backward compat):
```yaml
environment:
  - ANTHROPIC_API_KEY
  - AWS_PROFILE
  - AWS_REGION
```

**Detection logic** in `internal/docker/auth.go`:
1. Check for provider-specific credential files
2. Check for environment variables
3. Fail early with actionable error if neither exists

**Per-provider patterns** based on design doc:

- **Anthropic API**: Pass `ANTHROPIC_API_KEY` env var
- **AWS Bedrock**: Mount `~/.aws` + set `AWS_PROFILE`, `AWS_REGION`, `CLAUDE_CODE_USE_BEDROCK=1`
- **Max**: Mount `~/.claude/.credentials.json`
- **OpenRouter**: Pass `OPENROUTER_API_KEY` env var
- **Local** (LM Studio/Ollama): Rewrite `localhost` → `host.docker.internal` in base URL

### Network Isolation

Implement **proxy-based allowlisting**:

```yaml
services:
  proxy:
    image: muster-proxy
    build:
      context: .
      dockerfile: proxy.Dockerfile
    volumes:
      - ./allowed-domains.txt:/etc/squid/allowed-domains.txt:ro
    networks:
      - isolation

  dev-agent:
    depends_on:
      - proxy
    environment:
      - HTTP_PROXY=http://proxy:3128
      - HTTPS_PROXY=http://proxy:3128
      - NO_PROXY=localhost,127.0.0.1,host.docker.internal
    networks:
      - isolation

networks:
  isolation:
    internal: false  # Allow proxy to reach internet
```

**Dynamic allowlist generation**:
- Base allowlist from `.muster/dev-agent/config.yml`
- Add provider-specific domains (anthropic.com, aws.amazon.com)
- Add `host.docker.internal` for local inference servers
- Write merged list before starting containers

**Firewall rules** as additional layer:
- Block direct egress in entrypoint.sh using iptables/nftables
- Whitelist proxy + DNS only
- Prevents accidental allowlist bypass

### Docker Compose Generation

Follow the **layered composition** pattern:

**Base template** (embedded in binary via go:embed):
```yaml
services:
  proxy:
    # Fixed configuration

  dev-agent:
    image: muster-dev-agent:latest
    depends_on:
      - proxy
    # Base mounts, env, networks
```

**Programmatic overrides** in `internal/docker/compose.go`:
1. Add project/slug labels
2. Add auth-specific mounts and env vars
3. Add user-defined mounts from `.muster/dev-agent/config.yml`
4. Add provider-specific domains to allowed-domains.txt
5. Rewrite localhost URLs for local providers

**User overrides** (optional, gitignored):
- `.muster/dev-agent/docker-compose.override.yml`
- Merged last, highest priority

**Write final composed YAML** to `~/.cache/muster/{project}/docker-compose.yml` before invoking `docker compose`.

### Container Lifecycle Operations

**Startup sequence**:
1. Generate composed docker-compose.yml
2. Extract Docker assets to `~/.cache/muster/docker-assets/` (versioned by hash)
3. Build images if needed: `docker compose build`
4. Start services: `docker compose up -d`
5. Wait for health checks: poll `docker compose ps` until healthy
6. Execute lifecycle hook if defined: `docker compose exec dev-agent /workspace/.muster/dev-agent/scripts/setup.sh`

**Step execution**:
```bash
# Claude step with specific model
docker compose exec dev-agent \
  claude --prompt "Run /plan-feature" --model opus --provider anthropic

# OpenCode step
docker compose exec dev-agent \
  opencode --prompt "Run /execute-plan"
```

**Teardown** (`muster down`):
```bash
# Single slug
docker compose -p muster-${PROJECT}-${SLUG} down

# All for project
for slug in $(docker ps --filter label=muster.project=${PROJECT} --format '{{.Label "muster.slug"}}'); do
  docker compose -p muster-${PROJECT}-${slug} down
done

# Orphans (slug not in roadmap.json)
# Compare label output against .muster/roadmap.json, down the delta
```

**Persistence strategy**:
- Default: `docker compose stop` (containers remain for debugging)
- Explicit cleanup: `docker compose down` (remove containers)
- Volume cleanup: `docker compose down -v` (remove volumes too)

### Step Tool Resolution

From design doc, each pipeline step needs to resolve:

```
(tool, provider, model) → docker compose exec command
```

**Resolution flow**:
1. Determine tool (claude or opencode)
2. Determine provider (anthropic, bedrock, max, openrouter, local)
3. Resolve model tier (fast/standard/deep) or use literal model name
4. Construct exec command based on tool's CLI

**Auth validation**: Before starting container, validate all required auth is available:
```go
for step, config := range pipeline.steps {
    provider := resolveProvider(step)
    if !hasAuth(provider) {
        return fmt.Errorf("step %s requires %s auth but no credentials found", step, provider)
    }
}
```

### Multi-Tool Container Image

Design doc calls for **single image with both tools**:

```dockerfile
FROM ubuntu:22.04

# Install Claude Code
RUN curl -fsSL https://get.claude.dev/install.sh | sh

# Install OpenCode
RUN curl -fsSL https://get.opencode.dev/install.sh | sh

# Common dev tools
RUN apt-get update && apt-get install -y \
    git curl jq \
    build-essential \
    && rm -rf /var/lib/apt/lists/*

USER node
WORKDIR /workspace
```

**Benefit**: Steps can switch tools without restarting the container. Only the exec command changes.

---

## Open Questions

### Container Restart vs. Exec for Tool Switching

**Question**: Should muster restart the container when switching between Claude and OpenCode, or just exec the different tool?

**Observations**:
- Restarting allows tool-specific entrypoint logic
- Exec is faster and preserves state (installed packages, running services)
- Design doc implies exec ("exec whichever tool is configured for each pipeline step")

**Recommendation**: Use exec. Container stays up across pipeline, only the invoked binary changes.

### Named Volumes vs. Bind Mounts for Git

**Question**: Should worktrees use named volumes for cleaner abstraction, or bind mounts for direct filesystem access?

**Observations**:
- Bind mounts: Host and container see identical filesystem (easier debugging)
- Named volumes: Docker-managed, better performance on Mac/Windows
- Worktrees need host filesystem access (user may edit files outside container)

**Recommendation**: Use bind mounts. Muster's use case requires host filesystem visibility for the worktree.

### Firewall Implementation: iptables vs. nftables

**Question**: Which firewall technology to use for egress filtering?

**Observations**:
- iptables: Older, universally available, well-documented
- nftables: Modern replacement, cleaner syntax, same kernel subsystem
- Docker Desktop on Mac/Windows doesn't expose iptables to containers

**Recommendation**: Implement both with platform detection. Linux: prefer nftables, fallback to iptables. Mac/Windows Docker Desktop: rely solely on proxy (no firewall available).

### Container Naming: Compose Default vs. Custom

**Question**: Should muster override Compose's default container naming?

**Observations**:
- Compose default: `{project}_{service}_{replica}` (e.g., `muster-main-docker-orchestration_dev-agent_1`)
- Custom via `container_name`: Shorter but loses replica support
- Discovery via labels works regardless of naming

**Recommendation**: Keep Compose defaults. Labels provide discovery; custom names create edge cases.

### Auth Precedence: Environment vs. File

**Question**: When both environment variable and credential file exist, which takes precedence?

**Observations**:
- Environment is more explicit (user set it in their shell)
- Files are more permanent (survive shell restarts)
- Tools differ: AWS CLI prefers files, many APIs prefer env vars

**Recommendation**: Environment takes precedence (explicit intent). Document clearly in error messages when conflict exists.

---

## References

### Specifications
- **Devcontainer Spec**: https://containers.dev/implementors/spec/
- **Devcontainer JSON Reference**: https://containers.dev/implementors/json_reference/
- **Docker Compose Specification**: https://github.com/compose-spec/compose-spec

### Tools
- **Devcontainer CLI**: https://github.com/devcontainers/cli
- **Coder**: https://github.com/coder/coder (Go, Terraform-based)
- **Gitpod** (now Ona): https://github.com/gitpod-io/gitpod (TypeScript/Go)
- **Devbox**: https://github.com/jetify-com/devbox (Nix-based isolation)
- **GitHub Codespaces**: https://github.com/features/codespaces

### Documentation
- **Docker Compose**: https://docs.docker.com/compose/
- **Docker CLI Reference**: https://docs.docker.com/reference/cli/docker/
- **Docker Engine API**: https://docs.docker.com/engine/api/
- **VSCode Remote Containers**: https://code.visualstudio.com/docs/remote/containers

### Key Patterns
- **Label filtering**: https://docs.docker.com/reference/cli/docker/container/ls/
- **Multiple compose files**: https://docs.docker.com/compose/how-tos/multiple-compose-files/
- **Compose profiles**: https://docs.docker.com/compose/how-tos/profiles/
- **Docker Secrets**: https://docs.docker.com/compose/how-tos/use-secrets/
- **Environment variables**: https://docs.docker.com/compose/how-tos/environment-variables/
- **Networking**: https://docs.docker.com/compose/how-tos/networking/
