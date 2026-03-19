package config

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestDefaultUserConfig(t *testing.T) {
	cfg := DefaultUserConfig()

	require.NotNil(t, cfg)

	// Defaults is nil — tool, provider, and model are resolved by ResolveStep
	assert.Nil(t, cfg.Defaults)

	// Tools and Providers maps should be initialized (not nil)
	assert.NotNil(t, cfg.Tools)
	assert.NotNil(t, cfg.Providers)
	assert.Empty(t, cfg.Tools)
	assert.Empty(t, cfg.Providers)

	// ModelTiers can be nil (not required)
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
	assert.Equal(t, "sonnet", DefaultModel)
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

func TestValidateMultipleMissingProviders(t *testing.T) {
	cfg := &Config{
		User: &UserConfig{
			Defaults: &DefaultsConfig{
				Provider: strPtr("missing-provider-1"),
			},
		},
		Project: &ProjectConfig{
			Pipeline: map[string]*PipelineStepConfig{
				"code": {
					Provider: strPtr("missing-provider-2"),
				},
			},
		},
	}

	errs := cfg.Validate()

	// Should collect multiple provider errors
	require.NotEmpty(t, errs)
	assert.GreaterOrEqual(t, len(errs), 2, "should collect at least 2 missing provider errors")

	// Check error messages contain actionable instructions
	errStrs := make([]string, len(errs))
	for i, err := range errs {
		errStrs[i] = err.Error()
	}

	allErrors := strings.Join(errStrs, "\n")
	assert.Contains(t, allErrors, "missing-provider-1")
	assert.Contains(t, allErrors, "missing-provider-2")
	assert.Contains(t, allErrors, "~/.config/muster/config.yml")
}

func TestValidateMissingEnvVar(t *testing.T) {
	// Create a provider that requires an env var that doesn't exist
	uniqueEnvVar := "MUSTER_TEST_MISSING_KEY_12345"

	// Ensure the env var is NOT set
	_ = os.Unsetenv(uniqueEnvVar)

	cfg := &Config{
		User: &UserConfig{
			Defaults: &DefaultsConfig{
				Provider: strPtr("test-provider"),
			},
			Providers: map[string]*ProviderConfig{
				"test-provider": {
					APIKeyEnv: strPtr(uniqueEnvVar),
				},
			},
		},
	}

	errs := cfg.Validate()

	// Should have error for missing env var
	require.NotEmpty(t, errs)

	// Find the env var error
	var envVarError error
	for _, err := range errs {
		if strings.Contains(err.Error(), uniqueEnvVar) {
			envVarError = err
			break
		}
	}
	require.NotNil(t, envVarError, "should have error for missing env var")

	// Check error message contains actionable fix instructions
	errMsg := envVarError.Error()
	assert.Contains(t, errMsg, uniqueEnvVar, "should mention the env var name")
	assert.Contains(t, errMsg, "export", "should include export command")
	assert.Contains(t, errMsg, "~/.config/muster/config.yml", "should mention config file location")
}

func TestValidateUnknownTool(t *testing.T) {
	cfg := &Config{
		User: &UserConfig{
			// No tools defined
			Tools: map[string]*ToolConfig{},
		},
		Project: &ProjectConfig{
			Defaults: &DefaultsConfig{
				Tool: strPtr("unknown-tool"),
			},
		},
	}

	errs := cfg.Validate()

	// Should have error for unknown tool
	require.NotEmpty(t, errs)

	// Find the tool error
	var toolError error
	for _, err := range errs {
		if strings.Contains(err.Error(), "unknown-tool") {
			toolError = err
			break
		}
	}
	require.NotNil(t, toolError, "should have error for unknown tool")

	// Check error message contains actionable fix instructions
	errMsg := toolError.Error()
	assert.Contains(t, errMsg, "unknown-tool", "should mention the tool name")
	assert.Contains(t, errMsg, "~/.config/muster/config.yml", "should mention config file location")
	assert.Contains(t, errMsg, "under tools", "should mention tools section")
}

func TestValidateWithEnvVarSet(t *testing.T) {
	// Create a provider with an env var that IS set
	uniqueEnvVar := "MUSTER_TEST_PRESENT_KEY_54321"
	_ = os.Setenv(uniqueEnvVar, "test-value")
	defer func() { _ = os.Unsetenv(uniqueEnvVar) }()

	cfg := &Config{
		User: &UserConfig{
			Defaults: &DefaultsConfig{
				Provider: strPtr("test-provider"),
			},
			Providers: map[string]*ProviderConfig{
				"test-provider": {
					APIKeyEnv: strPtr(uniqueEnvVar),
				},
			},
		},
	}

	errs := cfg.Validate()

	// Should NOT have error for this env var (may have other errors)
	for _, err := range errs {
		assert.NotContains(t, err.Error(), uniqueEnvVar, "should not error on set env var")
	}
}

func TestValidateNilConfig(t *testing.T) {
	var cfg *Config
	errs := cfg.Validate()

	// Nil config should not panic and should return nil
	assert.Nil(t, errs)
}

func TestValidateDefaultToolAndProvider(t *testing.T) {
	// Using default tool and provider should validate successfully
	cfg := &Config{
		User: &UserConfig{
			Defaults: &DefaultsConfig{
				Tool:     strPtr(DefaultTool),
				Provider: strPtr(DefaultProvider),
			},
			Providers: map[string]*ProviderConfig{
				DefaultProvider: {
					APIKeyEnv: strPtr("ANTHROPIC_API_KEY"),
				},
			},
		},
	}

	// Set the env var
	_ = os.Setenv("ANTHROPIC_API_KEY", "test-key")
	defer func() { _ = os.Unsetenv("ANTHROPIC_API_KEY") }()

	errs := cfg.Validate()

	// Default tool doesn't need to be defined in tools map
	// Should have no errors if env var is set
	assert.Empty(t, errs)
}

func TestToolExecutable(t *testing.T) {
	tests := []struct {
		tool     string
		expected string
	}{
		{"claude-code", "claude"},
		{"opencode", "opencode"},
		{"some-other-tool", "some-other-tool"},
		{"claude", "claude"},
	}

	for _, tt := range tests {
		t.Run(tt.tool, func(t *testing.T) {
			assert.Equal(t, tt.expected, ToolExecutable(tt.tool))
		})
	}
}

// Helper functions for tests
func intPtr(i int) *int {
	return &i
}

func floatPtr(f float64) *float64 {
	return &f
}
