# Docker SDK for Go

*Researched: 2026-03-18*
*Scope: Docker SDK capabilities, compose CLI integration, container lifecycle APIs, label filtering, authentication handling, and when to use SDK vs. docker compose CLI*

---

## Key Findings

**Primary Architecture Decision**: The design document correctly specifies using `docker compose` CLI for core orchestration (start/stop/exec) while leveraging the Docker SDK for container queries and label filtering. This hybrid approach is the optimal pattern.

**Why this hybrid works**:
- Docker Compose CLI handles multi-container orchestration, service dependencies, health checks, and YAML-based declarative configuration
- Docker SDK provides programmatic querying, filtering by labels, and fine-grained inspection that the compose CLI doesn't expose well
- Combining both gives you declarative orchestration (compose) with dynamic runtime queries (SDK)

**Key SDK Capabilities for Muster**:
1. **Container listing with label filters** — `ContainerList()` with filters for `muster.managed=true`, `muster.project={name}`, `muster.slug={slug}`
2. **Container inspection** — get detailed container state, labels, and configuration
3. **Image pull with authentication** — programmatic image pulls with registry auth
4. **Volume inspection** — query volume mounts and configurations

**Authentication is straightforward**: The SDK automatically reads from `~/.docker/config.json` when pulling images. No special auth setup needed unless using custom registries or credential helpers.

**Version negotiation is critical**: Always use `client.WithAPIVersionNegotiation()` to avoid version mismatches between SDK and Docker daemon.

---

## Detailed Analysis

### Docker SDK Overview

**Package**: `github.com/docker/docker/client`
**Current Version**: v28.5.2+incompatible
**License**: Apache-2.0
**Go Version**: Requires a currently supported Go version (1.21+)

The Docker SDK for Go is the official client library for the Docker Engine API. It provides programmatic access to all Docker operations: containers, images, networks, volumes, and swarm services.

### Installation

```bash
go get github.com/docker/docker/client
```

### Client Initialization

```go
package main

import (
    "context"
    "github.com/docker/docker/client"
)

func main() {
    // Best practice: use FromEnv + API version negotiation
    cli, err := client.NewClientWithOpts(
        client.FromEnv,
        client.WithAPIVersionNegotiation(),
    )
    if err != nil {
        panic(err)
    }
    defer cli.Close()

    // Client is ready to use
}
```

**Environment Variables**:
- `DOCKER_HOST` — Docker daemon host (e.g., `unix:///var/run/docker.sock`)
- `DOCKER_API_VERSION` — API version override (e.g., `1.41`)
- `DOCKER_CERT_PATH` — Directory with TLS certificates
- `DOCKER_TLS_VERIFY` — Enable TLS verification

**Custom Configuration**:
```go
cli, err := client.NewClientWithOpts(
    client.WithHost("unix:///var/run/docker.sock"),
    client.WithVersion("1.41"),
    client.WithTimeout(30 * time.Second),
    client.WithAPIVersionNegotiation(),  // Highly recommended
)
```

### Container Lifecycle Operations

#### List Containers with Label Filters

**This is the primary SDK operation Muster needs**:

```go
import (
    "github.com/docker/docker/api/types/container"
    "github.com/docker/docker/api/types/filters"
)

// Query containers by labels
filters := filters.NewArgs()
filters.Add("label", "muster.managed=true")
filters.Add("label", "muster.project=my-api")
filters.Add("label", "muster.slug=add-retry-logic")

containers, err := cli.ContainerList(ctx, container.ListOptions{
    Filters: filters,
    All:     true,  // Include stopped containers
})

for _, ctr := range containers {
    fmt.Printf("Container: %s\n", ctr.ID)
    fmt.Printf("  Image: %s\n", ctr.Image)
    fmt.Printf("  Status: %s\n", ctr.Status)
    fmt.Printf("  Labels: %v\n", ctr.Labels)
}
```

**Filter Syntax**:
- `filters.Add("label", "key")` — matches containers with the label key (any value)
- `filters.Add("label", "key=value")` — matches containers where label equals specific value
- Multiple `Add()` calls create AND conditions

**Other useful filters**:
- `filters.Add("status", "running")` — only running containers
- `filters.Add("status", "exited")` — only stopped containers
- `filters.Add("name", "prefix")` — filter by container name

#### Container Inspection

```go
import "github.com/docker/docker/api/types"

inspect, err := cli.ContainerInspect(ctx, containerID)
if err != nil {
    // Handle error
}

// Access detailed info
fmt.Printf("State: %s\n", inspect.State.Status)
fmt.Printf("Started at: %s\n", inspect.State.StartedAt)
fmt.Printf("Mounts: %v\n", inspect.Mounts)
fmt.Printf("Labels: %v\n", inspect.Config.Labels)
```

#### Execute Commands in Containers

**Useful for debugging or health checks**:

```go
import "github.com/docker/docker/api/types/container"

// Create exec session
execConfig := container.ExecOptions{
    Cmd:          []string{"sh", "-c", "echo hello"},
    AttachStdout: true,
    AttachStderr: true,
}

execResp, err := cli.ContainerExecCreate(ctx, containerID, execConfig)
if err != nil {
    // Handle error
}

// Start exec
err = cli.ContainerExecStart(ctx, execResp.ID, container.ExecStartOptions{})

// Inspect result
execInspect, err := cli.ContainerExecInspect(ctx, execResp.ID)
fmt.Printf("Exit code: %d\n", execInspect.ExitCode)
```

#### Start and Stop Containers

**Note**: For Muster, prefer `docker compose start/stop` CLI commands instead. The SDK is primarily for queries.

```go
// Start a container
err := cli.ContainerStart(ctx, containerID, container.StartOptions{})

// Stop with grace period
timeout := 10 * time.Second
err = cli.ContainerStop(ctx, containerID, container.StopOptions{
    Timeout: &timeout,
})

// Force kill
err = cli.ContainerKill(ctx, containerID, "SIGKILL")

// Remove
err = cli.ContainerRemove(ctx, containerID, container.RemoveOptions{
    Force:         true,
    RemoveVolumes: true,
})
```

### Image Operations

#### Pull Images with Authentication

```go
import (
    "encoding/base64"
    "encoding/json"
    "github.com/docker/docker/api/types/image"
    "github.com/docker/docker/api/types/registry"
)

// Simple pull (uses ~/.docker/config.json automatically)
reader, err := cli.ImagePull(ctx, "alpine:latest", image.PullOptions{})
if err != nil {
    // Handle error
}
defer reader.Close()

// Read pull output
io.Copy(os.Stdout, reader)

// Pull with explicit auth
authConfig := registry.AuthConfig{
    Username: "user",
    Password: "pass",
    ServerAddress: "docker.io",
}
encodedAuth, err := encodeAuthConfig(authConfig)
if err != nil {
    // Handle error
}

reader, err = cli.ImagePull(ctx, "alpine:latest", image.PullOptions{
    RegistryAuth: encodedAuth,
})

// Helper function
func encodeAuthConfig(authConfig registry.AuthConfig) (string, error) {
    buf, err := json.Marshal(authConfig)
    if err != nil {
        return "", err
    }
    return base64.URLEncoding.EncodeToString(buf), nil
}
```

**Authentication Sources** (in priority order):
1. Explicitly passed `RegistryAuth` in `PullOptions`
2. `~/.docker/config.json` (automatically read by SDK when using `client.FromEnv`)
3. Credential helpers configured in `config.json` (`credsStore`, `credHelpers`)

**For Muster**: No special auth handling needed unless pulling from private registries. The SDK automatically uses Docker CLI's saved credentials.

#### List and Inspect Images

```go
images, err := cli.ImageList(ctx, image.ListOptions{})

for _, img := range images {
    fmt.Printf("Image ID: %s\n", img.ID)
    fmt.Printf("  Tags: %v\n", img.RepoTags)
    fmt.Printf("  Size: %d\n", img.Size)
}

// Inspect specific image
inspect, _, err := cli.ImageInspectWithRaw(ctx, imageID)
fmt.Printf("Architecture: %s\n", inspect.Architecture)
fmt.Printf("Created: %s\n", inspect.Created)
```

### Volume Operations

```go
import "github.com/docker/docker/api/types/volume"

// List volumes
volumes, err := cli.VolumeList(ctx, volume.ListOptions{})

for _, vol := range volumes.Volumes {
    fmt.Printf("Volume: %s\n", vol.Name)
    fmt.Printf("  Driver: %s\n", vol.Driver)
    fmt.Printf("  Mountpoint: %s\n", vol.Mountpoint)
}

// Inspect volume
vol, err := cli.VolumeInspect(ctx, volumeName)
```

### API Version Negotiation

**Critical for Muster**:

```go
cli, err := client.NewClientWithOpts(
    client.FromEnv,
    client.WithAPIVersionNegotiation(),  // ALWAYS include this
)
```

**Why this matters**:
- Docker daemon version may differ from SDK version
- Without negotiation, SDK may request endpoints the daemon doesn't support
- `WithAPIVersionNegotiation()` automatically selects the highest common version
- Prevents runtime errors from version mismatches

**Manual version override** (not recommended):
```go
cli, err := client.NewClientWithOpts(
    client.WithVersion("1.41"),  // Force specific API version
)
```

**Environment variable override**:
```bash
export DOCKER_API_VERSION=1.41
```

**Compatibility guarantees**:
- Docker API is backward compatible
- Newer daemon supports all older API versions
- Client can be newer than daemon (but won't see new features)
- Breaking changes are rare and documented prominently

### Key Package Imports

```go
import (
    "github.com/docker/docker/client"                    // Main client
    "github.com/docker/docker/api/types"                 // General types
    "github.com/docker/docker/api/types/container"       // Container types
    "github.com/docker/docker/api/types/image"           // Image types
    "github.com/docker/docker/api/types/volume"          // Volume types
    "github.com/docker/docker/api/types/network"         // Network types
    "github.com/docker/docker/api/types/filters"         // Filter utilities
    "github.com/docker/docker/api/types/registry"        // Registry auth
    "github.com/docker/docker/pkg/stdcopy"               // Stream demuxing
)
```

### Docker Compose CLI vs SDK

**Docker Compose CLI** is optimal for:
- Multi-container orchestration with service dependencies
- Declarative infrastructure as YAML configuration
- Service lifecycle management (up, down, restart)
- Health checks and service ordering
- Network and volume creation
- Single-command deployment of entire application stacks

**Docker SDK** is optimal for:
- Programmatic container queries (listing, filtering, inspection)
- Fine-grained container operations with error handling
- Image management (pull, build, push with auth)
- Dynamic runtime decisions based on container state
- Integration with other Go code and control flows

**For Muster's use case**:
- **Use compose CLI** for: `docker compose up`, `docker compose down`, `docker compose start`, `docker compose stop`, `docker compose exec`
- **Use SDK** for: querying containers by labels (`muster down --orphans`), inspecting container state (`muster status`), verifying images exist before starting

**Why compose CLI for orchestration**:
1. Handles multi-service dependencies (proxy + agent)
2. Health checks and service ordering are built-in
3. Compose file is declarative, version-controlled configuration
4. Automatic network and volume management
5. No need to reimplement orchestration logic in Go

**Why SDK for queries**:
1. Label filtering is natural: `filters.Add("label", "muster.managed=true")`
2. Programmatic inspection returns structured data
3. Easy integration with Go's control flow (if/else, loops)
4. Type-safe access to container properties

### Container Labels in Compose Files

**Setting labels in docker-compose.yml**:

```yaml
services:
  agent:
    image: muster/dev-agent:latest
    labels:
      muster.managed: "true"
      muster.project: "my-api"
      muster.slug: "add-retry-logic"
      environment: "dev"
```

**Important distinction**:
- Labels under `deploy:` are **service-level metadata** (for orchestration platforms)
- Labels under the service (not nested in `deploy`) are **container-level metadata**
- **Muster should use container-level labels** (not under `deploy`)

**Querying labeled containers**:

```bash
# CLI approach
docker ps --filter "label=muster.managed=true" --filter "label=muster.project=my-api"

# Go SDK approach (preferred for Muster)
filters := filters.NewArgs()
filters.Add("label", "muster.managed=true")
filters.Add("label", "muster.project=my-api")
containers, err := cli.ContainerList(ctx, container.ListOptions{Filters: filters})
```

### Authentication Handling

**Default behavior**: Docker SDK automatically uses credentials from `~/.docker/config.json`:
- Linux: `$HOME/.docker/config.json`
- macOS: `~/.docker/config.json` (or native keychain via credential helper)
- Windows: `%USERPROFILE%/.docker/config.json` (or Windows Credential Manager)

**Credential storage options**:

1. **Direct in config.json** (less secure):
```json
{
  "auths": {
    "https://index.docker.io/v1/": {
      "auth": "base64_encoded_username:password"
    }
  }
}
```

2. **Credential store** (recommended):
```json
{
  "credsStore": "osxkeychain"
}
```

3. **Per-registry credential helpers**:
```json
{
  "credHelpers": {
    "myregistry.example.com": "secretservice"
  }
}
```

**For Muster**:
- No special auth handling required for image pulls/pushes
- SDK automatically reads Docker CLI's credentials
- If user can `docker pull` from CLI, SDK will work identically
- Only need explicit auth for programmatic login (e.g., CI environments)

**Programmatic auth** (only if needed):

```go
authConfig := registry.AuthConfig{
    Username: "user",
    Password: os.Getenv("REGISTRY_PASSWORD"),
    ServerAddress: "registry.example.com",
}

// Option 1: Login (saves to config.json)
_, err := cli.RegistryLogin(ctx, authConfig)

// Option 2: One-time auth for pull
encodedAuth := base64.URLEncoding.EncodeToString([]byte(
    fmt.Sprintf(`{"username":"%s","password":"%s"}`,
        authConfig.Username, authConfig.Password),
))
reader, err := cli.ImagePull(ctx, imageName, image.PullOptions{
    RegistryAuth: encodedAuth,
})
```

---

## Recommendations

### When to Use Docker SDK

**Use the SDK for**:
1. **Container queries** — `muster status`, `muster down --orphans`
   - Filter containers by `muster.managed=true` and other labels
   - Inspect container state, uptime, and configuration
   - Cross-reference running containers with `.muster/roadmap.json`

2. **Pre-flight checks** — `muster doctor`
   - Verify Docker daemon is running (`cli.Ping(ctx)`)
   - Check if images exist before starting containers
   - Validate volume mounts and network configurations

3. **Image verification** — before starting containers
   - Check if dev-agent image exists locally
   - Optionally pull images programmatically with progress reporting

**Code pattern for Muster**:

```go
package docker

import (
    "context"
    "github.com/docker/docker/client"
    "github.com/docker/docker/api/types/container"
    "github.com/docker/docker/api/types/filters"
)

type Client struct {
    cli *client.Client
}

func NewClient() (*Client, error) {
    cli, err := client.NewClientWithOpts(
        client.FromEnv,
        client.WithAPIVersionNegotiation(),
    )
    if err != nil {
        return nil, err
    }
    return &Client{cli: cli}, nil
}

// ListMusterContainers finds all containers managed by muster
func (c *Client) ListMusterContainers(ctx context.Context, project, slug string) ([]container.Container, error) {
    filters := filters.NewArgs()
    filters.Add("label", "muster.managed=true")

    if project != "" {
        filters.Add("label", fmt.Sprintf("muster.project=%s", project))
    }
    if slug != "" {
        filters.Add("label", fmt.Sprintf("muster.slug=%s", slug))
    }

    return c.cli.ContainerList(ctx, container.ListOptions{
        Filters: filters,
        All:     true,
    })
}

// IsRunning checks if a container is currently running
func (c *Client) IsRunning(ctx context.Context, containerID string) (bool, error) {
    inspect, err := c.cli.ContainerInspect(ctx, containerID)
    if err != nil {
        return false, err
    }
    return inspect.State.Running, nil
}

// Close releases SDK resources
func (c *Client) Close() error {
    return c.cli.Close()
}
```

### When to Use Docker Compose CLI

**Use `docker compose` CLI for**:
1. **Container orchestration** — all lifecycle operations
   - `docker compose up -d` — start services with dependencies
   - `docker compose down` — stop and remove containers, networks
   - `docker compose start/stop` — manage running services
   - `docker compose exec agent claude --prompt "..."` — run commands in containers

2. **Service management**
   - Health checks and service ordering (proxy waits for agent)
   - Network creation and isolation
   - Volume mounting and management
   - Environment variable injection

3. **Configuration-driven operations**
   - Generate compose files programmatically in Go
   - Use `os/exec` to shell to `docker compose` CLI
   - Let compose handle all the orchestration complexity

**Code pattern for Muster**:

```go
package docker

import (
    "os/exec"
    "context"
)

// ComposeUp starts services defined in compose file
func (c *Client) ComposeUp(ctx context.Context, composeFile string) error {
    cmd := exec.CommandContext(ctx, "docker", "compose",
        "-f", composeFile,
        "up", "-d",
    )
    cmd.Stdout = os.Stdout
    cmd.Stderr = os.Stderr
    return cmd.Run()
}

// ComposeExec runs a command in a service
func (c *Client) ComposeExec(ctx context.Context, composeFile, service string, command []string) error {
    args := []string{"compose", "-f", composeFile, "exec", service}
    args = append(args, command...)

    cmd := exec.CommandContext(ctx, "docker", args...)
    cmd.Stdout = os.Stdout
    cmd.Stderr = os.Stderr
    return cmd.Run()
}

// ComposeDown stops and removes everything
func (c *Client) ComposeDown(ctx context.Context, composeFile string) error {
    cmd := exec.CommandContext(ctx, "docker", "compose",
        "-f", composeFile,
        "down", "--volumes",
    )
    cmd.Stdout = os.Stdout
    cmd.Stderr = os.Stderr
    return cmd.Run()
}
```

### Package Structure

**Recommended organization** for `internal/docker/`:

```
internal/docker/
├── client.go          // SDK client wrapper with helper methods
├── compose.go         // Compose file generation and CLI execution
├── auth.go            // Provider auth detection (Bedrock, Max, etc.)
├── labels.go          // Label constants and builder functions
└── container_test.go  // Tests for container queries
```

**Key imports to use**:

```go
// In client.go (SDK operations)
import (
    "github.com/docker/docker/client"
    "github.com/docker/docker/api/types/container"
    "github.com/docker/docker/api/types/filters"
)

// In compose.go (CLI operations)
import (
    "os/exec"
    "gopkg.in/yaml.v3"
)
```

### Error Handling

**Common SDK errors**:

```go
import (
    "github.com/docker/docker/client"
    "github.com/docker/docker/errdefs"
)

// Check for connection failures
if client.IsErrConnectionFailed(err) {
    return fmt.Errorf("Docker daemon is not running: %w", err)
}

// Check for not found errors
if errdefs.IsNotFound(err) {
    return fmt.Errorf("container not found: %w", err)
}

// Check if error is due to unauthorized
if errdefs.IsUnauthorized(err) {
    return fmt.Errorf("authentication failed: %w", err)
}
```

### Best Practices for Muster

1. **Initialize SDK client once** — create in `docker.NewClient()` and reuse
2. **Always use API version negotiation** — `client.WithAPIVersionNegotiation()`
3. **Use label filters for all container queries** — never rely on container names
4. **Prefer compose CLI for orchestration** — don't reimplement container lifecycle in SDK
5. **Use SDK for queries only** — ContainerList, ContainerInspect, ImageList
6. **Let Docker handle auth** — rely on `~/.docker/config.json` for credentials
7. **Shell to compose CLI** — use `os/exec` for all compose operations
8. **Generate compose files programmatically** — build YAML in Go, write to temp file, pass to CLI

---

## Open Questions

1. **Compose file merging**: Should Muster use compose's built-in override mechanism (`docker-compose.yml` + `docker-compose.override.yml`) or generate a single merged file? The design doc mentions "single merged compose" — this approach is simpler and avoids compose's override semantics.

2. **Label propagation**: Do labels set on a service in docker-compose.yml automatically propagate to containers? **YES** — labels at the service level (not under `deploy:`) are applied to containers. The design is correct.

3. **Credential helper compatibility**: If a user has `"credsStore": "osxkeychain"` in config.json, will the SDK automatically use it? **YES** — the SDK respects credential helpers automatically.

4. **Stale container detection**: For `muster down --orphans`, how should we detect containers whose slug is no longer in `.muster/roadmap.json`? **Answer**: Query all containers with `muster.managed=true`, extract their `muster.slug` labels, cross-reference with roadmap items marked `in_progress`.

5. **Container name collisions**: If multiple containers have the same `muster.slug` label (e.g., from failed cleanup), should `muster down` remove all of them? **YES** — remove all matching containers. This is cleanup by definition.

---

## References

### Official Documentation
- Docker SDK for Go: https://pkg.go.dev/github.com/docker/docker/client
- Docker Engine API: https://docs.docker.com/engine/api/
- Docker Compose CLI: https://docs.docker.com/compose/
- Docker API SDKs: https://docs.docker.com/engine/api/sdk/

### Code Examples
- Docker SDK Examples: https://github.com/docker/docker/tree/master/client (in the main repo)
- Container filtering: https://docs.docker.com/reference/cli/docker/container/ls/

### Related
- Go API client documentation: https://pkg.go.dev/github.com/docker/docker
- API versioning strategy: https://docs.docker.com/engine/api/
- Credential storage: https://docs.docker.com/reference/cli/docker/login/
