// Package prompt provides prompt template rendering and context management for muster.
//
// The PromptContext struct acts as the data model for prompt templates, providing access
// to resolved configuration (tool, provider, model), file system paths (worktree, plan
// directory, main repo), and model tier mappings. The Models field is populated from the
// resolved tool's tier configuration, ensuring that template-generated commands (like
// sub-agent invocations) use the correct model names for the currently active tool.
// For example, if the resolved tool is "opencode", Models.Fast will contain opencode's
// fast tier model, not claude's, so that templates can generate commands that work with
// the active tool's expectations.
package prompt

import "github.com/abenz1267/muster/internal/config"

// PromptContext holds the data available to prompt templates during rendering.
// It includes resolved configuration, file paths, and model tier mappings.
type PromptContext struct {
	// Interactive indicates whether this is an interactive session
	Interactive bool

	// Tool is the resolved tool name (e.g., "claude-code", "opencode")
	Tool string

	// Provider is the resolved provider name (e.g., "anthropic", "openai")
	Provider string

	// Model is the resolved model name (e.g., "claude-sonnet-4.5")
	Model string

	// Slug is the identifier for the current roadmap item or task
	Slug string

	// WorktreePath is the absolute path to the git worktree for this item
	WorktreePath string

	// MainRepoPath is the absolute path to the main repository
	MainRepoPath string

	// PlanDir is the absolute path to the plan directory for this item
	PlanDir string

	// Models contains tier-to-model mappings for template-generated commands.
	// These are populated from the resolved tool's tier mappings to ensure
	// commands use the correct model names for the active tool.
	Models struct {
		// Fast is the model name for the fast/lightweight tier
		Fast string

		// Standard is the model name for the standard/balanced tier
		Standard string

		// Deep is the model name for the deep/capable tier
		Deep string
	}

	// Extra allows passing additional data to templates without modifying the shared struct.
	// Templates access via {{.Extra.Key}}. For example, sync templates use this to pass
	// SourceItems and TargetItems.
	Extra map[string]interface{} `json:"-"`
}

// NewPromptContext creates a new PromptContext from resolved configuration and paths.
// The Models struct is populated from tier mappings with project config taking precedence
// over user config: project tool tiers > project model tiers > user tool tiers > user model tiers.
func NewPromptContext(
	resolved *config.ResolvedConfig,
	projectCfg *config.ProjectConfig,
	userCfg *config.UserConfig,
	interactive bool,
	slug string,
	worktreePath string,
	mainRepoPath string,
	planDir string,
) *PromptContext {
	ctx := &PromptContext{
		Interactive:  interactive,
		Tool:         resolved.Tool,
		Provider:     resolved.Provider,
		Model:        resolved.Model,
		Slug:         slug,
		WorktreePath: worktreePath,
		MainRepoPath: mainRepoPath,
		PlanDir:      planDir,
		Extra:        make(map[string]interface{}),
	}

	// Populate Models from tier mappings in priority order:
	// project tool > project global > user tool > user global > hard-coded defaults
	tierSources := collectTierSources(resolved.Tool, projectCfg, userCfg)
	for _, tierName := range []string{"fast", "standard", "deep"} {
		for _, src := range tierSources {
			if m := src.Resolve(tierName); m != nil {
				switch tierName {
				case "fast":
					if ctx.Models.Fast == "" {
						ctx.Models.Fast = *m
					}
				case "standard":
					if ctx.Models.Standard == "" {
						ctx.Models.Standard = *m
					}
				case "deep":
					if ctx.Models.Deep == "" {
						ctx.Models.Deep = *m
					}
				}
				break
			}
		}
	}

	// Fall back to hard-coded defaults if still empty
	if ctx.Models.Fast == "" {
		ctx.Models.Fast = config.DefaultFastModel
	}
	if ctx.Models.Standard == "" {
		ctx.Models.Standard = config.DefaultStandardModel
	}
	if ctx.Models.Deep == "" {
		ctx.Models.Deep = config.DefaultDeepModel
	}

	return ctx
}

// collectTierSources returns ModelTiersConfigs in priority order for the given tool.
func collectTierSources(tool string, projectCfg *config.ProjectConfig, userCfg *config.UserConfig) []*config.ModelTiersConfig {
	var sources []*config.ModelTiersConfig

	// 1. Project tool tiers
	if projectCfg != nil && projectCfg.Tools != nil {
		if toolCfg, ok := projectCfg.Tools[tool]; ok && toolCfg != nil {
			sources = append(sources, toolCfg.ModelTiers)
		}
	}
	// 2. Project model tiers
	if projectCfg != nil {
		sources = append(sources, projectCfg.ModelTiers)
	}
	// 3. User tool tiers
	if userCfg != nil && userCfg.Tools != nil {
		if toolCfg, ok := userCfg.Tools[tool]; ok && toolCfg != nil {
			sources = append(sources, toolCfg.ModelTiers)
		}
	}
	// 4. User model tiers
	if userCfg != nil {
		sources = append(sources, userCfg.ModelTiers)
	}

	return sources
}
