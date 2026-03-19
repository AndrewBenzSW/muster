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
// The Models struct is populated from the resolved tool's tier mappings in userCfg.
// If the resolved tool is "opencode", Models.Fast will use opencode's fast tier mapping,
// not claude's. This ensures template-generated commands use correct model names.
func NewPromptContext(
	resolved *config.ResolvedConfig,
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

	// Populate Models from the resolved tool's tier mappings
	// This ensures template-generated commands use the correct model names for the active tool
	if userCfg.Tools != nil {
		if toolCfg, ok := userCfg.Tools[resolved.Tool]; ok && toolCfg != nil && toolCfg.ModelTiers != nil {
			if toolCfg.ModelTiers.Fast != nil {
				ctx.Models.Fast = *toolCfg.ModelTiers.Fast
			}
			if toolCfg.ModelTiers.Standard != nil {
				ctx.Models.Standard = *toolCfg.ModelTiers.Standard
			}
			if toolCfg.ModelTiers.Deep != nil {
				ctx.Models.Deep = *toolCfg.ModelTiers.Deep
			}
		}
	}

	// Fall back to user-level model tiers if tool-specific tiers are not available
	if ctx.Models.Fast == "" && userCfg.ModelTiers != nil && userCfg.ModelTiers.Fast != nil {
		ctx.Models.Fast = *userCfg.ModelTiers.Fast
	}
	if ctx.Models.Standard == "" && userCfg.ModelTiers != nil && userCfg.ModelTiers.Standard != nil {
		ctx.Models.Standard = *userCfg.ModelTiers.Standard
	}
	if ctx.Models.Deep == "" && userCfg.ModelTiers != nil && userCfg.ModelTiers.Deep != nil {
		ctx.Models.Deep = *userCfg.ModelTiers.Deep
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
