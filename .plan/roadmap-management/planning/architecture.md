# Roadmap Management: Architecture

*Date: 2026-03-18*

## Overview

Roadmap management adds three flat commands (`muster status`, `muster add`, `muster sync`) backed by a new `internal/roadmap/` package for file I/O and validation, new formatter functions in `internal/ui/`, a picker component in `internal/ui/picker.go` using `charmbracelet/huh`, and two new prompt templates for AI-assisted operations. The design mirrors established patterns throughout: backward-compatible file loading with fallback paths (as in `internal/config/project.go`), centralized output formatting switched on `ui.OutputMode` (as in `internal/ui/output.go`), config resolution via `config.ResolveStep()` for per-operation model selection, and template-based AI prompting through the existing `internal/prompt/` system.

The core architectural decision is to keep all roadmap persistence logic in `internal/roadmap/` -- isolated from commands and UI -- so that the file format, validation rules, and migration behavior can be tested independently. Commands in `cmd/` remain thin orchestrators: load config, load roadmap, call business logic, format output. AI-assisted operations (`add`, `sync`) render prompt templates, pipe them to the resolved external tool via stdin, buffer the response, and parse structured JSON output. This follows the established orchestration pattern from `cmd/code.go` rather than introducing direct API calls.

The picker component lives in `internal/ui/` alongside existing output formatting code. It wraps `charmbracelet/huh`'s `Select` widget behind a simple `ShowPicker()` function that accepts typed options and returns the selected value. This provides fuzzy filtering out of the box while keeping the dependency encapsulated behind a stable internal API that `cmd/status.go`, `cmd/add.go`, and future commands (`in`, `out`, `plan`) can share.

## Data Models

### internal/roadmap/roadmap.go

```go
package roadmap

import "errors"

// Sentinel errors for the roadmap package
var (
    // ErrRoadmapParse indicates a JSON parsing error in a roadmap file
    ErrRoadmapParse = errors.New("roadmap parse error")
)

// Priority represents a roadmap item priority level.
// Valid values: "high", "medium", "low", "lower"
type Priority string

const (
    PriorityHigh   Priority = "high"
    PriorityMedium Priority = "medium"
    PriorityLow    Priority = "low"
    PriorityLower  Priority = "lower"
)

// ValidPriorities returns the set of valid priority values.
func ValidPriorities() []Priority {
    return []Priority{PriorityHigh, PriorityMedium, PriorityLow, PriorityLower}
}

// IsValid returns true if the priority is a recognized value.
func (p Priority) IsValid() bool {
    switch p {
    case PriorityHigh, PriorityMedium, PriorityLow, PriorityLower:
        return true
    }
    return false
}

// Status represents a roadmap item lifecycle status.
// Valid values: "planned", "in_progress", "completed", "blocked"
type Status string

const (
    StatusPlanned    Status = "planned"
    StatusInProgress Status = "in_progress"
    StatusCompleted  Status = "completed"
    StatusBlocked    Status = "blocked"
)

// ValidStatuses returns the set of valid status values.
func ValidStatuses() []Status {
    return []Status{StatusPlanned, StatusInProgress, StatusCompleted, StatusBlocked}
}

// IsValid returns true if the status is a recognized value.
func (s Status) IsValid() bool {
    switch s {
    case StatusPlanned, StatusInProgress, StatusCompleted, StatusBlocked:
        return true
    }
    return false
}

// RoadmapItem represents a single roadmap entry.
// Required fields: Slug, Title, Priority, Status, Context.
// Optional fields use pointer types with omitempty to avoid serializing nil values.
type RoadmapItem struct {
    Slug     string   `json:"slug"`
    Title    string   `json:"title"`
    Priority Priority `json:"priority"`
    Status   Status   `json:"status"`
    Context  string   `json:"context"`
    PRUrl    *string  `json:"pr_url,omitempty"`
    Branch   *string  `json:"branch,omitempty"`
}

// Roadmap is the top-level container for roadmap items.
// This is always the in-memory representation, regardless of
// whether the file used array format or wrapper format on disk.
type Roadmap struct {
    Items []RoadmapItem `json:"items"`
}

// roadmapWrapper is the JSON wrapper format used for deserialization.
// It detects whether the file uses {"items": [...]} structure.
type roadmapWrapper struct {
    Items []RoadmapItem `json:"items"`
}
```

[Req 11: RoadmapItem struct with required/optional fields]
[Req 12: Priority and status enums with validation]

## Component Design

### internal/roadmap/loader.go -- File I/O

```go
package roadmap

import (
    "encoding/json"
    "fmt"
    "os"
    "path/filepath"
)

// LoadRoadmap loads the roadmap from .muster/roadmap.json (preferred) or
// .roadmap.json (legacy fallback). Returns an empty Roadmap if neither file
// exists. If the preferred file exists but is malformed, returns an error
// immediately without trying the fallback.
//
// After loading, Validate() is called automatically. Callers receive a
// validated Roadmap or an error -- never an invalid Roadmap.
func LoadRoadmap(dir string) (*Roadmap, error) {
    newPath := filepath.Join(dir, ".muster", "roadmap.json")
    roadmap, err := loadRoadmapFile(newPath)
    if err != nil {
        if !os.IsNotExist(err) {
            // Parse/read error on preferred file -- do NOT fall back
            return nil, fmt.Errorf("failed to load roadmap from %s: %w", newPath, err)
        }

        // Preferred file does not exist -- try legacy location
        legacyPath := filepath.Join(dir, ".roadmap.json")
        roadmap, err = loadRoadmapFile(legacyPath)
        if err != nil {
            if os.IsNotExist(err) {
                // Neither file exists -- return empty roadmap
                return &Roadmap{Items: []RoadmapItem{}}, nil
            }
            return nil, fmt.Errorf("failed to load roadmap from %s: %w", legacyPath, err)
        }
    }

    // Post-load validation
    if err := roadmap.Validate(); err != nil {
        return nil, err
    }

    return roadmap, nil
}

// loadRoadmapFile reads and parses a single roadmap file.
// Transparently handles both wrapper format ({"items": [...]}) and
// legacy array format ([...]).
func loadRoadmapFile(path string) (*Roadmap, error) {
    data, err := os.ReadFile(path)
    if err != nil {
        return nil, err // preserves os.PathError (IsNotExist works)
    }

    // Try wrapper format first (new canonical format)
    var wrapper roadmapWrapper
    if err := json.Unmarshal(data, &wrapper); err == nil && wrapper.Items != nil {
        return &Roadmap{Items: wrapper.Items}, nil
    }

    // Try array format (legacy)
    var items []RoadmapItem
    if err := json.Unmarshal(data, &items); err != nil {
        return nil, fmt.Errorf("failed to parse %s: %w: %w", path, ErrRoadmapParse, err)
    }

    return &Roadmap{Items: items}, nil
}

// SaveRoadmap writes the roadmap to .muster/roadmap.json in wrapper format.
// Creates the .muster/ directory if it does not exist.
// Always writes wrapper format regardless of the format that was loaded.
func SaveRoadmap(dir string, roadmap *Roadmap) error {
    musterDir := filepath.Join(dir, ".muster")
    if err := os.MkdirAll(musterDir, 0755); err != nil {
        return fmt.Errorf("failed to create .muster directory: %w", err)
    }

    data, err := json.MarshalIndent(roadmap, "", "  ")
    if err != nil {
        return fmt.Errorf("failed to marshal roadmap: %w", err)
    }

    // Append trailing newline for POSIX compliance
    data = append(data, '\n')

    path := filepath.Join(musterDir, "roadmap.json")
    if err := os.WriteFile(path, data, 0644); err != nil {
        return fmt.Errorf("failed to write roadmap to %s: %w", path, err)
    }

    return nil
}
```

[Req 1: Backward-compatible loading with fallback]
[Req 2: Automatic format detection (wrapper vs array)]
[Req 3: Consistent save to .muster/roadmap.json in wrapper format]
[Req 4: Parse error isolation -- no fallback on malformed primary]
[Req 18: Error context wrapping with file paths]
[Req 19: Directory creation safety with MkdirAll]

### internal/roadmap/validate.go -- Post-Load Validation

```go
package roadmap

import "fmt"

// Validate checks semantic correctness of the roadmap after loading.
// Fatal errors (returned as error):
//   - missing required fields (slug, title, priority, status, context)
//   - duplicate slugs
//   - invalid priority or status enum values
//
// Warnings (printed to stderr, not fatal):
//   - empty context (technically required but may be blank during creation)
func (r *Roadmap) Validate() error {
    seen := make(map[string]bool)

    for i, item := range r.Items {
        if item.Slug == "" {
            return fmt.Errorf("roadmap item %d: slug is required", i)
        }
        if item.Title == "" {
            return fmt.Errorf("roadmap item %d (%s): title is required", i, item.Slug)
        }
        if !item.Priority.IsValid() {
            return fmt.Errorf("roadmap item %d (%s): invalid priority %q (valid: high, medium, low, lower)", i, item.Slug, item.Priority)
        }
        if !item.Status.IsValid() {
            return fmt.Errorf("roadmap item %d (%s): invalid status %q (valid: planned, in_progress, completed, blocked)", i, item.Slug, item.Status)
        }
        if item.Context == "" {
            return fmt.Errorf("roadmap item %d (%s): context is required", i, item.Slug)
        }
        if seen[item.Slug] {
            return fmt.Errorf("roadmap: duplicate slug %q", item.Slug)
        }
        seen[item.Slug] = true
    }

    return nil
}

// FindBySlug returns the item with the given slug, or nil if not found.
func (r *Roadmap) FindBySlug(slug string) *RoadmapItem {
    for i := range r.Items {
        if r.Items[i].Slug == slug {
            return &r.Items[i]
        }
    }
    return nil
}

// AddItem appends a new item to the roadmap after validating it
// does not create a duplicate slug.
func (r *Roadmap) AddItem(item RoadmapItem) error {
    if r.FindBySlug(item.Slug) != nil {
        return fmt.Errorf("roadmap item with slug %q already exists", item.Slug)
    }
    r.Items = append(r.Items, item)
    return nil
}
```

[Req 5: Post-load validation of required fields, unique slugs, enum values]

### internal/ui/picker.go -- Interactive Picker

```go
package ui

import (
    "fmt"

    "github.com/charmbracelet/huh"
)

// PickerOption represents a single selectable option in the picker.
type PickerOption struct {
    Label string // Display text shown to user
    Value string // Machine value returned on selection
}

// PickerConfig holds optional configuration for the picker.
type PickerConfig struct {
    Height         int    // Number of visible options (default: 15)
    DefaultValue   string // Pre-selected value (empty = no default)
    FilteringLabel string // Placeholder text for filter input
}

// DefaultPickerConfig returns sensible defaults for picker configuration.
func DefaultPickerConfig() PickerConfig {
    return PickerConfig{
        Height:         15,
        FilteringLabel: "Type to filter...",
    }
}

// ShowPicker displays an interactive fuzzy-filterable selection list.
// Returns the selected option's Value, or an error if the user cancels
// (ESC/Ctrl+C) or if options is empty.
//
// Usage:
//
//     selected, err := ui.ShowPicker("Select item:", options, ui.DefaultPickerConfig())
//     if err != nil {
//         // user cancelled or error
//     }
func ShowPicker(title string, options []PickerOption, cfg PickerConfig) (string, error) {
    if len(options) == 0 {
        return "", fmt.Errorf("picker: no options provided")
    }

    if cfg.Height <= 0 {
        cfg.Height = 15
    }

    var selected string

    huhOptions := make([]huh.Option[string], len(options))
    for i, opt := range options {
        huhOptions[i] = huh.NewOption(opt.Label, opt.Value)
    }

    selectField := huh.NewSelect[string]().
        Title(title).
        Options(huhOptions...).
        Value(&selected).
        Filtering(true).
        Height(cfg.Height)

    err := huh.NewForm(
        huh.NewGroup(selectField),
    ).Run()

    if err != nil {
        return "", fmt.Errorf("picker cancelled: %w", err)
    }

    return selected, nil
}
```

[Req 16: Fuzzy-filterable picker using charmbracelet/huh]
[Req 17: Type-safe API with PickerOption structs, returns value + error]
[Req 28: Configuration struct for height, default selection]
[Req 29: Cancellation error handling]

### internal/ui/roadmap_format.go -- Output Formatting

```go
package ui

import (
    "encoding/json"
    "fmt"
    "strings"
    "text/tabwriter"
)

// RoadmapTableItem is the display model for table/JSON output.
// Kept separate from internal/roadmap.RoadmapItem to decouple
// display representation from persistence model.
type RoadmapTableItem struct {
    Slug     string `json:"slug"`
    Title    string `json:"title"`
    Priority string `json:"priority"`
    Status   string `json:"status"`
}

// RoadmapDetailItem is the display model for single-item detail view.
type RoadmapDetailItem struct {
    Slug     string  `json:"slug"`
    Title    string  `json:"title"`
    Priority string  `json:"priority"`
    Status   string  `json:"status"`
    Context  string  `json:"context"`
    PRUrl    *string `json:"pr_url,omitempty"`
    Branch   *string `json:"branch,omitempty"`
}

// FormatRoadmapTable formats a list of roadmap items for table or JSON output.
// TableMode renders a tabwriter-aligned table with columns: SLUG, TITLE, PRIORITY, STATUS.
// JSONMode renders a JSON array of objects.
func FormatRoadmapTable(items []RoadmapTableItem) (string, error) {
    mode := GetOutputMode()

    switch mode {
    case JSONMode:
        data, err := json.MarshalIndent(items, "", "  ")
        if err != nil {
            return "", err
        }
        return string(data), nil

    case TableMode:
        fallthrough
    default:
        if len(items) == 0 {
            return "No roadmap items found. Run 'muster add' to create one.", nil
        }

        var buf strings.Builder
        w := tabwriter.NewWriter(&buf, 0, 0, 2, ' ', 0)
        fmt.Fprintln(w, "SLUG\tTITLE\tPRIORITY\tSTATUS")
        for _, item := range items {
            fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", item.Slug, item.Title, item.Priority, item.Status)
        }
        w.Flush()
        return strings.TrimRight(buf.String(), "\n"), nil
    }
}

// FormatRoadmapDetail formats a single roadmap item in detail or JSON output.
// TableMode renders a labeled key-value layout.
// JSONMode renders a single JSON object.
func FormatRoadmapDetail(item RoadmapDetailItem) (string, error) {
    mode := GetOutputMode()

    switch mode {
    case JSONMode:
        data, err := json.MarshalIndent(item, "", "  ")
        if err != nil {
            return "", err
        }
        return string(data), nil

    case TableMode:
        fallthrough
    default:
        var buf strings.Builder
        fmt.Fprintf(&buf, "Slug:       %s\n", item.Slug)
        fmt.Fprintf(&buf, "Title:      %s\n", item.Title)
        fmt.Fprintf(&buf, "Priority:   %s\n", item.Priority)
        fmt.Fprintf(&buf, "Status:     %s\n", item.Status)
        fmt.Fprintf(&buf, "Context:    %s", item.Context)
        if item.PRUrl != nil {
            fmt.Fprintf(&buf, "\nPR URL:     %s", *item.PRUrl)
        }
        if item.Branch != nil {
            fmt.Fprintf(&buf, "\nBranch:     %s", *item.Branch)
        }
        return buf.String(), nil
    }
}

// FormatRoadmapItem formats a single item confirmation (used after add/sync).
// JSONMode outputs the full item as JSON. TableMode outputs a brief confirmation.
func FormatRoadmapItem(item RoadmapDetailItem) (string, error) {
    mode := GetOutputMode()

    switch mode {
    case JSONMode:
        data, err := json.MarshalIndent(item, "", "  ")
        if err != nil {
            return "", err
        }
        return string(data), nil

    case TableMode:
        fallthrough
    default:
        return fmt.Sprintf("Added: %s (%s, %s)", item.Slug, item.Priority, item.Status), nil
    }
}
```

[Req 10: Centralized output formatting through internal/ui formatters]
[Req 23: Table formatting with text/tabwriter]

### cmd/status.go

```go
var statusCmd = &cobra.Command{
    Use:   "status [slug]",
    Short: "Display roadmap status",
    Long: `Display roadmap items in table, detail, or JSON format.

With no arguments, shows all roadmap items in a table.
With a slug argument, shows detailed information for that item.

The output format respects the --format flag (json|table).`,
    Args: cobra.MaximumNArgs(1),
    RunE: func(cmd *cobra.Command, args []string) error {
        rm, err := roadmap.LoadRoadmap(".")
        if err != nil {
            if errors.Is(err, roadmap.ErrRoadmapParse) {
                return fmt.Errorf("roadmap file malformed: %w", err)
            }
            return fmt.Errorf("failed to load roadmap: %w", err)
        }

        if len(args) == 1 {
            // Detail view for a specific slug
            item := rm.FindBySlug(args[0])
            if item == nil {
                return fmt.Errorf("roadmap item %q not found", args[0])
            }
            detail := toDetailItem(item)
            output, err := ui.FormatRoadmapDetail(detail)
            if err != nil {
                return fmt.Errorf("failed to format roadmap detail: %w", err)
            }
            fmt.Fprintln(cmd.OutOrStdout(), output)
            return nil
        }

        // Table view of all items
        tableItems := toTableItems(rm.Items)
        output, err := ui.FormatRoadmapTable(tableItems)
        if err != nil {
            return fmt.Errorf("failed to format roadmap table: %w", err)
        }
        fmt.Fprintln(cmd.OutOrStdout(), output)
        return nil
    },
}

func init() {
    rootCmd.AddCommand(statusCmd)
}
```

The `toTableItems()` and `toDetailItem()` helper functions perform the mapping from `roadmap.RoadmapItem` to `ui.RoadmapTableItem` / `ui.RoadmapDetailItem`. These live in `cmd/status.go` as private helpers since the mapping is command-specific.

[Req 6: Status command with table/detail/JSON output]
[Req 9: Cobra registration via init()]

### cmd/add.go

```go
var addCmd = &cobra.Command{
    Use:   "add",
    Short: "Add a new roadmap item",
    Long: `Add a new roadmap item with optional AI assistance.

In interactive mode (default), prompts for item details and uses AI to
generate a slug from the title and expand the context description.

In batch mode (when --title is provided), creates the item directly
from flag values without AI assistance.

Flags:
  --title      Item title (required for batch mode)
  --priority   Priority: high|medium|low|lower (default: medium)
  --status     Status: planned|in_progress|completed|blocked (default: planned)
  --context    Context description (required for batch mode, or use - for stdin)`,
    Args: cobra.NoArgs,
    RunE: func(cmd *cobra.Command, args []string) error {
        // 1. Load roadmap
        rm, err := roadmap.LoadRoadmap(".")
        // ... error handling per established pattern

        // 2. Determine mode: batch (flags provided) vs interactive (AI-assisted)
        title, _ := cmd.Flags().GetString("title")

        if title != "" {
            // Batch mode: create item directly from flags
            item := buildItemFromFlags(cmd)
            if err := rm.AddItem(item); err != nil {
                return err
            }
            if err := roadmap.SaveRoadmap(".", rm); err != nil {
                return err
            }
            // Format and print confirmation
            return nil
        }

        // 3. Interactive/AI mode
        if !ui.IsInteractive() {
            return fmt.Errorf("interactive mode requires a terminal; use --title for batch mode")
        }

        // 4. Load config, resolve step
        userCfg, _ := config.LoadUserConfig("")
        projectCfg, _ := config.LoadProjectConfig(".")
        resolved, _ := config.ResolveStep("add", projectCfg, userCfg)

        // 5. Create prompt context, render template
        ctx := prompt.NewPromptContext(resolved, userCfg, true, "", ".", ".", "")
        rendered, _ := prompt.RenderTemplate("prompts/add-item/add-item-prompt.md.tmpl", ctx)

        // 6. Execute AI tool, capture response
        execCmd := exec.Command(resolved.Tool, "--prompt", "-")
        execCmd.Stdin = strings.NewReader(rendered)
        var stdout bytes.Buffer
        execCmd.Stdout = &stdout
        execCmd.Stderr = os.Stderr
        // ... run, parse JSON response, validate, add to roadmap, save

        return nil
    },
}

func init() {
    rootCmd.AddCommand(addCmd)
    addCmd.Flags().String("title", "", "Item title (enables batch mode)")
    addCmd.Flags().String("priority", "medium", "Priority: high|medium|low|lower")
    addCmd.Flags().String("status", "planned", "Status: planned|in_progress|completed|blocked")
    addCmd.Flags().String("context", "", "Context description (use - for stdin)")
}
```

**Batch mode flow**: When `--title` is provided, `add` bypasses AI entirely. A slug is generated by lowercasing the title, replacing spaces with hyphens, and stripping non-alphanumeric characters. No external tool invocation is needed.

**Interactive/AI mode flow**:
1. Resolve config for step `"add"` (uses fast model tier by default)
2. Render `add-item-prompt.md.tmpl` which instructs the AI to output a JSON object
3. Pipe rendered prompt via stdin to the resolved tool
4. Buffer stdout, parse as JSON into `RoadmapItem`
5. If parse fails, return error with the raw response for debugging
6. Show generated item to user for confirmation (using picker or simple y/n)
7. On confirmation, add to roadmap and save

[Req 7: Add command with AI-assisted interactive and batch modes]
[Req 13: Config resolution per operation via ResolveStep("add")]
[Req 15: Fast model tier for lightweight add operations]

### cmd/sync.go

```go
var syncCmd = &cobra.Command{
    Use:   "sync",
    Short: "Sync roadmap from external source",
    Long: `Sync roadmap items from a source file into the target roadmap using
AI-assisted fuzzy matching to reconcile differences.

By default, syncs from .roadmap.json (source) into .muster/roadmap.json (target).
Use --source and --target to override file paths.

The sync is unidirectional: source -> target. Items in the source that match
existing target items (by slug or AI fuzzy match) update the target. New items
in the source are added. Target items not in the source are preserved unless
--delete is specified.`,
    Args: cobra.NoArgs,
    RunE: func(cmd *cobra.Command, args []string) error {
        dryRun, _ := cmd.Flags().GetBool("dry-run")
        yes, _ := cmd.Flags().GetBool("yes")
        deleteMissing, _ := cmd.Flags().GetBool("delete")
        source, _ := cmd.Flags().GetString("source")
        target, _ := cmd.Flags().GetString("target")

        // 1. Load source and target roadmaps
        // 2. Match items by slug (exact match first)
        // 3. For unmatched items, use AI fuzzy matching
        //    - Resolve config for step "sync"
        //    - Render sync-match-prompt.md.tmpl with source/target items
        //    - Parse AI response for match pairs with confidence scores
        // 4. For low-confidence matches, show picker for manual resolution
        //    (unless --yes flag skips confirmation)
        // 5. If --dry-run, display planned changes and exit
        // 6. Apply changes: update matched items, add new items,
        //    optionally delete unmatched target items (--delete)
        // 7. Save updated target roadmap
        // 8. Display summary

        return nil
    },
}

func init() {
    rootCmd.AddCommand(syncCmd)
    syncCmd.Flags().String("source", ".roadmap.json", "Source roadmap file path")
    syncCmd.Flags().String("target", ".muster/roadmap.json", "Target roadmap file path")
    syncCmd.Flags().Bool("yes", false, "Skip confirmation prompts")
    syncCmd.Flags().Bool("dry-run", false, "Preview changes without applying")
    syncCmd.Flags().Bool("delete", false, "Remove target items not found in source")
}
```

**Sync algorithm**:
1. Exact slug match: items with identical slugs are matched immediately
2. AI fuzzy match: remaining unmatched items are sent to the AI tool via `sync-match-prompt.md.tmpl`, which returns match pairs with confidence scores
3. Low-confidence matches (below a threshold) are shown to the user via the picker for manual confirmation, unless `--yes` is set
4. Changes are categorized as: updated (matched items with different field values), added (source items with no match), deleted (target items not in source, only if `--delete`)
5. `--dry-run` prints the change summary without writing

[Req 8: Sync command with --yes and --dry-run flags]

### Prompt Templates

Two new template files are added to `internal/prompt/prompts/`:

**`internal/prompt/prompts/add-item/add-item-prompt.md.tmpl`**

```markdown
You are a roadmap item generator. Given user input, produce a single
JSON object with the following schema:

{
  "slug": "kebab-case-identifier",
  "title": "Descriptive title",
  "priority": "high|medium|low|lower",
  "status": "planned",
  "context": "Expanded technical context"
}

Rules:
- slug: derive from title, lowercase, hyphens only, max 40 chars
- title: concise but descriptive, no trailing period
- priority: suggest based on context; default to "medium"
- context: expand user's description into actionable technical context

Model: {{.Models.Fast}}

{{if .Interactive}}
The user will provide a title and optional description. Generate the item
and output ONLY the JSON object, no markdown fences, no explanation.
{{else}}
Generate the item from the provided input. Output ONLY the JSON object.
{{end}}
```

**`internal/prompt/prompts/sync-match/sync-match-prompt.md.tmpl`**

```markdown
You are a roadmap item matcher. Given a list of source items and target
items, identify which source items correspond to which target items.

Output a JSON array of match objects:

[
  {
    "source_slug": "source-item-slug",
    "target_slug": "target-item-slug",
    "confidence": 0.95,
    "reason": "Brief explanation of match"
  }
]

If a source item has no match in the target, omit it from the array.
Output ONLY the JSON array, no markdown fences, no explanation.

Model: {{.Models.Fast}}

## Source Items
{{range .SourceItems}}
- slug: {{.Slug}}, title: {{.Title}}
{{end}}

## Target Items
{{range .TargetItems}}
- slug: {{.Slug}}, title: {{.Title}}
{{end}}
```

Note: The sync template requires extending `PromptContext` with `SourceItems` and `TargetItems` fields. This is handled by adding an `Extra map[string]interface{}` field to `PromptContext`, or by creating a `SyncPromptContext` struct that embeds `PromptContext` and adds the item lists. The latter approach is preferred for type safety.

[Req 14: Template-based prompting in internal/prompt/prompts/]
[Req 15: Fast model tier via {{.Models.Fast}}]

## Integration Points

### Config Resolution

Commands `add` and `sync` call `config.ResolveStep("add", ...)` and `config.ResolveStep("sync", ...)` respectively, enabling users to configure different models per operation in `.muster/config.yml`:

```yaml
pipeline:
  add:
    model: muster-fast
  sync:
    model: muster-fast
```

This follows the established pattern from `cmd/code.go` which calls `config.ResolveCode()` (a wrapper around `ResolveStep("code", ...)`).

### Output Formatting

All three commands use `cmd.OutOrStdout()` for output (enabling test buffer redirection) and call `ui.FormatRoadmap*()` functions that switch on `ui.GetOutputMode()`. The mode is set by `root.go`'s `PersistentPreRunE` based on the `--format` flag or TTY auto-detection. No command needs to handle mode switching itself.

### Error Handling

Each command follows the three-tier pattern:
1. Check `errors.Is(err, roadmap.ErrRoadmapParse)` for malformed file errors, providing file-specific guidance
2. Check `errors.As(err, &execErr)` for tool-not-found errors during AI invocation, providing installation guidance
3. Wrap all other errors with `fmt.Errorf("context: %w", err)` for chain traceability

### Verbose Output

When `--verbose` is set (inherited from root), `add` and `sync` print to stderr:
- Resolved config triple (tool, provider, model)
- Template path being rendered
- AI tool invocation command

This follows the pattern at `cmd/code.go:72-75`.

### Prompt Context Extension

For `sync`, the standard `PromptContext` needs source/target item data. Rather than polluting the shared struct, define a `SyncPromptData` struct:

```go
// In cmd/sync.go or internal/roadmap/sync.go
type SyncPromptData struct {
    *prompt.PromptContext
    SourceItems []roadmap.RoadmapItem
    TargetItems []roadmap.RoadmapItem
}
```

The sync template is rendered by creating a new `template.Template` from the embedded template string and executing it against `SyncPromptData`. This avoids modifying the shared `PromptContext` struct while still having access to `{{.Models.Fast}}` and other standard fields.

For `add`, the standard `PromptContext` is sufficient since the user's input is piped as the prompt body, not embedded in the template.

## File Changes

### New Files

| File | Purpose |
|------|---------|
| `internal/roadmap/roadmap.go` | Type definitions: RoadmapItem, Roadmap, Priority, Status, sentinel errors |
| `internal/roadmap/loader.go` | LoadRoadmap(), loadRoadmapFile(), SaveRoadmap() |
| `internal/roadmap/validate.go` | Validate(), FindBySlug(), AddItem() |
| `internal/roadmap/roadmap_test.go` | Tests for types, validation, enum methods |
| `internal/roadmap/loader_test.go` | Tests for load/save with format detection, fallback, error cases |
| `internal/ui/picker.go` | ShowPicker() wrapping charmbracelet/huh |
| `internal/ui/picker_test.go` | Tests for picker option building, empty options error |
| `internal/ui/roadmap_format.go` | FormatRoadmapTable(), FormatRoadmapDetail(), FormatRoadmapItem() |
| `internal/ui/roadmap_format_test.go` | Tests for table/detail/JSON formatting |
| `cmd/status.go` | Status command with table/detail views |
| `cmd/status_test.go` | Tests for status command flags, output modes, slug lookup |
| `cmd/add.go` | Add command with batch and AI-assisted modes |
| `cmd/add_test.go` | Tests for add command flags, batch mode, error handling |
| `cmd/sync.go` | Sync command with fuzzy matching |
| `cmd/sync_test.go` | Tests for sync command flags, dry-run, delete behavior |
| `internal/prompt/prompts/add-item/add-item-prompt.md.tmpl` | AI prompt for item generation |
| `internal/prompt/prompts/sync-match/sync-match-prompt.md.tmpl` | AI prompt for fuzzy matching |

### Modified Files

| File | Change |
|------|--------|
| `go.mod` | Add `github.com/charmbracelet/huh` dependency |
| `go.sum` | Updated by `go mod tidy` |
| `internal/prompt/stage.go` | Add `"roadmap-add-item"` and `"roadmap-sync-match"` to `expectedSkills` verification list |

### Unchanged Files

`cmd/root.go`, `cmd/code.go`, `cmd/version.go`, `internal/config/*`, `internal/prompt/template.go`, `internal/prompt/context.go`, `internal/prompt/embed.go` -- these files require no modifications. The new commands register themselves via `init()` and the new templates are picked up automatically by the embedded filesystem.

## Technology Choices

### charmbracelet/huh for Interactive Picker

**Rationale**: Best balance of API ergonomics (method chaining, generics), built-in fuzzy filtering (`.Filtering(true)`), active maintenance (March 2026), and future extensibility via Bubbletea integration. The library is purpose-built for forms and pickers.

**Trade-off**: Adds ~10 direct dependencies from the Charm ecosystem. Acceptable because (a) the picker is a user-facing interactive component where polish matters, (b) the dependencies are well-maintained by the same team, and (c) huh provides a migration path to richer TUI features if needed in later phases.

**Alternatives rejected**: pterm (string-only values, no type safety), promptui (unmaintained since 2021), raw Bubbletea (too much boilerplate for simple picker).

### text/tabwriter for Table Output

**Rationale**: Standard library, zero additional dependencies, sufficient for aligned column output. The status table has 4 columns (slug, title, priority, status) with predictable widths. No need for box-drawing characters or color at this stage.

**Alternative deferred**: lipgloss/table (from Charm ecosystem) or tablewriter. These can be adopted later if users request richer table formatting. Starting with stdlib follows the project convention of minimal dependencies.

### json.MarshalIndent for Roadmap Persistence

**Rationale**: JSON is the established format for `.roadmap.json`. Using `encoding/json` from stdlib with 2-space indent matches the existing file format exactly. No need for a third-party JSON library.

### Stdin Piping for AI Prompts

**Rationale**: Rendered prompts are piped to the AI tool via `cmd.Stdin = strings.NewReader(prompt)`. This is simpler than writing temp files, avoids cleanup concerns, and works consistently across claude-code and opencode. A `--debug-prompt` flag can write the prompt to stderr for debugging without changing the execution path.

**Trade-off**: Stdin piping makes it harder to inspect prompts post-hoc compared to temp files. The `--debug-prompt` flag mitigates this. The `--keep-staged` pattern from `cmd/code.go` is not applicable here since add/sync render a single prompt string, not a directory of skill files.

### Buffered Response Capture

**Rationale**: AI responses for `add` and `sync` are captured into a `bytes.Buffer` rather than streamed to stdout. This enables JSON parsing of the complete response. Streaming would provide better perceived latency but complicates parsing and is unnecessary for the small payloads these operations produce (a single JSON object or a short match array).

### Unidirectional Sync (Source to Target)

**Rationale**: The MVP sync direction is source -> target. This covers the primary use case of updating `.muster/roadmap.json` from an external source. Bidirectional sync adds significant complexity (conflict resolution, merge strategies) that is not needed for the initial implementation. The `--source` and `--target` flags provide flexibility for different file pairs.
