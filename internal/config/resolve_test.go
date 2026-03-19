package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveStep(t *testing.T) {
	tests := []struct {
		name        string
		stepName    string
		projectCfg  *ProjectConfig
		userCfg     *UserConfig
		expected    *ResolvedConfig
		expectError bool
	}{
		{
			name:       "all defaults - hard-coded",
			stepName:   "code",
			projectCfg: &ProjectConfig{},
			userCfg:    &UserConfig{},
			expected: &ResolvedConfig{
				Tool:     DefaultTool,
				Provider: DefaultProvider,
				Model:    DefaultModel,
			},
			expectError: false,
		},
		{
			name:       "user defaults only",
			stepName:   "code",
			projectCfg: &ProjectConfig{},
			userCfg: &UserConfig{
				Defaults: &DefaultsConfig{
					Tool:     strPtr("custom-tool"),
					Provider: strPtr("custom-provider"),
					Model:    strPtr("custom-model"),
				},
			},
			expected: &ResolvedConfig{
				Tool:     "custom-tool",
				Provider: "custom-provider",
				Model:    "custom-model",
			},
			expectError: false,
		},
		{
			name:     "project defaults override user defaults",
			stepName: "code",
			projectCfg: &ProjectConfig{
				Defaults: &DefaultsConfig{
					Tool:     strPtr("project-tool"),
					Provider: strPtr("project-provider"),
					Model:    strPtr("project-model"),
				},
			},
			userCfg: &UserConfig{
				Defaults: &DefaultsConfig{
					Tool:     strPtr("user-tool"),
					Provider: strPtr("user-provider"),
					Model:    strPtr("user-model"),
				},
			},
			expected: &ResolvedConfig{
				Tool:     "project-tool",
				Provider: "project-provider",
				Model:    "project-model",
			},
			expectError: false,
		},
		{
			name:     "step config overrides project defaults",
			stepName: "code",
			projectCfg: &ProjectConfig{
				Defaults: &DefaultsConfig{
					Tool:     strPtr("project-tool"),
					Provider: strPtr("project-provider"),
					Model:    strPtr("project-model"),
				},
				Pipeline: map[string]*PipelineStepConfig{
					"code": {
						Tool:     strPtr("step-tool"),
						Provider: strPtr("step-provider"),
						Model:    strPtr("step-model"),
					},
				},
			},
			userCfg: &UserConfig{},
			expected: &ResolvedConfig{
				Tool:     "step-tool",
				Provider: "step-provider",
				Model:    "step-model",
			},
			expectError: false,
		},
		{
			name:     "step config partial override - tool only",
			stepName: "code",
			projectCfg: &ProjectConfig{
				Defaults: &DefaultsConfig{
					Provider: strPtr("project-provider"),
					Model:    strPtr("project-model"),
				},
				Pipeline: map[string]*PipelineStepConfig{
					"code": {
						Tool: strPtr("step-tool"),
					},
				},
			},
			userCfg: &UserConfig{},
			expected: &ResolvedConfig{
				Tool:     "step-tool",
				Provider: "project-provider",
				Model:    "project-model",
			},
			expectError: false,
		},
		{
			name:     "tier resolution - fast",
			stepName: "code",
			projectCfg: &ProjectConfig{
				Defaults: &DefaultsConfig{
					Tool:     strPtr("claude-code"),
					Provider: strPtr("anthropic"),
					Model:    strPtr("muster-fast"),
				},
			},
			userCfg: &UserConfig{
				Tools: map[string]*ToolConfig{
					"claude-code": {
						ModelTiers: &struct {
							Fast     *string `yaml:"fast"`
							Standard *string `yaml:"standard"`
							Deep     *string `yaml:"deep"`
						}{
							Fast:     strPtr("claude-sonnet-4.5"),
							Standard: strPtr("claude-sonnet-4.5"),
							Deep:     strPtr("claude-opus-4"),
						},
					},
				},
			},
			expected: &ResolvedConfig{
				Tool:     "claude-code",
				Provider: "anthropic",
				Model:    "claude-sonnet-4.5",
			},
			expectError: false,
		},
		{
			name:     "tier resolution - balanced",
			stepName: "code",
			projectCfg: &ProjectConfig{
				Defaults: &DefaultsConfig{
					Tool:  strPtr("claude-code"),
					Model: strPtr("muster-standard"),
				},
			},
			userCfg: &UserConfig{
				Tools: map[string]*ToolConfig{
					"claude-code": {
						ModelTiers: &struct {
							Fast     *string `yaml:"fast"`
							Standard *string `yaml:"standard"`
							Deep     *string `yaml:"deep"`
						}{
							Fast:     strPtr("claude-sonnet-4.5"),
							Standard: strPtr("claude-sonnet-4.5"),
							Deep:     strPtr("claude-opus-4"),
						},
					},
				},
			},
			expected: &ResolvedConfig{
				Tool:     "claude-code",
				Provider: DefaultProvider,
				Model:    "claude-sonnet-4.5",
			},
			expectError: false,
		},
		{
			name:     "tier resolution - capable",
			stepName: "code",
			projectCfg: &ProjectConfig{
				Defaults: &DefaultsConfig{
					Tool:  strPtr("claude-code"),
					Model: strPtr("muster-deep"),
				},
			},
			userCfg: &UserConfig{
				Tools: map[string]*ToolConfig{
					"claude-code": {
						ModelTiers: &struct {
							Fast     *string `yaml:"fast"`
							Standard *string `yaml:"standard"`
							Deep     *string `yaml:"deep"`
						}{
							Fast:     strPtr("claude-sonnet-4.5"),
							Standard: strPtr("claude-sonnet-4.5"),
							Deep:     strPtr("claude-opus-4"),
						},
					},
				},
			},
			expected: &ResolvedConfig{
				Tool:     "claude-code",
				Provider: DefaultProvider,
				Model:    "claude-opus-4",
			},
			expectError: false,
		},
		{
			name:     "literal model passthrough",
			stepName: "code",
			projectCfg: &ProjectConfig{
				Defaults: &DefaultsConfig{
					Tool:     strPtr("cursor"),
					Provider: strPtr("openai"),
					Model:    strPtr("gpt-4o"),
				},
			},
			userCfg: &UserConfig{},
			expected: &ResolvedConfig{
				Tool:     "cursor",
				Provider: "openai",
				Model:    "gpt-4o",
			},
			expectError: false,
		},
		{
			name:     "unknown tier error",
			stepName: "code",
			projectCfg: &ProjectConfig{
				Defaults: &DefaultsConfig{
					Tool:  strPtr("claude-code"),
					Model: strPtr("muster-fast"),
				},
			},
			userCfg: &UserConfig{
				Tools: map[string]*ToolConfig{
					"claude-code": {
						ModelTiers: &struct {
							Fast     *string `yaml:"fast"`
							Standard *string `yaml:"standard"`
							Deep     *string `yaml:"deep"`
						}{
							// Missing fast tier
							Standard: strPtr("claude-sonnet-4.5"),
							Deep:     strPtr("claude-opus-4"),
						},
					},
				},
			},
			expected:    nil,
			expectError: true,
		},
		{
			name:     "tool override with tier resolution",
			stepName: "code",
			projectCfg: &ProjectConfig{
				Defaults: &DefaultsConfig{
					Model: strPtr("muster-deep"),
				},
				Pipeline: map[string]*PipelineStepConfig{
					"code": {
						Tool: strPtr("cursor"),
					},
				},
			},
			userCfg: &UserConfig{
				Tools: map[string]*ToolConfig{
					"claude-code": {
						ModelTiers: &struct {
							Fast     *string `yaml:"fast"`
							Standard *string `yaml:"standard"`
							Deep     *string `yaml:"deep"`
						}{
							Fast:     strPtr("claude-sonnet-4.5"),
							Standard: strPtr("claude-sonnet-4.5"),
							Deep:     strPtr("claude-opus-4"),
						},
					},
					"cursor": {
						ModelTiers: &struct {
							Fast     *string `yaml:"fast"`
							Standard *string `yaml:"standard"`
							Deep     *string `yaml:"deep"`
						}{
							Fast:     strPtr("gpt-4o-mini"),
							Standard: strPtr("gpt-4o"),
							Deep:     strPtr("o1-preview"),
						},
					},
				},
			},
			expected: &ResolvedConfig{
				Tool:     "cursor",
				Provider: DefaultProvider,
				Model:    "o1-preview",
			},
			expectError: false,
		},
		{
			name:       "nil configs use defaults",
			stepName:   "code",
			projectCfg: nil,
			userCfg:    nil,
			expected: &ResolvedConfig{
				Tool:     DefaultTool,
				Provider: DefaultProvider,
				Model:    DefaultModel,
			},
			expectError: false,
		},
		{
			name:       "empty step name still resolves",
			stepName:   "",
			projectCfg: &ProjectConfig{},
			userCfg:    &UserConfig{},
			expected: &ResolvedConfig{
				Tool:     DefaultTool,
				Provider: DefaultProvider,
				Model:    DefaultModel,
			},
			expectError: false,
		},
		{
			name:     "step not in pipeline falls back to defaults",
			stepName: "nonexistent",
			projectCfg: &ProjectConfig{
				Defaults: &DefaultsConfig{
					Tool:     strPtr("project-tool"),
					Provider: strPtr("project-provider"),
					Model:    strPtr("project-model"),
				},
				Pipeline: map[string]*PipelineStepConfig{
					"other": {
						Tool: strPtr("other-tool"),
					},
				},
			},
			userCfg: &UserConfig{},
			expected: &ResolvedConfig{
				Tool:     "project-tool",
				Provider: "project-provider",
				Model:    "project-model",
			},
			expectError: false,
		},
		{
			name:       "add step defaults to fast tier model",
			stepName:   "add",
			projectCfg: &ProjectConfig{},
			userCfg:    &UserConfig{},
			expected: &ResolvedConfig{
				Tool:     DefaultTool,
				Provider: DefaultProvider,
				Model:    DefaultFastModel,
			},
			expectError: false,
		},
		{
			name:       "sync step defaults to fast tier model",
			stepName:   "sync",
			projectCfg: &ProjectConfig{},
			userCfg:    &UserConfig{},
			expected: &ResolvedConfig{
				Tool:     DefaultTool,
				Provider: DefaultProvider,
				Model:    DefaultFastModel,
			},
			expectError: false,
		},
		{
			name:     "add step uses user fast tier when configured",
			stepName: "add",
			projectCfg: &ProjectConfig{},
			userCfg: &UserConfig{
				Tools: map[string]*ToolConfig{
					"claude-code": {
						ModelTiers: &struct {
							Fast     *string `yaml:"fast"`
							Standard *string `yaml:"standard"`
							Deep     *string `yaml:"deep"`
						}{
							Fast: strPtr("custom-fast-model"),
						},
					},
				},
			},
			expected: &ResolvedConfig{
				Tool:     DefaultTool,
				Provider: DefaultProvider,
				Model:    "custom-fast-model",
			},
			expectError: false,
		},
		{
			name:     "add step explicit model overrides fast default",
			stepName: "add",
			projectCfg: &ProjectConfig{
				Pipeline: map[string]*PipelineStepConfig{
					"add": {
						Model: strPtr("explicit-model"),
					},
				},
			},
			userCfg: &UserConfig{},
			expected: &ResolvedConfig{
				Tool:     DefaultTool,
				Provider: DefaultProvider,
				Model:    "explicit-model",
			},
			expectError: false,
		},
		{
			name:     "add step project default model overrides fast default",
			stepName: "add",
			projectCfg: &ProjectConfig{
				Defaults: &DefaultsConfig{
					Model: strPtr("project-model"),
				},
			},
			userCfg: &UserConfig{},
			expected: &ResolvedConfig{
				Tool:     DefaultTool,
				Provider: DefaultProvider,
				Model:    "project-model",
			},
			expectError: false,
		},
		{
			name:       "code step still uses standard default",
			stepName:   "code",
			projectCfg: &ProjectConfig{},
			userCfg:    &UserConfig{},
			expected: &ResolvedConfig{
				Tool:     DefaultTool,
				Provider: DefaultProvider,
				Model:    DefaultModel,
			},
			expectError: false,
		},
		{
			name:     "mixed fallback chain",
			stepName: "code",
			projectCfg: &ProjectConfig{
				Defaults: &DefaultsConfig{
					Tool: strPtr("project-tool"),
					// Provider not set - will fall back
				},
				Pipeline: map[string]*PipelineStepConfig{
					"code": {
						Model: strPtr("step-model"),
						// Tool and Provider not set - will fall back
					},
				},
			},
			userCfg: &UserConfig{
				Defaults: &DefaultsConfig{
					Provider: strPtr("user-provider"),
					// Tool and Model not set in user defaults
				},
			},
			expected: &ResolvedConfig{
				Tool:     "project-tool",
				Provider: "user-provider",
				Model:    "step-model",
			},
			expectError: false,
		},
		{
			name:     "user-level tier resolution fallback",
			stepName: "code",
			projectCfg: &ProjectConfig{
				Defaults: &DefaultsConfig{
					Tool:  strPtr("custom-tool"),
					Model: strPtr("muster-fast"),
				},
			},
			userCfg: &UserConfig{
				ModelTiers: &struct {
					Fast     *string `yaml:"fast"`
					Standard *string `yaml:"standard"`
					Deep     *string `yaml:"deep"`
				}{
					Fast:     strPtr("global-fast-model"),
					Standard: strPtr("global-balanced-model"),
					Deep:     strPtr("global-capable-model"),
				},
			},
			expected: &ResolvedConfig{
				Tool:     "custom-tool",
				Provider: DefaultProvider,
				Model:    "global-fast-model",
			},
			expectError: false,
		},
		{
			name:     "tool-level tier overrides user-level tier",
			stepName: "code",
			projectCfg: &ProjectConfig{
				Defaults: &DefaultsConfig{
					Tool:  strPtr("claude-code"),
					Model: strPtr("muster-fast"),
				},
			},
			userCfg: &UserConfig{
				Tools: map[string]*ToolConfig{
					"claude-code": {
						ModelTiers: &struct {
							Fast     *string `yaml:"fast"`
							Standard *string `yaml:"standard"`
							Deep     *string `yaml:"deep"`
						}{
							Fast: strPtr("tool-specific-fast"),
						},
					},
				},
				ModelTiers: &struct {
					Fast     *string `yaml:"fast"`
					Standard *string `yaml:"standard"`
					Deep     *string `yaml:"deep"`
				}{
					Fast: strPtr("global-fast"),
				},
			},
			expected: &ResolvedConfig{
				Tool:     "claude-code",
				Provider: DefaultProvider,
				Model:    "tool-specific-fast",
			},
			expectError: false,
		},
		{
			name:     "all literal models - no tier resolution",
			stepName: "code",
			projectCfg: &ProjectConfig{
				Pipeline: map[string]*PipelineStepConfig{
					"code": {
						Tool:     strPtr("cursor"),
						Provider: strPtr("openai"),
						Model:    strPtr("o1-mini"),
					},
				},
			},
			userCfg: &UserConfig{},
			expected: &ResolvedConfig{
				Tool:     "cursor",
				Provider: "openai",
				Model:    "o1-mini",
			},
			expectError: false,
		},
		{
			name:     "complex nested step override",
			stepName: "execute",
			projectCfg: &ProjectConfig{
				Defaults: &DefaultsConfig{
					Tool:     strPtr("default-tool"),
					Provider: strPtr("default-provider"),
					Model:    strPtr("default-model"),
				},
				Pipeline: map[string]*PipelineStepConfig{
					"execute": {
						Tool:  strPtr("execute-tool"),
						Model: strPtr("execute-model"),
						// Provider will fall back to project defaults
					},
				},
			},
			userCfg: &UserConfig{
				Defaults: &DefaultsConfig{
					Tool:     strPtr("user-tool"),
					Provider: strPtr("user-provider"),
					Model:    strPtr("user-model"),
				},
			},
			expected: &ResolvedConfig{
				Tool:     "execute-tool",
				Provider: "default-provider",
				Model:    "execute-model",
			},
			expectError: false,
		},
		{
			name:     "tier resolution with unknown tool - fallback to user tier",
			stepName: "code",
			projectCfg: &ProjectConfig{
				Defaults: &DefaultsConfig{
					Tool:  strPtr("unknown-tool"),
					Model: strPtr("muster-standard"),
				},
			},
			userCfg: &UserConfig{
				ModelTiers: &struct {
					Fast     *string `yaml:"fast"`
					Standard *string `yaml:"standard"`
					Deep     *string `yaml:"deep"`
				}{
					Standard: strPtr("fallback-balanced-model"),
				},
			},
			expected: &ResolvedConfig{
				Tool:     "unknown-tool",
				Provider: DefaultProvider,
				Model:    "fallback-balanced-model",
			},
			expectError: false,
		},
		{
			name:     "tier resolution completely missing",
			stepName: "code",
			projectCfg: &ProjectConfig{
				Defaults: &DefaultsConfig{
					Tool:  strPtr("some-tool"),
					Model: strPtr("muster-deep"),
				},
			},
			userCfg:     &UserConfig{},
			expected:    nil,
			expectError: true,
		},
		{
			name:     "partial provider and model from different levels",
			stepName: "verify",
			projectCfg: &ProjectConfig{
				Defaults: &DefaultsConfig{
					Provider: strPtr("project-provider"),
				},
				Pipeline: map[string]*PipelineStepConfig{
					"verify": {
						Model: strPtr("verify-model"),
					},
				},
			},
			userCfg: &UserConfig{
				Defaults: &DefaultsConfig{
					Tool: strPtr("user-tool"),
				},
			},
			expected: &ResolvedConfig{
				Tool:     "user-tool",
				Provider: "project-provider",
				Model:    "verify-model",
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ResolveStep(tt.stepName, tt.projectCfg, tt.userCfg)
			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, result)
			} else {
				require.NoError(t, err)
				require.NotNil(t, result)
				assert.Equal(t, tt.expected.Tool, result.Tool)
				assert.Equal(t, tt.expected.Provider, result.Provider)
				assert.Equal(t, tt.expected.Model, result.Model)
			}
		})
	}
}

func TestResolveCode(t *testing.T) {
	tests := []struct {
		name        string
		projectCfg  *ProjectConfig
		userCfg     *UserConfig
		expected    *ResolvedConfig
		expectError bool
	}{
		{
			name: "code step with explicit config",
			projectCfg: &ProjectConfig{
				Pipeline: map[string]*PipelineStepConfig{
					"code": {
						Tool:     strPtr("code-tool"),
						Provider: strPtr("code-provider"),
						Model:    strPtr("code-model"),
					},
				},
			},
			userCfg: &UserConfig{},
			expected: &ResolvedConfig{
				Tool:     "code-tool",
				Provider: "code-provider",
				Model:    "code-model",
			},
			expectError: false,
		},
		{
			name:       "code step with defaults",
			projectCfg: &ProjectConfig{},
			userCfg:    &UserConfig{},
			expected: &ResolvedConfig{
				Tool:     DefaultTool,
				Provider: DefaultProvider,
				Model:    DefaultModel,
			},
			expectError: false,
		},
		{
			name:       "nil configs",
			projectCfg: nil,
			userCfg:    nil,
			expected: &ResolvedConfig{
				Tool:     DefaultTool,
				Provider: DefaultProvider,
				Model:    DefaultModel,
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ResolveCode(tt.projectCfg, tt.userCfg)
			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, result)
			} else {
				require.NoError(t, err)
				require.NotNil(t, result)
				assert.Equal(t, tt.expected.Tool, result.Tool)
				assert.Equal(t, tt.expected.Provider, result.Provider)
				assert.Equal(t, tt.expected.Model, result.Model)
			}
		})
	}
}

func TestResolveModelTier(t *testing.T) {
	tests := []struct {
		name        string
		modelStr    string
		tool        string
		userCfg     *UserConfig
		expected    string
		expectError bool
	}{
		{
			name:        "literal model passthrough",
			modelStr:    "gpt-4o",
			tool:        "cursor",
			userCfg:     &UserConfig{},
			expected:    "gpt-4o",
			expectError: false,
		},
		{
			name:     "tier fast resolution",
			modelStr: "muster-fast",
			tool:     "claude-code",
			userCfg: &UserConfig{
				Tools: map[string]*ToolConfig{
					"claude-code": {
						ModelTiers: &struct {
							Fast     *string `yaml:"fast"`
							Standard *string `yaml:"standard"`
							Deep     *string `yaml:"deep"`
						}{
							Fast: strPtr("claude-sonnet-4.5"),
						},
					},
				},
			},
			expected:    "claude-sonnet-4.5",
			expectError: false,
		},
		{
			name:     "tier balanced resolution",
			modelStr: "muster-standard",
			tool:     "cursor",
			userCfg: &UserConfig{
				Tools: map[string]*ToolConfig{
					"cursor": {
						ModelTiers: &struct {
							Fast     *string `yaml:"fast"`
							Standard *string `yaml:"standard"`
							Deep     *string `yaml:"deep"`
						}{
							Standard: strPtr("gpt-4o"),
						},
					},
				},
			},
			expected:    "gpt-4o",
			expectError: false,
		},
		{
			name:     "tier capable resolution",
			modelStr: "muster-deep",
			tool:     "claude-code",
			userCfg: &UserConfig{
				Tools: map[string]*ToolConfig{
					"claude-code": {
						ModelTiers: &struct {
							Fast     *string `yaml:"fast"`
							Standard *string `yaml:"standard"`
							Deep     *string `yaml:"deep"`
						}{
							Deep: strPtr("claude-opus-4"),
						},
					},
				},
			},
			expected:    "claude-opus-4",
			expectError: false,
		},
		{
			name:     "fallback to user-level tiers",
			modelStr: "muster-fast",
			tool:     "unknown-tool",
			userCfg: &UserConfig{
				ModelTiers: &struct {
					Fast     *string `yaml:"fast"`
					Standard *string `yaml:"standard"`
					Deep     *string `yaml:"deep"`
				}{
					Fast: strPtr("default-fast-model"),
				},
			},
			expected:    "default-fast-model",
			expectError: false,
		},
		{
			name:        "unknown tier error",
			modelStr:    "muster-fast",
			tool:        "some-tool",
			userCfg:     &UserConfig{},
			expected:    "",
			expectError: true,
		},
		{
			name:        "custom literal model",
			modelStr:    "my-custom-model-v2",
			tool:        "any-tool",
			userCfg:     &UserConfig{},
			expected:    "my-custom-model-v2",
			expectError: false,
		},
		{
			name:     "empty userconfig with tier reference returns helpful error",
			modelStr: "muster-standard",
			tool:     "custom-tool",
			userCfg: &UserConfig{
				Tools:      nil,
				ModelTiers: nil,
			},
			expected:    "",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := resolveModelTier(tt.modelStr, tt.tool, tt.userCfg)
			if tt.expectError {
				assert.Error(t, err)
				assert.Empty(t, result)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}
