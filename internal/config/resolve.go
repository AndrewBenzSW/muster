package config

import (
	"fmt"
)

// ResolveStep resolves configuration for a specific pipeline step.
// It implements a 5-step fallback chain:
// 1. Step config (projectCfg.Pipeline[stepName])
// 2. Project defaults (projectCfg.Defaults)
// 3. User defaults (userCfg.Defaults)
// 4. Tool defaults (userCfg.Tools[tool].Defaults after tool is resolved)
// 5. Hard-coded defaults (constants)
//
// Tool override rule: When a step overrides tool but not model, the model
// string resolution continues through the fallback chain normally, then tier
// resolution uses the newly selected tool's tier mapping.
func ResolveStep(stepName string, projectCfg *ProjectConfig, userCfg *UserConfig) (*ResolvedConfig, error) {
	if userCfg == nil {
		userCfg = DefaultUserConfig()
	}
	if projectCfg == nil {
		projectCfg = &ProjectConfig{}
	}

	var tool, provider, model *string

	// Step 1: Step config
	var stepCfg *PipelineStepConfig
	if projectCfg.Pipeline != nil {
		stepCfg = projectCfg.Pipeline[stepName]
	}
	if stepCfg != nil {
		if stepCfg.Tool != nil {
			tool = stepCfg.Tool
		}
		if stepCfg.Provider != nil {
			provider = stepCfg.Provider
		}
		if stepCfg.Model != nil {
			model = stepCfg.Model
		}
	}

	// Step 2: Project defaults
	if projectCfg.Defaults != nil {
		if tool == nil && projectCfg.Defaults.Tool != nil {
			tool = projectCfg.Defaults.Tool
		}
		if provider == nil && projectCfg.Defaults.Provider != nil {
			provider = projectCfg.Defaults.Provider
		}
		if model == nil && projectCfg.Defaults.Model != nil {
			model = projectCfg.Defaults.Model
		}
	}

	// Step 3: User defaults
	if userCfg.Defaults != nil {
		if tool == nil && userCfg.Defaults.Tool != nil {
			tool = userCfg.Defaults.Tool
		}
		if provider == nil && userCfg.Defaults.Provider != nil {
			provider = userCfg.Defaults.Provider
		}
		if model == nil && userCfg.Defaults.Model != nil {
			model = userCfg.Defaults.Model
		}
	}

	// At this point, we need to have a tool to proceed with step 4
	// If we still don't have a tool, use hard-coded default
	if tool == nil {
		defaultTool := DefaultTool
		tool = &defaultTool
	}

	// Step 4: Tool defaults (only for model, if not yet resolved)
	// This is where we check the tool-specific model tiers
	// Note: Tool defaults don't provide tool/provider, only model tiers

	// Step 5: Hard-coded defaults (for anything still missing)
	if provider == nil {
		defaultProvider := DefaultProvider
		provider = &defaultProvider
	}
	if model == nil {
		defaultModel := DefaultModel
		model = &defaultModel
	}

	// Now resolve the model string (handle tier resolution)
	resolvedModel, err := resolveModelTier(*model, *tool, userCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve model tier: %w", err)
	}

	return &ResolvedConfig{
		Tool:     *tool,
		Provider: *provider,
		Model:    resolvedModel,
	}, nil
}

// resolveModelTier resolves tier names (muster-fast, muster-standard, muster-deep)
// to concrete model names using the specified tool's tier mapping.
// Literal model names are passed through unchanged.
func resolveModelTier(modelStr string, tool string, userCfg *UserConfig) (string, error) {
	// Check if this is a tier reference
	var tierName string
	switch modelStr {
	case "muster-fast":
		tierName = "fast"
	case "muster-standard":
		tierName = "standard"
	case "muster-deep":
		tierName = "deep"
	default:
		// Not a tier reference - pass through as literal model name
		return modelStr, nil
	}

	// Look up the tier in the tool's model tiers
	if userCfg.Tools != nil {
		if toolCfg, ok := userCfg.Tools[tool]; ok && toolCfg != nil && toolCfg.ModelTiers != nil {
			var tierModel *string
			switch tierName {
			case "fast":
				tierModel = toolCfg.ModelTiers.Fast
			case "standard":
				tierModel = toolCfg.ModelTiers.Standard
			case "deep":
				tierModel = toolCfg.ModelTiers.Deep
			}
			if tierModel != nil {
				return *tierModel, nil
			}
		}
	}

	// Fall back to user-level model tiers
	if userCfg.ModelTiers != nil {
		var tierModel *string
		switch tierName {
		case "fast":
			tierModel = userCfg.ModelTiers.Fast
		case "standard":
			tierModel = userCfg.ModelTiers.Standard
		case "deep":
			tierModel = userCfg.ModelTiers.Deep
		}
		if tierModel != nil {
			return *tierModel, nil
		}
	}

	// Tier not found
	return "", fmt.Errorf("unknown model tier %q for tool %q", modelStr, tool)
}

// ResolveCode resolves configuration for the "code" step.
// This is a convenience wrapper that calls ResolveStep with stepName="code".
func ResolveCode(projectCfg *ProjectConfig, userCfg *UserConfig) (*ResolvedConfig, error) {
	return ResolveStep("code", projectCfg, userCfg)
}
