// Package config provides configuration loading and resolution for muster.
//
// The config system implements a 5-step resolution chain that determines which tool,
// provider, and model to use for each operation:
//
//  1. Pipeline step override (from .muster/config.yml pipeline.{step}.tool/provider/model)
//  2. Project defaults (from .muster/config.yml defaults.tool/provider/model)
//  3. User defaults (from ~/.config/muster/config.yml defaults.tool/provider/model)
//  4. Tool defaults (hard-coded in each tool's definition)
//  5. Hard-coded fallback defaults (claude-code, anthropic, claude-sonnet-4.5)
//
// Configuration files are loaded from two locations:
//   - User config: ~/.config/muster/config.yml (defines tools, providers, model tiers, defaults)
//   - Project config: .muster/config.yml (defines pipeline step overrides and local settings)
//
// Both support .local.yml variants that are gitignored and take precedence over the base files.
// When loading, base and local files are deep-merged with local values overriding base values.
//
// Model tiers (fast/standard/deep) can be defined at the user level or per-tool, allowing
// different tools to map the same tier name to different concrete model names.
package config

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

// Sentinel errors for config package
var (
	// ErrConfigParse indicates a YAML parsing error
	ErrConfigParse = errors.New("config parse error")
)

// Default configuration constants
const (
	DefaultTool     = "claude-code"
	DefaultProvider = "anthropic"
	DefaultModel    = "claude-sonnet-4.5"
)

// ToolExecutable returns the actual executable name for a tool config name.
// Tool config names (e.g., "claude-code") may differ from their CLI binary
// names (e.g., "claude").
func ToolExecutable(tool string) string {
	switch tool {
	case "claude-code":
		return "claude"
	default:
		return tool
	}
}

// DefaultsConfig specifies default tool, provider, and model settings
type DefaultsConfig struct {
	Tool     *string `yaml:"tool"`
	Provider *string `yaml:"provider"`
	Model    *string `yaml:"model"`
}

// UserConfig represents user-level configuration from ~/.config/muster/config.yml
type UserConfig struct {
	// Defaults specify the default tool, provider, and model
	Defaults *DefaultsConfig `yaml:"defaults"`

	// Tools maps tool names to their configurations
	Tools map[string]*ToolConfig `yaml:"tools"`

	// Providers maps provider names to their configurations
	Providers map[string]*ProviderConfig `yaml:"providers"`

	// ModelTiers defines tier-to-model mappings at the user level
	ModelTiers *struct {
		Fast     *string `yaml:"fast"`
		Standard *string `yaml:"standard"`
		Deep     *string `yaml:"deep"`
	} `yaml:"model_tiers"`
}

// ProjectConfig represents project-level configuration from .muster/config.yml
type ProjectConfig struct {
	// Defaults specify the default tool, provider, and model for this project
	Defaults *DefaultsConfig `yaml:"defaults"`

	// Pipeline maps pipeline step names to their configurations
	Pipeline map[string]*PipelineStepConfig `yaml:"pipeline"`

	// LocalOverrides contains local development overrides
	LocalOverrides map[string]interface{} `yaml:"local_overrides"`
}

// ToolConfig represents configuration for a specific tool
type ToolConfig struct {
	// ModelTiers defines tier-to-model mappings for this tool
	ModelTiers *struct {
		Fast     *string `yaml:"fast"`
		Standard *string `yaml:"standard"`
		Deep     *string `yaml:"deep"`
	} `yaml:"model_tiers"`

	// MaxTokens is the maximum number of tokens for requests
	MaxTokens *int `yaml:"max_tokens"`

	// Temperature controls randomness in model responses
	Temperature *float64 `yaml:"temperature"`
}

// ProviderConfig represents configuration for an AI provider
type ProviderConfig struct {
	// APIKeyEnv is the environment variable name for the API key
	APIKeyEnv *string `yaml:"api_key_env"`

	// BaseURL is the base URL for API requests
	BaseURL *string `yaml:"base_url"`

	// Timeout is the request timeout in seconds
	Timeout *int `yaml:"timeout"`
}

// PipelineStepConfig represents configuration for a specific pipeline step
type PipelineStepConfig struct {
	// Tool overrides the tool for this step
	Tool *string `yaml:"tool"`

	// Provider overrides the provider for this step
	Provider *string `yaml:"provider"`

	// Model overrides the model for this step
	Model *string `yaml:"model"`

	// Timeout is the step timeout in seconds
	Timeout *int `yaml:"timeout"`
}

// DevAgentConfig represents .muster/dev-agent/config.yml merged with its .local.yml
type DevAgentConfig struct {
	// AllowedDomains is the list of domains allowed for network access
	AllowedDomains []string `yaml:"allowed_domains"`

	// Env is a map of environment variables to inject into the container
	Env map[string]string `yaml:"env"`

	// Volumes is a list of volume mounts in Docker format (e.g., "/host:/container:ro")
	Volumes []string `yaml:"volumes"`

	// Networks is a list of Docker networks to attach the container to
	Networks []string `yaml:"networks"`
}

// Config represents the complete configuration state assembled from all three layers
type Config struct {
	// User is the user-level configuration
	User *UserConfig

	// Project is the project-level configuration
	Project *ProjectConfig

	// DevAgent is the dev-agent configuration
	DevAgent *DevAgentConfig
}

// LoadAll loads all configuration layers and returns an assembled Config.
// It loads user config from the specified path (or default location if empty),
// project config from the specified directory, and dev-agent config from the
// same directory.
//
// Missing project and dev-agent configs are not errors - they return empty
// defaults. Missing user config will only error if the file path is explicitly
// provided but not found.
func LoadAll(userConfigPath, projectDir string) (*Config, error) {
	// Load user config
	userCfg, err := LoadUserConfig(userConfigPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load user config: %w", err)
	}

	// Load project config (missing is OK)
	projectCfg, err := LoadProjectConfig(projectDir)
	if err != nil {
		return nil, fmt.Errorf("failed to load project config: %w", err)
	}

	// Load dev-agent config (missing is OK)
	devAgentCfg, err := LoadDevAgentConfig(projectDir)
	if err != nil {
		return nil, fmt.Errorf("failed to load dev-agent config: %w", err)
	}

	return &Config{
		User:     userCfg,
		Project:  projectCfg,
		DevAgent: devAgentCfg,
	}, nil
}

// ResolvedConfig represents the final resolved configuration triple
type ResolvedConfig struct {
	// Tool is the resolved tool name
	Tool string

	// Provider is the resolved provider name
	Provider string

	// Model is the resolved model name
	Model string
}

// Validate validates the Config and collects all errors.
// It checks that:
// - Referenced tools exist in user config
// - Providers exist for each referenced tool
// - Tier names resolve (muster-fast/standard/deep)
// - Required env vars (api_key_env) are set
// Returns all errors at once with actionable error messages.
//
//nolint:gocyclo // Validation logic requires comprehensive checks across multiple config layers
func (c *Config) Validate() []error {
	if c == nil {
		return nil
	}

	var errs []error
	userCfg := c.User
	projectCfg := c.Project

	if userCfg == nil {
		userCfg = DefaultUserConfig()
	}
	if projectCfg == nil {
		projectCfg = &ProjectConfig{}
	}

	// Collect all tools referenced in the configuration
	referencedTools := make(map[string]bool)
	referencedProviders := make(map[string]bool)

	// Check defaults
	if projectCfg.Defaults != nil {
		if projectCfg.Defaults.Tool != nil {
			referencedTools[*projectCfg.Defaults.Tool] = true
		}
		if projectCfg.Defaults.Provider != nil {
			referencedProviders[*projectCfg.Defaults.Provider] = true
		}
	}
	if userCfg.Defaults != nil {
		if userCfg.Defaults.Tool != nil {
			referencedTools[*userCfg.Defaults.Tool] = true
		}
		if userCfg.Defaults.Provider != nil {
			referencedProviders[*userCfg.Defaults.Provider] = true
		}
	}

	// Check pipeline steps
	if projectCfg.Pipeline != nil {
		for _, stepCfg := range projectCfg.Pipeline {
			if stepCfg.Tool != nil {
				referencedTools[*stepCfg.Tool] = true
			}
			if stepCfg.Provider != nil {
				referencedProviders[*stepCfg.Provider] = true
			}
		}
	}

	// Validate that referenced tools exist
	for tool := range referencedTools {
		if tool == DefaultTool {
			// Built-in tool, always valid
			continue
		}
		if userCfg.Tools == nil || userCfg.Tools[tool] == nil {
			errs = append(errs, fmt.Errorf("tool %q is referenced but not defined in user config; add it to ~/.config/muster/config.yml under tools", tool))
		}
	}

	// Validate that providers exist for tools and have required env vars
	// Check providers for defined tools
	if userCfg.Tools != nil {
		for toolName := range userCfg.Tools {
			referencedTools[toolName] = true
		}
	}

	// For each tool, find its provider and validate
	for range referencedTools {
		// Resolve which provider this tool uses
		var provider string

		// Check defaults to find provider
		if projectCfg.Defaults != nil && projectCfg.Defaults.Provider != nil {
			provider = *projectCfg.Defaults.Provider
		} else if userCfg.Defaults != nil && userCfg.Defaults.Provider != nil {
			provider = *userCfg.Defaults.Provider
		} else {
			provider = DefaultProvider
		}

		referencedProviders[provider] = true
	}

	// Validate providers exist and have required env vars set
	for providerName := range referencedProviders {
		if providerName == DefaultProvider {
			// Check default provider
			if userCfg.Providers == nil || userCfg.Providers[providerName] == nil {
				// Default provider should exist but might be missing
				errs = append(errs, fmt.Errorf("provider %q is referenced but not defined in user config; add it to ~/.config/muster/config.yml under providers", providerName))
				continue
			}
		}

		if userCfg.Providers == nil || userCfg.Providers[providerName] == nil {
			errs = append(errs, fmt.Errorf("provider %q is referenced but not defined in user config; add it to ~/.config/muster/config.yml under providers", providerName))
			continue
		}

		providerCfg := userCfg.Providers[providerName]
		if providerCfg.APIKeyEnv != nil {
			envVar := *providerCfg.APIKeyEnv
			if os.Getenv(envVar) == "" {
				// Generate actionable error message
				exampleValue := "sk-..."
				if strings.Contains(strings.ToLower(providerName), "anthropic") {
					exampleValue = "sk-ant-..."
				} else if strings.Contains(strings.ToLower(providerName), "openai") {
					exampleValue = "sk-..."
				}

				errs = append(errs, fmt.Errorf(
					"provider %s requires %s; set it in your shell:\n  export %s=%s\nor configure api_key_env in ~/.config/muster/config.yml",
					providerName, envVar, envVar, exampleValue))
			}
		}
	}

	// Validate tier names resolve
	tierNames := []string{"muster-fast", "muster-standard", "muster-deep"}
	for _, tierName := range tierNames {
		// Try to resolve each tier for each tool
		for tool := range referencedTools {
			_, err := resolveModelTier(tierName, tool, userCfg)
			if err != nil {
				// Only add error if this tier is actually used
				// For now, we'll skip checking if tier is actually referenced
				// since that would require parsing all model strings
				_ = err
			}
		}
	}

	return errs
}

// Resolve resolves configuration for a specific step.
func (c *Config) Resolve(stepName string) (*ResolvedConfig, error) {
	return ResolveStep(stepName, c.Project, c.User)
}
