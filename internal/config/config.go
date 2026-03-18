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
//  - User config: ~/.config/muster/config.yml (defines tools, providers, model tiers, defaults)
//  - Project config: .muster/config.yml (defines pipeline step overrides and local settings)
//
// Both support .local.yml variants that are gitignored and take precedence over the base files.
// When loading, base and local files are deep-merged with local values overriding base values.
//
// Model tiers (fast/standard/deep) can be defined at the user level or per-tool, allowing
// different tools to map the same tier name to different concrete model names.
package config

import "errors"

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

// ResolvedConfig represents the final resolved configuration triple
type ResolvedConfig struct {
	// Tool is the resolved tool name
	Tool string

	// Provider is the resolved provider name
	Provider string

	// Model is the resolved model name
	Model string
}
