# CLI Command Structure

*Researched: 2026-03-18*
*Scope: Audit existing cmd/ files to understand command setup, flag handling, output formatting patterns, error handling, and subcommand organization for implementing cmd/status.go, cmd/add.go, and cmd/sync.go*

---

## Key Findings

The muster CLI uses **Cobra** (github.com/spf13/cobra v1.8.0) with a consistent command structure pattern. Commands are simple, self-contained files in the `cmd/` package with dedicated test coverage. The codebase has 3 commands: root (cmd/root.go), version (cmd/version.go), and code (cmd/code.go).

**Critical patterns for new roadmap commands:**

1. **Output formatting is centralized** - root command's PersistentPreRunE handles `--format` flag validation and sets ui.OutputMode (TableMode or JSONMode). Commands use internal/ui package formatters that switch on the current mode.

2. **Flag organization** - PersistentFlags() for flags inherited by subcommands (e.g., --tool in code.go), regular Flags() for command-specific flags (e.g., --yolo in code.go). Root has --format and --verbose as persistent flags available to all commands.

3. **Error handling follows a hierarchy** - Sentinel errors (e.g., config.ErrConfigParse, prompt.ErrTemplateRender) allow categorized error messages with context-specific user guidance using errors.Is() and errors.As().

4. **Command registration** - Each command file has an init() function that calls rootCmd.AddCommand(). Commands are registered at package init time, not in main.go.

5. **Testing pattern** - Each command has a corresponding _test.go file with unit tests for flags, command existence, output modes, and error conditions using testify/assert and testify/require.

## Detailed Analysis

### Command Registration and Setup

**Pattern: Command variables + init() registration**

Each command is defined as a package-level variable then registered in init():

```go
// cmd/version.go:17-38
var versionCmd = &cobra.Command{
    Use:   "version",
    Short: "Print version information",
    Args:  cobra.NoArgs,
    RunE: func(cmd *cobra.Command, args []string) error {
        // implementation
    },
}

func init() {
    rootCmd.AddCommand(versionCmd)
}
```

**Root command setup** (cmd/root.go:10-46):
- Package-level var `rootCmd` with Use="muster"
- PersistentPreRunE validates --format flag and sets ui.OutputMode
- Supports auto-detection: if IsInteractive() then TableMode, else JSONMode
- init() defines persistent flags: --format/-f (json|table), --verbose/-v

**Command Arguments validation**:
- version.go:20 uses `Args: cobra.NoArgs` to reject arguments
- code.go has no Args field, accepts arbitrary args (passed to underlying tool)

### Flag Parsing Patterns

**Flag types and definitions** (cmd/code.go:146-157):

1. **PersistentFlags** - inherited by subcommands if any existed
   ```go
   codeCmd.PersistentFlags().String("tool", "", "Override the tool to use (e.g., claude-code, opencode)")
   codeCmd.PersistentFlags().Bool("no-plugin", false, "Run the tool without the staged skills plugin")
   codeCmd.PersistentFlags().Bool("keep-staged", false, "Keep staged skills directory after command exits")
   ```

2. **Local Flags** - only for this command
   ```go
   codeCmd.Flags().Bool("yolo", false, "Run in sandboxed container mode (not yet implemented)")
   ```

**Flag access pattern** (cmd/code.go:29-35, 62-69, 78-81):
```go
yolo, err := cmd.Flags().GetBool("yolo")
if err != nil {
    return err
}
if yolo {
    return fmt.Errorf("...")
}
```

Flags use GetString(), GetBool() methods with error checking on every access.

**Flag shorthand**: Only root's --format flag uses shorthand (-f). Other flags are long-form only.

### Output Formatting (table, detail, JSON)

**Two-tier system**:

1. **Mode setting** (cmd/root.go:17-45) - PersistentPreRunE validates --format and calls ui.SetOutputMode()
2. **Per-type formatters** (internal/ui/output.go:56-82) - Commands call ui.FormatVersion(), ui.FormatStatus(), etc.

**internal/ui/output.go structure**:
- OutputMode type with TableMode and JSONMode constants (lines 12-20)
- Thread-safe global mode via sync.Mutex (lines 31-34, 42-53)
- IsInteractive() checks term.IsTerminal(os.Stdout.Fd()) (lines 36-39)
- Format functions switch on GetOutputMode() (lines 56-82)

**FormatVersion example** (internal/ui/output.go:56-82):
```go
func FormatVersion(info VersionInfo) (string, error) {
    modeMux.Lock()
    mode := currentMode
    modeMux.Unlock()

    switch mode {
    case JSONMode:
        data, err := json.MarshalIndent(info, "", "  ")
        if err != nil {
            return "", err
        }
        return string(data), nil
    case TableMode:
        fallthrough
    default:
        return fmt.Sprintf(`Version:    %s
Commit:     %s
Date:       %s
Go:         %s
Platform:   %s`,
            info.Version,
            info.Commit,
            info.Date,
            info.GoVersion,
            info.Platform), nil
    }
}
```

**Output pattern** (cmd/version.go:28-34):
```go
output, err := ui.FormatVersion(info)
if err != nil {
    return fmt.Errorf("failed to format version: %w", err)
}
fmt.Fprintln(cmd.OutOrStdout(), output)
```

Uses `cmd.OutOrStdout()` for testability (can redirect to buffer in tests).

**Data structures for formatting**:
- Define struct with json tags (internal/ui/output.go:22-29)
- Example: `type VersionInfo struct { Version string `json:"version"` ... }`
- Table mode formats manually with fmt.Sprintf
- JSON mode uses json.MarshalIndent with 2-space indent

### Error Handling and Exit Codes

**Three-tier error handling pattern**:

1. **Sentinel errors** for categorization (internal/config/config.go:26-29, internal/prompt/stage.go:14-16):
   ```go
   var (
       ErrConfigParse = errors.New("config parse error")
   )
   ```

2. **Error wrapping with context** (cmd/code.go:42-44, 51-53, 59):
   ```go
   if errors.Is(err, config.ErrConfigParse) {
       return fmt.Errorf("config file malformed: %w", err)
   }
   return fmt.Errorf("failed to load user config: %w", err)
   ```

3. **User-friendly messages with guidance** (cmd/code.go:137):
   ```go
   if errors.As(err, &execErr) && execErr.Err == exec.ErrNotFound {
       return fmt.Errorf("tool %q not found: %w\n\nPlease install %s and ensure it is in your PATH.\nFor Claude Code: https://docs.anthropic.com/claude-code\nFor OpenCode: https://github.com/opencodeinterpreter/opencode", resolved.Tool, err, resolved.Tool)
   }
   ```

**Exit code handling** (main.go:10-12):
```go
if err := cmd.Execute(); err != nil {
    os.Exit(1)
}
```

Simple pattern: any error from Execute() exits with code 1. Cobra handles showing error messages.

**Verbose flag pattern** (cmd/code.go:72-75):
```go
verbose, _ := cmd.Flags().GetBool("verbose")
if verbose {
    fmt.Fprintf(os.Stderr, "Using: tool=%s provider=%s model=%s\n", resolved.Tool, resolved.Provider, resolved.Model)
}
```

Verbose output goes to stderr. Root defines --verbose/-v but it's currently only used for debugging info, not detailed logging.

### Subcommand Organization

**Flat structure**: All commands are direct children of rootCmd. No nested subcommands exist yet.

**Command files**:
- cmd/root.go - root command + Execute() function
- cmd/version.go - version command
- cmd/code.go - code command
- Each has corresponding _test.go file

**Registration pattern**: Commands register themselves via init() (cmd/version.go:40-42, cmd/code.go:147-157)

**No command groups yet**: All commands appear at same level. Future roadmap commands (status, add, sync) could either:
1. Continue flat pattern: `muster status`, `muster add`, `muster sync`
2. Use subcommand grouping: `muster roadmap status`, `muster roadmap add`, etc.

Given the roadmap.json context shows "Phase 3: Roadmap management commands", flat pattern seems more appropriate for CLI brevity.

### Help Text Conventions

**Three-level help text** (cmd/code.go:14-26, cmd/version.go:17-20):

1. **Use**: Command name and args (e.g., "version", "code")
2. **Short**: Single-line description for command list (~50-80 chars)
3. **Long**: Multi-paragraph description with details, only shown in `--help`

**Long description format** (cmd/code.go:17-25):
```
Launch Claude/OpenCode CLI in the current directory with workflow skills staged.

This command:
  1. Loads project and user configuration
  2. Resolves the tool, provider, and model triple
  3. Stages workflow skill templates to a temporary directory
  4. Executes the resolved tool with the staged skills as a plugin

The staged skills enable AI assistants to orchestrate roadmap-driven workflows
using Claude Agent SDK skills.
```

Uses numbered steps for clarity, wraps at ~80 chars.

**Flag usage text**: All flags have descriptive usage strings (cmd/code.go:151-156):
```go
codeCmd.PersistentFlags().String("tool", "", "Override the tool to use (e.g., claude-code, opencode)")
```

### Testing Conventions

**Test file organization** (cmd/code_test.go, cmd/version_test.go):

1. **Existence tests**: Verify command exists, has correct Use field
2. **Flag tests**: Verify flags exist, have correct types and defaults
3. **Behavior tests**: Test command execution with various flags
4. **Error tests**: Test error conditions and messages
5. **Output tests**: Test both TableMode and JSONMode output

**Test patterns** (cmd/code_test.go:90-112):
```go
func TestCodeCommand_WithYoloFlag_ReturnsExpectedError(t *testing.T) {
    cmd := &cobra.Command{
        Use: "code",
        RunE: codeCmd.RunE,
    }
    cmd.Flags().Bool("yolo", false, "Run in sandboxed container mode")

    err := cmd.Flags().Set("yolo", "true")
    require.NoError(t, err, "setting yolo flag should not error")

    err = cmd.RunE(cmd, []string{})
    require.Error(t, err, "command should return error with --yolo flag")

    assert.Contains(t, errMsg, "yolo", "error should mention yolo flag")
    assert.Contains(t, errMsg, "not yet implemented", "error should mention not implemented")
}
```

**Output testing pattern** (cmd/version_test.go:79-108):
```go
func TestVersionCommand_WithFormatJSON(t *testing.T) {
    originalMode := ui.GetOutputMode()
    defer ui.SetOutputMode(originalMode)

    ui.SetOutputMode(ui.JSONMode)

    buf := new(bytes.Buffer)
    versionCmd.SetOut(buf)

    err := versionCmd.RunE(versionCmd, []string{})
    assert.NoError(t, err)

    var info ui.VersionInfo
    err = json.Unmarshal([]byte(output), &info)
    assert.NoError(t, err, "output should be valid JSON")
}
```

Tests use testify/assert for non-fatal assertions, testify/require for fatal ones.

## Recommendations

### For cmd/status.go

**Command structure**:
```go
var statusCmd = &cobra.Command{
    Use:   "status [slug]",
    Short: "Display roadmap status",
    Long: `Display roadmap status in table, detail, or JSON format.

With no arguments, shows all roadmap items in table format.
With a slug argument, shows detailed information for that item.`,
    Args: cobra.MaximumNArgs(1),
    RunE: func(cmd *cobra.Command, args []string) error {
        // Load roadmap from internal/roadmap package
        // If args provided, show detail view for that slug
        // Else show table view of all items
        // Use ui.FormatRoadmapTable() or ui.FormatRoadmapDetail()
    },
}

func init() {
    rootCmd.AddCommand(statusCmd)
    // No additional flags needed - --format from root is sufficient
}
```

**Output modes**:
- Table mode: Multi-column table with slug, title, priority, status (use text/tabwriter or simple formatting)
- Detail mode: When slug provided, show full item including context field
- JSON mode: Always output JSON array for table, single object for detail

**Reference**: cmd/version.go for simple command with formatters

### For cmd/add.go

**Command structure**:
```go
var addCmd = &cobra.Command{
    Use:   "add",
    Short: "Add a new roadmap item with AI assistance",
    Long: `Add a new roadmap item using AI to generate slug, title, and context.

Interactive mode prompts for user input and uses AI to structure the item.`,
    Args: cobra.NoArgs,
    RunE: func(cmd *cobra.Command, args []string) error {
        // Check if stdin is interactive (ui.IsInteractive())
        // If not interactive, return error requiring interactive mode
        // Load config to get fast model
        // Prompt user for item details
        // Stage AI templates (similar to code.go:88-106)
        // Generate structured roadmap item
        // Save to .roadmap.json via internal/roadmap
        // Output confirmation (respects --format flag)
    },
}

func init() {
    rootCmd.AddCommand(addCmd)
    addCmd.Flags().String("title", "", "Item title (skip interactive prompt)")
    addCmd.Flags().String("priority", "", "Priority: high|medium|low")
    addCmd.Flags().String("context", "", "Context description (skip AI generation)")
}
```

**Flags for batch mode**: Allow non-interactive add with --title, --priority, --context
**AI integration**: Follow code.go pattern for config resolution (lines 38-60), template staging (lines 88-106)
**Error handling**: Check ui.IsInteractive() and return clear error if not interactive and required flags missing

**Reference**: cmd/code.go for config loading, flag overrides, AI integration

### For cmd/sync.go

**Command structure**:
```go
var syncCmd = &cobra.Command{
    Use:   "sync",
    Short: "Sync roadmap items with AI-assisted fuzzy matching",
    Long: `Sync roadmap items using AI to fuzzy-match between source and target.

Useful for reconciling changes between .roadmap.json and .muster/roadmap.json
or updating items based on external sources.`,
    Args: cobra.NoArgs,
    RunE: func(cmd *cobra.Command, args []string) error {
        // Load both roadmap files
        // Use AI with fuzzy matching to reconcile
        // Stage AI templates for matching logic
        // Present changes for confirmation (unless --yes flag)
        // Apply updates
        // Output summary (respects --format flag)
    },
}

func init() {
    rootCmd.AddCommand(syncCmd)
    syncCmd.Flags().String("source", ".roadmap.json", "Source roadmap file")
    syncCmd.Flags().String("target", ".muster/roadmap.json", "Target roadmap file")
    syncCmd.Flags().Bool("yes", false, "Skip confirmation prompts")
    syncCmd.Flags().Bool("dry-run", false, "Show changes without applying")
}
```

**Flags**:
- --source, --target for file paths (default to legacy and new locations)
- --yes to skip confirmation (for automation)
- --dry-run to preview changes

**Safety**: Require explicit confirmation unless --yes flag, similar to destructive git operations

**Reference**: cmd/code.go for multi-step operations with flags and confirmation patterns

### General patterns for all three commands

1. **Use internal/roadmap package** - Create this package to handle .roadmap.json and .muster/roadmap.json loading/saving with backward compatibility

2. **Follow config error pattern** (cmd/code.go:41-44):
   ```go
   if errors.Is(err, roadmap.ErrRoadmapParse) {
       return fmt.Errorf("roadmap file malformed: %w", err)
   }
   ```

3. **Output via ui package formatters**:
   - Create ui.FormatRoadmapTable() for status table view
   - Create ui.FormatRoadmapDetail() for status detail view
   - Create ui.FormatRoadmapItem() for single item output (add/sync confirmation)

4. **Test coverage**:
   - Create cmd/status_test.go, cmd/add_test.go, cmd/sync_test.go
   - Test flag existence, types, defaults (pattern from code_test.go:21-53)
   - Test both output modes (pattern from version_test.go:25-60)
   - Test error conditions

5. **Data structures** (for internal/roadmap package):
   ```go
   type RoadmapItem struct {
       Slug     string `json:"slug"`
       Title    string `json:"title"`
       Priority string `json:"priority"` // high|medium|low|lower
       Status   string `json:"status"`   // planned|in_progress|completed
       Context  string `json:"context"`
       PRUrl    string `json:"pr_url,omitempty"`
       Branch   string `json:"branch,omitempty"`
   }
   ```

6. **File locations**:
   - Legacy: .roadmap.json (root of repo)
   - New: .muster/roadmap.json
   - Support both formats: array or {"items": []} wrapper (see .roadmap.json:1-54)

## Open Questions

1. **Table formatting library**: Should we add a dependency like olekukonko/tablewriter or github.com/charmbracelet/lipgloss for table output, or stick with simple text/tabwriter from stdlib?
   - **What matters**: Table readability and cross-platform compatibility
   - **What tried**: Examined go.mod - only spf13/cobra for CLI, no table libraries
   - **Recommendation**: Start with text/tabwriter (stdlib), add library if needed

2. **Interactive picker implementation**: The roadmap mentions internal/ui/picker.go for item selection. Should this use a TUI library like charmbracelet/bubbletea or keep it simple with text prompts?
   - **What matters**: User experience vs dependency bloat
   - **What tried**: Grepped for existing interactive patterns - only found basic terminal checks
   - **Recommendation**: Start simple (fmt.Scan/bufio), defer TUI library until Phase 6 (full pipeline)

3. **Roadmap file format precedence**: When both .roadmap.json and .muster/roadmap.json exist, which takes precedence?
   - **What matters**: Clear migration path and user expectations
   - **Context**: Phase 3 mentions "backward compat for .roadmap.json"
   - **Recommendation**: .muster/roadmap.json takes precedence if exists, .roadmap.json is legacy fallback. status/add/sync should write to .muster/roadmap.json only.

4. **AI model selection for add/sync**: Should these commands use the "fast" tier or allow override via flag?
   - **What matters**: Speed vs quality for roadmap operations
   - **Reference**: cmd/code.go shows flag override pattern (lines 62-69)
   - **Recommendation**: Use "fast" tier by default (mentioned in Phase 3 context), add --model flag for override following code.go pattern

5. **JSON output structure for status table view**: Should JSON mode output array of items directly, or wrap in {"items": []} for consistency with file format?
   - **What matters**: Simplicity vs consistency
   - **Recommendation**: Output array directly for CLI output (simpler for jq piping), use wrapper only in file format

## References

### Command patterns
- cmd/root.go:10-57 - Root command with PersistentPreRunE, format flag validation, mode setting
- cmd/version.go:17-42 - Simple command with Args validation, formatter usage
- cmd/code.go:14-157 - Complex command with config loading, flag overrides, subprocess execution

### Flag patterns
- cmd/code.go:146-157 - PersistentFlags() vs Flags() distinction, string and bool flags
- cmd/root.go:48-52 - Root persistent flags (--format, --verbose)
- cmd/code.go:62-69, 78-81 - Flag access pattern with error checking

### Output formatting
- internal/ui/output.go:12-82 - OutputMode system, formatters, thread-safe mode management
- cmd/version.go:21-34 - Command output pattern with ui.FormatVersion() and cmd.OutOrStdout()
- cmd/root.go:17-45 - PersistentPreRunE handling --format flag and auto-detection

### Error handling
- internal/config/config.go:26-29 - Sentinel error definition pattern
- cmd/code.go:41-44, 51-53, 103-105, 136-139 - Error categorization with errors.Is/As, user-friendly messages
- main.go:10-12 - Exit code handling

### Testing
- cmd/code_test.go:15-53 - Flag testing pattern
- cmd/code_test.go:90-138 - Error message testing
- cmd/version_test.go:25-108 - Output mode testing with cleanup
- cmd/code_test.go:288-322 - Integration test structure

### Data structures
- .roadmap.json:1-54 - Roadmap file format ({"items": [...]})
- internal/ui/output.go:22-29 - VersionInfo struct with json tags
- internal/config/config.go:38-130 - Config struct patterns with pointer fields for optional values
