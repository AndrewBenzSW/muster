package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// LoadProjectConfig loads project configuration from .muster/config.yml (or .dev-agent/config.yml for backward compatibility)
// and merges it with .muster/config.local.yml if it exists.
// If no config files exist, it returns a default empty ProjectConfig (not an error).
// Only returns an error if a config file exists but is malformed.
func LoadProjectConfig(dir string) (*ProjectConfig, error) {
	// Try to load base config from .muster/config.yml first
	basePath := filepath.Join(dir, ".muster", "config.yml")
	baseConfig, err := loadProjectConfigFile(basePath)
	if err != nil {
		// If the file exists but has a parse error, don't try fallback
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("failed to load project config from %s: %w", basePath, err)
		}

		// Fall back to .dev-agent/config.yml for backward compatibility
		fallbackPath := filepath.Join(dir, ".dev-agent", "config.yml")
		baseConfig, err = loadProjectConfigFile(fallbackPath)
		if err != nil {
			// If neither file exists, return default empty config
			if os.IsNotExist(err) {
				baseConfig = &ProjectConfig{}
			} else {
				return nil, fmt.Errorf("failed to load project config from %s: %w", fallbackPath, err)
			}
		}
	}

	// Try to load local override config
	localPath := filepath.Join(dir, ".muster", "config.local.yml")
	localConfig, err := loadProjectConfigFile(localPath)
	if err != nil {
		if os.IsNotExist(err) {
			// No local config, return base config only
			return baseConfig, nil
		}
		return nil, fmt.Errorf("failed to load local config from %s: %w", localPath, err)
	}

	// Merge local config over base config
	merged := mergeProjectConfigs(baseConfig, localConfig)
	return merged, nil
}

// loadProjectConfigFile loads a single project config file
func loadProjectConfigFile(path string) (*ProjectConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var config ProjectConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse %s: %w: %w", path, ErrConfigParse, err)
	}

	return &config, nil
}

// mergeProjectConfigs performs a field-level deep merge of two project configs.
// Fields from override replace corresponding fields in base.
// Lists replace entirely (not append).
// Override can add new sections that don't exist in base.
func mergeProjectConfigs(base, override *ProjectConfig) *ProjectConfig {
	if base == nil {
		if override == nil {
			return &ProjectConfig{}
		}
		return override
	}
	if override == nil {
		return base
	}

	// Create result starting with base
	result := &ProjectConfig{
		Defaults:       base.Defaults,
		Pipeline:       make(map[string]*PipelineStepConfig),
		LocalOverrides: make(map[string]interface{}),
	}

	// Copy base pipeline
	for k, v := range base.Pipeline {
		result.Pipeline[k] = v
	}

	// Copy base local overrides
	for k, v := range base.LocalOverrides {
		result.LocalOverrides[k] = v
	}

	// Merge defaults (field-level override)
	if override.Defaults != nil {
		if result.Defaults == nil {
			result.Defaults = override.Defaults
		} else {
			// Create a new defaults struct to avoid modifying base
			merged := &DefaultsConfig{
				Tool:     result.Defaults.Tool,
				Provider: result.Defaults.Provider,
				Model:    result.Defaults.Model,
			}

			// Override individual fields
			if override.Defaults.Tool != nil {
				merged.Tool = override.Defaults.Tool
			}
			if override.Defaults.Provider != nil {
				merged.Provider = override.Defaults.Provider
			}
			if override.Defaults.Model != nil {
				merged.Model = override.Defaults.Model
			}

			result.Defaults = merged
		}
	}

	// Merge pipeline (entire step configs replace, not field-by-field within steps)
	for k, v := range override.Pipeline {
		result.Pipeline[k] = v
	}

	// Merge local overrides (entire values replace)
	for k, v := range override.LocalOverrides {
		result.LocalOverrides[k] = v
	}

	return result
}
