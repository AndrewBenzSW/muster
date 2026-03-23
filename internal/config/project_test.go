package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadProjectConfig_BaseOnly(t *testing.T) {
	// Create temp directory with only base config
	tmpDir := t.TempDir()
	musterDir := filepath.Join(tmpDir, ".muster")
	require.NoError(t, os.MkdirAll(musterDir, 0755)) //nolint:gosec // G301: Test directory permissions

	baseConfig := `defaults:
  tool: claude-code
  model: claude-sonnet-4.5

pipeline:
  plan:
    tool: claude-code
    model: claude-opus-4
`
	require.NoError(t, os.WriteFile(filepath.Join(musterDir, "config.yml"), []byte(baseConfig), 0644)) //nolint:gosec // G306: Test file permissions

	config, err := LoadProjectConfig(tmpDir)
	require.NoError(t, err)
	require.NotNil(t, config)
	require.NotNil(t, config.Defaults)
	assert.Equal(t, "claude-code", *config.Defaults.Tool)
	assert.Equal(t, "claude-sonnet-4.5", *config.Defaults.Model)
	assert.Nil(t, config.Defaults.Provider)
	assert.NotNil(t, config.Pipeline["plan"])
	assert.Equal(t, "claude-code", *config.Pipeline["plan"].Tool)
}

func TestLoadProjectConfig_BackwardCompat(t *testing.T) {
	// Create temp directory with .dev-agent config (backward compatibility)
	tmpDir := t.TempDir()
	devAgentDir := filepath.Join(tmpDir, ".dev-agent")
	require.NoError(t, os.MkdirAll(devAgentDir, 0755)) //nolint:gosec // G301: Test directory permissions

	baseConfig := `defaults:
  tool: cursor
  provider: openai
`
	require.NoError(t, os.WriteFile(filepath.Join(devAgentDir, "config.yml"), []byte(baseConfig), 0644)) //nolint:gosec // G306: Test file permissions

	config, err := LoadProjectConfig(tmpDir)
	require.NoError(t, err)
	require.NotNil(t, config)
	require.NotNil(t, config.Defaults)
	assert.Equal(t, "cursor", *config.Defaults.Tool)
	assert.Equal(t, "openai", *config.Defaults.Provider)
}

func TestLoadProjectConfig_DeepMerge(t *testing.T) {
	tests := []struct {
		name        string
		baseConfig  string
		localConfig string
		verifyFunc  func(t *testing.T, config *ProjectConfig)
	}{
		{
			name: "field-level override in defaults",
			baseConfig: `defaults:
  tool: cursor
  provider: openai
  model: gpt-4o
`,
			localConfig: `defaults:
  model: claude-opus-4
`,
			verifyFunc: func(t *testing.T, config *ProjectConfig) {
				require.NotNil(t, config.Defaults)
				// tool and provider from base preserved
				assert.Equal(t, "cursor", *config.Defaults.Tool)
				assert.Equal(t, "openai", *config.Defaults.Provider)
				// model overridden by local
				assert.Equal(t, "claude-opus-4", *config.Defaults.Model)
			},
		},
		{
			name: "list replacement not append",
			baseConfig: `pipeline:
  plan:
    tool: claude-code
    model: claude-opus-4
  execute:
    tool: cursor
    model: gpt-4o
`,
			localConfig: `pipeline:
  plan:
    tool: claude-code
    model: claude-sonnet-4.5
`,
			verifyFunc: func(t *testing.T, config *ProjectConfig) {
				// plan step replaced entirely
				assert.NotNil(t, config.Pipeline["plan"])
				assert.Equal(t, "claude-code", *config.Pipeline["plan"].Tool)
				assert.Equal(t, "claude-sonnet-4.5", *config.Pipeline["plan"].Model)
				// execute step from base still present
				assert.NotNil(t, config.Pipeline["execute"])
				assert.Equal(t, "cursor", *config.Pipeline["execute"].Tool)
			},
		},
		{
			name: "new section addition via override",
			baseConfig: `defaults:
  tool: claude-code
`,
			localConfig: `defaults:
  provider: anthropic

pipeline:
  plan:
    tool: cursor
    model: gpt-4o

local_overrides:
  dev_mode: true
`,
			verifyFunc: func(t *testing.T, config *ProjectConfig) {
				// defaults merged
				require.NotNil(t, config.Defaults)
				assert.Equal(t, "claude-code", *config.Defaults.Tool)
				assert.Equal(t, "anthropic", *config.Defaults.Provider)
				// new pipeline section added
				assert.NotNil(t, config.Pipeline["plan"])
				// new local_overrides section added
				assert.True(t, config.LocalOverrides["dev_mode"].(bool))
			},
		},
		{
			name: "empty override no changes",
			baseConfig: `defaults:
  tool: claude-code
  provider: anthropic
  model: claude-sonnet-4.5

pipeline:
  plan:
    tool: claude-code
`,
			localConfig: ``,
			verifyFunc: func(t *testing.T, config *ProjectConfig) {
				// All base values preserved
				require.NotNil(t, config.Defaults)
				assert.Equal(t, "claude-code", *config.Defaults.Tool)
				assert.Equal(t, "anthropic", *config.Defaults.Provider)
				assert.Equal(t, "claude-sonnet-4.5", *config.Defaults.Model)
				assert.NotNil(t, config.Pipeline["plan"])
			},
		},
		{
			name: "override all defaults fields",
			baseConfig: `defaults:
  tool: cursor
  provider: openai
  model: gpt-4o
`,
			localConfig: `defaults:
  tool: claude-code
  provider: anthropic
  model: claude-opus-4
`,
			verifyFunc: func(t *testing.T, config *ProjectConfig) {
				require.NotNil(t, config.Defaults)
				assert.Equal(t, "claude-code", *config.Defaults.Tool)
				assert.Equal(t, "anthropic", *config.Defaults.Provider)
				assert.Equal(t, "claude-opus-4", *config.Defaults.Model)
			},
		},
		{
			name: "local overrides map replacement",
			baseConfig: `local_overrides:
  dev_mode: false
  skip_verify: false
  extra_flag: true
`,
			localConfig: `local_overrides:
  dev_mode: true
  skip_verify: true
`,
			verifyFunc: func(t *testing.T, config *ProjectConfig) {
				// Override values replace base values
				assert.True(t, config.LocalOverrides["dev_mode"].(bool))
				assert.True(t, config.LocalOverrides["skip_verify"].(bool))
				// Base value still present if not overridden
				assert.True(t, config.LocalOverrides["extra_flag"].(bool))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			musterDir := filepath.Join(tmpDir, ".muster")
			require.NoError(t, os.MkdirAll(musterDir, 0755)) //nolint:gosec // G301: Test directory permissions

			require.NoError(t, os.WriteFile(filepath.Join(musterDir, "config.yml"), []byte(tt.baseConfig), 0644))        //nolint:gosec // G306: Test file permissions
			require.NoError(t, os.WriteFile(filepath.Join(musterDir, "config.local.yml"), []byte(tt.localConfig), 0644)) //nolint:gosec // G306: Test file permissions

			config, err := LoadProjectConfig(tmpDir)
			require.NoError(t, err)
			require.NotNil(t, config)

			tt.verifyFunc(t, config)
		})
	}
}

func TestLoadProjectConfig_Errors(t *testing.T) {
	tests := []struct {
		name        string
		setup       func(t *testing.T) string
		wantErr     bool
		errContains string
	}{
		{
			name: "missing both base configs returns default",
			setup: func(t *testing.T) string {
				return t.TempDir()
			},
			wantErr:     false,
			errContains: "",
		},
		{
			name: "malformed base config",
			setup: func(t *testing.T) string {
				tmpDir := t.TempDir()
				musterDir := filepath.Join(tmpDir, ".muster")
				require.NoError(t, os.MkdirAll(musterDir, 0755))                                                                    //nolint:gosec // G301: Test directory permissions
				require.NoError(t, os.WriteFile(filepath.Join(musterDir, "config.yml"), []byte("invalid: yaml: content: ["), 0644)) //nolint:gosec // G306: Test file permissions
				return tmpDir
			},
			wantErr:     true,
			errContains: "failed to parse",
		},
		{
			name: "malformed local override config",
			setup: func(t *testing.T) string {
				tmpDir := t.TempDir()
				musterDir := filepath.Join(tmpDir, ".muster")
				require.NoError(t, os.MkdirAll(musterDir, 0755))                                                                         //nolint:gosec // G301: Test directory permissions
				require.NoError(t, os.WriteFile(filepath.Join(musterDir, "config.yml"), []byte("defaults:\n  tool: claude-code"), 0644)) //nolint:gosec // G306: Test file permissions
				require.NoError(t, os.WriteFile(filepath.Join(musterDir, "config.local.yml"), []byte("invalid: [yaml"), 0644))           //nolint:gosec // G306: Test file permissions
				return tmpDir
			},
			wantErr:     true,
			errContains: "failed to load local config",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := tt.setup(t)
			config, err := LoadProjectConfig(dir)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
				assert.Nil(t, config)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, config)
			}
		})
	}
}

func TestMergeProjectConfigs(t *testing.T) {
	tests := []struct {
		name       string
		base       *ProjectConfig
		override   *ProjectConfig
		verifyFunc func(t *testing.T, result *ProjectConfig)
	}{
		{
			name: "nil base with override",
			base: nil,
			override: &ProjectConfig{
				Defaults: &DefaultsConfig{
					Tool: strPtr("claude-code"),
				},
			},
			verifyFunc: func(t *testing.T, result *ProjectConfig) {
				require.NotNil(t, result)
				require.NotNil(t, result.Defaults)
				assert.Equal(t, "claude-code", *result.Defaults.Tool)
			},
		},
		{
			name: "base with nil override",
			base: &ProjectConfig{
				Defaults: &DefaultsConfig{
					Tool: strPtr("cursor"),
				},
			},
			override: nil,
			verifyFunc: func(t *testing.T, result *ProjectConfig) {
				require.NotNil(t, result)
				require.NotNil(t, result.Defaults)
				assert.Equal(t, "cursor", *result.Defaults.Tool)
			},
		},
		{
			name:     "both nil",
			base:     nil,
			override: nil,
			verifyFunc: func(t *testing.T, result *ProjectConfig) {
				require.NotNil(t, result)
				assert.Nil(t, result.Defaults)
			},
		},
		{
			name: "field-level merge in defaults",
			base: &ProjectConfig{
				Defaults: &DefaultsConfig{
					Tool:     strPtr("cursor"),
					Provider: strPtr("openai"),
					Model:    strPtr("gpt-4o"),
				},
			},
			override: &ProjectConfig{
				Defaults: &DefaultsConfig{
					Model: strPtr("claude-opus-4"),
				},
			},
			verifyFunc: func(t *testing.T, result *ProjectConfig) {
				require.NotNil(t, result.Defaults)
				assert.Equal(t, "cursor", *result.Defaults.Tool)
				assert.Equal(t, "openai", *result.Defaults.Provider)
				assert.Equal(t, "claude-opus-4", *result.Defaults.Model)
			},
		},
		{
			name: "pipeline step replacement",
			base: &ProjectConfig{
				Pipeline: map[string]*PipelineStepConfig{
					"plan": {
						Tool:  strPtr("claude-code"),
						Model: strPtr("claude-opus-4"),
					},
					"execute": {
						Tool:  strPtr("cursor"),
						Model: strPtr("gpt-4o"),
					},
				},
			},
			override: &ProjectConfig{
				Pipeline: map[string]*PipelineStepConfig{
					"plan": {
						Tool:  strPtr("claude-code"),
						Model: strPtr("claude-sonnet-4.5"),
					},
				},
			},
			verifyFunc: func(t *testing.T, result *ProjectConfig) {
				require.NotNil(t, result.Pipeline)
				// plan replaced
				assert.Equal(t, "claude-sonnet-4.5", *result.Pipeline["plan"].Model)
				// execute preserved from base
				assert.NotNil(t, result.Pipeline["execute"])
				assert.Equal(t, "gpt-4o", *result.Pipeline["execute"].Model)
			},
		},
		{
			name: "add new pipeline step via override",
			base: &ProjectConfig{
				Pipeline: map[string]*PipelineStepConfig{
					"plan": {
						Tool: strPtr("claude-code"),
					},
				},
			},
			override: &ProjectConfig{
				Pipeline: map[string]*PipelineStepConfig{
					"verify": {
						Tool:  strPtr("cursor"),
						Model: strPtr("gpt-4o"),
					},
				},
			},
			verifyFunc: func(t *testing.T, result *ProjectConfig) {
				require.NotNil(t, result.Pipeline)
				// base plan still there
				assert.NotNil(t, result.Pipeline["plan"])
				// new verify added
				assert.NotNil(t, result.Pipeline["verify"])
				assert.Equal(t, "cursor", *result.Pipeline["verify"].Tool)
			},
		},
		{
			name: "local_overrides merge",
			base: &ProjectConfig{
				LocalOverrides: map[string]interface{}{
					"dev_mode":    false,
					"skip_verify": false,
				},
			},
			override: &ProjectConfig{
				LocalOverrides: map[string]interface{}{
					"dev_mode": true,
					"new_flag": "value",
				},
			},
			verifyFunc: func(t *testing.T, result *ProjectConfig) {
				require.NotNil(t, result.LocalOverrides)
				// overridden value
				assert.True(t, result.LocalOverrides["dev_mode"].(bool))
				// base value preserved
				assert.False(t, result.LocalOverrides["skip_verify"].(bool))
				// new value added
				assert.Equal(t, "value", result.LocalOverrides["new_flag"].(string))
			},
		},
		{
			name: "merge_strategy from base only",
			base: &ProjectConfig{
				MergeStrategy: strPtr("direct"),
			},
			override: &ProjectConfig{},
			verifyFunc: func(t *testing.T, result *ProjectConfig) {
				require.NotNil(t, result.MergeStrategy)
				assert.Equal(t, "direct", *result.MergeStrategy)
			},
		},
		{
			name: "merge_strategy override replaces base",
			base: &ProjectConfig{
				MergeStrategy: strPtr("direct"),
			},
			override: &ProjectConfig{
				MergeStrategy: strPtr("github-pr"),
			},
			verifyFunc: func(t *testing.T, result *ProjectConfig) {
				require.NotNil(t, result.MergeStrategy)
				assert.Equal(t, "github-pr", *result.MergeStrategy)
			},
		},
		{
			name: "merge_strategy nil in both",
			base: &ProjectConfig{
				Defaults: &DefaultsConfig{
					Tool: strPtr("claude-code"),
				},
			},
			override: &ProjectConfig{},
			verifyFunc: func(t *testing.T, result *ProjectConfig) {
				assert.Nil(t, result.MergeStrategy)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mergeProjectConfigs(tt.base, tt.override)
			tt.verifyFunc(t, result)
		})
	}
}

func TestLoadProjectConfig_WithTestdata(t *testing.T) {
	tests := []struct {
		name       string
		setupFunc  func(t *testing.T) string
		verifyFunc func(t *testing.T, config *ProjectConfig)
	}{
		{
			name: "project-full.yml",
			setupFunc: func(t *testing.T) string {
				tmpDir := t.TempDir()
				musterDir := filepath.Join(tmpDir, ".muster")
				require.NoError(t, os.MkdirAll(musterDir, 0755)) //nolint:gosec // G301: Test directory permissions

				// Copy project-full.yml
				data, err := os.ReadFile("testdata/project-full.yml") //nolint:gosec // G304: Test fixture path
				require.NoError(t, err)
				require.NoError(t, os.WriteFile(filepath.Join(musterDir, "config.yml"), data, 0644)) //nolint:gosec // G306: Test file permissions

				return tmpDir
			},
			verifyFunc: func(t *testing.T, config *ProjectConfig) {
				require.NotNil(t, config.Defaults)
				assert.Equal(t, "cursor", *config.Defaults.Tool)
				assert.Equal(t, "openai", *config.Defaults.Provider)
				assert.Equal(t, "gpt-4o", *config.Defaults.Model)

				assert.NotNil(t, config.Pipeline["plan"])
				assert.Equal(t, "claude-code", *config.Pipeline["plan"].Tool)
				assert.Equal(t, "claude-opus-4", *config.Pipeline["plan"].Model)
			},
		},
		{
			name: "project-minimal.yml",
			setupFunc: func(t *testing.T) string {
				tmpDir := t.TempDir()
				musterDir := filepath.Join(tmpDir, ".muster")
				require.NoError(t, os.MkdirAll(musterDir, 0755)) //nolint:gosec // G301: Test directory permissions

				// Copy project-minimal.yml
				data, err := os.ReadFile("testdata/project-minimal.yml") //nolint:gosec // G304: Test fixture path
				require.NoError(t, err)
				require.NoError(t, os.WriteFile(filepath.Join(musterDir, "config.yml"), data, 0644)) //nolint:gosec // G306: Test file permissions

				return tmpDir
			},
			verifyFunc: func(t *testing.T, config *ProjectConfig) {
				require.NotNil(t, config.Defaults)
				assert.Equal(t, "claude-code", *config.Defaults.Tool)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := tt.setupFunc(t)
			config, err := LoadProjectConfig(dir)
			require.NoError(t, err)
			tt.verifyFunc(t, config)
		})
	}
}

// strPtr is a helper to create string pointers for tests
func strPtr(s string) *string {
	return &s
}
