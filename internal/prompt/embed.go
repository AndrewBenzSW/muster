package prompt

import "embed"

//go:embed all:prompts
var Prompts embed.FS

// Prompts contains embedded prompt templates.
// Phase 1 will add real templates; this is Phase 0 scaffolding.
