package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadUserConfig(t *testing.T) {
	tests := []struct {
		name     string
		fixture  string
		wantErr  bool
		validate func(t *testing.T, cfg *UserConfig)
	}{
		{
			name:    "valid full config",
			fixture: "user-full.yml",
			wantErr: false,
			validate: func(t *testing.T, cfg *UserConfig) {
				require.NotNil(t, cfg.Defaults)
				assert.NotNil(t, cfg.Defaults.Tool)
				assert.Equal(t, "claude-code", *cfg.Defaults.Tool)
				assert.NotNil(t, cfg.Defaults.Provider)
				assert.Equal(t, "anthropic", *cfg.Defaults.Provider)
				assert.NotNil(t, cfg.Defaults.Model)
				assert.Equal(t, "claude-sonnet-4.5", *cfg.Defaults.Model)

				// Check tools
				assert.Len(t, cfg.Tools, 2)
				assert.Contains(t, cfg.Tools, "claude-code")
				assert.Contains(t, cfg.Tools, "cursor")

				claudeTool := cfg.Tools["claude-code"]
				require.NotNil(t, claudeTool)
				require.NotNil(t, claudeTool.ModelTiers)
				assert.NotNil(t, claudeTool.ModelTiers.Fast)
				assert.Equal(t, "claude-sonnet-4.5", *claudeTool.ModelTiers.Fast)
				assert.NotNil(t, claudeTool.MaxTokens)
				assert.Equal(t, 8192, *claudeTool.MaxTokens)
				assert.NotNil(t, claudeTool.Temperature)
				assert.Equal(t, 0.7, *claudeTool.Temperature)

				// Check providers
				assert.Len(t, cfg.Providers, 2)
				assert.Contains(t, cfg.Providers, "anthropic")
				assert.Contains(t, cfg.Providers, "openai")

				anthropic := cfg.Providers["anthropic"]
				require.NotNil(t, anthropic)
				assert.NotNil(t, anthropic.APIKeyEnv)
				assert.Equal(t, "ANTHROPIC_API_KEY", *anthropic.APIKeyEnv)
				assert.NotNil(t, anthropic.BaseURL)
				assert.Equal(t, "https://api.anthropic.com", *anthropic.BaseURL)
				assert.NotNil(t, anthropic.Timeout)
				assert.Equal(t, 300, *anthropic.Timeout)

				// Check model tiers
				require.NotNil(t, cfg.ModelTiers)
				assert.NotNil(t, cfg.ModelTiers.Fast)
				assert.Equal(t, "claude-sonnet-4.5", *cfg.ModelTiers.Fast)
				assert.NotNil(t, cfg.ModelTiers.Deep)
				assert.Equal(t, "claude-opus-4", *cfg.ModelTiers.Deep)
			},
		},
		{
			name:    "minimal valid config",
			fixture: "user-minimal.yml",
			wantErr: false,
			validate: func(t *testing.T, cfg *UserConfig) {
				require.NotNil(t, cfg.Defaults)
				assert.NotNil(t, cfg.Defaults.Tool)
				assert.Equal(t, "claude-code", *cfg.Defaults.Tool)
				assert.NotNil(t, cfg.Defaults.Provider)
				assert.Equal(t, "anthropic", *cfg.Defaults.Provider)
				assert.NotNil(t, cfg.Defaults.Model)
				assert.Equal(t, "claude-sonnet-4.5", *cfg.Defaults.Model)
			},
		},
		{
			name:    "empty file returns default",
			fixture: "user-empty.yml",
			wantErr: false,
			validate: func(t *testing.T, cfg *UserConfig) {
				// Empty file should return default config
				assert.NotNil(t, cfg)
				// Defaults is nil — ResolveStep handles defaults
				assert.Nil(t, cfg.Defaults)
			},
		},
		{
			name:    "missing file returns default",
			fixture: "nonexistent.yml",
			wantErr: false,
			validate: func(t *testing.T, cfg *UserConfig) {
				// Missing file should return default config (not an error)
				assert.NotNil(t, cfg)
				// Defaults is nil — ResolveStep handles defaults
				assert.Nil(t, cfg.Defaults)
			},
		},
		{
			name:    "malformed YAML returns error",
			fixture: "malformed.yml",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup: create malformed YAML if needed
			var path string
			switch tt.fixture {
			case "malformed.yml":
				tmpDir := t.TempDir()
				path = filepath.Join(tmpDir, "malformed.yml")
				err := os.WriteFile(path, []byte("invalid: yaml: content:\n  - missing\n    closing"), 0644) //nolint:gosec // G306: Test file permissions
				require.NoError(t, err)
			case "nonexistent.yml":
				// Use a path that doesn't exist
				path = filepath.Join(t.TempDir(), "nonexistent.yml")
			default:
				// Use testdata fixtures
				path = filepath.Join("testdata", tt.fixture)
			}

			// Execute
			cfg, err := LoadUserConfig(path)

			// Assert
			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, cfg)
			} else {
				assert.NoError(t, err)
				require.NotNil(t, cfg)
				if tt.validate != nil {
					tt.validate(t, cfg)
				}
			}
		})
	}
}

func TestLoadUserConfig_UnknownFieldsIgnored(t *testing.T) {
	// Create a config with unknown fields
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "unknown-fields.yml")
	content := `defaults:
  tool: claude-code
  provider: anthropic
  model: claude-sonnet-4.5
unknown_field: should_be_ignored
another_unknown:
  nested: value
`
	err := os.WriteFile(path, []byte(content), 0644) //nolint:gosec // G306: Test file permissions
	require.NoError(t, err)

	// Load config - should succeed and ignore unknown fields
	cfg, err := LoadUserConfig(path)
	assert.NoError(t, err)
	require.NotNil(t, cfg)
	require.NotNil(t, cfg.Defaults)
	assert.Equal(t, "claude-code", *cfg.Defaults.Tool)
}

func TestLoadUserConfig_NestedStructUnmarshal(t *testing.T) {
	// Test that nested structures are properly unmarshaled
	path := filepath.Join("testdata", "user-full.yml")
	cfg, err := LoadUserConfig(path)
	require.NoError(t, err)
	require.NotNil(t, cfg)

	// Check nested tool config
	require.Contains(t, cfg.Tools, "claude-code")
	tool := cfg.Tools["claude-code"]
	require.NotNil(t, tool)
	require.NotNil(t, tool.ModelTiers)
	assert.NotNil(t, tool.ModelTiers.Fast)
	assert.NotNil(t, tool.ModelTiers.Standard)
	assert.NotNil(t, tool.ModelTiers.Deep)

	// Check nested provider config
	require.Contains(t, cfg.Providers, "anthropic")
	provider := cfg.Providers["anthropic"]
	require.NotNil(t, provider)
	assert.NotNil(t, provider.APIKeyEnv)
	assert.NotNil(t, provider.BaseURL)
	assert.NotNil(t, provider.Timeout)
}

func TestLoadUserConfig_PointerFieldUnmarshal(t *testing.T) {
	// Test that pointer fields are correctly unmarshaled
	path := filepath.Join("testdata", "user-minimal.yml")
	cfg, err := LoadUserConfig(path)
	require.NoError(t, err)
	require.NotNil(t, cfg)

	// Check pointer fields in defaults
	require.NotNil(t, cfg.Defaults)
	assert.NotNil(t, cfg.Defaults.Tool)
	assert.NotNil(t, cfg.Defaults.Provider)
	assert.NotNil(t, cfg.Defaults.Model)
	assert.Equal(t, "claude-sonnet-4.5", *cfg.Defaults.Model) // from fixture file

	// When fields are not present in config, they should be nil or empty
	assert.Nil(t, cfg.ModelTiers)
	assert.Empty(t, cfg.Tools)
	assert.Empty(t, cfg.Providers)
}

// TestUserConfigDir_PlatformPaths tests that os.UserConfigDir() returns
// the expected platform-specific configuration directory paths.
// This test documents expected behavior across platforms but doesn't enforce
// exact paths since they may vary based on environment variables.
func TestUserConfigDir_PlatformPaths(t *testing.T) {
	configDir, err := os.UserConfigDir()
	require.NoError(t, err, "os.UserConfigDir() should not error")
	require.NotEmpty(t, configDir, "config directory should not be empty")

	// Platform-specific path expectations (documented for reference):
	// - Linux:   $XDG_CONFIG_HOME or ~/.config (typically /home/username/.config)
	// - macOS:   ~/Library/Application Support (e.g., /Users/username/Library/Application Support)
	// - Windows: %AppData% (e.g., C:\Users\username\AppData\Roaming)
	//
	// We can't assert exact paths since they depend on environment and user,
	// but we can verify basic properties
	t.Logf("Platform config directory: %s", configDir)

	// Verify the path is absolute
	assert.True(t, filepath.IsAbs(configDir), "config directory should be an absolute path")

	// Verify filepath.Join produces correct separators
	musterConfigPath := filepath.Join(configDir, "muster", "config.yml")
	t.Logf("Expected muster config path: %s", musterConfigPath)

	// The joined path should use platform-appropriate separators
	// On Windows: contains backslashes
	// On Unix-like: contains forward slashes
	if filepath.Separator == '\\' {
		assert.Contains(t, musterConfigPath, "\\", "Windows paths should contain backslashes")
	} else {
		assert.Contains(t, musterConfigPath, "/", "Unix paths should contain forward slashes")
	}
}

// TestLoadUserConfig_PermissionDenied tests that LoadUserConfig returns an
// actionable error when a config file exists but isn't readable due to permissions.
func TestLoadUserConfig_PermissionDenied(t *testing.T) {
	// Create a temp directory with a config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yml")

	// Write a valid config file
	content := `defaults:
  tool: claude-code
  provider: anthropic
  model: claude-sonnet-4.5
`
	err := os.WriteFile(configPath, []byte(content), 0644) //nolint:gosec // G306: Test file permissions
	require.NoError(t, err)

	// Make the file unreadable (000 permissions)
	err = os.Chmod(configPath, 0000)
	require.NoError(t, err)

	// Try to load the config
	cfg, err := LoadUserConfig(configPath)

	// Should return an error
	assert.Error(t, err)
	assert.Nil(t, cfg)

	// Error message should be actionable - mention permission or access issue
	errMsg := err.Error()
	assert.True(t,
		contains(errMsg, "permission") || contains(errMsg, "denied") || contains(errMsg, "access"),
		"error message should mention permission/access issue: %s", errMsg)
}

// contains is a helper function for substring search
func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
