package docker

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// ComposeFile represents a Docker Compose file structure.
type ComposeFile struct {
	Version  string              `yaml:"version,omitempty"`
	Services map[string]*Service `yaml:"services,omitempty"`
	Volumes  map[string]any      `yaml:"volumes,omitempty"`
	Networks map[string]any      `yaml:"networks,omitempty"`
	Name     string              `yaml:"name,omitempty"`
}

// Service represents a Docker Compose service definition.
type Service struct {
	Image         string            `yaml:"image,omitempty"`
	Build         *BuildConfig      `yaml:"build,omitempty"`
	Command       any               `yaml:"command,omitempty"`    // string or []string
	Entrypoint    any               `yaml:"entrypoint,omitempty"` // string or []string
	Environment   map[string]string `yaml:"environment,omitempty"`
	Volumes       []string          `yaml:"volumes,omitempty"`
	Ports         []string          `yaml:"ports,omitempty"`
	DependsOn     []string          `yaml:"depends_on,omitempty"`
	Networks      []string          `yaml:"networks,omitempty"`
	Labels        map[string]string `yaml:"labels,omitempty"`
	WorkingDir    string            `yaml:"working_dir,omitempty"`
	User          string            `yaml:"user,omitempty"`
	Stdin         bool              `yaml:"stdin_open,omitempty"`
	Tty           bool              `yaml:"tty,omitempty"`
	RestartPolicy string            `yaml:"restart,omitempty"`
	ExtraHosts    []string          `yaml:"extra_hosts,omitempty"`
}

// BuildConfig represents Docker Compose build configuration.
type BuildConfig struct {
	Context    string            `yaml:"context,omitempty"`
	Dockerfile string            `yaml:"dockerfile,omitempty"`
	Args       map[string]string `yaml:"args,omitempty"`
	Target     string            `yaml:"target,omitempty"`
}

// mergeComposeFiles merges override into base, modifying base in place.
// Per review S9: when types conflict, override type always wins (replace entirely).
func mergeComposeFiles(base, override *ComposeFile) {
	if base == nil || override == nil {
		return
	}

	// Merge version (override wins)
	if override.Version != "" {
		base.Version = override.Version
	}

	// Merge name (override wins)
	if override.Name != "" {
		base.Name = override.Name
	}

	// Merge services
	if base.Services == nil {
		base.Services = make(map[string]*Service)
	}
	for name, svc := range override.Services {
		if baseSvc, exists := base.Services[name]; exists {
			// Merge service definitions
			base.Services[name] = mergeService(baseSvc, svc)
		} else {
			// New service
			base.Services[name] = svc
		}
	}

	// Merge volumes (map[string]any)
	if override.Volumes != nil {
		if base.Volumes == nil {
			base.Volumes = make(map[string]any)
		}
		for k, v := range override.Volumes {
			base.Volumes[k] = v
		}
	}

	// Merge networks (map[string]any)
	if override.Networks != nil {
		if base.Networks == nil {
			base.Networks = make(map[string]any)
		}
		for k, v := range override.Networks {
			base.Networks[k] = v
		}
	}
}

// mergeService merges override service into base service.
// Returns a new merged service.
// Per review S9: when types conflict between base and override, override type always wins.
func mergeService(base, override *Service) *Service {
	if base == nil {
		return override
	}
	if override == nil {
		return base
	}

	// Create merged service starting from base
	merged := &Service{
		Image:         base.Image,
		Build:         base.Build,
		Command:       base.Command,
		Entrypoint:    base.Entrypoint,
		Environment:   make(map[string]string),
		Volumes:       base.Volumes,
		Ports:         base.Ports,
		DependsOn:     base.DependsOn,
		Networks:      base.Networks,
		Labels:        make(map[string]string),
		WorkingDir:    base.WorkingDir,
		User:          base.User,
		Stdin:         base.Stdin,
		Tty:           base.Tty,
		RestartPolicy: base.RestartPolicy,
		ExtraHosts:    base.ExtraHosts,
	}

	// Copy base environment
	for k, v := range base.Environment {
		merged.Environment[k] = v
	}

	// Copy base labels
	for k, v := range base.Labels {
		merged.Labels[k] = v
	}

	// Apply overrides (scalar fields - override wins)
	if override.Image != "" {
		merged.Image = override.Image
	}
	if override.Build != nil {
		merged.Build = override.Build
	}
	if override.Command != nil {
		// Type conflict handling: override type always wins
		merged.Command = override.Command
	}
	if override.Entrypoint != nil {
		// Type conflict handling: override type always wins
		merged.Entrypoint = override.Entrypoint
	}
	if override.WorkingDir != "" {
		merged.WorkingDir = override.WorkingDir
	}
	if override.User != "" {
		merged.User = override.User
	}
	if override.RestartPolicy != "" {
		merged.RestartPolicy = override.RestartPolicy
	}

	// Boolean fields (override if explicitly set)
	// Note: We can't distinguish between false and unset, so we always apply
	if override.Stdin {
		merged.Stdin = override.Stdin
	}
	if override.Tty {
		merged.Tty = override.Tty
	}

	// Merge environment (override wins for each key)
	for k, v := range override.Environment {
		merged.Environment[k] = v
	}

	// Merge labels (override wins for each key)
	for k, v := range override.Labels {
		merged.Labels[k] = v
	}

	// Append and deduplicate lists
	if override.Volumes != nil {
		merged.Volumes = appendUnique(merged.Volumes, override.Volumes)
	}
	if override.Ports != nil {
		merged.Ports = appendUnique(merged.Ports, override.Ports)
	}
	if override.DependsOn != nil {
		merged.DependsOn = appendUnique(merged.DependsOn, override.DependsOn)
	}
	if override.Networks != nil {
		merged.Networks = appendUnique(merged.Networks, override.Networks)
	}
	if override.ExtraHosts != nil {
		merged.ExtraHosts = appendUnique(merged.ExtraHosts, override.ExtraHosts)
	}

	return merged
}

// appendUnique appends items from add to base, deduplicating entries.
// Returns a new slice with unique items, preserving order (base items first, then new add items).
func appendUnique(base, add []string) []string {
	if add == nil {
		return base
	}

	// Build set from base
	seen := make(map[string]bool)
	for _, item := range base {
		seen[item] = true
	}

	// Append only new items from add
	result := make([]string, len(base))
	copy(result, base)
	for _, item := range add {
		if !seen[item] {
			result = append(result, item)
			seen[item] = true
		}
	}

	return result
}

// parseComposeFile parses YAML bytes into a ComposeFile struct.
func parseComposeFile(data []byte) (*ComposeFile, error) {
	var compose ComposeFile
	if err := yaml.Unmarshal(data, &compose); err != nil {
		return nil, fmt.Errorf("failed to parse compose file: %w", err)
	}
	return &compose, nil
}

// marshalComposeFile marshals a ComposeFile to YAML bytes.
func marshalComposeFile(compose *ComposeFile) ([]byte, error) {
	data, err := yaml.Marshal(compose)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal compose file: %w", err)
	}
	return data, nil
}

// normalizeCommand normalizes a command field to handle type variations.
// Docker Compose allows both string and []string for command/entrypoint.
// This helper ensures consistent handling.
func normalizeCommand(cmd any) string {
	if cmd == nil {
		return ""
	}

	switch v := cmd.(type) {
	case string:
		return v
	case []string:
		return strings.Join(v, " ")
	case []any:
		// Handle YAML array parsed as []interface{}
		parts := make([]string, 0, len(v))
		for _, part := range v {
			parts = append(parts, fmt.Sprintf("%v", part))
		}
		return strings.Join(parts, " ")
	default:
		return fmt.Sprintf("%v", v)
	}
}

// ComposeOptions contains all options for generating a compose file.
type ComposeOptions struct {
	Project     string            // project name (from directory)
	Slug        string            // roadmap item slug (empty for interactive)
	Auth        *AuthRequirements // collected auth config
	DevAgent    interface{}       // DevAgentConfig - using interface{} to avoid import cycle
	WorktreeDir string            // host path to worktree (or project dir)
	MainRepoDir string            // host path to main repo (.git location)
	AssetDir    string            // path to extracted docker assets
	CacheDir    string            // path to cache directory for compose file
}

// GenerateComposeFile builds a merged compose file from the embedded base,
// auth overrides, workspace config, and user overrides. Writes the result
// to cacheDir/docker-compose.yml and returns the path.
func GenerateComposeFile(opts ComposeOptions) (string, error) {
	// 1. Load base compose file from embedded assets
	baseData, err := Assets.ReadFile("docker/docker-compose.yml")
	if err != nil {
		return "", fmt.Errorf("failed to read embedded base compose: %w", err)
	}

	base, err := parseComposeFile(baseData)
	if err != nil {
		return "", fmt.Errorf("failed to parse base compose: %w", err)
	}

	// 2. Apply auth overrides
	if opts.Auth != nil {
		applyAuth(base, opts.Auth, opts.AssetDir)
	}

	// 3. Apply workspace mounts
	applyWorkspaceMounts(base, opts.WorktreeDir, opts.MainRepoDir)

	// 4. Apply dev-agent config
	if opts.DevAgent != nil {
		applyDevAgentConfig(base, opts.DevAgent)
	}

	// 5. Apply labels
	labels := Labels(opts.Project, opts.Slug)
	applyLabels(base, labels)

	// 6. Apply user override if exists
	overridePath := ".muster/dev-agent/docker-compose.override.yml"
	if overrideData, err := os.ReadFile(overridePath); err == nil {
		override, err := parseComposeFile(overrideData)
		if err != nil {
			return "", fmt.Errorf("failed to parse user override compose: %w", err)
		}
		mergeComposeFiles(base, override)
	}

	// 7. Marshal to YAML
	data, err := marshalComposeFile(base)
	if err != nil {
		return "", fmt.Errorf("failed to marshal compose file: %w", err)
	}

	// 8. Write atomically: temp file -> validate -> rename
	// Per review S4: atomic writes, per review B4: validation with opt-out
	composePath := fmt.Sprintf("%s/docker-compose.yml", opts.CacheDir)
	tmpPath := composePath + ".tmp"

	//nolint:gosec // G306: File permissions 0644 are appropriate for non-sensitive compose file
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return "", fmt.Errorf("failed to write temp compose file: %w", err)
	}

	// Validate unless explicitly skipped
	if os.Getenv("MUSTER_SKIP_COMPOSE_VALIDATION") != "1" {
		if err := validateComposeFile(tmpPath); err != nil {
			_ = os.Remove(tmpPath) // Clean up temp file
			return "", fmt.Errorf("compose validation failed: %w", err)
		}
	}

	// Atomic rename
	if err := os.Rename(tmpPath, composePath); err != nil {
		_ = os.Remove(tmpPath) // Clean up temp file
		return "", fmt.Errorf("failed to rename compose file: %w", err)
	}

	return composePath, nil
}

// applyAuth applies auth-specific overrides to the compose file.
func applyAuth(compose *ComposeFile, auth *AuthRequirements, assetDir string) {
	agent, ok := compose.Services["dev-agent"]
	if !ok {
		return
	}

	// Ensure maps are initialized
	if agent.Environment == nil {
		agent.Environment = make(map[string]string)
	}
	if agent.Volumes == nil {
		agent.Volumes = []string{}
	}
	if agent.ExtraHosts == nil {
		agent.ExtraHosts = []string{}
	}

	// Apply Bedrock auth
	if auth.Bedrock != nil {
		homeDir, _ := os.UserHomeDir()
		awsDir := fmt.Sprintf("%s/.aws", homeDir)
		agent.Volumes = append(agent.Volumes, fmt.Sprintf("%s:/home/node/.aws:ro", awsDir))
		agent.Environment["AWS_PROFILE"] = auth.Bedrock.AWSProfile
		agent.Environment["AWS_REGION"] = auth.Bedrock.AWSRegion
		agent.Environment["CLAUDE_CODE_USE_BEDROCK"] = "1"

		// Mount merged settings.json if available
		if assetDir != "" {
			settingsPath := fmt.Sprintf("%s/settings.json", assetDir)
			agent.Volumes = append(agent.Volumes, fmt.Sprintf("%s:/home/node/.claude/settings.json:ro", settingsPath))
		}
	}

	// Apply Max auth
	if auth.Max != nil {
		agent.Volumes = append(agent.Volumes,
			fmt.Sprintf("%s:/home/node/.claude/.credentials.json:ro", auth.Max.CredentialsPath))
	}

	// Apply API key auth
	for _, apiKey := range auth.APIKeys {
		agent.Environment[apiKey.EnvVar] = apiKey.Value
	}

	// Apply local provider settings
	if len(auth.Local) > 0 {
		// Add host.docker.internal to extra_hosts (for Linux)
		agent.ExtraHosts = appendUnique(agent.ExtraHosts, []string{"host.docker.internal:host-gateway"})
	}
}

// applyWorkspaceMounts adds workspace and git directory mounts.
func applyWorkspaceMounts(compose *ComposeFile, worktreeDir, mainRepoDir string) {
	agent, ok := compose.Services["dev-agent"]
	if !ok {
		return
	}

	if agent.Volumes == nil {
		agent.Volumes = []string{}
	}

	// Mount worktree read-write
	if worktreeDir != "" {
		agent.Volumes = append(agent.Volumes, fmt.Sprintf("%s:/workspace", worktreeDir))
	}

	// Mount .git read-only
	// If worktree and main repo are different, mount main repo .git separately
	if mainRepoDir != "" && mainRepoDir != worktreeDir {
		agent.Volumes = append(agent.Volumes, fmt.Sprintf("%s:/workspace/.git:ro", mainRepoDir))
	}
}

// applyDevAgentConfig applies user-defined dev-agent configuration.
func applyDevAgentConfig(compose *ComposeFile, devAgentRaw interface{}) {
	agent, ok := compose.Services["dev-agent"]
	if !ok {
		return
	}

	// Type assert to get the config (avoiding import cycle by using interface{})
	// Caller should pass a struct with these fields
	devAgent, ok := devAgentRaw.(struct {
		AllowedDomains []string
		Env            map[string]string
		Volumes        []string
		Networks       []string
	})
	if !ok {
		return
	}

	// Apply environment variables
	if agent.Environment == nil {
		agent.Environment = make(map[string]string)
	}
	for k, v := range devAgent.Env {
		agent.Environment[k] = v
	}

	// Apply volumes
	if agent.Volumes == nil {
		agent.Volumes = []string{}
	}
	agent.Volumes = appendUnique(agent.Volumes, devAgent.Volumes)

	// Apply networks
	if agent.Networks == nil {
		agent.Networks = []string{}
	}
	agent.Networks = appendUnique(agent.Networks, devAgent.Networks)
}

// applyLabels applies standard muster labels to all services.
func applyLabels(compose *ComposeFile, labels map[string]string) {
	for _, service := range compose.Services {
		if service.Labels == nil {
			service.Labels = make(map[string]string)
		}
		for k, v := range labels {
			service.Labels[k] = v
		}
	}
}

// validateComposeFile validates a compose file using docker compose config.
func validateComposeFile(path string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	//nolint:gosec // G204: path is from internal temp file, not user input
	cmd := exec.CommandContext(ctx, "docker", "compose", "-f", path, "config")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("validation failed: %w\nOutput: %s", err, string(output))
	}
	return nil
}
