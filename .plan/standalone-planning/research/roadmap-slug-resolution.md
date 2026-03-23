# Roadmap Slug Resolution and Pickers

*Researched: 2026-03-23*
*Scope: Audit of internal/roadmap and internal/ui to understand how slugs are loaded, validated, and presented in interactive pickers*

---

## Key Findings

### Slug Resolution Pattern
- **FindBySlug method**: `/Users/andrew.benz/work/muster/muster-main/internal/roadmap/validate.go:63-77` provides the core slug lookup functionality
- **Returns nil** when slug not found (safe nil handling)
- **Case-sensitive** exact match required
- Used by existing commands: `out` (line 103), `status` (line 30)

### Picker Infrastructure
- **HuhPicker implementation**: `/Users/andrew.benz/work/muster/muster-main/internal/ui/picker.go:37-81` uses charmbracelet/huh for TUI
- **MockPicker available** for testing at `picker_test.go:13-23`
- **DefaultPicker** package-level instance exported at line 84
- **Filtering enabled by default** on all pickers (line 64)
- Currently used by: `cmd/add.go:229-237` for confirm/cancel choice after AI generation

### Roadmap Loading
- **LoadRoadmap**: `/Users/andrew.benz/work/muster/muster-main/internal/roadmap/loader.go:14-36` handles dual-location fallback
- Primary: `.muster/roadmap.json`
- Legacy: `.roadmap.json`
- Returns empty roadmap if neither exists (not an error)
- Parse errors from primary file do NOT fall back to legacy

### Error Handling Patterns
Commands consistently check:
1. `roadmap.ErrRoadmapParse` for malformed JSON (wrapped errors)
2. `config.ErrConfigParse` for config file issues
3. `nil` result from FindBySlug for "item not found" user errors

---

## Detailed Analysis

### Slug Generation and Validation

**GenerateSlug function** (`internal/roadmap/slug.go:24-54`):
- Converts title to lowercase
- Replaces spaces/underscores with hyphens
- Strips all non-alphanumeric characters except hyphens
- Collapses consecutive hyphens
- Trims leading/trailing hyphens
- Truncates to 40 character maximum
- Used in batch mode of `add` command (line 94)

**Slug constraints** (from `validate.go` and tests):
- Required field (cannot be empty or whitespace-only)
- Must be unique across all items
- Max 40 characters
- Format: `[a-z0-9-]+` (kebab-case)

### Interactive Mode Detection

**IsInteractive check** (`internal/ui/output.go` via `cmd/root.go:37-41`):
- Used to auto-detect output mode (table vs JSON)
- Set via `--format` flag or TTY detection
- `add` command requires both `ui.IsInteractive()` AND `term.IsTerminal(int(os.Stdin.Fd()))` for AI mode

### Picker Usage Pattern (from add.go)

```go
// 1. Create options slice
options := []ui.PickerOption{
    {Label: "Confirm - Add this item", Value: "confirm"},
    {Label: "Cancel - Don't add", Value: "cancel"},
}

// 2. Show picker with default config
choice, err := ui.DefaultPicker.Show("What would you like to do?", options, ui.DefaultPickerConfig())
if err != nil {
    return fmt.Errorf("picker failed: %w", err)
}

// 3. Handle selection
if choice != "confirm" {
    // User cancelled
}
```

### Config Resolution for Steps

**ResolveStep function** pattern (from `cmd/out.go:114-117`, `cmd/add.go:59-62`):
- `config.ResolveStep(stepName, projectCfg, userCfg)` returns `*config.ResolvedConfig`
- Step names: "add", "sync", "out", "code" (special case: `ResolveCode`)
- Returns tool/provider/model triple with source annotations
- Steps can have default model tiers (e.g., "add" and "sync" default to `muster-fast`)

### Command Argument Patterns

**Slug as positional argument**:
- `out` command: `Use: "out [slug]"` with `Args: cobra.ExactArgs(1)` (line 27, 43)
- `status` command: `Use: "status [slug]"` with `Args: cobra.MaximumNArgs(1)` (line 13, 15)
  - Optional slug: if provided, shows detail view; if omitted, shows table view

---

## Recommendations

### For muster plan [slug] Implementation

**Slug resolution strategy**:
1. If slug arg provided: use `roadmap.LoadRoadmap(".")` then `rm.FindBySlug(slug)`
   - Error if roadmap malformed (check `roadmap.ErrRoadmapParse`)
   - Error if slug not found: `fmt.Errorf("roadmap item %q not found", slug)`
2. If no slug arg: use interactive picker
   - Require TTY with `ui.IsInteractive()`
   - Build picker options from `rm.Items`
   - Handle picker cancellation (ESC/Ctrl+C returns error)

**Picker option construction**:
```go
// Convert roadmap items to picker options
options := make([]ui.PickerOption, len(rm.Items))
for i, item := range rm.Items {
    // Label format: "slug - title [priority, status]"
    // This matches the brief format from ui.FormatRoadmapItem
    label := fmt.Sprintf("%s - %s [%s, %s]",
        item.Slug, item.Title, item.Priority, item.Status)
    options[i] = ui.PickerOption{
        Label: label,
        Value: item.Slug, // Return slug for resolution
    }
}

cfg := ui.DefaultPickerConfig()
// Consider filtering out completed items:
// cfg.FilteringLabel = "Filter:" (already default)

slug, err := ui.DefaultPicker.Show("Select roadmap item to plan", options, cfg)
```

**Error handling** (follow existing patterns):
- Roadmap parse errors: `if errors.Is(err, roadmap.ErrRoadmapParse)`
- Config parse errors: `if errors.Is(err, config.ErrConfigParse)`
- Item not found: direct user-facing error (not wrapped)
- Picker cancellation: inform user, do not treat as fatal error

**Command structure**:
```go
Use:   "plan [slug]",
Short: "Create an implementation plan for a roadmap item",
Args:  cobra.MaximumNArgs(1), // Optional slug
```

**Argument handling**:
```go
var slug string
if len(args) > 0 {
    // Explicit slug from args
    slug = args[0]
} else {
    // Interactive picker mode
    if !ui.IsInteractive() {
        return fmt.Errorf("interactive mode requires a terminal (TTY)")
    }
    // Show picker, get slug
}
```

### Filtering Recommendations

**Item filtering** for picker:
- Consider excluding `status == StatusCompleted` items (planning completed items is likely a mistake)
- Do NOT exclude items with existing `.muster/work/{slug}/plan/` directories (user may want to regenerate)
- Sort by priority: high > medium > low > lower (use `roadmap.ValidPriorities()` order)

**Empty roadmap handling**:
```go
if len(rm.Items) == 0 {
    return fmt.Errorf("no roadmap items found. Run 'muster add' to create one")
}
// After filtering:
if len(options) == 0 {
    return fmt.Errorf("no eligible items to plan")
}
```

### Template Context

Use `prompt.NewPromptContext()` with resolved config:
- `slug`: the resolved slug value
- `worktreePath`: "." (plan happens in main repo)
- `mainRepoPath`: "."
- `planDir`: `.muster/work/{slug}/plan` (create if doesn't exist)
- `interactive`: true (plan command is always interactive in nature)

### Model Tier Selection

Use `muster-deep` tier for planning phase (highest capability):
- Research: iterate over topics, web search, codebase exploration
- Synthesis: analyze research, identify patterns
- Planning: comprehensive design, anticipate edge cases

From `cmd/out.go:458`: `promptCtx.Models.Standard` shows how to access tier models in templates.

---

## Open Questions

**Q1: Should plan command allow re-planning an item that already has a plan?**
Why it matters: User may want to regenerate after scope changes.
Decision needed: Overwrite existing plan, create versioned backup, or error?
Recommendation: Allow overwrite with warning message.

**Q2: Should the picker show items with status=completed?**
Why it matters: Planning a completed item is likely user error.
Decision needed: Filter by default, or require explicit flag?
Recommendation: Exclude completed by default. User can edit status first if needed.

**Q3: What should plan command do if .muster/work/{slug}/ already exists?**
Why it matters: May contain partial work or previous attempts.
Decision needed: Clean directory, merge with existing, or error?
Recommendation: Create plan/ subdirectory regardless. Research and synthesis outputs are ephemeral.

**Q4: Should picker sort items by priority, status, or slug?**
Why it matters: User experience when selecting from many items.
Decision needed: Natural order vs. prioritized order.
Recommendation: Sort by priority (high first), then by slug alphabetically within each priority.

---

## References

### File Paths
- `/Users/andrew.benz/work/muster/muster-main/internal/roadmap/validate.go` - FindBySlug implementation
- `/Users/andrew.benz/work/muster/muster-main/internal/roadmap/loader.go` - LoadRoadmap implementation
- `/Users/andrew.benz/work/muster/muster-main/internal/roadmap/slug.go` - GenerateSlug implementation
- `/Users/andrew.benz/work/muster/muster-main/internal/ui/picker.go` - HuhPicker implementation
- `/Users/andrew.benz/work/muster/muster-main/cmd/add.go:229-237` - Picker usage example
- `/Users/andrew.benz/work/muster/muster-main/cmd/out.go:103` - FindBySlug usage example
- `/Users/andrew.benz/work/muster/muster-main/cmd/status.go:30` - FindBySlug usage example
- `/Users/andrew.benz/work/muster/muster-main/internal/prompt/context.go` - PromptContext structure

### Key Patterns
- Error categorization: `errors.Is(err, roadmap.ErrRoadmapParse)`
- Config resolution: `config.ResolveStep(stepName, projectCfg, userCfg)`
- TTY detection: `ui.IsInteractive() && term.IsTerminal(int(os.Stdin.Fd()))`
- Picker usage: `ui.DefaultPicker.Show(title, options, cfg)`
- Model tiers: `promptCtx.Models.Fast/Standard/Deep`

### Data Structures
- `roadmap.RoadmapItem` - 7 fields (5 required, 2 optional pointers)
- `ui.PickerOption` - Label (display) and Value (return)
- `ui.PickerConfig` - Height, DefaultValue, FilteringLabel
- `prompt.PromptContext` - 8 core fields + Extra map

### Test Coverage
- `internal/roadmap/roadmap_test.go` - Comprehensive FindBySlug tests (lines 315-398)
- `internal/ui/picker_test.go` - MockPicker patterns and HuhPicker validation
- All commands use testify/assert and testify/require
