# Existing CLI Command Patterns

*Researched: 2026-03-23*
*Scope: Audit cmd/add.go, cmd/sync.go, cmd/code.go, cmd/status.go, cmd/out.go to understand Cobra command structure, flag handling, argument parsing, and integration with internal packages*

---

## Key Findings

### Command Structure Pattern

All muster commands follow a consistent 8-step execution pattern:

1. **Flag parsing** — Use `cmd.Flags().GetString()`, `GetBool()`, etc. for local flags and `rootCmd.PersistentFlags()` for global flags (`--verbose`, `--format`)
2. **Config loading** — Load both user and project config with `config.LoadUserConfig("")` and `config.LoadProjectConfig(".")`, categorizing errors with `errors.Is(err, config.ErrConfigParse)`
3. **Config resolution** — Resolve step-specific config with `config.ResolveStep(stepName, projectCfg, userCfg)` which returns `*config.ResolvedConfig` with tool/provider/model triple
4. **Verbose logging** — If `--verbose` flag is set, print resolved triple and operation details to stderr
5. **Data loading** — Load roadmap via `roadmap.LoadRoadmap(".")` which handles fallback chain (`.muster/roadmap.json` → `.roadmap.json` → empty roadmap)
6. **Business logic** — Execute the command's core functionality (may involve AI invocation, VCS operations, etc.)
7. **Data persistence** — Save roadmap with `roadmap.SaveRoadmap(".", rm)` which always writes to `.muster/roadmap.json` in wrapper format
8. **Output formatting** — Use `ui.FormatRoadmapTable()` / `ui.FormatRoadmapDetail()` for structured output, or `fmt.Fprintf()` for simple messages

### Argument Handling Patterns

Commands use three distinct argument patterns:

- **No arguments** — `cmd/add.go` (interactive mode when no `--title` flag), `cmd/code.go`, `cmd/sync.go`
- **Optional single argument** — `cmd/status.go` uses `cobra.MaximumNArgs(1)` to accept `[slug]` for detail view vs table view
- **Required single argument** — `cmd/out.go` uses `cobra.ExactArgs(1)` to require a slug for post-merge cleanup

### Flag Categories

Commands define flags in three distinct categories:

1. **Mode flags** — Control execution mode (batch vs interactive, dry-run vs real)
   - `add`: `--title` (batch mode), `--priority`, `--status`, `--context`
   - `sync`: `--yes` (skip confirmations), `--dry-run`, `--delete` (remove unmatched items)
   - `code`: `--yolo` (sandboxed container), `--no-plugin`, `--keep-staged`
   - `out`: `--no-fix` (skip AI fixes), `--wait` (poll for completion), `--dry-run`

2. **Path/source flags** — Specify file paths or data sources
   - `sync`: `--source` (default `.roadmap.json`), `--target` (default `.muster/roadmap.json`)

3. **Global persistent flags** — Available to all commands via rootCmd
   - `--verbose`/`-v` — Enable detailed logging (set on rootCmd, checked in all commands)
   - `--format`/`-f` — Output format (`json` | `table`, auto-detected based on TTY)

### Slug Resolution Pattern

Commands that accept a slug argument follow a consistent resolution pattern:

1. **Direct argument** — If slug is provided as positional argument, use it directly
2. **Interactive picker** — If no argument and in interactive mode, use `ui.DefaultPicker.Show()` to select from roadmap items
3. **Validation** — Look up item with `rm.FindBySlug(slug)` and return error if not found

Example from `cmd/status.go:28-33`:
```go
if len(args) > 0 {
    slug := args[0]
    item := rm.FindBySlug(slug)
    if item == nil {
        return fmt.Errorf("roadmap item with slug %q not found", slug)
    }
}
```

### AI Invocation Pattern

Commands that use AI (`add`, `sync`, `out`) follow a 5-step invocation pattern:

1. **Create prompt context** — `prompt.NewPromptContext(resolved, projectCfg, userCfg, interactive, slug, worktreePath, mainRepoPath, planDir)`
2. **Populate Extra data** — Add command-specific data to `ctx.Extra` map (e.g., `UserInput`, `SourceItems`, `FailedChecks`)
3. **Render template** — `prompt.RenderTemplate("prompts/{folder}/{file}.md.tmpl", ctx)`
4. **Invoke AI** — `ai.InvokeAI(ai.InvokeConfig{Tool, Model, Prompt, Verbose, Env})` returns `*ai.InvokeResult`
5. **Parse response** — Use `ai.ExtractJSON(result.RawOutput)` to strip markdown fences, then unmarshal JSON

Example from `cmd/add.go:202-212`:
```go
result, err := ai.InvokeAI(ai.InvokeConfig{
    Tool:    config.ToolExecutable(resolved.Tool),
    Model:   resolved.Model,
    Prompt:  promptContent,
    Verbose: verbose,
    Env:     config.ToolEnvOverrides(resolved, projectCfg, userCfg),
})
if err != nil {
    return fmt.Errorf("AI invocation failed: %w", err)
}
jsonStr := ai.ExtractJSON(result.RawOutput)
```

### Error Handling Pattern

Commands categorize errors into three levels:

1. **Config errors** — Use `errors.Is(err, config.ErrConfigParse)` to distinguish malformed config from missing config
2. **Roadmap errors** — Use `errors.Is(err, roadmap.ErrRoadmapParse)` to distinguish parse errors from file-not-found
3. **Execution errors** — Return descriptive errors with context (e.g., `fmt.Errorf("failed to add item: %w", err)`)

Example from `cmd/add.go:42-56`:
```go
userCfg, err := config.LoadUserConfig("")
if err != nil {
    if errors.Is(err, config.ErrConfigParse) {
        return fmt.Errorf("config file malformed: %w", err)
    }
    return fmt.Errorf("failed to load user config: %w", err)
}
```

### Interactive Mode Detection

Commands check for TTY to determine if interactive features are available:

```go
if !ui.IsInteractive() || !term.IsTerminal(int(os.Stdin.Fd())) {
    return fmt.Errorf("interactive mode requires a terminal (TTY). Use --title flag for batch mode")
}
```

This pattern is used in:
- `cmd/add.go:159` — Falls back to batch mode prompting
- `cmd/sync.go:290-372` — Prompts user for low-confidence match acceptance
- All TUI picker invocations via `ui.DefaultPicker.Show()`

---

## Detailed Analysis

### Cobra Command Registration

Commands are registered in `init()` functions following this pattern:

```go
func init() {
    rootCmd.AddCommand(addCmd)

    // Define local flags
    addCmd.Flags().String("title", "", "Item title (batch mode)")
    addCmd.Flags().String("priority", string(roadmap.PriorityMedium), "Priority: high, medium, low, lower")

    // No persistent flags — those are defined on rootCmd only
}
```

- All commands are added to `rootCmd` (never nested subcommands)
- Flags use typed getters (`GetString`, `GetBool`, `GetDuration`)
- Default values are specified in flag definition, not in business logic
- Help text is concise and includes examples for non-obvious flags

### Config Resolution Chain

The 5-step resolution chain (from `internal/config/resolve.go`) is invoked via `config.ResolveStep(stepName, projectCfg, userCfg)`:

1. Pipeline step config (e.g., `plan` step in project config)
2. Project defaults
3. User defaults
4. Tool defaults
5. Hard-coded defaults (`claude-code`, `anthropic`, `sonnet`)

Each command passes its step name:
- `add` → `"add"` (uses `muster-fast` tier by default)
- `sync` → `"sync"` (uses `muster-fast` tier by default)
- `code` → calls `config.ResolveCode()` which uses `"interactive"` step
- `out` → `"out"` (uses `muster-standard` tier for CI fixes)

### Roadmap File Handling

Commands use a consistent 3-layer approach to roadmap files:

1. **Loading** — `roadmap.LoadRoadmap(".")` checks `.muster/roadmap.json` first, falls back to `.roadmap.json`, returns empty roadmap if neither exists
2. **Manipulation** — Use `rm.FindBySlug(slug)` for lookups, `rm.AddItem(item)` for additions (validates uniqueness and required fields)
3. **Saving** — `roadmap.SaveRoadmap(".", rm)` always writes to `.muster/roadmap.json` in wrapper format `{"items": [...]}`

Commands never read/write roadmap files directly — they always use the roadmap package API.

### Template Rendering and Staging

Two distinct patterns for using templates:

#### Pattern 1: Single-shot AI invocation (add, sync, out)

Used for one-off AI requests that produce structured output:

```go
ctx := prompt.NewPromptContext(resolved, projectCfg, userCfg, interactive, slug, worktreePath, mainRepoPath, planDir)
ctx.Extra["UserInput"] = userInput
promptContent, err := prompt.RenderTemplate("prompts/add-item/add-item-prompt.md.tmpl", ctx)
result, err := ai.InvokeAI(ai.InvokeConfig{Tool, Model, Prompt: promptContent, ...})
```

#### Pattern 2: Skill staging for interactive sessions (code)

Used for long-running AI agent sessions:

```go
ctx := prompt.NewPromptContext(resolved, projectCfg, userCfg, true, "", ".", ".", "")
tmpDir, cleanup, err := prompt.StageSkills(ctx)
defer cleanup()
execCmd := exec.Command(config.ToolExecutable(resolved.Tool), "--plugin-dir", tmpDir, "--model", resolved.Model)
```

Key difference: Pattern 1 renders a single template and passes it as a prompt string. Pattern 2 stages all skill templates to a temp directory and passes the directory to the tool via `--plugin-dir`.

### Model Tier Usage

Commands use tier names to abstract model selection:

- `muster-fast` — Lightweight operations (add, sync)
- `muster-standard` — Balanced operations (CI fixes, code review)
- `muster-deep` — Complex reasoning (deep planning, refactoring)

The `PromptContext.Models` struct is populated during creation and provides tier-to-model mappings for the resolved tool:

```go
ctx := prompt.NewPromptContext(resolved, projectCfg, userCfg, ...)
// ctx.Models.Fast, .Standard, .Deep are now populated based on config resolution
```

Templates can reference `{{.Models.Fast}}` to generate commands that use the correct model name for the active tool.

### Output Formatting

Commands use `internal/ui` package for structured output that adapts to mode:

1. **Table mode** (TTY detected or `--format table`)
   - `ui.FormatRoadmapTable(items)` — Tabular layout with headers
   - `ui.FormatRoadmapDetail(item)` — Labeled key-value layout
   - Empty state shows friendly message: "No roadmap items found. Run 'muster add' to create one."

2. **JSON mode** (non-TTY or `--format json`)
   - `ui.FormatRoadmapTable(items)` — Returns unwrapped array `[{...}, {...}]`
   - `ui.FormatRoadmapDetail(item)` — Returns single object `{...}`
   - Empty state returns `[]`

Output mode is set globally via `ui.SetOutputMode()` in `rootCmd.PersistentPreRunE`, so individual commands don't need to check TTY.

### Stdin Handling

Commands handle stdin input in two scenarios:

1. **Batch mode with piped context** (`cmd/add.go:99-121`)
   - Detects `--context "-"` flag
   - Reads from `os.Stdin` with 1MB limit using `io.LimitReader`
   - Validates non-empty after trimming whitespace

2. **Interactive text input** (`cmd/add.go:164-175`)
   - Uses `bufio.Scanner` for line-by-line reading
   - Reads until Enter keypress (single line)
   - Validates non-empty input

3. **TUI picker input** (via `ui.DefaultPicker.Show()`)
   - Uses charmbracelet/huh for interactive selection
   - Automatically filters options as user types
   - Returns selected value or error if cancelled (ESC/Ctrl+C)

### Test Patterns

Commands are tested using functional tests that:

1. Create temp directories with fixture data
2. Change working directory to temp location
3. Capture command output to `bytes.Buffer`
4. Execute `cmd.RunE(cmd, args)` directly (not via Cobra's Execute)
5. Assert on output content and error conditions

Example from `cmd/status_test.go:38-88`:
```go
func TestStatusCommand_TableOutputWithItems(t *testing.T) {
    tmpDir := t.TempDir()
    musterDir := filepath.Join(tmpDir, ".muster")
    require.NoError(t, os.MkdirAll(musterDir, 0755))

    roadmapContent := `{"items": [...]}`
    require.NoError(t, os.WriteFile(filepath.Join(musterDir, "roadmap.json"), []byte(roadmapContent), 0644))

    origDir, err := os.Getwd()
    require.NoError(t, err)
    require.NoError(t, os.Chdir(tmpDir))
    defer func() { _ = os.Chdir(origDir) }()

    buf := new(bytes.Buffer)
    statusCmd.SetOut(buf)

    err = statusCmd.RunE(statusCmd, []string{})
    assert.NoError(t, err)

    output := buf.String()
    assert.Contains(t, output, "SLUG")
    assert.Contains(t, output, "feature-a")
}
```

Tests use `testify/assert` and `testify/require` consistently across all command tests.

---

## Recommendations

Based on the analyzed command patterns, the `muster plan` command should:

### Must Have

1. **Follow the 8-step execution pattern**
   - Start with flag parsing and config loading
   - Use `config.ResolveStep("plan", projectCfg, userCfg)` for config resolution
   - Load roadmap with `roadmap.LoadRoadmap(".")`
   - Execute planning logic (research → synthesis → plan writing)
   - No roadmap save needed (plan command doesn't modify roadmap)

2. **Use the standard slug resolution pattern**
   - Accept optional positional argument `[slug]`
   - If no argument and interactive, use `ui.DefaultPicker.Show()` to select from roadmap items
   - Validate slug exists with `rm.FindBySlug(slug)`, return error if not found

3. **Stage templates for planning phases**
   - Create `prompt.NewPromptContext()` with slug and plan directory path
   - Stage templates from `prompts/plan-feature/` directory
   - Pass plan directory path as `.plan/{slug}/` following existing test pattern

4. **Define flags following existing categories**
   - Mode flags: `--verbose` (inherited from rootCmd)
   - Path flags: Consider `--output` to override default plan location
   - Skip flags like `--dry-run` or `--yes` (planning is read-only from roadmap perspective)

5. **Use AI invocation pattern for multi-phase planning**
   - Phase 1: Research — Invoke with research prompt, capture findings
   - Phase 2: Synthesis — Invoke with synthesis prompt + research output
   - Phase 3: Plan writing — Invoke with planning prompt + synthesis output
   - Each phase writes output to `.plan/{slug}/research/`, `.plan/{slug}/synthesis/`, `.plan/{slug}/plan/`

6. **Handle errors following established pattern**
   - Check `errors.Is(err, config.ErrConfigParse)` for config errors
   - Check `errors.Is(err, roadmap.ErrRoadmapParse)` for roadmap errors
   - Return descriptive errors with context for planning failures

### Should Have

7. **Create plan directory structure**
   - Base directory: `.plan/{slug}/` (following test pattern from `internal/prompt/template_test.go`)
   - Research output: `.plan/{slug}/research/`
   - Synthesis output: `.plan/{slug}/synthesis/`
   - Implementation plan: `.plan/{slug}/plan/implementation-plan.md`

8. **Detect and handle existing plans**
   - Check if `.plan/{slug}/plan/implementation-plan.md` exists
   - Prompt user to confirm overwrite or exit
   - Consider `--force` flag to skip confirmation

9. **Output summary after completion**
   - Print path to generated plan
   - Print brief summary of plan contents (e.g., number of tasks, estimated complexity)
   - Use `fmt.Fprintf(cmd.OutOrStdout(), ...)` for output

### Could Have

10. **Support resumption of interrupted planning**
    - Detect partial plans (e.g., research exists but no synthesis)
    - Offer to continue from last completed phase
    - Skip phases that already have output files

11. **Add `--format` flag support for JSON output**
    - Table mode: Human-readable summary
    - JSON mode: Structured plan metadata (slug, phases completed, file paths)

### Should Not Have

12. **Do NOT create skill files in plan command**
    - Planning is orchestrated by muster itself, not by Claude Agent SDK skills
    - Skills are for commands that launch interactive agent sessions (like `code`)

13. **Do NOT modify the roadmap**
    - Planning is a read-only operation from roadmap perspective
    - Status updates happen via separate commands (`muster code`, `muster out`)

14. **Do NOT implement `--yolo` or Docker orchestration**
    - Planning is safe to run locally (no destructive operations)
    - Docker orchestration is only for interactive coding sessions

---

## Open Questions

### 1. Should `muster plan` support partial planning?

**Why it matters:** Users may want to run just the research phase, or re-run synthesis after reviewing research findings.

**What I tried:** Examined `cmd/out.go` which has a `--no-fix` flag to skip AI fixes, suggesting muster commands support partial execution.

**Recommendation:** Consider adding phase-specific flags (e.g., `--research-only`, `--skip-research`) in a future iteration, but start with full-pipeline execution for simplicity.

### 2. How should conflicts with existing plans be handled?

**Why it matters:** Users may re-run `muster plan` after making manual edits to research or synthesis files.

**What I tried:** Looked for precedent in `cmd/add.go` and `cmd/sync.go`, which validate before writing (e.g., duplicate slug check), but don't handle file conflicts.

**Recommendation:** Detect existing implementation plan and prompt for confirmation before overwriting. Use `--force` flag to skip confirmation. Log warning if partial plans exist (research/synthesis but no final plan).

### 3. Should planning output be added to .gitignore?

**Why it matters:** `.plan/` directories may contain large AI-generated content that shouldn't be committed.

**What I tried:** Searched for `.gitignore` handling in codebase — found none. Git operations in `internal/git/` don't modify ignore rules.

**Recommendation:** Document in CLAUDE.md that users should add `.plan/` to their project's `.gitignore`, but don't modify .gitignore programmatically (following principle of least surprise).

### 4. What should happen if roadmap item status is already "completed"?

**Why it matters:** Planning for completed items doesn't make sense, but user may want to re-plan based on lessons learned.

**What I tried:** Examined `cmd/out.go:108-111` which explicitly errors on completed items.

**Recommendation:** Allow planning for completed items with a warning message. Users may want to create implementation plans for reference even after completing work manually.

---

## References

### Source Files Analyzed

- `/Users/andrew.benz/work/muster/muster-main/cmd/add.go` — Batch and interactive modes, AI invocation, TUI picker usage
- `/Users/andrew.benz/work/muster/muster-main/cmd/sync.go` — Multi-source operations, AI matching, confirmation prompts
- `/Users/andrew.benz/work/muster/muster-main/cmd/code.go` — Skill staging, exec command building, env overrides
- `/Users/andrew.benz/work/muster/muster-main/cmd/status.go` — Optional argument handling, output formatting
- `/Users/andrew.benz/work/muster/muster-main/cmd/out.go` — CI monitoring, polling loops, cleanup workflows
- `/Users/andrew.benz/work/muster/muster-main/cmd/root.go` — Global flag handling, output mode detection

### Internal Packages Examined

- `/Users/andrew.benz/work/muster/muster-main/internal/roadmap/roadmap.go` — Data model, enums, validation
- `/Users/andrew.benz/work/muster/muster-main/internal/roadmap/loader.go` — File loading with fallback chain
- `/Users/andrew.benz/work/muster/muster-main/internal/roadmap/validate.go` — FindBySlug, AddItem, Validate methods
- `/Users/andrew.benz/work/muster/muster-main/internal/ui/picker.go` — TUI picker interface, charmbracelet/huh integration
- `/Users/andrew.benz/work/muster/muster-main/internal/ui/roadmap_format.go` — Output formatting for table and JSON modes
- `/Users/andrew.benz/work/muster/muster-main/internal/prompt/context.go` — PromptContext struct, model tier resolution
- `/Users/andrew.benz/work/muster/muster-main/internal/prompt/template.go` — Template parsing and rendering
- `/Users/andrew.benz/work/muster/muster-main/internal/prompt/stage.go` — Skill staging for plugin directories
- `/Users/andrew.benz/work/muster/muster-main/internal/ai/invoke.go` — AI tool invocation, JSON extraction

### Test Files Reviewed

- `/Users/andrew.benz/work/muster/muster-main/cmd/status_test.go` — Command testing patterns, fixture setup, output capture
- Test patterns in `internal/prompt/template_test.go` — Plan directory path conventions (`.plan/{slug}/`)

### Documentation

- `/Users/andrew.benz/work/muster/muster-main/CLAUDE.md` — Project structure, key patterns, build/test instructions
