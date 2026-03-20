# Command Patterns

*Researched: 2026-03-20*
*Scope: Cobra command implementation patterns in muster*

---

## Key Findings

1. **All commands use `RunE` (not `Run`)** -- every command returns errors to Cobra rather than handling them internally. This is the universal pattern across all 6 commands.

2. **Config loading follows a strict template** -- commands that need config use an identical 3-step pattern: LoadUserConfig, LoadProjectConfig, then ResolveStep/ResolveCode. Error categorization uses `errors.Is(err, config.ErrConfigParse)` to surface "config file malformed" messages.

3. **Two command categories exist**: "pipeline step" commands (`code`, `add`, `sync`) that load config and resolve tool/provider/model, and "data-only" commands (`status`, `version`, `down`) that don't need AI config resolution.

4. **The `out` command will be a pipeline step command** that needs config resolution (to invoke AI for CI fix pushes), roadmap access (to update item status), and external tool invocation (git, gh CLI).

5. **Flag registration always happens in `init()` functions** at the bottom of each file. Commands are registered to `rootCmd` via `rootCmd.AddCommand()` in `init()`.

## Detailed Analysis

### Command Structure

Every command file follows this exact structure:

```
1. package cmd
2. import block
3. var xxxCmd = &cobra.Command{ ... }
4. Helper functions (if any)
5. func init() { rootCmd.AddCommand(xxxCmd); flag registration }
```

Key structural details:
- Commands are defined as package-level `var` declarations (`cmd/code.go:18`, `cmd/add.go:21`, `cmd/sync.go:18`, `cmd/status.go:12`, `cmd/down.go:15`, `cmd/version.go:17`)
- `Use` field includes argument placeholders when applicable: `"status [slug]"` (`cmd/status.go:13`), `"down [slug]"` (`cmd/down.go:16`)
- `Args` validator is set when needed: `cobra.MaximumNArgs(1)` in `cmd/status.go:14`, `cobra.NoArgs` in `cmd/version.go:20`
- `Short` descriptions are under 100 characters (tested in `cmd/code_test.go:183-191`)
- `Long` descriptions use backtick multi-line strings with indented bullet points

### Flag Handling

Two flag types are used:

**Local flags** (command-specific, registered in `init()` with `cmd.Flags()`):
- `cmd/code.go:358` -- `codeCmd.Flags().Bool("yolo", ...)`
- `cmd/add.go:263-266` -- title, priority, status, context flags
- `cmd/sync.go:404-408` -- source, target, yes, dry-run, delete flags
- `cmd/down.go:308-310` -- all, orphans, project flags

**Persistent flags** (inherited by subcommands, registered with `cmd.PersistentFlags()`):
- `cmd/root.go:49-51` -- `format` (StringP with shorthand "f") and `verbose` (BoolP with shorthand "v")
- `cmd/code.go:353-355` -- tool, no-plugin, keep-staged are persistent on codeCmd

Flag retrieval pattern in RunE:
```go
// Pattern 1: Direct retrieval with error check (used for critical flags)
yolo, err := cmd.Flags().GetBool("yolo")
if err != nil {
    return err
}

// Pattern 2: Ignoring error (used for persistent/well-known flags like verbose)
verbose, _ := cmd.Flags().GetBool("verbose")
```

The first pattern (`cmd/code.go:33-36`, `cmd/sync.go:35-53`, `cmd/down.go:35-47`) is used for local command flags. The second pattern (`cmd/add.go:39`, `cmd/sync.go:54`, `cmd/down.go:47`) is used for the `verbose` persistent flag.

### Error Handling

Errors follow a consistent categorization strategy:

**Config errors** -- three levels of wrapping:
```go
// Parse errors get special "malformed" message
if errors.Is(err, config.ErrConfigParse) {
    return fmt.Errorf("config file malformed: %w", err)
}
return fmt.Errorf("failed to load user config: %w", err)
```
This exact pattern appears in `cmd/code.go:44-49`, `cmd/code.go:52-57`, `cmd/add.go:43-48`, `cmd/add.go:51-56`, `cmd/sync.go:58-63`, `cmd/sync.go:65-71`.

**Roadmap errors** -- similar pattern:
```go
if errors.Is(err, roadmap.ErrRoadmapParse) {
    return fmt.Errorf("roadmap file is malformed: %w", err)
}
return fmt.Errorf("failed to load roadmap: %w", err)
```
Used in `cmd/add.go:74-79`, `cmd/status.go:21-25`.

**Tool execution errors** -- descriptive messages with install guidance:
```go
var execErr *exec.Error
if errors.As(err, &execErr) && execErr.Err == exec.ErrNotFound {
    return fmt.Errorf("tool %q not found: %w\n\nPlease install ...", executable, err, executable)
}
return fmt.Errorf("failed to execute %s: %w", executable, err)
```
Used in `cmd/code.go:155-161`.

**Docker/external tool errors** -- wrapped with context:
```go
return fmt.Errorf("failed to create Docker client: %w", err)
return fmt.Errorf("docker check failed: %w", err)
```
Used throughout `cmd/down.go`.

**Not-yet-implemented features** -- gated with clear message:
```go
return fmt.Errorf("--yolo (sandboxed container mode) is not yet implemented")
```
Used in `cmd/code.go:39`.

### Output Formatting

Two output channels are used:

**stderr** (`fmt.Fprintf(os.Stderr, ...)`) for:
- Verbose logging: `cmd/code.go:78`, `cmd/add.go:66-69`, `cmd/sync.go:81-87`
- Progress messages: `cmd/add.go:164` ("Describe the roadmap item..."), `cmd/add.go:202` ("Generating roadmap item with AI...")
- Status messages: `cmd/down.go:109` ("No containers to stop."), `cmd/down.go:114-118` (container listing)
- Warnings: `cmd/down.go:145` ("Warning: docker compose down failed...")

**stdout** (`fmt.Printf(...)` or `cmd.OutOrStdout()`) for:
- Actual command output: `cmd/status.go:40` (roadmap detail), `cmd/status.go:52` (roadmap table)
- Sync summary: `cmd/sync.go:126-132`, `cmd/sync.go:141-146`
- Confirmation: `cmd/add.go:148-151`

The `cmd.OutOrStdout()` pattern is used in `cmd/status.go` and `cmd/sync.go` to enable output capture in tests. Commands that just print confirmation messages use direct `fmt.Printf()` (`cmd/add.go:148-151`, `cmd/add.go:254`).

The `ui` package provides:
- `ui.FormatRoadmapTable()` and `ui.FormatRoadmapDetail()` for roadmap display
- `ui.FormatVersion()` for version display
- `ui.SetOutputMode()` / `ui.GetOutputMode()` for JSON/table mode switching
- `ui.IsInteractive()` for TTY detection
- `ui.DefaultPicker.Show()` for interactive selection

### Config Access Pattern

Pipeline step commands follow this exact sequence:

```go
// 1. Load user config
userCfg, err := config.LoadUserConfig("")
if err != nil {
    if errors.Is(err, config.ErrConfigParse) {
        return fmt.Errorf("config file malformed: %w", err)
    }
    return fmt.Errorf("failed to load user config: %w", err)
}

// 2. Load project config
projectCfg, err := config.LoadProjectConfig(".")
if err != nil {
    if errors.Is(err, config.ErrConfigParse) {
        return fmt.Errorf("config file malformed: %w", err)
    }
    return fmt.Errorf("failed to load project config: %w", err)
}

// 3. Resolve for specific step
resolved, err := config.ResolveStep("stepName", projectCfg, userCfg)
if err != nil {
    return fmt.Errorf("failed to resolve config: %w", err)
}

// 4. Verbose logging
if verbose {
    fmt.Fprintf(os.Stderr, "Using: tool=%s (%s) provider=%s (%s) model=%s (%s)\n",
        resolved.Tool, resolved.ToolSource,
        resolved.Provider, resolved.ProviderSource,
        resolved.Model, resolved.ModelSource)
}
```

This pattern is used verbatim in `cmd/code.go:42-82`, `cmd/add.go:42-69`, `cmd/sync.go:57-87`.

`config.ResolveStep()` accepts a step name string. Currently registered step tiers: "add" -> muster-fast, "sync" -> muster-fast (`internal/config/resolve.go:10-13`). The `out` command would need to be added to `stepDefaultTiers` if it should have a default tier.

### AI Tool Invocation

Commands that invoke AI (`add`, `sync`) follow this pattern:

```go
// 1. Create prompt context
ctx := prompt.NewPromptContext(resolved, projectCfg, userCfg, true, "", ".", ".", "")
ctx.Extra["Key"] = value  // Custom template data

// 2. Render template
promptContent, err := prompt.RenderTemplate("prompts/xxx/xxx-prompt.md.tmpl", ctx)

// 3. Invoke AI
result, err := ai.InvokeAI(ai.InvokeConfig{
    Tool:    config.ToolExecutable(resolved.Tool),
    Model:   resolved.Model,
    Prompt:  promptContent,
    Verbose: verbose,
    Env:     config.ToolEnvOverrides(resolved, projectCfg, userCfg),
})

// 4. Parse JSON response
jsonStr := ai.ExtractJSON(result.RawOutput)
if err := json.Unmarshal([]byte(jsonStr), &target); err != nil {
    return fmt.Errorf("failed to parse AI response as JSON: %w\nRaw response: %s", err, result.RawOutput)
}
```

Used in `cmd/add.go:178-219` and `cmd/sync.go:213-252`.

The `ai.InvokeAI` function is a package-level variable that can be replaced in tests with `testutil.MockInvokeAI()` or `testutil.MockInvokeAIWithQueue()`.

### Test Patterns

Tests follow consistent patterns across all command files:

**Existence and registration tests:**
```go
func TestXxxCommand_Exists(t *testing.T) {
    assert.NotNil(t, xxxCmd, "xxx command should exist")
    assert.Equal(t, "xxx", xxxCmd.Use)
}

func TestXxxCommand_IsAddedToRootCommand(t *testing.T) {
    found := false
    for _, cmd := range rootCmd.Commands() {
        if cmd.Name() == "xxx" { found = true; break }
    }
    assert.True(t, found)
}
```

**Flag existence tests:**
```go
func TestXxxCommand_HasExpectedFlags(t *testing.T) {
    flag := xxxCmd.Flags().Lookup("flagname")
    assert.NotNil(t, flag)
    assert.Equal(t, "string", flag.Value.Type())
    assert.Equal(t, "default", flag.DefValue)
}
```

**Integration tests with temp directory and chdir:**
```go
tmpDir := t.TempDir()
origDir, err := os.Getwd()
require.NoError(t, err)
defer func() { _ = os.Chdir(origDir) }()
err = os.Chdir(tmpDir)
require.NoError(t, err)

// Create test fixtures
musterDir := filepath.Join(tmpDir, ".muster")
err = os.MkdirAll(musterDir, 0755) //nolint:gosec // G301: Test directory permissions
require.NoError(t, err)
```

**Fresh command instances for isolation:**
```go
cmd := &cobra.Command{
    Use:  "xxx",
    RunE: xxxCmd.RunE,
}
cmd.Flags().String("flag", "", "usage")
cmd.Flags().Bool("verbose", false, "")
buf := new(bytes.Buffer)
cmd.SetOut(buf)
cmd.SetErr(buf)
```

**Mock AI tool for subprocess tests:**
```go
mockTool := testutil.NewMockAITool(t, `{"response": "value"}`)
configContent := "defaults:\n  tool: " + mockTool.Path() + "\n  provider: mock\n  model: mock-model\n"
```

**In-process AI mock for unit tests:**
```go
cleanup := testutil.MockInvokeAI("response", nil)
defer cleanup()
```

### Helper Function Patterns

Commands that grow complex extract helper functions within the same file:
- `cmd/add.go:92` -- `runBatchAdd()` and `cmd/add.go:157` -- `runInteractiveAdd()` split the two modes
- `cmd/sync.go:160` -- `performSync()` extracts the sync algorithm from RunE
- `cmd/sync.go:338` -- `updateItemFields()`, `cmd/sync.go:358` -- `promptUserForMatch()`, `cmd/sync.go:376` -- `saveRoadmapFile()`
- `cmd/down.go:172` -- `findOrphanContainers()`, `cmd/down.go:240` -- `loadInProgressSlugs()`

The pattern is to keep RunE as a thin orchestrator that delegates to well-named helper functions.

## Recommendations

### Command Registration

`cmd/out.go` should follow the standard pattern:

```go
var outCmd = &cobra.Command{
    Use:   "out [slug]",
    Short: "Complete post-PR lifecycle: CI, merge, cleanup",
    Long:  `...`,
    Args:  cobra.ExactArgs(1), // slug is required to identify which roadmap item
    RunE:  func(cmd *cobra.Command, args []string) error { ... },
}

func init() {
    rootCmd.AddCommand(outCmd)
    outCmd.Flags().Bool("dry-run", false, "Preview actions without executing")
    outCmd.Flags().Bool("yes", false, "Skip confirmations")
    outCmd.Flags().Duration("timeout", 30*time.Minute, "Maximum time to wait for CI/merge")
}
```

### Config Resolution

Since `out` may need to invoke AI for CI fix generation, it should:
1. Register as a pipeline step with `config.ResolveStep("out", projectCfg, userCfg)`
2. Consider adding `"out": "muster-standard"` to `stepDefaultTiers` in `internal/config/resolve.go` (standard tier since CI fixing is more complex than add/sync)
3. Follow the exact config loading pattern from `cmd/add.go:42-69`

### Error Handling

Follow existing patterns:
- Config errors: use `errors.Is(err, config.ErrConfigParse)` categorization
- Roadmap errors: use `errors.Is(err, roadmap.ErrRoadmapParse)` categorization
- External tool errors (git, gh): wrap with descriptive context like `fmt.Errorf("failed to check CI status: %w", err)`
- Docker errors: follow `cmd/down.go` patterns with context.WithTimeout
- Provide actionable errors: "gh CLI not found, install from https://cli.github.com"

### Output Formatting

- Use `os.Stderr` for all progress/status messages (CI check status, merge waiting, cleanup progress)
- Use `cmd.OutOrStdout()` for final summary output (what was done, final state)
- Support `--format json` output via the root command's PersistentPreRunE (already wired up in `cmd/root.go:17-45`)

### RunE Structure

Keep RunE thin, extract phases into helper functions:

```go
RunE: func(cmd *cobra.Command, args []string) error {
    slug := args[0]
    // ... config loading ...
    // ... flag retrieval ...

    if err := monitorCI(ctx, slug, ...); err != nil {
        return err
    }
    if err := waitForMerge(ctx, slug, ...); err != nil {
        return err
    }
    if err := cleanupWorktree(ctx, slug, ...); err != nil {
        return err
    }
    return nil
}
```

### Testing

Follow the established test structure:
1. `TestOutCommand_Exists` -- verify command registration
2. `TestOutCommand_HasExpectedFlags` -- verify all flags
3. `TestOutCommand_RequiresSlugArg` -- verify Args validation
4. Integration tests with temp dirs and mock tools
5. Use `testutil.MockInvokeAI()` for AI invocation tests
6. Use fresh `&cobra.Command{}` instances for isolation

### Nolint Annotations

Follow existing convention for:
- Temp dir permissions: `//nolint:gosec // G301: ...`
- File permissions: `//nolint:gosec // G306: ...`
- exec.Command: `//nolint:gosec // G204: ...`
- Test-specific: `//nolint:gosec // G301: Test directory permissions`

## Open Questions

1. **Should `out` require the `--yolo` flag or work without Docker?**
   - The `code` command gates Docker mode behind `--yolo` (`cmd/code.go:37-39`), but `out` is about post-PR lifecycle, not interactive development. It likely works on the host machine directly with git/gh CLI.
   - Matters because it determines whether Docker orchestration code is needed.

2. **Should `out` update roadmap status (in_progress -> completed)?**
   - The `status` command reads roadmap data. The `add` and `sync` commands write to it. If `out` marks items as completed, it needs roadmap write access.
   - `roadmap.LoadRoadmap(".")` and `roadmap.SaveRoadmap(".", rm)` are the established patterns for this.

3. **What model tier should `out` default to?**
   - `add` and `sync` default to `muster-fast` (`internal/config/resolve.go:11-12`).
   - If `out` needs AI for CI fix generation, `muster-standard` or `muster-deep` may be more appropriate since fixing CI failures is a complex task.
   - If `out` only needs AI for simple analysis/summarization, `muster-fast` may suffice.

4. **How should `out` discover the PR URL?**
   - Roadmap items have optional `pr_url` and `branch` fields.
   - The `out` command could either: (a) read PR URL from the roadmap item, (b) detect it from the current branch via `gh pr view`, or (c) accept it as a flag.

## References

### Source Files (all paths relative to repo root)
- `cmd/root.go` -- Root command, persistent flags (format, verbose), PersistentPreRunE for output mode
- `cmd/code.go` -- Code command, config loading reference pattern, tool execution, Docker flow
- `cmd/add.go` -- Add command, batch vs interactive modes, AI invocation, roadmap modification
- `cmd/sync.go` -- Sync command, AI fuzzy matching, dry-run, flag-heavy command
- `cmd/status.go` -- Status command, minimal command, roadmap read-only, ui formatting
- `cmd/down.go` -- Down command, Docker cleanup, context.WithTimeout, orphan detection
- `cmd/version.go` -- Version command, simplest command pattern
- `internal/config/resolve.go` -- ResolveStep, ResolveCode, stepDefaultTiers, model tier resolution
- `internal/config/config.go` -- ToolExecutable, ToolEnvOverrides, ErrConfigParse
- `internal/ai/invoke.go` -- InvokeAI (mockable var), InvokeConfig, InvokeResult, ExtractJSON
- `internal/prompt/context.go` -- NewPromptContext (line 65)
- `internal/ui/output.go` -- OutputMode, SetOutputMode, IsInteractive, FormatVersion
- `internal/ui/roadmap_format.go` -- FormatRoadmapTable, FormatRoadmapDetail
- `internal/testutil/helpers.go` -- AssertGoldenFile, RequireCommand
- `internal/testutil/mockai.go` -- MockInvokeAI, MockInvokeAIWithQueue, MockAITool, NewMockAITool
- `internal/testutil/fixtures.go` -- ValidRoadmapItemJSON, InvalidJSON test constants

### Test Files
- `cmd/code_test.go` -- Flag tests, help test, config failure tests, integration tests
- `cmd/add_test.go` -- Batch mode tests, stdin tests, interactive mode gate tests
- `cmd/sync_test.go` -- Dry-run, exact match, delete, AI fuzzy matching tests
- `cmd/status_test.go` -- Table/JSON output, empty roadmap, invalid slug tests
- `cmd/down_test.go` -- Flag tests, Docker availability, loadInProgressSlugs tests
- `cmd/root_test.go` -- Persistent flag tests
