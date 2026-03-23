package config

import (
	"fmt"
)

// stepDefaultTiers maps step names to their default model tier.
// Steps listed here use a tier-based default instead of DefaultModel
// when no model is configured anywhere in the resolution chain.
var stepDefaultTiers = map[string]string{
	"add":  "muster-fast",
	"sync": "muster-fast",
	"out":  "muster-standard",
	"plan": "muster-deep",
}

// concreteModelForTier returns the hard-coded default model for a tier name.
// This is used when tier resolution fails (no user config) but the step has
// a default tier that needs a concrete model name.
func concreteModelForTier(tier string) string {
	switch tier {
	case "muster-fast":
		return DefaultFastModel
	case "muster-standard":
		return DefaultStandardModel
	case "muster-deep":
		return DefaultDeepModel
	default:
		return DefaultModel
	}
}

// ResolveStep resolves configuration for a specific pipeline step.
// It implements a 5-step fallback chain:
// 1. Step config (projectCfg.Pipeline[stepName])
// 2. Project defaults (projectCfg.Defaults)
// 3. User defaults (userCfg.Defaults)
// 4. Tool defaults (userCfg.Tools[tool].Defaults after tool is resolved)
// 5. Hard-coded defaults (constants), with per-step tier defaults
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
	var toolSrc, providerSrc, modelSrc string
	stepLabel := fmt.Sprintf("pipeline.%s", stepName)

	// Step 1: Step config
	var stepCfg *PipelineStepConfig
	if projectCfg.Pipeline != nil {
		stepCfg = projectCfg.Pipeline[stepName]
	}
	if stepCfg != nil {
		if stepCfg.Tool != nil {
			tool = stepCfg.Tool
			toolSrc = stepLabel
		}
		if stepCfg.Provider != nil {
			provider = stepCfg.Provider
			providerSrc = stepLabel
		}
		if stepCfg.Model != nil {
			model = stepCfg.Model
			modelSrc = stepLabel
		}
	}

	// Step 2: Project defaults
	if projectCfg.Defaults != nil {
		if tool == nil && projectCfg.Defaults.Tool != nil {
			tool = projectCfg.Defaults.Tool
			toolSrc = "project defaults"
		}
		if provider == nil && projectCfg.Defaults.Provider != nil {
			provider = projectCfg.Defaults.Provider
			providerSrc = "project defaults"
		}
		if model == nil && projectCfg.Defaults.Model != nil {
			model = projectCfg.Defaults.Model
			modelSrc = "project defaults"
		}
	}

	// Step 3: User defaults
	if userCfg.Defaults != nil {
		if tool == nil && userCfg.Defaults.Tool != nil {
			tool = userCfg.Defaults.Tool
			toolSrc = "user defaults"
		}
		if provider == nil && userCfg.Defaults.Provider != nil {
			provider = userCfg.Defaults.Provider
			providerSrc = "user defaults"
		}
		if model == nil && userCfg.Defaults.Model != nil {
			model = userCfg.Defaults.Model
			modelSrc = "user defaults"
		}
	}

	// At this point, we need to have a tool to proceed with step 4
	// If we still don't have a tool, use hard-coded default
	if tool == nil {
		defaultTool := DefaultTool
		tool = &defaultTool
		toolSrc = "built-in default"
	}

	// Step 4: Tool defaults (only for model, if not yet resolved)
	// This is where we check the tool-specific model tiers
	// Note: Tool defaults don't provide tool/provider, only model tiers

	// Step 5: Hard-coded defaults (for anything still missing)
	if provider == nil {
		defaultProvider := DefaultProvider
		provider = &defaultProvider
		providerSrc = "built-in default"
	}
	if model == nil {
		if tier, ok := stepDefaultTiers[stepName]; ok {
			// Step has a default tier — try user's tier config first
			resolvedTier, err := resolveModelTier(tier, *tool, projectCfg, userCfg)
			if err == nil {
				model = &resolvedTier
				modelSrc = fmt.Sprintf("step default tier (%s) via user config", tier)
			} else {
				// No user tier config — use built-in default for this tier
				defaultModel := concreteModelForTier(tier)
				model = &defaultModel
				modelSrc = fmt.Sprintf("step default tier (%s)", tier)
			}
		} else {
			defaultModel := DefaultModel
			model = &defaultModel
			modelSrc = "built-in default"
		}
	}

	// Now resolve the model string (handle tier resolution)
	resolvedModel, err := resolveModelTier(*model, *tool, projectCfg, userCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve model tier: %w", err)
	}

	return &ResolvedConfig{
		Tool:           *tool,
		Provider:       *provider,
		Model:          resolvedModel,
		ToolSource:     toolSrc,
		ProviderSource: providerSrc,
		ModelSource:    modelSrc,
	}, nil
}

// resolveModelTier resolves tier names (muster-fast, muster-standard, muster-deep)
// to concrete model names. Resolution order: project tool tiers > project model tiers >
// user tool tiers > user model tiers. Literal model names are passed through unchanged.
func resolveModelTier(modelStr string, tool string, projectCfg *ProjectConfig, userCfg *UserConfig) (string, error) {
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

	// 1. Project tool tiers
	if projectCfg != nil && projectCfg.Tools != nil {
		if toolCfg, ok := projectCfg.Tools[tool]; ok && toolCfg != nil {
			if m := toolCfg.ModelTiers.Resolve(tierName); m != nil {
				return *m, nil
			}
		}
	}

	// 2. Project model tiers
	if projectCfg != nil {
		if m := projectCfg.ModelTiers.Resolve(tierName); m != nil {
			return *m, nil
		}
	}

	// 3. User tool tiers
	if userCfg != nil && userCfg.Tools != nil {
		if toolCfg, ok := userCfg.Tools[tool]; ok && toolCfg != nil {
			if m := toolCfg.ModelTiers.Resolve(tierName); m != nil {
				return *m, nil
			}
		}
	}

	// 4. User model tiers
	if userCfg != nil {
		if m := userCfg.ModelTiers.Resolve(tierName); m != nil {
			return *m, nil
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
