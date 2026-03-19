package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// LoadUserConfig loads user configuration from ~/.config/muster/config.yml
// If the file doesn't exist, it returns a default configuration (not an error).
// If the file exists but is malformed, it returns an error with context.
//
// Platform-specific behavior:
// os.UserConfigDir() returns platform-specific configuration directories:
//   - Linux:   $XDG_CONFIG_HOME/muster/config.yml (typically ~/.config/muster/config.yml)
//   - macOS:   ~/Library/Application Support/muster/config.yml
//   - Windows: %AppData%/muster/config.yml (e.g., C:\Users\username\AppData\Roaming\muster\config.yml)
//
// filepath.Join() automatically uses the correct path separator for the platform:
//   - Unix-like systems (Linux, macOS): forward slash (/)
//   - Windows: backslash (\)
func LoadUserConfig(path string) (*UserConfig, error) {
	// If no path is provided, use the default user config location
	if path == "" {
		// os.UserConfigDir() returns platform-specific config directory
		// See function documentation for platform-specific paths
		configDir, err := os.UserConfigDir()
		if err != nil {
			return nil, fmt.Errorf("failed to get user config directory: %w", err)
		}
		// filepath.Join() uses platform-appropriate path separators
		path = filepath.Join(configDir, "muster", "config.yml")
	}

	// Check if the file exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		// File doesn't exist - return default config (not an error)
		return DefaultUserConfig(), nil
	}

	// Read the file
	data, err := os.ReadFile(path) //nolint:gosec // G304: Path from config system, not user input
	if err != nil {
		return nil, fmt.Errorf("failed to read config file %s: %w", path, err)
	}

	// If file is empty, return default config
	if len(data) == 0 {
		return DefaultUserConfig(), nil
	}

	// Unmarshal the YAML
	var config UserConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config file %s: %w: %w", path, ErrConfigParse, err)
	}

	return &config, nil
}

// DefaultUserConfig returns a UserConfig with sensible defaults.
// Tool and provider defaults are set here; model is intentionally left nil
// so that ResolveStep can apply per-step tier defaults before falling back
// to the hard-coded DefaultModel.
func DefaultUserConfig() *UserConfig {
	return &UserConfig{
		Tools:      make(map[string]*ToolConfig),
		Providers:  make(map[string]*ProviderConfig),
		ModelTiers: nil,
	}
}
