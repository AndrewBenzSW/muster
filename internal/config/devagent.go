package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// LoadDevAgentConfig loads dev-agent configuration from .muster/dev-agent/config.yml
// and merges it with .muster/dev-agent/config.local.yml if it exists.
// If no config files exist, it returns an empty DevAgentConfig (not an error).
// Only returns an error if a config file exists but is malformed.
func LoadDevAgentConfig(dir string) (*DevAgentConfig, error) {
	// Try to load base config from .muster/dev-agent/config.yml
	basePath := filepath.Join(dir, ".muster", "dev-agent", "config.yml")
	baseConfig, err := loadDevAgentConfigFile(basePath)
	if err != nil {
		// If the file exists but has a parse error, return the error
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("failed to load dev-agent config from %s: %w", basePath, err)
		}
		// File doesn't exist - return empty defaults
		baseConfig = &DevAgentConfig{}
	}

	// Try to load local override config
	localPath := filepath.Join(dir, ".muster", "dev-agent", "config.local.yml")
	localConfig, err := loadDevAgentConfigFile(localPath)
	if err != nil {
		if os.IsNotExist(err) {
			// No local config, return base config only
			return baseConfig, nil
		}
		return nil, fmt.Errorf("failed to load local dev-agent config from %s: %w", localPath, err)
	}

	// Merge local config over base config
	merged := mergeDevAgentConfigs(baseConfig, localConfig)
	return merged, nil
}

// loadDevAgentConfigFile loads a single dev-agent config file
func loadDevAgentConfigFile(path string) (*DevAgentConfig, error) {
	data, err := os.ReadFile(path) //nolint:gosec // G304: Path from config system, not user input
	if err != nil {
		return nil, err
	}

	var config DevAgentConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse %s: %w: %w", path, ErrConfigParse, err)
	}

	return &config, nil
}

// mergeDevAgentConfigs performs a field-level merge of two dev-agent configs.
// Lists are replaced entirely (not appended).
// Maps are merged with override values replacing base values.
func mergeDevAgentConfigs(base, override *DevAgentConfig) *DevAgentConfig {
	if base == nil {
		if override == nil {
			return &DevAgentConfig{}
		}
		return override
	}
	if override == nil {
		return base
	}

	// Start with base
	result := &DevAgentConfig{
		AllowedDomains: base.AllowedDomains,
		Env:            make(map[string]string),
		Volumes:        base.Volumes,
		Networks:       base.Networks,
	}

	// Copy base env
	for k, v := range base.Env {
		result.Env[k] = v
	}

	// Override allowed domains if present (replace, not append)
	if override.AllowedDomains != nil {
		result.AllowedDomains = override.AllowedDomains
	}

	// Merge env vars (field-level override)
	for k, v := range override.Env {
		result.Env[k] = v
	}

	// Override volumes if present (replace, not append)
	if override.Volumes != nil {
		result.Volumes = override.Volumes
	}

	// Override networks if present (replace, not append)
	if override.Networks != nil {
		result.Networks = override.Networks
	}

	return result
}
