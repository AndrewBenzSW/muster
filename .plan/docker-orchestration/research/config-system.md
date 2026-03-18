# Config System

*Researched: 2026-03-18*
*Scope: Analyzed existing config parsing patterns (.muster/config.yml, dev-agent config, local overrides), the two-layer config design, provider auth detection mechanisms from abenz-workflow reference implementation, and integration points for Docker container orchestration*

---

## Key Findings

### Two-Layer Config Architecture

The config system has two distinct concerns split across four files:

1. **User config** (`~/.config/muster/config.yml`) — Single source of truth for tools, providers, model tiers, and user defaults
2. **Project workflow config** (`.muster/config.yml` + `.muster/config.local.yml`) — Pipeline step overrides, merge strategy, lifecycle hooks
3. **Container environment config** (`.muster/dev-agent/config.yml` + `.muster/dev-agent/config.local.yml`) — Allowed domains, env vars, volumes, networks

Both project-level config pairs support `.local.yml` overrides that are gitignored and deep-merged (local values override at field level, lists are replaced not appended).

### Provider Auth Detection (from abenz-workflow)

The reference implementation in `dev-agent/run.sh` (lines 132-229) implements sophisticated provider auth detection:

- **Claude/Bedrock**: Detects `CLAUDE_CODE_USE_BEDROCK` in `~/.claude/settings.json`, generates container settings by merging host env/model into base template, pre-resolves AWS credentials via `aws configure export-credentials`, mounts `~/.aws` read-only
- **Claude/Max**: Mounts `~/.claude/.credentials.json` read-only to inherit OAuth session
- **OpenCode/OpenRouter**: Passes through `OPENROUTER_API_KEY` environment variable
- **Local providers**: Would require rewriting `localhost` to `host.docker.internal` for container context

### Config Resolution Flow

Each pipeline step resolves a `(tool, provider, model)` triple through a fallback chain:

1. **Tool**: step-specific > `pipeline.defaults.tool` > `user.default.tool`
2. **Provider**: step-specific > `pipeline.defaults.provider` > `user.default.provider`
3. **Model string**: step-specific > `pipeline.defaults.model` > `user.default.model`
4. **Model resolution**: If starts with `muster-` prefix, look up tier in `user.tools[tool].models[tier]`; otherwise use literal model name
5. **Provider auth**: Look up provider in `user.tools[tool].providers[provider]` for auth config

### Compose Override Generation

The reference implementation (run.sh lines 235-367) demonstrates the compose override pattern:

- Parse workspace config with `yq` (YAML query tool)
- Build overlay YAML strings programmatically for environment variables, volumes, networks
- Merge allowed domains into proxy configuration
- Write generated overrides to temp files
- Pass multiple compose files to `docker compose -f base.yml -f override1.yml -f override2.yml`

### No Config Implementation Yet

The `internal/config/` package does not exist. Phase 0 completed only scaffolding (embed placeholders, test helpers, version command). The design doc specifies config as a Phase 1 deliverable alongside prompt staging.

---

## Detailed Analysis

### 1. User Config Schema

**Location**: `~/.config/muster/config.yml`

**Purpose**: Defines available tools (claude, opencode), their providers, model tier mappings, and user defaults.

**Structure** (from design.md lines 88-124):

```yaml
default:
  tool: claude
  provider: max           # which provider when not specified
  model: deep             # which tier when no model specified

tools:
  claude:
    models:
      fast: haiku
      standard: sonnet
      deep: opus
    providers:
      anthropic:                        # Anthropic API (direct)
        api_key_env: ANTHROPIC_API_KEY
      bedrock: {}                       # auto-detect from AWS config
      max: {}                           # auto-detect from ~/.claude/.credentials.json
      openrouter:
        api_key_env: OPENROUTER_API_KEY
      local:
        base_url: http://localhost:1234/v1

  opencode:
    models:
      fast: gemma3:4b
      standard: qwen3:32b
      deep: qwen3:235b
    providers:
      openrouter:
        api_key_env: OPENROUTER_API_KEY
      local:
        base_url: http://localhost:11434
```

**Key behaviors**:
- Providers with `{}` are auto-detected from host configuration
- Providers with `api_key_env` reference an environment variable name (not the key value itself)
- Providers with `base_url` point to locally running inference servers
- The same provider can appear under multiple tools (e.g., openrouter for both claude and opencode)

### 2. Project Workflow Config Schema

**Location**: `.muster/config.yml` + `.muster/config.local.yml` (gitignored)

**Purpose**: Configure pipeline behavior, step-specific tool/provider/model, merge strategy, lifecycle hooks.

**Structure** (from design.md lines 134-164):

```yaml
merge_strategy: direct  # or: github-pr, gitlab-mr

lifecycle:
  setup: .muster/dev-agent/scripts/setup.sh
  check: .muster/dev-agent/scripts/check.sh
  verify: .muster/dev-agent/scripts/verify.sh
  teardown: .muster/dev-agent/scripts/teardown.sh

# Per-step tool, provider, and model configuration
pipeline:
  defaults:
    tool: claude
    provider: max
    model: muster-standard        # resolves via user config
  plan:
    provider: anthropic           # override provider
    model: muster-deep            # resolves to opus
  execute:
    provider: local               # claude + local provider
    model: qwen3:32b              # literal model name
  review:
    provider: anthropic
    model: muster-deep
  add:
    model: muster-fast            # inherits tool + provider from defaults
```

**Deep merge behavior**:
- `.local.yml` overrides win at the field level
- Missing fields in local fall through to base config
- **Lists are replaced, not appended** — if local specifies `lifecycle.verify`, it replaces the entire value

### 3. Container Environment Config Schema

**Location**: `.muster/dev-agent/config.yml` + `.muster/dev-agent/config.local.yml` (gitignored)

**Purpose**: Configure sandbox environment — network allowlist, environment variables, volume mounts, external networks.

**Structure** (from design.md lines 167-177 and abenz-workflow example):

```yaml
allowed_domains:
  - .npmjs.org
  - .internal-registry.example.com

env:
  DATABASE_URL: "postgres://postgres:postgres@db:5432/myapp"

volumes:
  - "path:/container/path:ro"

networks:
  - external-network
```

**Reference implementation patterns** (dev-agent/example.dev-agent/config.yml):
- `allowed_domains`: Merged with default allowlist, passed to proxy container
- `env`: Environment variables injected into agent container
- `volumes`: Extra volume mounts, relative paths resolve from workspace root
- `networks`: Join existing external Docker networks (created if missing)

The reference implementation also includes `lifecycle` hooks in dev-agent config, but the design doc splits this into workflow config. Both patterns need to be supported for backward compatibility.

### 4. Provider Auth Detection Patterns

**Source**: `dev-agent/run.sh` lines 132-229

#### Claude/Bedrock Detection

```bash
detect_auth_mode() {
    local settings="${HOME}/.claude/settings.json"
    if [ -f "$settings" ] && command -v jq &>/dev/null; then
        if jq -e '.env.CLAUDE_CODE_USE_BEDROCK' "$settings" &>/dev/null; then
            return 0  # Bedrock mode
        fi
    fi
    return 1  # Max mode
}
```

**Key insight**: Detection is based on presence of `CLAUDE_CODE_USE_BEDROCK` key in user's global Claude settings, not its value.

#### Bedrock Settings Generation

When Bedrock mode is detected:
1. Read host's `~/.claude/settings.json` for dynamic values (AWS profile, region, model overrides)
2. Merge into container base template at `dev-agent/dot-claude/settings.json`
3. Export environment variables for docker-compose interpolation: `AWS_PROFILE`, `AWS_REGION`, `ANTHROPIC_MODEL`, `ANTHROPIC_SMALL_FAST_MODEL`
4. Pre-resolve credentials via `aws configure export-credentials` to avoid interactive refresh in container
5. Mount `~/.aws` directory read-only
6. Set `CLAUDE_CODE_USE_BEDROCK=1` environment variable

**Container base template** (`dot-claude/settings.json`):
```json
{
  "$schema": "https://json.schemastore.org/claude-code-settings.json",
  "awsAuthRefresh": "aws sso login --no-browser",
  "permissions": { "defaultMode": "bypassPermissions" },
  "effortLevel": "high",
  "enabledPlugins": {},
  "skipDangerousModePermissionPrompt": true,
  "theme": "dark",
  "hasTrustDialogAccepted": true
}
```

The `env` and `model` fields are merged from host settings at runtime.

#### Max Authentication

When Max mode is detected (default):
1. Use static settings file: `dev-agent/dot-claude/settings.max.json`
2. Mount `~/.claude/.credentials.json` read-only to inherit OAuth session
3. Generate compose override to mount credentials file

**Max settings template**:
```json
{
  "$schema": "https://json.schemastore.org/claude-code-settings.json",
  "env": {
    "CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS": "1"
  },
  "permissions": { "defaultMode": "bypassPermissions" },
  "enabledPlugins": {},
  "effortLevel": "high",
  "skipDangerousModePermissionPrompt": true,
  "theme": "dark",
  "hasTrustDialogAccepted": true
}
```

#### OpenCode/OpenRouter Authentication

From `docker-compose.yml` line 73 and `dot-opencode/opencode.json`:

```yaml
environment:
  - OPENROUTER_API_KEY=${OPENROUTER_API_KEY:-}
```

```json
{
  "$schema": "https://opencode.ai/config.json",
  "provider": { "openrouter": {} },
  "model": "openrouter/anthropic/claude-sonnet-4-5",
  "small_model": "openrouter/anthropic/claude-haiku-4-5"
}
```

**Pattern**: Pass through environment variable from host. Provider config uses empty object `{}` — the API key comes from env var.

### 5. Compose Override Generation

**Source**: `dev-agent/run.sh` lines 235-367

The reference implementation builds YAML strings programmatically and writes them to temp files:

```bash
# Example: Building environment variable overrides
local override="services:"
override+=$'\n'"  ${AGENT_SERVICE}:"
override+=$'\n'"    environment:"
while IFS= read -r key; do
    local value=$(yq -r ".env.\"$key\"" "$WORKSPACE_CONFIG")
    override+=$'\n'"      - ${key}=${value}"
done <<< "$env_keys"

echo "$override" > "$TEMP_DIR/generated-override.yml"
```

**Compose file layering**:
```bash
COMPOSE_ARGS=(-p "$PROJECT_NAME" -f "$BASE_COMPOSE_FILE" --profile "$AGENT_PROFILE")

# Add auth override
if [ -f "$TEMP_DIR/auth-override.yml" ]; then
    COMPOSE_ARGS+=(-f "$TEMP_DIR/auth-override.yml")
fi

# Add workspace overrides
if [ -f "$TEMP_DIR/generated-override.yml" ]; then
    COMPOSE_ARGS+=(-f "$TEMP_DIR/generated-override.yml")
fi

# Add proxy domain override
if [ -f "$TEMP_DIR/proxy-override.yml" ]; then
    COMPOSE_ARGS+=(-f "$TEMP_DIR/proxy-override.yml")
fi

docker compose "${COMPOSE_ARGS[@]}" up -d
```

**Key patterns**:
- Generate overrides to a temp directory per project (`~/.cache/dev-agent/${PROJECT_NAME}`)
- Build full compose command with multiple `-f` flags for layering
- Use string concatenation to build YAML programmatically (bash doesn't have YAML libraries)
- Temp directory must be under `$HOME` for Docker VM compatibility (colima on macOS)

### 6. Config Resolution Algorithm

**Source**: Design.md lines 180-190

For each pipeline step:

```
1. Determine tool:
   step.tool > pipeline.defaults.tool > user.default.tool

2. Determine provider:
   step.provider > pipeline.defaults.provider > user.default.provider

3. Determine model string:
   step.model > pipeline.defaults.model > user.default.model

4. Resolve model string:
   if starts with "muster-":
     tier = strip "muster-" prefix
     model = user.tools[tool].models[tier]
   else:
     model = model string as-is

5. Resolve provider auth:
   auth_config = user.tools[tool].providers[provider]
   return (tool, provider, model, auth_config)
```

**Example**:
- User config: `claude.models.deep = "opus"`, `default.provider = "max"`
- Project config: `pipeline.plan.model = "muster-deep"`
- Resolution: tool=claude (default), provider=max (default), model string="muster-deep" → resolves to "opus"

### 7. Multi-Tool Container Design

**Source**: Design.md lines 339-359

The Docker implementation uses a **single container image with both Claude Code and OpenCode installed**. The CLI determines which tool to `exec` based on the step configuration. This means:

- Container stays running across pipeline steps
- No restart needed to switch tools
- **All required auth must be injected at container startup** since multiple steps may use different tool+provider combinations

**Example scenario**:
- Plan step: `claude` + `anthropic` provider → needs `ANTHROPIC_API_KEY`
- Execute step: `claude` + `local` provider → needs `base_url` rewrite to `host.docker.internal`
- Review step: `opencode` + `openrouter` provider → needs `OPENROUTER_API_KEY`

The CLI must scan all pipeline steps before starting the container to collect all required auth configurations.

### 8. Local Provider Container Networking

**Source**: Design.md lines 345

When a provider has `base_url: http://localhost:1234`, the CLI must:
1. Detect this is a localhost URL
2. Rewrite to `host.docker.internal:1234` for container context
3. Update proxy allowlist to permit `host.docker.internal`
4. Update firewall rules in container to allow gateway access

This enables the container to reach LM Studio/Ollama/vLLM running on the host.

### 9. Existing Code Patterns

**No config package exists yet**. Based on other internal packages:

#### Package Organization Pattern
```
internal/config/
├── config.go          # Main types and loading
├── config_test.go     # Table-driven tests
├── user.go            # User config parsing
├── user_test.go
├── project.go         # Project config parsing and merging
├── project_test.go
└── resolve.go         # Step resolution algorithm
    └── resolve_test.go
```

#### Struct Naming Pattern (from internal/ui/output.go)
```go
type OutputMode string

const (
    JSONMode  OutputMode = "json"
    TableMode OutputMode = "table"
)

type VersionInfo struct {
    Version   string `json:"version"`
    BuildDate string `json:"build_date"`
}
```

#### YAML Parsing (from go.mod)
The project includes `gopkg.in/yaml.v3` as a dependency. Standard usage:

```go
import "gopkg.in/yaml.v3"

type Config struct {
    Field string `yaml:"field"`
}

func Load(path string) (*Config, error) {
    data, err := os.ReadFile(path)
    if err != nil {
        return nil, fmt.Errorf("failed to read config: %w", err)
    }

    var cfg Config
    if err := yaml.Unmarshal(data, &cfg); err != nil {
        return nil, fmt.Errorf("failed to parse YAML: %w", err)
    }

    return &cfg, nil
}
```

#### Error Handling Pattern (from cmd/root.go)
- Use `fmt.Errorf` with `%w` for wrapping
- Return errors, don't log and continue
- Provide context in error messages
- No custom error types for simple cases

#### Testing Pattern (from internal/ui/output_test.go)
- Table-driven tests are the default
- Use `testify/assert` for soft checks, `testify/require` for hard stops
- Use `t.Cleanup()` to restore state
- Golden files supported via `testutil.AssertGoldenFile()`

### 10. Container Labeling

**Source**: Design.md lines 350-357

Every container gets Docker labels for discovery:

```yaml
labels:
  muster.managed: "true"
  muster.project: "my-api"       # derived from project directory name
  muster.slug: "add-retry-logic" # omitted for interactive sessions
```

These labels enable:
- `muster down <slug>` — teardown by item slug
- `muster down --all` — teardown all containers for project
- `muster down --orphans` — cross-reference with roadmap to find stale containers
- `muster status` — show container state alongside roadmap items

Query pattern: `docker ps --filter label=muster.managed=true --filter label=muster.project=<name>`

---

## Recommendations

### For Config Package Implementation

1. **Split into focused files**:
   - `user.go` — User config struct, loader, defaults
   - `project.go` — Project config struct, loader, deep merge logic for `.local.yml` overrides
   - `devagent.go` — Container environment config struct, loader, merge logic
   - `resolve.go` — Step resolution algorithm (tool, provider, model triple)
   - `auth.go` — Provider auth config extraction and validation

2. **Use `gopkg.in/yaml.v3` for parsing**:
   - Define structs with `yaml:"fieldName"` tags
   - Use `yaml.Unmarshal()` for parsing
   - Support both missing files (use defaults) and malformed files (return error)

3. **Implement deep merge for local overrides**:
   - Field-level merge: local value wins if present, otherwise fall through to base
   - List replacement: if local defines a list field, it replaces the entire list
   - Use struct embedding or reflection for generic merge logic
   - Test with fixtures in `testdata/`

4. **Validate at load time**:
   - Check that referenced tools exist in user config
   - Check that referenced providers exist for the tool
   - Check that tier names (`muster-fast`, etc.) resolve to concrete models
   - Check that env var names in `api_key_env` fields are set on the host
   - Return all validation errors at once (collect them), don't fail on first error

5. **Provide resolution tracing for `muster doctor --config`**:
   - Track which config layer provided each value (user default, project default, step override)
   - Include this metadata in the resolved step config struct
   - Format as table showing: step name | tool | provider | model | source
   - Example: `plan | claude | anthropic | opus | step override (provider), user default (tool)`

### For Docker Auth Detection

1. **Scan all pipeline steps before container startup**:
   - Parse project config to get all step configurations
   - Resolve (tool, provider) pairs for each step
   - Deduplicate provider list across steps
   - Collect auth requirements for all referenced providers

2. **Implement provider-specific detection functions**:
   - `DetectBedrockAuth() (AwsConfig, error)` — Check `~/.claude/settings.json` for Bedrock flag
   - `DetectMaxAuth() (string, error)` — Check `~/.claude/.credentials.json` exists
   - `DetectAPIKeyAuth(envVar string) (string, error)` — Check env var is set
   - `DetectLocalProvider(baseURL string) (string, error)` — Validate URL is reachable, return container-rewritten URL
   - Each function returns auth config or error if auth is missing/invalid

3. **Generate auth-specific compose overrides**:
   - **Bedrock**: Merge host settings, mount `~/.aws`, export env vars, pre-resolve credentials
   - **Max**: Mount `~/.claude/.credentials.json`
   - **API key**: Pass through env var
   - **Local provider**: Rewrite URL, add to proxy allowlist, configure firewall rules
   - Write each override to temp file, return list of paths for compose `-f` args

4. **Validate before starting container**:
   - Run all detection functions for required providers
   - Collect all errors and present them together
   - For missing auth: provide specific fix instructions (e.g., "Run `aws configure sso` for Bedrock")
   - This validation should also power `muster doctor` checks

### For Compose Override Generation

1. **Use programmatic YAML building in Go**:
   - Define struct types for compose service overrides
   - Use `yaml.Marshal()` to generate YAML
   - Write to temp directory per project: `~/.cache/muster/{project}/`
   - Much cleaner than bash string concatenation

2. **Layer overrides in order**:
   - Base compose file (embedded asset)
   - Auth overrides (generated from provider detection)
   - Workspace config overrides (generated from `.muster/dev-agent/config.yml`)
   - User local overrides (from `.muster/dev-agent/config.local.yml`)
   - Pass all to `docker compose` with multiple `-f` flags

3. **Handle domain allowlist merging**:
   - Start with default domains from embedded `allowed-domains.txt`
   - Append project domains from `allowed_domains` config field
   - Append local domains from local config override
   - Write merged list to temp file
   - Generate proxy override to mount this file

4. **Test compose generation without Docker**:
   - Unit tests take config structs, assert generated YAML content
   - Compare against golden files or inline expectations
   - No actual `docker compose` invocation needed for unit tests
   - Integration tests (guarded by `testing.Short()`) can test full lifecycle

### For Integration Points

1. **`muster code --yolo` needs**:
   - Load user config to determine default tool
   - Load project config (workflow + container environment)
   - Detect worktree to determine workspace mount path
   - Resolve all auth for the default tool's provider
   - Generate all compose overrides
   - Start container with labels: `muster.managed=true`, `muster.project=<name>`, no slug (interactive)
   - Stage prompt templates (Phase 1 work)
   - Exec the tool in container with staged skill directory

2. **`muster in <slug>` pipeline needs**:
   - Load user config, project configs
   - For each step: resolve (tool, provider, model) triple
   - Collect all provider auth requirements
   - Generate compose overrides including all required auth
   - Start container with slug label
   - For each step: exec appropriate tool command with model override

3. **`muster down [slug]` needs**:
   - Query containers by labels: `muster.managed=true` + `muster.project=<project>`
   - If slug provided, filter to `muster.slug=<slug>`
   - For each container: `docker compose -p <name> down`
   - For `--orphans`: cross-reference running container slugs against roadmap status (find containers where slug is not `in_progress`)

---

## Open Questions

### 1. How to handle missing user config?

**Question**: If `~/.config/muster/config.yml` doesn't exist, should the CLI:
- A) Error immediately and require `muster init` to create it
- B) Use hardcoded defaults (claude + max provider + standard model)
- C) Prompt user interactively to create it

**Why it matters**: First-run experience. If we require config file to exist, users must run `muster init` first. If we use defaults, we need to document what those defaults are.

**Attempted**: Checked design doc — no explicit guidance. The `muster init` command (Phase 7) "stages loop-setup prompt, invokes Claude to auto-detect tooling and generate `.muster/` structure" but doesn't mention generating user config.

**Recommendation**: Error with helpful message pointing to `muster init` or manual config creation. User config defines which tools are available — can't make assumptions about whether user has Claude Max, Bedrock, or OpenRouter.

### 2. How to validate local provider reachability?

**Question**: When a provider has `base_url: http://localhost:1234`, should the CLI:
- A) Try to connect and validate the endpoint responds
- B) Trust the user's config and only fail when actually invoking the tool
- C) Validate during `muster doctor` but not during normal operation

**Why it matters**: Starting a container is expensive. Better to fail fast if the local inference server isn't running. But checking every time adds startup latency.

**Attempted**: Design doc mentions "The CLI validates at startup that all referenced providers have valid auth config (e.g., the env var actually exists, the local URL is reachable)." This suggests validation at startup.

**Recommendation**: Validate during container startup (fail fast), also include in `muster doctor` checks. Use a simple HTTP GET with timeout (e.g., 2 seconds). Cache the validation result per session to avoid repeated checks.

### 3. How to handle API key rotation?

**Question**: If a user updates `ANTHROPIC_API_KEY` or `~/.claude/.credentials.json` while a container is running:
- A) Require explicit `muster down` and restart to pick up new auth
- B) Support hot-reload of auth (generate new overrides, recreate container)
- C) Document this as a known limitation

**Why it matters**: Long-running pipeline (`muster in --all`) might hit auth expiration mid-execution.

**Attempted**: Design doc mentions pre-resolving AWS credentials for Bedrock to avoid interactive refresh, but doesn't address key rotation.

**Recommendation**: Document as known limitation for Phase 2. Require `muster down` and restart. In future phases, could add `muster reload-auth` command to regenerate overrides and recreate container.

### 4. What happens when step references undefined provider?

**Question**: If project config says `pipeline.plan.provider: anthropic` but user config has no `tools.claude.providers.anthropic`:
- A) Error at config load time (fail fast)
- B) Error when executing that specific step
- C) Fall back to default provider with warning

**Why it matters**: Validation timing affects error messages. Load-time validation is better UX but requires full config resolution upfront.

**Attempted**: Design doc says "The CLI validates at startup that all referenced providers have valid auth config" — suggests load-time validation.

**Recommendation**: Validate during config resolution (before starting container). Collect all validation errors and present them together with file path + line number if possible. Use YAML line number tracking in `yaml.v3` decoder.

### 5. Should config support environment variable expansion?

**Question**: Should config values like `base_url: http://localhost:${LM_STUDIO_PORT}` expand environment variables?
- A) Yes, expand all `${VAR}` and `$VAR` references using `os.ExpandEnv()`
- B) No, keep config literal — users can generate config files from templates if needed
- C) Support only in specific fields (e.g., `env` values in dev-agent config)

**Why it matters**: Flexibility vs. complexity. Env var expansion is common in configs but adds parsing complexity and potential for injection bugs.

**Attempted**: Design doc doesn't mention env var expansion. Reference implementation doesn't use it (URLs are literal).

**Recommendation**: Don't support in Phase 2. Keep config literal. If users need dynamic values, they can use `.local.yml` overrides or shell script to generate config. Can add in future phase if demand exists.

---

## References

### Files Examined

**Design Documentation**:
- `/Users/andrew.benz/work/muster/muster-main/docs/design.md` — Lines 84-192 (Configuration section), 194-237 (Architecture), 339-359 (Docker Integration)

**Reference Implementation (abenz-workflow)**:
- `/Users/andrew.benz/work/claude/abenz-workflow/dev-agent/run.sh` — Lines 132-229 (auth detection), 235-367 (compose override generation), 369-420 (worktree detection)
- `/Users/andrew.benz/work/claude/abenz-workflow/dev-agent/docker-compose.yml` — Lines 1-83 (service definitions, volume mounts, environment variables)
- `/Users/andrew.benz/work/claude/abenz-workflow/dev-agent/dot-claude/settings.json` — Bedrock container settings template
- `/Users/andrew.benz/work/claude/abenz-workflow/dev-agent/dot-claude/settings.max.json` — Max container settings template
- `/Users/andrew.benz/work/claude/abenz-workflow/dev-agent/dot-opencode/opencode.json` — OpenCode provider configuration
- `/Users/andrew.benz/work/claude/abenz-workflow/dev-agent/example.dev-agent/config.yml` — Example workspace config

**Current Implementation**:
- `/Users/andrew.benz/work/muster/muster-main/cmd/root.go` — Global flags, TTY detection, output mode setup
- `/Users/andrew.benz/work/muster/muster-main/internal/ui/output.go` — Type definitions, JSON tags, format switching
- `/Users/andrew.benz/work/muster/muster-main/internal/testutil/helpers.go` — Test helper patterns
- `/Users/andrew.benz/work/muster/muster-main/go.mod` — Dependencies including `gopkg.in/yaml.v3`

### External Resources

- **YAML v3 Library**: https://github.com/go-yaml/yaml — Used for parsing, supports struct tags, line number tracking
- **Docker Compose**: CLI used for container lifecycle (not Docker SDK) — compose handles service ordering, health checks, overlay merging
- **jq**: Used in reference implementation for JSON parsing (bash) — muster will use Go's `encoding/json` instead

### Related Roadmap Items

- **prompt-staging** (Phase 1, in_progress): Template resolution and staging — config resolution feeds template context
- **docker-orchestration** (Phase 2, planned): This feature — consumes config system for auth detection and compose generation
- **tooling-commands** (Phase 7, planned): `muster doctor` command — uses config validation and auth detection for health checks
