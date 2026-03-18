package config

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestDefaultUserConfig(t *testing.T) {
	cfg := DefaultUserConfig()

	require.NotNil(t, cfg)
	require.NotNil(t, cfg.Defaults)
	require.NotNil(t, cfg.Defaults.Tool)
	require.NotNil(t, cfg.Defaults.Provider)
	require.NotNil(t, cfg.Defaults.Model)

	assert.Equal(t, DefaultTool, *cfg.Defaults.Tool)
	assert.Equal(t, DefaultProvider, *cfg.Defaults.Provider)
	assert.Equal(t, DefaultModel, *cfg.Defaults.Model)

	// Tools and Providers maps should be initialized (not nil)
	assert.NotNil(t, cfg.Tools)
	assert.NotNil(t, cfg.Providers)
	assert.Empty(t, cfg.Tools)
	assert.Empty(t, cfg.Providers)

	// ModelTiers can be nil (not required)
	// Just verify it doesn't panic
	_ = cfg.ModelTiers
}

func TestZeroValueUserConfig(t *testing.T) {
	// Zero-value config should be valid (all pointer fields are nil)
	var cfg UserConfig

	// Should not panic when accessed
	assert.Nil(t, cfg.Defaults)
	assert.Nil(t, cfg.Tools)
	assert.Nil(t, cfg.Providers)
	assert.Nil(t, cfg.ModelTiers)

	// Should be usable in resolution (will use all defaults)
	resolved, err := ResolveCode(nil, &cfg)
	require.NoError(t, err)
	assert.Equal(t, DefaultTool, resolved.Tool)
	assert.Equal(t, DefaultProvider, resolved.Provider)
	assert.Equal(t, DefaultModel, resolved.Model)
}

func TestZeroValueProjectConfig(t *testing.T) {
	// Zero-value config should be valid
	var cfg ProjectConfig

	assert.Nil(t, cfg.Defaults)
	assert.Nil(t, cfg.Pipeline)
	assert.Nil(t, cfg.LocalOverrides)

	// Should be usable in resolution
	resolved, err := ResolveCode(&cfg, nil)
	require.NoError(t, err)
	assert.Equal(t, DefaultTool, resolved.Tool)
	assert.Equal(t, DefaultProvider, resolved.Provider)
	assert.Equal(t, DefaultModel, resolved.Model)
}

func TestUserConfigYAMLRoundTrip(t *testing.T) {
	original := &UserConfig{
		Defaults: &DefaultsConfig{
			Tool:     strPtr("test-tool"),
			Provider: strPtr("test-provider"),
			Model:    strPtr("test-model"),
		},
		Tools: map[string]*ToolConfig{
			"test-tool": {
				ModelTiers: &struct {
					Fast     *string `yaml:"fast"`
					Standard *string `yaml:"standard"`
					Deep     *string `yaml:"deep"`
				}{
					Fast:     strPtr("fast-model"),
					Standard: strPtr("standard-model"),
					Deep:     strPtr("deep-model"),
				},
				MaxTokens:   intPtr(8192),
				Temperature: floatPtr(0.7),
			},
		},
		Providers: map[string]*ProviderConfig{
			"test-provider": {
				APIKeyEnv: strPtr("TEST_API_KEY"),
				BaseURL:   strPtr("https://api.test.com"),
				Timeout:   intPtr(300),
			},
		},
		ModelTiers: &struct {
			Fast     *string `yaml:"fast"`
			Standard *string `yaml:"standard"`
			Deep     *string `yaml:"deep"`
		}{
			Fast:     strPtr("global-fast"),
			Standard: strPtr("global-standard"),
			Deep:     strPtr("global-deep"),
		},
	}

	// Marshal to YAML
	data, err := yaml.Marshal(original)
	require.NoError(t, err)

	// Unmarshal back
	var roundtrip UserConfig
	err = yaml.Unmarshal(data, &roundtrip)
	require.NoError(t, err)

	// Verify structure
	require.NotNil(t, roundtrip.Defaults)
	require.NotNil(t, roundtrip.Defaults.Tool)
	require.NotNil(t, roundtrip.Defaults.Provider)
	require.NotNil(t, roundtrip.Defaults.Model)
	assert.Equal(t, "test-tool", *roundtrip.Defaults.Tool)
	assert.Equal(t, "test-provider", *roundtrip.Defaults.Provider)
	assert.Equal(t, "test-model", *roundtrip.Defaults.Model)

	// Verify tools
	require.NotNil(t, roundtrip.Tools)
	require.Contains(t, roundtrip.Tools, "test-tool")
	toolCfg := roundtrip.Tools["test-tool"]
	require.NotNil(t, toolCfg)
	require.NotNil(t, toolCfg.ModelTiers)
	assert.Equal(t, "fast-model", *toolCfg.ModelTiers.Fast)
	assert.Equal(t, "standard-model", *toolCfg.ModelTiers.Standard)
	assert.Equal(t, "deep-model", *toolCfg.ModelTiers.Deep)
	require.NotNil(t, toolCfg.MaxTokens)
	assert.Equal(t, 8192, *toolCfg.MaxTokens)
	require.NotNil(t, toolCfg.Temperature)
	assert.Equal(t, 0.7, *toolCfg.Temperature)

	// Verify providers
	require.NotNil(t, roundtrip.Providers)
	require.Contains(t, roundtrip.Providers, "test-provider")
	providerCfg := roundtrip.Providers["test-provider"]
	require.NotNil(t, providerCfg)
	assert.Equal(t, "TEST_API_KEY", *providerCfg.APIKeyEnv)
	assert.Equal(t, "https://api.test.com", *providerCfg.BaseURL)
	assert.Equal(t, 300, *providerCfg.Timeout)

	// Verify model tiers
	require.NotNil(t, roundtrip.ModelTiers)
	assert.Equal(t, "global-fast", *roundtrip.ModelTiers.Fast)
	assert.Equal(t, "global-standard", *roundtrip.ModelTiers.Standard)
	assert.Equal(t, "global-deep", *roundtrip.ModelTiers.Deep)
}

func TestProjectConfigYAMLRoundTrip(t *testing.T) {
	original := &ProjectConfig{
		Defaults: &DefaultsConfig{
			Tool:     strPtr("project-tool"),
			Provider: strPtr("project-provider"),
			Model:    strPtr("project-model"),
		},
		Pipeline: map[string]*PipelineStepConfig{
			"code": {
				Tool:     strPtr("code-tool"),
				Provider: strPtr("code-provider"),
				Model:    strPtr("code-model"),
				Timeout:  intPtr(600),
			},
			"verify": {
				Model:   strPtr("verify-model"),
				Timeout: intPtr(300),
			},
		},
		LocalOverrides: map[string]interface{}{
			"dev_mode":    true,
			"skip_verify": false,
			"custom_flag": "value",
		},
	}

	// Marshal to YAML
	data, err := yaml.Marshal(original)
	require.NoError(t, err)

	// Unmarshal back
	var roundtrip ProjectConfig
	err = yaml.Unmarshal(data, &roundtrip)
	require.NoError(t, err)

	// Verify defaults
	require.NotNil(t, roundtrip.Defaults)
	assert.Equal(t, "project-tool", *roundtrip.Defaults.Tool)
	assert.Equal(t, "project-provider", *roundtrip.Defaults.Provider)
	assert.Equal(t, "project-model", *roundtrip.Defaults.Model)

	// Verify pipeline steps
	require.NotNil(t, roundtrip.Pipeline)
	require.Contains(t, roundtrip.Pipeline, "code")
	require.Contains(t, roundtrip.Pipeline, "verify")

	codeStep := roundtrip.Pipeline["code"]
	require.NotNil(t, codeStep)
	assert.Equal(t, "code-tool", *codeStep.Tool)
	assert.Equal(t, "code-provider", *codeStep.Provider)
	assert.Equal(t, "code-model", *codeStep.Model)
	assert.Equal(t, 600, *codeStep.Timeout)

	verifyStep := roundtrip.Pipeline["verify"]
	require.NotNil(t, verifyStep)
	assert.Equal(t, "verify-model", *verifyStep.Model)
	assert.Equal(t, 300, *verifyStep.Timeout)

	// Verify local overrides
	require.NotNil(t, roundtrip.LocalOverrides)
	assert.Equal(t, true, roundtrip.LocalOverrides["dev_mode"])
	assert.Equal(t, false, roundtrip.LocalOverrides["skip_verify"])
	assert.Equal(t, "value", roundtrip.LocalOverrides["custom_flag"])
}

func TestUserConfigJSONRoundTrip(t *testing.T) {
	original := &UserConfig{
		Defaults: &DefaultsConfig{
			Tool:     strPtr("json-tool"),
			Provider: strPtr("json-provider"),
			Model:    strPtr("json-model"),
		},
		Tools: map[string]*ToolConfig{
			"json-tool": {
				MaxTokens: intPtr(4096),
			},
		},
	}

	// Marshal to JSON
	data, err := json.Marshal(original)
	require.NoError(t, err)

	// Unmarshal back
	var roundtrip UserConfig
	err = json.Unmarshal(data, &roundtrip)
	require.NoError(t, err)

	// Verify
	require.NotNil(t, roundtrip.Defaults)
	assert.Equal(t, "json-tool", *roundtrip.Defaults.Tool)
	assert.Equal(t, "json-provider", *roundtrip.Defaults.Provider)
	assert.Equal(t, "json-model", *roundtrip.Defaults.Model)

	require.NotNil(t, roundtrip.Tools)
	require.Contains(t, roundtrip.Tools, "json-tool")
	assert.Equal(t, 4096, *roundtrip.Tools["json-tool"].MaxTokens)
}

func TestResolvedConfigStructure(t *testing.T) {
	// Verify ResolvedConfig has the expected fields and they are non-pointer
	resolved := &ResolvedConfig{
		Tool:     "test-tool",
		Provider: "test-provider",
		Model:    "test-model",
	}

	assert.Equal(t, "test-tool", resolved.Tool)
	assert.Equal(t, "test-provider", resolved.Provider)
	assert.Equal(t, "test-model", resolved.Model)

	// Zero value should work
	var zero ResolvedConfig
	assert.Empty(t, zero.Tool)
	assert.Empty(t, zero.Provider)
	assert.Empty(t, zero.Model)
}

func TestConfigConstants(t *testing.T) {
	// Verify constants are set to expected values
	assert.NotEmpty(t, DefaultTool)
	assert.NotEmpty(t, DefaultProvider)
	assert.NotEmpty(t, DefaultModel)

	// Verify they're reasonable values (not test placeholders)
	assert.Contains(t, DefaultTool, "claude")
	assert.Equal(t, "anthropic", DefaultProvider)
	assert.Contains(t, DefaultModel, "claude")
}

func TestToolConfigOptionalFields(t *testing.T) {
	// Tool config with only model tiers
	cfg := &ToolConfig{
		ModelTiers: &struct {
			Fast     *string `yaml:"fast"`
			Standard *string `yaml:"standard"`
			Deep     *string `yaml:"deep"`
		}{
			Fast: strPtr("fast-model"),
		},
	}

	assert.NotNil(t, cfg.ModelTiers)
	assert.Nil(t, cfg.MaxTokens)
	assert.Nil(t, cfg.Temperature)

	// Marshal and unmarshal
	data, err := yaml.Marshal(cfg)
	require.NoError(t, err)

	var roundtrip ToolConfig
	err = yaml.Unmarshal(data, &roundtrip)
	require.NoError(t, err)

	assert.NotNil(t, roundtrip.ModelTiers)
	assert.Equal(t, "fast-model", *roundtrip.ModelTiers.Fast)
}

func TestProviderConfigOptionalFields(t *testing.T) {
	// Provider config with only some fields
	cfg := &ProviderConfig{
		APIKeyEnv: strPtr("API_KEY"),
	}

	assert.NotNil(t, cfg.APIKeyEnv)
	assert.Nil(t, cfg.BaseURL)
	assert.Nil(t, cfg.Timeout)

	// Marshal and unmarshal
	data, err := yaml.Marshal(cfg)
	require.NoError(t, err)

	var roundtrip ProviderConfig
	err = yaml.Unmarshal(data, &roundtrip)
	require.NoError(t, err)

	assert.Equal(t, "API_KEY", *roundtrip.APIKeyEnv)
	assert.Nil(t, roundtrip.BaseURL)
}

func TestPipelineStepConfigPartialFields(t *testing.T) {
	// Step config with only model override
	cfg := &PipelineStepConfig{
		Model: strPtr("step-model"),
	}

	assert.Nil(t, cfg.Tool)
	assert.Nil(t, cfg.Provider)
	assert.NotNil(t, cfg.Model)
	assert.Nil(t, cfg.Timeout)

	// Marshal and unmarshal
	data, err := yaml.Marshal(cfg)
	require.NoError(t, err)

	var roundtrip PipelineStepConfig
	err = yaml.Unmarshal(data, &roundtrip)
	require.NoError(t, err)

	assert.Equal(t, "step-model", *roundtrip.Model)
	assert.Nil(t, roundtrip.Tool)
}

// Helper functions for tests
func intPtr(i int) *int {
	return &i
}

func floatPtr(f float64) *float64 {
	return &f
}
