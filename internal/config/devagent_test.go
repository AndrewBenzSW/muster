package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadDevAgentConfig(t *testing.T) {
	tests := []struct {
		name     string
		fixture  string
		wantErr  bool
		validate func(t *testing.T, cfg *DevAgentConfig)
	}{
		{
			name:    "valid full config",
			fixture: "devagent.yml",
			wantErr: false,
			validate: func(t *testing.T, cfg *DevAgentConfig) {
				require.NotNil(t, cfg)

				// Check allowed domains
				assert.Len(t, cfg.AllowedDomains, 4)
				assert.Contains(t, cfg.AllowedDomains, "*.anthropic.com")
				assert.Contains(t, cfg.AllowedDomains, "*.openai.com")
				assert.Contains(t, cfg.AllowedDomains, "github.com")
				assert.Contains(t, cfg.AllowedDomains, "api.example.com")

				// Check env vars
				assert.Len(t, cfg.Env, 3)
				assert.Equal(t, "true", cfg.Env["DEBUG"])
				assert.Equal(t, "info", cfg.Env["LOG_LEVEL"])
				assert.Equal(t, "30", cfg.Env["API_TIMEOUT"])

				// Check volumes
				assert.Len(t, cfg.Volumes, 2)
				assert.Contains(t, cfg.Volumes, "/host/data:/container/data")
				assert.Contains(t, cfg.Volumes, "/host/cache:/container/cache:ro")

				// Check networks
				assert.Len(t, cfg.Networks, 2)
				assert.Contains(t, cfg.Networks, "muster-net")
				assert.Contains(t, cfg.Networks, "external-net")
			},
		},
		{
			name:    "empty defaults when file missing",
			fixture: "nonexistent.yml",
			wantErr: false,
			validate: func(t *testing.T, cfg *DevAgentConfig) {
				require.NotNil(t, cfg)
				assert.Empty(t, cfg.AllowedDomains)
				assert.Empty(t, cfg.Env)
				assert.Empty(t, cfg.Volumes)
				assert.Empty(t, cfg.Networks)
			},
		},
		{
			name:    "invalid YAML returns error",
			fixture: "devagent.invalid.yml",
			wantErr: true,
			validate: func(t *testing.T, cfg *DevAgentConfig) {
				// Should not be called when error expected
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a temporary directory for the test
			tmpDir := t.TempDir()

			// If fixture is not "nonexistent.yml", copy it to the temp dir
			if tt.fixture != "nonexistent.yml" {
				devAgentDir := filepath.Join(tmpDir, ".muster", "dev-agent")
				err := os.MkdirAll(devAgentDir, 0755) //nolint:gosec // G301: Test directory permissions
				require.NoError(t, err)

				// Read fixture from testdata
				fixturePath := filepath.Join("testdata", tt.fixture)
				data, err := os.ReadFile(fixturePath) //nolint:gosec // G304: Test fixture path
				require.NoError(t, err)

				// Write to temp location
				configPath := filepath.Join(devAgentDir, "config.yml")
				err = os.WriteFile(configPath, data, 0644) //nolint:gosec // G306: Test file permissions
				require.NoError(t, err)
			}

			// Load the config
			cfg, err := LoadDevAgentConfig(tmpDir)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				tt.validate(t, cfg)
			}
		})
	}
}

func TestLoadDevAgentConfigWithLocal(t *testing.T) {
	// Create a temporary directory
	tmpDir := t.TempDir()
	devAgentDir := filepath.Join(tmpDir, ".muster", "dev-agent")
	err := os.MkdirAll(devAgentDir, 0755) //nolint:gosec // G301: Test directory permissions
	require.NoError(t, err)

	// Write base config
	baseConfig := `allowed_domains:
  - "*.anthropic.com"
  - "github.com"

env:
  DEBUG: "false"
  LOG_LEVEL: "warn"

volumes:
  - "/host/data:/container/data"

networks:
  - "base-net"
`
	baseConfigPath := filepath.Join(devAgentDir, "config.yml")
	err = os.WriteFile(baseConfigPath, []byte(baseConfig), 0644) //nolint:gosec // G306: Test file permissions
	require.NoError(t, err)

	// Write local config (override)
	localConfig := `allowed_domains:
  - "*.openai.com"
  - "api.example.com"

env:
  DEBUG: "true"
  API_TIMEOUT: "60"

volumes:
  - "/host/cache:/container/cache:ro"

networks:
  - "local-net"
`
	localConfigPath := filepath.Join(devAgentDir, "config.local.yml")
	err = os.WriteFile(localConfigPath, []byte(localConfig), 0644) //nolint:gosec // G306: Test file permissions
	require.NoError(t, err)

	// Load the merged config
	cfg, err := LoadDevAgentConfig(tmpDir)
	require.NoError(t, err)
	require.NotNil(t, cfg)

	// Verify allowed domains were replaced (not appended)
	assert.Len(t, cfg.AllowedDomains, 2)
	assert.Contains(t, cfg.AllowedDomains, "*.openai.com")
	assert.Contains(t, cfg.AllowedDomains, "api.example.com")

	// Verify env vars were merged (field-level override)
	assert.Len(t, cfg.Env, 3)
	assert.Equal(t, "true", cfg.Env["DEBUG"])     // overridden
	assert.Equal(t, "warn", cfg.Env["LOG_LEVEL"]) // preserved from base
	assert.Equal(t, "60", cfg.Env["API_TIMEOUT"]) // added from local

	// Verify volumes were replaced (not appended)
	assert.Len(t, cfg.Volumes, 1)
	assert.Contains(t, cfg.Volumes, "/host/cache:/container/cache:ro")

	// Verify networks were replaced (not appended)
	assert.Len(t, cfg.Networks, 1)
	assert.Contains(t, cfg.Networks, "local-net")
}

func TestMergeDevAgentConfigs(t *testing.T) {
	tests := []struct {
		name     string
		base     *DevAgentConfig
		override *DevAgentConfig
		expected *DevAgentConfig
	}{
		{
			name:     "both nil",
			base:     nil,
			override: nil,
			expected: &DevAgentConfig{},
		},
		{
			name: "base nil, override present",
			base: nil,
			override: &DevAgentConfig{
				AllowedDomains: []string{"example.com"},
				Env:            map[string]string{"KEY": "value"},
			},
			expected: &DevAgentConfig{
				AllowedDomains: []string{"example.com"},
				Env:            map[string]string{"KEY": "value"},
			},
		},
		{
			name: "base present, override nil",
			base: &DevAgentConfig{
				AllowedDomains: []string{"example.com"},
				Env:            map[string]string{"KEY": "value"},
			},
			override: nil,
			expected: &DevAgentConfig{
				AllowedDomains: []string{"example.com"},
				Env:            map[string]string{"KEY": "value"},
			},
		},
		{
			name: "lists replace entirely",
			base: &DevAgentConfig{
				AllowedDomains: []string{"base1.com", "base2.com"},
				Volumes:        []string{"/base:/base"},
				Networks:       []string{"base-net"},
			},
			override: &DevAgentConfig{
				AllowedDomains: []string{"override.com"},
				Volumes:        []string{"/override:/override"},
				Networks:       []string{"override-net"},
			},
			expected: &DevAgentConfig{
				AllowedDomains: []string{"override.com"},
				Env:            map[string]string{},
				Volumes:        []string{"/override:/override"},
				Networks:       []string{"override-net"},
			},
		},
		{
			name: "maps merge field-level",
			base: &DevAgentConfig{
				Env: map[string]string{
					"BASE_KEY":   "base",
					"SHARED_KEY": "base",
				},
			},
			override: &DevAgentConfig{
				Env: map[string]string{
					"OVERRIDE_KEY": "override",
					"SHARED_KEY":   "override",
				},
			},
			expected: &DevAgentConfig{
				Env: map[string]string{
					"BASE_KEY":     "base",
					"SHARED_KEY":   "override",
					"OVERRIDE_KEY": "override",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mergeDevAgentConfigs(tt.base, tt.override)
			assert.Equal(t, tt.expected.AllowedDomains, result.AllowedDomains)
			assert.Equal(t, tt.expected.Env, result.Env)
			assert.Equal(t, tt.expected.Volumes, result.Volumes)
			assert.Equal(t, tt.expected.Networks, result.Networks)
		})
	}
}
