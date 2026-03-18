# Docker Orchestration Architecture

*Phase 2 of muster CLI -- Docker container orchestration for sandboxed AI coding sessions*

---

## Overview

This document defines the technical architecture for Phase 2: Docker container orchestration. The feature enables `muster code --yolo` (sandboxed interactive sessions) and `muster down` (container teardown), replacing the 540-line `run.sh` bash script with typed Go code.

The architecture follows a **hybrid Docker SDK + Compose CLI** pattern: `docker compose` CLI handles all lifecycle orchestration (up/down/exec), while the Docker Go SDK handles container queries and label-based discovery. Compose files are generated programmatically via `gopkg.in/yaml.v3` YAML marshaling from an embedded base template plus runtime overrides.

---

## Package Layout

All new code lives under `internal/docker/` and `internal/config/`, with new commands in `cmd/`. The existing `internal/docker/embed.go` remains unchanged.

```
internal/
  config/
    config.go           # Top-level loader: LoadAll(userPath, projectDir) -> *Config
    types.go            # UserConfig, ProjectConfig, DevAgentConfig structs
    user.go             # LoadUserConfig(path) -> *UserConfig
    project.go          # LoadProjectConfig(dir) -> *ProjectConfig (with .local.yml merge)
    devagent.go         # LoadDevAgentConfig(dir) -> *DevAgentConfig (with .local.yml merge)
    merge.go            # deepMerge helper for .local.yml overlay
    resolve.go          # ResolveStep(step, projectCfg, userCfg) -> StepConfig
    resolve_test.go     # Table-driven: step triple resolution, tier lookup, fallback
    user_test.go        # Table-driven: parsing, missing file, malformed YAML
    project_test.go     # Table-driven: deep merge, list replacement, field fallthrough
    devagent_test.go    # Table-driven: domain merge, volume parsing
    testdata/           # Fixture config files

  docker/
    embed.go            # (existing) go:embed all:docker
    embed_test.go       # (existing)
    docker/             # (existing, will be populated with actual Docker assets)
      docker-compose.yml      # Embedded base compose file
      agent.Dockerfile        # Multi-tool dev-agent image
      proxy.Dockerfile        # Squid proxy image
      entrypoint.sh           # Container entrypoint
      init-firewall.sh        # Firewall setup (Linux only)
      squid.conf              # Squid proxy configuration
      allowed-domains.txt     # Base domain allowlist
      settings.json           # Claude Bedrock settings template
      settings.max.json       # Claude Max settings template
    auth.go             # Provider auth detection and validation
    auth_test.go        # Table-driven: mock filesystem + env vars
    compose.go          # Compose file generation and writing
    compose_test.go     # Unit tests: assert YAML structure (no Docker required)
    container.go        # Docker SDK client + compose CLI wrapper
    container_test.go   # Integration tests (guarded by testing.Short)
    labels.go           # Label constants and builder
    testdata/
      compose/          # Golden files for generated compose output
      auth/             # Mock credential fixtures

cmd/
    code.go             # muster code [--yolo] [--tool] [--no-plugin]
    down.go             # muster down [slug] [--all] [--orphans] [--project]
```

---

## Data Models

### Config Types (`internal/config/types.go`)

```go
package config

// Config holds the fully loaded configuration from all layers.
type Config struct {
    User     *UserConfig
    Project  *ProjectConfig
    DevAgent *DevAgentConfig
}

// UserConfig represents ~/.config/muster/config.yml
type UserConfig struct {
    Default UserDefault          `yaml:"default"`
    Tools   map[string]ToolConfig `yaml:"tools"` // keyed by "claude", "opencode"
}

type UserDefault struct {
    Tool     string `yaml:"tool"`
    Provider string `yaml:"provider"`
    Model    string `yaml:"model"`
}

type ToolConfig struct {
    Models    map[string]string       `yaml:"models"`    // tier -> concrete model name
    Providers map[string]ProviderConfig `yaml:"providers"` // provider name -> config
}

type ProviderConfig struct {
    APIKeyEnv string `yaml:"api_key_env,omitempty"` // env var name holding the key
    BaseURL   string `yaml:"base_url,omitempty"`    // for local providers
    // Empty struct ({}) means auto-detected (bedrock, max)
}

// ProjectConfig represents .muster/config.yml merged with .muster/config.local.yml
type ProjectConfig struct {
    MergeStrategy string          `yaml:"merge_strategy"`
    Lifecycle     LifecycleConfig `yaml:"lifecycle"`
    Pipeline      PipelineConfig  `yaml:"pipeline"`
}

type LifecycleConfig struct {
    Setup    string `yaml:"setup,omitempty"`
    Check    string `yaml:"check,omitempty"`
    Verify   string `yaml:"verify,omitempty"`
    Teardown string `yaml:"teardown,omitempty"`
}

type PipelineConfig struct {
    Defaults StepOverride            `yaml:"defaults"`
    Steps    map[string]StepOverride `yaml:",inline"` // plan, execute, review, etc.
}

type StepOverride struct {
    Tool     string `yaml:"tool,omitempty"`
    Provider string `yaml:"provider,omitempty"`
    Model    string `yaml:"model,omitempty"`
}

// DevAgentConfig represents .muster/dev-agent/config.yml merged with its .local.yml
type DevAgentConfig struct {
    AllowedDomains []string          `yaml:"allowed_domains"`
    Env            map[string]string `yaml:"env"`
    Volumes        []string          `yaml:"volumes"`
    Networks       []string          `yaml:"networks"`
}

// StepConfig is the fully resolved (tool, provider, model) triple for a pipeline step.
type StepConfig struct {
    Tool     string // "claude" or "opencode"
    Provider string // "anthropic", "bedrock", "max", "openrouter", "local"
    Model    string // concrete model name after tier resolution
}
```

**Design rationale**: `PipelineConfig` uses `Steps map[string]StepOverride` with `yaml:",inline"` to capture arbitrary step names (plan, execute, review, add, etc.) without enumerating them in the struct. The `Defaults` field is parsed explicitly so the fallback chain works naturally: check step map first, then defaults, then user defaults.

Note: The `yaml:",inline"` approach for `PipelineConfig` needs special handling because `Defaults` and `Steps` would both try to capture YAML keys. A cleaner implementation would use a custom `UnmarshalYAML` method on `PipelineConfig` that separates the `defaults` key from other keys:

```go
func (p *PipelineConfig) UnmarshalYAML(value *yaml.Node) error {
    // Decode all keys from the mapping node
    // "defaults" goes to p.Defaults, everything else goes to p.Steps
}
```

### Docker Compose Types (`internal/docker/compose.go`)

Minimal structs covering only fields muster actually uses. These are generation-focused, not parsing-focused -- we control the output.

```go
package docker

// ComposeFile represents a Docker Compose file with only the fields muster uses.
type ComposeFile struct {
    Services map[string]*Service `yaml:"services"`
    Volumes  map[string]any      `yaml:"volumes,omitempty"`
    Networks map[string]any      `yaml:"networks,omitempty"`
    Name     string              `yaml:"name,omitempty"`
}

type Service struct {
    Image       string              `yaml:"image,omitempty"`
    Build       *BuildConfig        `yaml:"build,omitempty"`
    Ports       []string            `yaml:"ports,omitempty"`
    Volumes     []string            `yaml:"volumes,omitempty"`
    Environment map[string]string   `yaml:"environment,omitempty"`
    Labels      map[string]string   `yaml:"labels,omitempty"`
    Networks    map[string]any      `yaml:"networks,omitempty"`
    DependsOn   map[string]any      `yaml:"depends_on,omitempty"`
    ExtraHosts  []string            `yaml:"extra_hosts,omitempty"`
    Command     string              `yaml:"command,omitempty"`
    Entrypoint  []string            `yaml:"entrypoint,omitempty"`
    WorkingDir  string              `yaml:"working_dir,omitempty"`
    User        string              `yaml:"user,omitempty"`
    Profiles    []string            `yaml:"profiles,omitempty"`
}

type BuildConfig struct {
    Context    string `yaml:"context"`
    Dockerfile string `yaml:"dockerfile,omitempty"`
}
```

**Design rationale**: `Volumes` and `Networks` at the top level use `map[string]any` because we need to represent both empty mappings (`proxy-net: {}`) and configured entries. `DependsOn` uses `map[string]any` to support both simple form (`depends_on: [proxy]`) and extended form with conditions. Service-level `Networks` is a map because compose supports per-network aliases.

### Auth Detection Types (`internal/docker/auth.go`)

```go
package docker

// AuthRequirements collects all auth needed across pipeline steps.
type AuthRequirements struct {
    Bedrock   *BedrockAuth
    Max       *MaxAuth
    APIKeys   []APIKeyAuth   // one per unique (provider, env var) pair
    Local     []LocalAuth    // one per unique base_url
}

type BedrockAuth struct {
    AWSDir        string            // path to ~/.aws
    AWSProfile    string            // from host env or settings
    AWSRegion     string            // from host env or settings
    SettingsJSON  []byte            // merged settings.json for container
}

type MaxAuth struct {
    CredentialsPath string // path to ~/.claude/.credentials.json
    SettingsJSON    []byte // settings.max.json content
}

type APIKeyAuth struct {
    Provider string // "anthropic", "openrouter"
    EnvVar   string // env var name (e.g., "ANTHROPIC_API_KEY")
    Value    string // resolved value from host env
}

type LocalAuth struct {
    Provider     string // "local"
    HostURL      string // original URL (e.g., http://localhost:1234)
    ContainerURL string // rewritten URL (e.g., http://host.docker.internal:1234)
}
```

### Container Labels (`internal/docker/labels.go`)

```go
package docker

const (
    LabelManaged = "muster.managed"
    LabelProject = "muster.project"
    LabelSlug    = "muster.slug"
)

// Labels builds the label map for a container.
func Labels(project, slug string) map[string]string {
    labels := map[string]string{
        LabelManaged: "true",
        LabelProject: project,
    }
    if slug != "" {
        labels[LabelSlug] = slug
    }
    return labels
}
```

---

## Component Architecture

### 1. Config Resolution (`internal/config/resolve.go`)

**Function signature**:

```go
// ResolveStep resolves the (tool, provider, model) triple for a named pipeline step.
// It follows the fallback chain: step-specific > pipeline.defaults > user.default.
// Model strings prefixed with "muster-" are resolved via user.tools[tool].models[tier].
func ResolveStep(stepName string, project *ProjectConfig, user *UserConfig) (StepConfig, error)
```

**Resolution algorithm** (synthesis requirement: Config Resolution Flow):

1. Look up `stepName` in `project.Pipeline.Steps` for step-specific overrides
2. Fill missing fields from `project.Pipeline.Defaults`
3. Fill remaining missing fields from `user.Default`
4. If model starts with `muster-`, strip prefix and look up tier in `user.Tools[tool].Models[tier]`
5. Validate: tool must exist in `user.Tools`, provider must exist in `user.Tools[tool].Providers`
6. Return `StepConfig` or error with actionable message

**Integration point**: Called by `CollectAuthRequirements` to scan all steps, and by pipeline step execution to determine exec command.

### 2. Config Loading and Merging (`internal/config/`)

**Function signatures**:

```go
func LoadUserConfig(path string) (*UserConfig, error)
func LoadProjectConfig(dir string) (*ProjectConfig, error)
func LoadDevAgentConfig(dir string) (*DevAgentConfig, error)
func LoadAll(userConfigPath, projectDir string) (*Config, error)
```

**Deep merge for .local.yml** (`internal/config/merge.go`):

```go
// deepMerge overlays local config onto base config using YAML round-trip.
// Field-level override: local values win when present.
// List replacement: if local defines a list, it replaces the entire base list.
func deepMerge(base, local []byte) ([]byte, error)
```

Implementation approach: Unmarshal both into `map[string]any`, walk the maps recursively. For scalar and list values, local wins. For map values, recurse. This matches the synthesis requirement that lists are replaced, not appended.

**Validation** collects all errors before returning:

```go
func (c *Config) Validate() []error
```

Checks: referenced tools exist, referenced providers exist for tool, tier names resolve, `api_key_env` vars are set on host. Returns all errors at once per synthesis requirement.

### 3. Auth Detection (`internal/docker/auth.go`)

**Top-level function** scans all pipeline steps, collects unique providers, detects auth for each:

```go
// CollectAuthRequirements scans all pipeline steps, resolves their providers,
// and detects auth for each. Returns collected auth or all validation errors.
func CollectAuthRequirements(
    cfg *config.Config,
    steps []string,
) (*AuthRequirements, []error)
```

**Per-provider detection functions**:

```go
// DetectBedrockAuth checks ~/.claude/settings.json for CLAUDE_CODE_USE_BEDROCK,
// resolves AWS profile/region, pre-validates credentials, and returns auth config.
func DetectBedrockAuth() (*BedrockAuth, error)

// DetectMaxAuth checks ~/.claude/.credentials.json exists and returns its path.
func DetectMaxAuth() (*MaxAuth, error)

// DetectAPIKeyAuth checks the named environment variable is set and non-empty.
func DetectAPIKeyAuth(envVar string) (*APIKeyAuth, error)

// DetectLocalProvider rewrites localhost URLs to host.docker.internal
// and optionally validates reachability with a 2-second timeout.
func DetectLocalProvider(baseURL string) (*LocalAuth, error)
```

**Detection flow** (synthesis requirement: Scan All Steps for Auth Upfront):

1. For each step name, call `config.ResolveStep` to get `(tool, provider)`
2. Deduplicate `(tool, provider)` pairs across steps
3. For each unique provider, call the appropriate detection function
4. Collect results into `AuthRequirements`, collect errors into error list
5. If any errors, return all together with actionable messages

**Bedrock detection** (from reference implementation `run.sh:132-229`):

1. Read `~/.claude/settings.json`, check for `env.CLAUDE_CODE_USE_BEDROCK` key
2. Extract `AWS_PROFILE`, `AWS_REGION` from settings or environment
3. Merge host settings values into embedded `settings.json` template
4. Return `BedrockAuth` with merged settings and AWS config

**Local provider URL rewriting** (synthesis requirement: Localhost URL Rewriting):

```go
func rewriteLocalhostURL(rawURL string) (string, error) {
    u, err := url.Parse(rawURL)
    if err != nil {
        return "", fmt.Errorf("invalid base_url %q: %w", rawURL, err)
    }
    host := u.Hostname()
    if host == "localhost" || host == "127.0.0.1" || host == "::1" {
        u.Host = "host.docker.internal:" + u.Port()
    }
    return u.String(), nil
}
```

### 4. Compose Generation (`internal/docker/compose.go`)

**Primary function**:

```go
// GenerateComposeFile builds a merged compose file from the embedded base,
// auth overrides, workspace config, and user overrides. Writes the result
// to ~/.cache/muster/{project}/docker-compose.yml and returns the path.
func GenerateComposeFile(opts ComposeOptions) (string, error)

type ComposeOptions struct {
    Project     string              // project name (from directory)
    Slug        string              // roadmap item slug (empty for interactive)
    Auth        *AuthRequirements   // collected auth config
    DevAgent    *config.DevAgentConfig // container environment config
    WorktreeDir string              // host path to worktree (or project dir)
    MainRepoDir string              // host path to main repo (.git location)
    AssetDir    string              // path to extracted docker assets
}
```

**Generation flow** (synthesis requirement: Single Merged Compose File):

1. **Load base**: Unmarshal embedded `docker-compose.yml` into `ComposeFile`
2. **Apply auth overrides**: Call `applyAuth(&base, opts.Auth)` which adds volumes, env vars, and extra_hosts based on detected providers
3. **Apply workspace mounts**: Add worktree bind mount, .git read-only mount, credential mounts
4. **Apply dev-agent config**: Merge user-defined env vars, volumes, networks from `DevAgentConfig`
5. **Apply domain allowlist**: Merge base `allowed-domains.txt` with project `allowed_domains`, write to asset dir, mount into proxy
6. **Apply labels**: Add `muster.managed`, `muster.project`, `muster.slug` to all services
7. **Apply user override**: If `.muster/dev-agent/docker-compose.override.yml` exists, parse and merge it
8. **Write**: Marshal to YAML, write to `~/.cache/muster/{project}/docker-compose.yml`
9. **Validate**: Run `docker compose -f {path} config` to catch generation bugs

**Auth override application** (`applyAuth`):

```go
func applyAuth(compose *ComposeFile, auth *AuthRequirements) {
    agent := compose.Services["dev-agent"]

    if auth.Bedrock != nil {
        // Mount ~/.aws read-only
        agent.Volumes = append(agent.Volumes,
            fmt.Sprintf("%s:/home/node/.aws:ro", auth.Bedrock.AWSDir))
        agent.Environment["AWS_PROFILE"] = auth.Bedrock.AWSProfile
        agent.Environment["AWS_REGION"] = auth.Bedrock.AWSRegion
        agent.Environment["CLAUDE_CODE_USE_BEDROCK"] = "1"
        // Write merged settings.json to asset dir, mount it
    }

    if auth.Max != nil {
        // Mount credentials file read-only
        agent.Volumes = append(agent.Volumes,
            fmt.Sprintf("%s:/home/node/.claude/.credentials.json:ro",
                auth.Max.CredentialsPath))
        // Write settings.max.json to asset dir, mount it
    }

    for _, key := range auth.APIKeys {
        agent.Environment[key.EnvVar] = key.Value
    }

    for _, local := range auth.Local {
        // Add host.docker.internal to extra_hosts (for Linux)
        agent.ExtraHosts = appendUnique(agent.ExtraHosts,
            []string{"host.docker.internal:host-gateway"})
    }
}
```

**Merge helpers** (synthesis requirement: Compose Merge Semantics):

```go
// mergeComposeFiles applies override onto base following Docker Compose semantics.
// Single-value fields: replace. Map fields: merge by key. List fields: append unique.
func mergeComposeFiles(base, override *ComposeFile)

// mergeService merges an override service into a base service.
func mergeService(base, override *Service) *Service

// appendUnique appends items to a slice, skipping duplicates.
func appendUnique(base, add []string) []string
```

**Asset extraction** (synthesis requirement: Asset Extraction):

```go
// ExtractAssets extracts embedded Docker assets to ~/.cache/muster/docker-assets/
// versioned by content hash. Returns the extraction directory.
// Skips extraction if the hash directory already exists (already extracted).
func ExtractAssets() (string, error)
```

Implementation: compute SHA-256 of the embedded FS content, use `~/.cache/muster/docker-assets/{hash}/` as the target. If directory exists, skip. Otherwise extract all files from the `embed.FS`. This ensures assets refresh when the muster binary updates.

### 5. Container Lifecycle (`internal/docker/container.go`)

**Client struct** wraps both Docker SDK and compose CLI operations:

```go
package docker

import (
    "context"
    dockerclient "github.com/docker/docker/client"
)

// Client wraps Docker SDK for queries and docker compose CLI for orchestration.
type Client struct {
    sdk *dockerclient.Client
}

// NewClient creates a Docker client with API version negotiation.
func NewClient() (*Client, error) {
    sdk, err := dockerclient.NewClientWithOpts(
        dockerclient.FromEnv,
        dockerclient.WithAPIVersionNegotiation(),
    )
    if err != nil {
        return nil, fmt.Errorf("failed to create Docker client: %w", err)
    }
    return &Client{sdk: sdk}, nil
}

func (c *Client) Close() error { return c.sdk.Close() }
```

**Compose CLI operations** -- shell to `docker compose` via `os/exec`:

```go
// ComposeUp starts services from the compose file.
// Streams stdout/stderr to the caller's output.
func (c *Client) ComposeUp(ctx context.Context, composeFile, projectName string) error {
    return c.runCompose(ctx, composeFile, projectName, "up", "-d")
}

// ComposeDown stops and removes all services.
func (c *Client) ComposeDown(ctx context.Context, composeFile, projectName string) error {
    return c.runCompose(ctx, composeFile, projectName, "down")
}

// ComposeExec runs a command inside a service container.
func (c *Client) ComposeExec(ctx context.Context, composeFile, projectName, service string, cmd []string) error {
    args := append([]string{"exec", service}, cmd...)
    return c.runCompose(ctx, composeFile, projectName, args...)
}

// runCompose builds and runs a docker compose command.
func (c *Client) runCompose(ctx context.Context, composeFile, projectName string, args ...string) error {
    cmdArgs := []string{"compose", "-f", composeFile, "-p", projectName}
    cmdArgs = append(cmdArgs, args...)
    cmd := exec.CommandContext(ctx, "docker", cmdArgs...)
    cmd.Stdout = os.Stdout
    cmd.Stderr = os.Stderr
    cmd.Stdin = os.Stdin
    return cmd.Run()
}
```

**SDK query operations** -- used by `muster down` and `muster status`:

```go
// ListContainers finds muster-managed containers, optionally filtered by project and slug.
func (c *Client) ListContainers(ctx context.Context, project, slug string) ([]ContainerInfo, error)

// ContainerInfo is a simplified view of a Docker container for muster's needs.
type ContainerInfo struct {
    ID      string
    Name    string
    Status  string
    Project string
    Slug    string
    Labels  map[string]string
}

// Ping checks if the Docker daemon is reachable.
func (c *Client) Ping(ctx context.Context) error
```

**`ListContainers` implementation** uses Docker SDK label filtering:

```go
func (c *Client) ListContainers(ctx context.Context, project, slug string) ([]ContainerInfo, error) {
    f := filters.NewArgs()
    f.Add("label", LabelManaged+"=true")
    if project != "" {
        f.Add("label", fmt.Sprintf("%s=%s", LabelProject, project))
    }
    if slug != "" {
        f.Add("label", fmt.Sprintf("%s=%s", LabelSlug, slug))
    }

    containers, err := c.sdk.ContainerList(ctx, container.ListOptions{
        Filters: f,
        All:     true,
    })
    if err != nil {
        return nil, fmt.Errorf("failed to list containers: %w", err)
    }

    var result []ContainerInfo
    for _, ctr := range containers {
        result = append(result, ContainerInfo{
            ID:      ctr.ID[:12],
            Name:    strings.TrimPrefix(ctr.Names[0], "/"),
            Status:  ctr.Status,
            Project: ctr.Labels[LabelProject],
            Slug:    ctr.Labels[LabelSlug],
            Labels:  ctr.Labels,
        })
    }
    return result, nil
}
```

### 6. Domain Allowlist Generation

The proxy container receives a merged domain allowlist:

```go
// GenerateAllowlist merges the base embedded allowed-domains.txt with
// provider-specific domains and project-configured domains.
// Returns the path to the written merged file.
func GenerateAllowlist(
    assetDir string,
    auth *AuthRequirements,
    devAgent *config.DevAgentConfig,
) (string, error)
```

**Provider-specific domains** added automatically:

| Provider | Domains |
|----------|---------|
| Bedrock | `.amazonaws.com`, `.aws.amazon.com` |
| Max / Anthropic | `.anthropic.com`, `.claude.ai` |
| OpenRouter | `.openrouter.ai` |
| Local | `host.docker.internal` |

These are appended to the base `allowed-domains.txt` and any user-configured `allowed_domains` from `DevAgentConfig`.

---

## Command Integration

### `cmd/code.go` -- `muster code`

```go
var codeCmd = &cobra.Command{
    Use:   "code",
    Short: "Spawn interactive coding agent with workflow skills",
    RunE:  runCode,
}

func init() {
    rootCmd.AddCommand(codeCmd)
    codeCmd.Flags().Bool("yolo", false, "Run in sandboxed dev-agent container")
    codeCmd.Flags().String("tool", "", "Tool to use (claude|opencode)")
    codeCmd.Flags().Bool("no-plugin", false, "Start bare without workflow skills")
    codeCmd.Flags().String("main-branch", "", "Main branch name for worktree detection")
    codeCmd.Flags().String("current-path", "", "Path to mount as workspace")
}
```

**`runCode` flow when `--yolo` is set**:

1. Load config: `config.LoadAll(userConfigPath, projectDir)`
2. Validate config: `cfg.Validate()`
3. Determine tool from `--tool` flag or `cfg.User.Default.Tool`
4. Collect auth: `docker.CollectAuthRequirements(cfg, allStepNames)`
5. Extract assets: `docker.ExtractAssets()`
6. Detect workspace: determine worktree dir and main repo dir from git
7. Generate compose: `docker.GenerateComposeFile(opts)`
8. Create Docker client: `docker.NewClient()`
9. Start containers: `client.ComposeUp(ctx, composePath, projectName)`
10. Stage prompts to temp dir (Phase 1 work)
11. Exec tool: `client.ComposeExec(ctx, composePath, projectName, "dev-agent", toolCmd)`
12. On exit, leave container running (user runs `muster down` to clean up)

**`runCode` flow without `--yolo`** (local mode, Phase 1 scope):

1. Load config, resolve tool
2. Stage prompts to temp dir
3. Exec tool locally with `--plugin-dir` flag
4. Clean up temp dir

### `cmd/down.go` -- `muster down`

```go
var downCmd = &cobra.Command{
    Use:   "down [slug]",
    Short: "Tear down dev-agent containers",
    RunE:  runDown,
}

func init() {
    rootCmd.AddCommand(downCmd)
    downCmd.Flags().Bool("all", false, "Tear down all containers for current project")
    downCmd.Flags().Bool("orphans", false, "Find and tear down containers with no active work item")
    downCmd.Flags().String("project", "", "Target a different project")
}
```

**`runDown` flow**:

1. Create Docker client: `docker.NewClient()`
2. Determine project name from `--project` flag or current directory
3. If `--orphans`: load `.muster/roadmap.json`, list all project containers, cross-reference slugs, down those not `in_progress`
4. If `--all`: list all project containers, down each
5. If slug argument: down containers matching slug
6. For each target: find compose file at `~/.cache/muster/{project}/docker-compose.yml`, run `ComposeDown`
7. If no compose file found: fall back to `docker rm -f` on containers found by label

---

## Dependency Management

### New Dependencies

| Dependency | Purpose | Justification |
|-----------|---------|---------------|
| `github.com/docker/docker/client` | Container queries (list, inspect) via SDK | Required for label-based discovery (`muster down`, `muster status`). Synthesis requirement: "Use Docker Go SDK exclusively for container queries." |
| `gopkg.in/yaml.v3` | YAML marshal/unmarshal for compose generation | Already an indirect dependency via testify. Promote to direct. Synthesis decision: YAML marshaling over compose-go. |

**Not added** (per synthesis SHOULD NOT):
- `github.com/compose-spec/compose-go/v2` -- too heavy, load-focused not generation-focused
- No custom error type packages -- `fmt.Errorf` with `%w` is sufficient

### Dependency Tree Impact

The Docker SDK (`github.com/docker/docker`) brings in several transitive dependencies (Docker API types, OpenTelemetry, etc.). This is acceptable because:
1. The SDK is the official Go client -- no lighter alternative exists for label-based container queries
2. Binary size increase is offset by the alternative (parsing `docker ps` text output, which is fragile)
3. CGO remains disabled -- the SDK is pure Go

---

## Cross-Platform Considerations

### Path Handling

All path operations use `filepath.Join()` and `filepath.Abs()`. The compose file generator resolves paths to absolute before embedding in YAML since Docker requires absolute paths for bind mounts on all platforms.

```go
// resolveHostPath converts a potentially relative path to absolute,
// suitable for Docker bind mount source.
func resolveHostPath(base, path string) string {
    if filepath.IsAbs(path) {
        return path
    }
    return filepath.Join(base, path)
}
```

### Cache Directory

```go
func cacheDir(project string) string {
    home, _ := os.UserHomeDir()
    return filepath.Join(home, ".cache", "muster", project)
}
```

On macOS/Linux: `~/.cache/muster/{project}/`
On Windows: `C:\Users\{user}\.cache\muster\{project}\`

The cache must be under `$HOME` for Docker Desktop VM compatibility (macOS Colima, Windows WSL2 -- Docker Desktop mounts the home directory by default).

### Docker Compose CLI

Invoke as `exec.Command("docker", "compose", ...)` with argument arrays. Never use `sh -c` wrapping. Docker Compose v2 is a CLI plugin invoked as `docker compose` (not the legacy `docker-compose` binary).

### Linux Extra Hosts

On Linux, `host.docker.internal` does not resolve by default. The compose generator adds `extra_hosts: ["host.docker.internal:host-gateway"]` to the agent service when local providers are detected. On macOS/Windows Docker Desktop, this hostname resolves natively.

---

## Error Handling

Follows project conventions (synthesis requirement: Error Handling):

1. **Wrap with context**: `fmt.Errorf("failed to detect bedrock auth: %w", err)`
2. **Validate early**: All auth/config validation happens before `docker compose up`
3. **Collect all errors**: `CollectAuthRequirements` and `Config.Validate` return `[]error`, not just the first failure
4. **Actionable messages**: Every error tells the user what to do:
   - Missing API key: `"provider anthropic requires ANTHROPIC_API_KEY; set it in your shell or check api_key_env in ~/.config/muster/config.yml"`
   - Missing credentials: `"bedrock auth requires ~/.aws directory; run 'aws configure sso' to set up"`
   - Docker not running: `"Docker daemon is not running; start Docker Desktop or run 'sudo systemctl start docker'"`
5. **Leave state for debugging**: On container failure, do not clean up. `muster down` is the explicit cleanup.

---

## Testing Strategy

### Unit Tests (no Docker required)

**Config resolution** (`internal/config/resolve_test.go`):
- Table-driven tests covering: step-specific override, fallback to defaults, fallback to user defaults, tier resolution (`muster-deep` -> `opus`), literal model passthrough, missing tool error, missing provider error, missing tier error
- Fixtures in `testdata/` with various config files

**Compose generation** (`internal/docker/compose_test.go`):
- Table-driven tests: given `ComposeOptions`, assert generated YAML contains expected services, labels, volumes, environment variables
- Golden file tests: compare full generated YAML against checked-in `.golden` files with `-update` flag
- No Docker daemon needed -- tests only verify YAML structure

**Auth detection** (`internal/docker/auth_test.go`):
- Table-driven tests using `t.TempDir()` for mock filesystem
- Mock `~/.claude/settings.json` with/without Bedrock flag
- Mock `~/.claude/.credentials.json` presence/absence
- Use `t.Setenv()` for environment variable mocking
- Test localhost URL rewriting: `localhost`, `127.0.0.1`, `::1` all rewrite to `host.docker.internal`
- Test reachability validation with unreachable URLs

**Merge logic** (`internal/config/merge_test.go`):
- Field-level override: local scalar wins over base scalar
- List replacement: local list replaces base list entirely
- Nested merge: local map merges into base map recursively
- Missing local file: base used unchanged

### Integration Tests (Docker required, short-guarded)

**Container lifecycle** (`internal/docker/container_test.go`):
```go
func TestComposeUp_Integration(t *testing.T) {
    if testing.Short() {
        t.Skip("skipping integration test requiring Docker")
    }
    // ...
}
```

- Start a minimal compose file, verify containers have labels, exec a command, tear down
- Verify `ListContainers` returns expected results with label filters
- Verify `ComposeDown` removes containers

**Compose validation** (in compose tests):
- Run `docker compose config` on generated files to validate syntax
- Only in integration tests, not unit tests

---

## Sequence Diagrams

### `muster code --yolo` Startup

```
User                  cmd/code.go          config/           docker/
 |                        |                   |                  |
 |-- muster code --yolo ->|                   |                  |
 |                        |-- LoadAll() ----->|                  |
 |                        |<-- Config --------|                  |
 |                        |                   |                  |
 |                        |-- CollectAuthRequirements() -------->|
 |                        |   (resolves all steps,              |
 |                        |    detects providers)               |
 |                        |<-- AuthRequirements ----------------|
 |                        |                                     |
 |                        |-- ExtractAssets() ----------------->|
 |                        |<-- assetDir ------------------------|
 |                        |                                     |
 |                        |-- GenerateComposeFile() ----------->|
 |                        |   (base + auth + workspace          |
 |                        |    + devagent + labels + validate)  |
 |                        |<-- composePath ---------------------|
 |                        |                                     |
 |                        |-- NewClient() --------------------->|
 |                        |<-- client --------------------------|
 |                        |                                     |
 |                        |-- client.ComposeUp() -------------->|
 |                        |   docker compose -f ... up -d       |
 |                        |<-- ok ------------------------------|
 |                        |                                     |
 |                        |-- client.ComposeExec() ------------>|
 |                        |   docker compose exec dev-agent     |
 |                        |   claude --plugin-dir /skills       |
 |                        |<-- (interactive session) -----------|
 |                        |                                     |
 |<-- session ends -------|                                     |
 |                        |   (container left running)          |
```

### `muster down` Teardown

```
User                  cmd/down.go           docker/
 |                        |                    |
 |-- muster down slug --->|                    |
 |                        |-- NewClient() ---->|
 |                        |<-- client ---------|
 |                        |                    |
 |                        |-- ListContainers   |
 |                        |   (project, slug)->|
 |                        |<-- containers -----|
 |                        |                    |
 |                        |-- ComposeDown() -->|
 |                        |   docker compose   |
 |                        |   -f ... down      |
 |                        |<-- ok -------------|
 |<-- done ---------------|                    |
```

---

## Key Design Decisions

### Single Merged Compose File (not multiple -f flags)

Generate one `docker-compose.yml` at `~/.cache/muster/{project}/` rather than passing multiple `-f` flags. This gives us:
- Full control over merge semantics (synthesis decision)
- Easier debugging (`cat` the file to see exactly what will run)
- Single source of truth for container state
- Simpler compose CLI invocations

### Docker SDK for Queries Only

The SDK is used exclusively for `ContainerList` with label filters and `ContainerInspect`. All orchestration (`up`, `down`, `exec`, `start`, `stop`) goes through `docker compose` CLI. This avoids reimplementing orchestration logic (service dependencies, health checks, network creation) that compose already handles.

### Env Vars Take Precedence Over Credential Files

When both an environment variable and a credential file exist for the same provider, the environment variable wins. This matches AWS CLI conventions and represents explicit user intent (synthesis resolved question).

### Persistent Containers Across Pipeline Steps

Containers stay running across all pipeline steps. Different steps exec different tools (claude vs opencode) without restarting the container. `muster down` is the explicit cleanup command. On failure, containers are left running for debugging.

### Proxy-Only Network Isolation

Network isolation uses a Squid proxy with domain allowlisting. No iptables/nftables firewall layer. This provides a single consistent security model across all platforms (Linux, macOS Docker Desktop, Windows Docker Desktop).

---

## File-by-File Deliverables

| File | Lines (est.) | Purpose |
|------|-------------|---------|
| `internal/config/types.go` | ~80 | All config struct definitions |
| `internal/config/config.go` | ~30 | Top-level `LoadAll` |
| `internal/config/user.go` | ~40 | User config loader |
| `internal/config/project.go` | ~50 | Project config loader with .local.yml merge |
| `internal/config/devagent.go` | ~50 | Dev-agent config loader with .local.yml merge |
| `internal/config/merge.go` | ~60 | Generic deep merge for YAML maps |
| `internal/config/resolve.go` | ~80 | Step resolution with fallback chain + validation |
| `internal/docker/labels.go` | ~20 | Label constants and builder |
| `internal/docker/auth.go` | ~200 | Provider detection (Bedrock, Max, API key, Local) |
| `internal/docker/compose.go` | ~250 | Compose generation, merge helpers, asset extraction |
| `internal/docker/container.go` | ~150 | SDK client, compose CLI wrapper, container queries |
| `cmd/code.go` | ~120 | `muster code` with `--yolo` flow |
| `cmd/down.go` | ~100 | `muster down` with label-based teardown |
| **Tests** | | |
| `internal/config/resolve_test.go` | ~150 | Table-driven step resolution |
| `internal/config/user_test.go` | ~80 | User config parsing |
| `internal/config/project_test.go` | ~100 | Deep merge tests |
| `internal/config/devagent_test.go` | ~80 | Dev-agent config parsing |
| `internal/docker/auth_test.go` | ~200 | Auth detection with mock FS/env |
| `internal/docker/compose_test.go` | ~200 | Compose generation + golden files |
| `internal/docker/container_test.go` | ~100 | Integration tests (short-guarded) |

**Total**: ~2,140 lines of new code across 21 files.

---

## References

- Synthesis: `.plan/docker-orchestration/synthesis/synthesis.md`
- Research: `.plan/docker-orchestration/research/{config-system,docker-sdk,compose-generation,project-structure,devcontainer-patterns}.md`
- Design: `docs/design.md` -- Configuration (lines 84-192), Architecture (lines 194-237), Docker Integration (lines 339-359)
- Existing code: `cmd/root.go`, `cmd/version.go`, `internal/ui/output.go`, `internal/docker/embed.go`, `internal/testutil/helpers.go`
