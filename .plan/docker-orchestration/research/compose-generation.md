# Compose Generation

*Researched: 2026-03-18*
*Scope: Programmatic compose file generation in Go - evaluate compose-go library vs. template-based approach vs. YAML marshaling*

---

## Key Findings

Three viable approaches exist for programmatic compose file generation in Go:

1. **compose-go library**: Official Compose Specification parser/loader with type-safe structures
2. **YAML marshaling with gopkg.in/yaml.v3**: Direct YAML manipulation using Go structs
3. **text/template with YAML**: Template-based generation with embedded base file

**Recommendation**: Use **YAML marshaling (gopkg.in/yaml.v3)** for this project. It offers the best balance of simplicity, control, and maintainability without adding heavy dependencies.

---

## Detailed Analysis

### Approach 1: compose-go Library

**Library**: `github.com/compose-spec/compose-go/v2`
**Current Version**: v2.10.1 (stable, Apache-2.0 licensed)
**Used By**: Docker Compose, containerd/nerdctl, Kubernetes Kompose, Tilt.dev

#### How It Works

compose-go provides type-safe Go structures matching the Compose Specification. The library excels at *parsing and loading* existing compose files with built-in validation, normalization, and multi-file merging.

```go
import (
    "github.com/compose-spec/compose-go/v2/cli"
    "github.com/compose-spec/compose-go/v2/loader"
    "github.com/compose-spec/compose-go/v2/types"
)

// Load compose file(s)
options, err := cli.NewProjectOptions(
    []string{"docker-compose.yml", "override.yml"},
    cli.WithOsEnv,
    cli.WithDotEnv,
)
project, err := options.LoadProject(ctx)

// Serialize back to YAML
yamlBytes, err := project.MarshalYAML()
```

#### Multi-File Merging

compose-go handles the Docker Compose merge semantics automatically:

- **Single-value fields** (image, command, mem_limit): replaced by later files
- **Multi-value fields** (ports, volumes, environment): merged with later files overriding by key
- **Path resolution**: All paths relative to base file
- **Profiles**: Supports filtering services by profile with `WithProfiles()`

The `LoadWithContext()` function accepts multiple file paths and returns a fully merged `types.Project`.

#### Programmatic Manipulation

After loading, you can modify the project:

```go
// Access/modify services
service := project.GetService("dev-agent")
service.Environment["NEW_VAR"] = "value"
service.Labels["muster.managed"] = "true"

// Add volumes
project.Volumes["workspace"] = types.VolumeConfig{
    Name: "workspace",
    Driver: "local",
}

// Write back to YAML
yamlBytes, err := project.MarshalYAML()
```

#### Pros

- **Type-safe**: Comprehensive type definitions with validation
- **Spec-compliant**: Implements full Compose Specification
- **Merge semantics**: Handles multi-file merge rules automatically
- **Battle-tested**: Used by Docker Compose itself
- **Feature-rich**: Profiles, interpolation, normalization, extends/include support

#### Cons

- **Heavy dependency**: Pulls in extensive dependency tree (Docker types, etc.)
- **Load-focused**: Optimized for parsing existing files, not building from scratch
- **Complexity**: API has learning curve; overkill for simple generation
- **Version coupling**: Breaking changes between v1 and v2; must track Compose spec evolution
- **Marshal limitations**: `MarshalYAML()` exists but documentation is sparse on controlling output format

#### Verdict for This Project

**Not recommended**. While compose-go is excellent for validating and loading user-provided compose files, this project needs to:

1. Start with an embedded base compose file
2. Layer simple overrides (auth mounts, labels, volumes, env vars)
3. Write a single merged output

compose-go adds significant complexity and dependencies for this straightforward use case. The merge semantics can be implemented simply with YAML manipulation.

---

### Approach 2: YAML Marshaling with gopkg.in/yaml.v3

**Library**: `gopkg.in/yaml.v3` (already in project dependencies)
**Version**: v3.0.1 (stable, MIT/Apache-2.0 licensed)
**Status**: Pure Go, no external dependencies beyond stdlib

#### How It Works

Define Go structs matching compose file structure, marshal/unmarshal as YAML:

```go
import "gopkg.in/yaml.v3"

type ComposeFile struct {
    Version  string                       `yaml:"version,omitempty"`
    Services map[string]ServiceConfig     `yaml:"services"`
    Volumes  map[string]VolumeConfig      `yaml:"volumes,omitempty"`
    Networks map[string]NetworkConfig     `yaml:"networks,omitempty"`
}

type ServiceConfig struct {
    Image       string              `yaml:"image,omitempty"`
    Build       *BuildConfig        `yaml:"build,omitempty"`
    Ports       []string            `yaml:"ports,omitempty"`
    Volumes     []string            `yaml:"volumes,omitempty"`
    Environment map[string]string   `yaml:"environment,omitempty"`
    Labels      map[string]string   `yaml:"labels,omitempty"`
    Networks    []string            `yaml:"networks,omitempty"`
    DependsOn   []string            `yaml:"depends_on,omitempty"`
    // ... other fields as needed
}

// Load base file
var base ComposeFile
err := yaml.Unmarshal(baseYAML, &base)

// Apply overrides
base.Services["dev-agent"].Labels["muster.managed"] = "true"
base.Services["dev-agent"].Environment["AUTH_TOKEN"] = token
base.Volumes["workspace"] = VolumeConfig{Driver: "local"}

// Write merged output
output, err := yaml.Marshal(&base)
```

#### Override Merging Strategy

Implement Docker Compose merge semantics manually:

```go
func mergeServices(base, override ServiceConfig) ServiceConfig {
    // Single-value fields: replace
    if override.Image != "" {
        base.Image = override.Image
    }

    // Map fields: merge by key
    for k, v := range override.Environment {
        base.Environment[k] = v
    }
    for k, v := range override.Labels {
        base.Labels[k] = v
    }

    // List fields: append unique
    base.Ports = append(base.Ports, override.Ports...)
    base.Volumes = append(base.Volumes, override.Volumes...)

    return base
}
```

#### Struct Tags and Control

The `yaml` tag provides precise control:

```yaml
`yaml:"field_name,omitempty"`  // Skip if zero value
`yaml:"field_name,flow"`       // Use flow style [1, 2, 3]
`yaml:",inline"`               // Merge fields into parent
`yaml:"-"`                     // Ignore field
```

#### Pros

- **Already available**: gopkg.in/yaml.v3 is in go.mod (testify dependency)
- **Lightweight**: Pure Go, minimal dependencies
- **Full control**: Explicit merge logic, no magic
- **Simple API**: Marshal/Unmarshal are straightforward
- **Flexible**: Easy to add custom fields or behaviors
- **Stable**: YAML v3 is mature and widely used

#### Cons

- **Manual struct definitions**: Must define compose file structure yourself
- **No validation**: Won't catch invalid compose file errors (but can add if needed)
- **Merge logic**: Must implement merge semantics manually
- **Type safety**: No enforcement of Compose Specification constraints

#### Verdict for This Project

**Recommended**. This is the sweet spot for muster's requirements:

1. Define minimal structs covering only the fields actually used
2. Load embedded base compose file once at startup
3. Apply overrides programmatically (auth, labels, volumes, env)
4. Marshal to ~/.cache/muster/{project}/docker-compose.yml
5. No heavy dependencies, easy to test, full control

The manual merge logic is simple for this use case - mostly map and list concatenation.

---

### Approach 3: text/template with YAML

**Library**: `text/template` (Go stdlib)
**Pattern**: Embed base compose YAML, render with template syntax

#### How It Works

Embed a compose file template with Go template syntax:

```yaml
# docker-compose.yml.tmpl
services:
  dev-agent:
    image: dev-agent:latest
    environment:
      {{- range $key, $val := .Environment }}
      {{ $key }}: {{ $val }}
      {{- end }}
    volumes:
      {{- range .Volumes }}
      - {{ . }}
      {{- end }}
    labels:
      {{- range $key, $val := .Labels }}
      {{ $key }}: {{ $val }}
      {{- end }}
```

Execute template:

```go
import "text/template"

type TemplateData struct {
    Environment map[string]string
    Volumes     []string
    Labels      map[string]string
}

tmpl := template.Must(template.ParseFiles("docker-compose.yml.tmpl"))
var buf bytes.Buffer
err := tmpl.Execute(&buf, data)
```

#### Override Handling

Overrides are handled by merging data before template execution:

```go
// Base data from embedded config
baseData := TemplateData{
    Environment: map[string]string{"BASE_VAR": "value"},
    Volumes:     []string{"/base:/container"},
}

// Apply overrides
for k, v := range userEnv {
    baseData.Environment[k] = v
}
baseData.Volumes = append(baseData.Volumes, userVolumes...)

// Execute template
tmpl.Execute(&buf, baseData)
```

#### Pros

- **Stdlib only**: No external dependencies
- **Familiar**: Many Go developers know text/template
- **Human-readable**: Template file is close to final YAML
- **Flexible**: Easy to add conditional blocks

#### Cons

- **Template complexity**: YAML whitespace is fragile; templates become hard to read
- **No structure validation**: Easy to generate invalid YAML
- **Debugging difficulty**: Template errors produce confusing output
- **Merge logic still needed**: Must merge data structures before rendering
- **Version control**: Template diffs are harder to review than code changes

#### Example of Template Complexity

```yaml
services:
  dev-agent:
    {{- if .Build }}
    build:
      context: {{ .Build.Context }}
      {{- if .Build.Args }}
      args:
        {{- range $key, $val := .Build.Args }}
        {{ $key }}: {{ $val }}
        {{- end }}
      {{- end }}
    {{- else }}
    image: {{ .Image }}
    {{- end }}
```

The template syntax obscures the YAML structure and becomes error-prone.

#### Verdict for This Project

**Not recommended**. While text/template is useful for prompt staging (where muster already uses it), it's a poor fit for structured data like compose files. The whitespace sensitivity of YAML combined with template syntax creates a maintenance burden. YAML marshaling is more reliable and testable.

---

## Recommendations

### Chosen Approach: YAML Marshaling (gopkg.in/yaml.v3)

**Rationale:**

1. **Already available**: No new dependencies (gopkg.in/yaml.v3 is in go.mod)
2. **Right level of abstraction**: Higher level than templates, lighter than compose-go
3. **Full control**: Explicit merge logic matches project needs exactly
4. **Easy to test**: Unit tests can verify generated YAML structure
5. **Maintainable**: Go code is easier to review and refactor than templates or library APIs

### Implementation Pattern

```go
// internal/docker/compose.go

package docker

import (
    "embed"
    "gopkg.in/yaml.v3"
)

//go:embed docker-compose.yml
var baseComposeFile []byte

type ComposeFile struct {
    Services map[string]Service `yaml:"services"`
    Volumes  map[string]Volume  `yaml:"volumes,omitempty"`
    Networks map[string]Network `yaml:"networks,omitempty"`
}

type Service struct {
    Image       string            `yaml:"image,omitempty"`
    Build       *Build            `yaml:"build,omitempty"`
    Ports       []string          `yaml:"ports,omitempty"`
    Volumes     []string          `yaml:"volumes,omitempty"`
    Environment map[string]string `yaml:"environment,omitempty"`
    Labels      map[string]string `yaml:"labels,omitempty"`
    Networks    []string          `yaml:"networks,omitempty"`
    DependsOn   []string          `yaml:"depends_on,omitempty"`
}

// GenerateComposeFile creates a merged compose file
func GenerateComposeFile(config Config) ([]byte, error) {
    // Load base
    var base ComposeFile
    if err := yaml.Unmarshal(baseComposeFile, &base); err != nil {
        return nil, err
    }

    // Apply auth overrides
    applyAuth(&base, config.Auth)

    // Apply workspace config
    applyWorkspace(&base, config.Workspace)

    // Apply user overrides
    if config.UserOverride != "" {
        var override ComposeFile
        if err := yaml.Unmarshal([]byte(config.UserOverride), &override); err != nil {
            return nil, err
        }
        mergeComposeFiles(&base, &override)
    }

    // Add container labels
    for name, svc := range base.Services {
        if svc.Labels == nil {
            svc.Labels = make(map[string]string)
        }
        svc.Labels["muster.managed"] = "true"
        svc.Labels["muster.project"] = config.Project
        if config.Slug != "" {
            svc.Labels["muster.slug"] = config.Slug
        }
        base.Services[name] = svc
    }

    // Marshal to YAML
    return yaml.Marshal(&base)
}

func mergeComposeFiles(base, override *ComposeFile) {
    // Merge services
    for name, svc := range override.Services {
        if _, exists := base.Services[name]; exists {
            base.Services[name] = mergeService(base.Services[name], svc)
        } else {
            base.Services[name] = svc
        }
    }

    // Merge volumes (simple replace)
    for name, vol := range override.Volumes {
        base.Volumes[name] = vol
    }

    // Merge networks (simple replace)
    for name, net := range override.Networks {
        base.Networks[name] = net
    }
}

func mergeService(base, override Service) Service {
    // Single-value fields: replace if set
    if override.Image != "" {
        base.Image = override.Image
    }

    // Map fields: merge by key
    if override.Environment != nil {
        if base.Environment == nil {
            base.Environment = make(map[string]string)
        }
        for k, v := range override.Environment {
            base.Environment[k] = v
        }
    }
    if override.Labels != nil {
        if base.Labels == nil {
            base.Labels = make(map[string]string)
        }
        for k, v := range override.Labels {
            base.Labels[k] = v
        }
    }

    // List fields: append (with deduplication if needed)
    base.Ports = appendUnique(base.Ports, override.Ports)
    base.Volumes = appendUnique(base.Volumes, override.Volumes)
    base.Networks = appendUnique(base.Networks, override.Networks)
    base.DependsOn = appendUnique(base.DependsOn, override.DependsOn)

    return base
}

func appendUnique(base, add []string) []string {
    seen := make(map[string]bool)
    for _, v := range base {
        seen[v] = true
    }
    for _, v := range add {
        if !seen[v] {
            base = append(base, v)
            seen[v] = true
        }
    }
    return base
}
```

### Testing Strategy

```go
// internal/docker/compose_test.go

func TestGenerateComposeFile(t *testing.T) {
    tests := []struct {
        name   string
        config Config
        verify func(*testing.T, []byte)
    }{
        {
            name: "adds container labels",
            config: Config{
                Project: "my-api",
                Slug:    "add-retry",
            },
            verify: func(t *testing.T, yaml []byte) {
                var compose ComposeFile
                require.NoError(t, yaml.Unmarshal(yaml, &compose))

                labels := compose.Services["dev-agent"].Labels
                assert.Equal(t, "true", labels["muster.managed"])
                assert.Equal(t, "my-api", labels["muster.project"])
                assert.Equal(t, "add-retry", labels["muster.slug"])
            },
        },
        {
            name: "merges environment variables",
            config: Config{
                Auth: AuthConfig{
                    AnthropicKey: "sk-ant-123",
                },
            },
            verify: func(t *testing.T, yaml []byte) {
                var compose ComposeFile
                require.NoError(t, yaml.Unmarshal(yaml, &compose))

                env := compose.Services["dev-agent"].Environment
                assert.Equal(t, "sk-ant-123", env["ANTHROPIC_API_KEY"])
            },
        },
        // ... more test cases
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            yaml, err := GenerateComposeFile(tt.config)
            require.NoError(t, err)
            tt.verify(t, yaml)
        })
    }
}
```

### File Structure

```
internal/docker/
├── compose.go           # Main compose generation logic
├── compose_test.go      # Unit tests
├── types.go             # ComposeFile, Service structs
└── merge.go             # Merge helper functions

docker/                  # Embedded assets
├── docker-compose.yml   # Base compose file
├── agent.Dockerfile
├── proxy.Dockerfile
└── ...
```

---

## Open Questions

### 1. Should we validate generated compose files?

**Options:**

- **None**: Trust our generation logic, rely on `docker compose` CLI errors
- **Schema validation**: Use compose-go's validator in test mode only
- **Docker validation**: Run `docker compose config` on generated file

**Recommendation**: Start with no validation. Add `docker compose config` validation in tests if issues arise.

### 2. How to handle user override files?

**Requirement**: Users can provide `.muster/dev-agent/config.yml` with custom volumes, networks, env vars.

**Approach**: Load user file as override ComposeFile, merge into base using same `mergeComposeFiles()` logic. This maintains consistent merge semantics.

### 3. What about compose file version field?

Modern Docker Compose ignores the version field (deprecated). Our structs can omit it entirely or include `version: "3"` for compatibility with older tools.

**Recommendation**: Include `version: "3"` in base file for compatibility, but mark as `omitempty` in structs.

### 4. Should we support all compose fields or just what we need?

**Recommendation**: Start minimal - define only the fields muster actually uses (image, build, ports, volumes, environment, labels, networks, depends_on). This reduces maintenance burden. Add fields incrementally if needed.

---

## References

### compose-go Library

- **Repository**: https://github.com/compose-spec/compose-go
- **Documentation**: https://pkg.go.dev/github.com/compose-spec/compose-go/v2
- **Compose Specification**: https://github.com/compose-spec/compose-spec

### gopkg.in/yaml.v3

- **Repository**: https://github.com/go-yaml/yaml
- **Documentation**: https://pkg.go.dev/gopkg.in/yaml.v3
- **API Examples**: Included in pkg.go.dev doc strings

### Docker Compose Merge Semantics

- **Official Docs**: https://docs.docker.com/compose/multiple-compose-files/merge/
- **Key Rule**: "Subsequent files may merge, override, or add to their predecessors"
- **Verification**: Use `docker compose -f base.yml -f override.yml config` to see merged output

### text/template

- **Documentation**: https://pkg.go.dev/text/template
- **Use Case**: Prompt staging in muster, not recommended for compose generation
