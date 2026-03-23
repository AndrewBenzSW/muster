# Architecture: `muster plan [slug]`

*Author: Software Architect*
*Date: 2026-03-23*
*Traces to: `.plan/standalone-planning/synthesis/synthesis.md`*

---

## 1. Overview

The `muster plan` command resolves a roadmap item (by argument or interactive picker), stages the `plan-feature` skill templates, invokes Claude Code as a long-running agent with the staged plugin directory, and produces an implementation plan at `.muster/work/{slug}/plan/implementation-plan.md`.

This is architecturally distinct from the single-shot `add` and `sync` commands that use `ai.InvokeAI()`. Like `muster code`, it uses `prompt.StageSkills()` and `exec.Command` to launch Claude Code with `--plugin-dir`, but unlike `code` it is non-interactive (`--print` mode) and targets a specific slug with a pre-created output directory.

---

## 2. Component Structure

### 2.1 New File: `cmd/plan.go`

Single new file containing the Cobra command definition and all plan-specific logic.

```
cmd/plan.go
  planCmd          *cobra.Command   — Use: "plan [slug]", Args: cobra.MaximumNArgs(1)
  runPlan()        func             — main execution logic (extracted from RunE for testability)
  buildPickerOpts  func             — filters/sorts roadmap items for picker display
  init()           func             — registers command and flags
```

No new packages are introduced. The command composes existing packages: `config`, `roadmap`, `prompt`, `ui`.

### 2.2 Modified File: `internal/config/resolve.go`

Add `"plan"` to `stepDefaultTiers` map:

```go
var stepDefaultTiers = map[string]string{
    "add":  "muster-fast",
    "sync": "muster-fast",
    "out":  "muster-standard",
    "plan": "muster-deep",     // NEW — Req 12
}
```

This is the only change outside `cmd/plan.go`. It ensures `config.ResolveStep("plan", ...)` defaults to the `muster-deep` tier when no explicit model is configured, which resolves to the concrete deep model through the existing tier resolution chain.

### 2.3 Existing Packages (no modifications)

| Package | Usage in `plan` | Key Functions |
|---|---|---|
| `config` | Load user/project config, resolve step, get env overrides, get tool executable | `LoadUserConfig`, `LoadProjectConfig`, `ResolveStep`, `ToolExecutable`, `ToolEnvOverrides` |
| `roadmap` | Load roadmap, find item by slug | `LoadRoadmap`, `FindBySlug`, `ValidPriorities` |
| `prompt` | Create context, stage skills | `NewPromptContext`, `StageSkills` |
| `ui` | TTY detection, interactive picker, output mode | `IsInteractive`, `DefaultPicker.Show`, `GetOutputMode` |

---

## 3. Execution Flow

The command follows the established 8-step pattern (Req 1) with plan-specific behavior at each step.

```
Step 1: Parse flags
  --force (bool)    skip overwrite confirmation
  --verbose (bool)  from rootCmd persistent flag
  --format (string) from rootCmd persistent flag (json|table)

Step 2: Load config
  userCfg    := config.LoadUserConfig("")
  projectCfg := config.LoadProjectConfig(".")

Step 3: Resolve step config
  resolved := config.ResolveStep("plan", projectCfg, userCfg)
  // defaults to muster-deep tier

Step 4: Verbose logging (if --verbose)
  Print tool/provider/model triple to stderr

Step 5: Load roadmap
  rm := roadmap.LoadRoadmap(".")

Step 6: Resolve slug (dual-mode — Req 2)
  if arg provided:  validate via rm.FindBySlug(slug)
  if no arg:        show picker (filtered, sorted)

Step 7: Execute planning (Req 3, 4, 5, 7)
  a. Resolve absolute paths via os.Getwd()
  b. Create .muster/work/{slug}/plan/{research,synthesis}/ directories
  c. Check for existing plan, warn/confirm (Req 8)
  d. Build PromptContext with slug and planDir
  e. Stage skills via prompt.StageSkills(ctx)
  f. Build exec.Command: claude --print --plugin-dir <tmpDir> --model <model>
  g. Connect stdout/stderr, apply env overrides
  h. Run command, check exit code
  i. Verify implementation-plan.md exists at expected path

Step 8: Output summary (Req 11)
  Print plan path to stdout
  If --format json: emit {"slug": "...", "plan_path": "..."}
```

---

## 4. Detailed Design

### 4.1 Slug Resolution (Req 2, 9, 10, 13)

```go
func resolveSlug(cmd *cobra.Command, args []string, rm *roadmap.Roadmap) (string, *roadmap.RoadmapItem, error)
```

**Argument mode** (len(args) == 1):
- Call `rm.FindBySlug(args[0])`.
- If nil, return `fmt.Errorf("roadmap item %q not found", slug)`.
- If item.Status == `StatusCompleted`, print warning to stderr in interactive mode (Req 9): `"Warning: planning an already-completed item"`.
- Return slug and item.

**Picker mode** (len(args) == 0):
- Guard: `if !ui.IsInteractive()` return error requiring slug argument.
- Guard: `if len(rm.Items) == 0` return `"No roadmap items found. Run 'muster add' to create one."` (Req 13).
- Filter: exclude items where `Status == StatusCompleted` (Req 9).
- If all filtered out: return `"No eligible items to plan. All items are completed."`.
- Sort: by priority tier order (high > medium > low > lower), then alphabetically by slug within each tier (Req 10). Use `roadmap.ValidPriorities()` for canonical ordering.
- Build `[]ui.PickerOption` with format: `"{slug} - {title} [{priority}, {status}]"`.
- Call `ui.DefaultPicker.Show("Select item to plan:", options, ui.DefaultPickerConfig())`.
- Look up selected slug via `rm.FindBySlug(selected)`.

### 4.2 Directory Creation (Req 4)

```go
func ensurePlanDir(projectRoot, slug string) (string, error)
```

- Compute `planDir = filepath.Join(projectRoot, ".muster", "work", slug, "plan")`.
- Create `planDir`, `planDir/research`, `planDir/synthesis` using `os.MkdirAll(..., 0755)`.
- Return absolute `planDir` path.

The function uses `os.Getwd()` to resolve `projectRoot` to an absolute path (Req Q4), ensuring templates receive absolute paths in `PromptContext.PlanDir`.

### 4.3 Existing Plan Detection (Req 8)

Before invoking Claude:

```go
planFile := filepath.Join(planDir, "implementation-plan.md")
if _, err := os.Stat(planFile); err == nil {
    // Plan exists
    if !force {
        // In interactive mode: prompt "Plan already exists. Overwrite? (y/N)"
        // Read single line from stdin, check for "y" or "Y"
        // If not confirmed, return nil (exit gracefully)
    }
    // Log: "Overwriting existing plan"
}
```

The `--force` flag bypasses the confirmation prompt. In non-interactive mode (piped stdin), the check still runs but defaults to "no" unless `--force` is set, preventing accidental overwrites in scripts.

### 4.4 PromptContext Construction (Req 5)

```go
cwd, err := os.Getwd()
// ...
ctx := prompt.NewPromptContext(
    resolved,
    projectCfg,
    userCfg,
    true,    // interactive: always true, plan skill uses teams
    slug,
    cwd,     // worktreePath: main repo (planning happens pre-worktree)
    cwd,     // mainRepoPath: same as worktreePath
    planDir,  // absolute path to .muster/work/{slug}/plan/
)
```

The `Extra` map is not populated (Req 5 specifies this). The skill templates access all needed data through the standard `PromptContext` fields: `{{.Slug}}`, `{{.PlanDir}}`, `{{.Models.Deep}}`, etc.

### 4.5 Skill Staging and Claude Invocation (Req 3, 7)

```go
tmpDir, cleanup, err := prompt.StageSkills(ctx)
if err != nil {
    // handle error (see 4.7)
}
defer cleanup()
```

This stages all skill directories (plan-feature, execute-plan, review-implementation) to `tmpDir/skills/roadmap-*/`. The existing `StageSkills` implementation already excludes add-item, sync-match, and out templates (Req 17). Only the plan-feature skill is relevant for this command, but staging all skills is harmless and consistent with `muster code`.

**Claude invocation:**

```go
cmdArgs := []string{"--print", "--plugin-dir", tmpDir, "--model", resolved.Model}
execCmd := exec.Command(config.ToolExecutable(resolved.Tool), cmdArgs...)
execCmd.Stdout = os.Stdout
execCmd.Stderr = os.Stderr

// Apply env overrides
envOverrides := config.ToolEnvOverrides(resolved, projectCfg, userCfg)
if len(envOverrides) > 0 {
    execCmd.Env = os.Environ()
    for k, v := range envOverrides {
        execCmd.Env = append(execCmd.Env, k+"="+v)
    }
}
```

Key difference from `muster code`: the plan command uses `--print` flag for non-interactive execution. Claude Code reads the skill, performs the research-synthesis-planning pipeline, writes files to `planDir`, and exits. Stdout/stderr are connected directly so the user sees progress output.

**Why `--print` and not interactive:** The plan command is a batch operation. The skill templates contain all the instructions Claude needs. There is no user interaction during planning -- the agent reads the codebase, synthesizes findings, and writes the plan. Using `--print` makes the command scriptable and allows `--format json` to work predictably.

### 4.6 Output Verification (Req 4, 7)

After Claude exits successfully:

```go
planFile := filepath.Join(planDir, "implementation-plan.md")
if _, err := os.Stat(planFile); os.IsNotExist(err) {
    return fmt.Errorf("planning completed but implementation-plan.md was not created at %s", planFile)
}
```

This is a post-condition check. If Claude ran without error but didn't produce the expected output file, the command reports this clearly rather than silently succeeding.

### 4.7 Error Handling (Req 6)

All errors follow the established wrapping pattern with contextual messages:

| Error source | Handling |
|---|---|
| `config.LoadUserConfig` / `LoadProjectConfig` | Check `errors.Is(err, config.ErrConfigParse)` for malformed config |
| `config.ResolveStep` | Wrap: `"failed to resolve config: %w"` |
| `roadmap.LoadRoadmap` | Check `errors.Is(err, roadmap.ErrRoadmapParse)` for malformed roadmap |
| `rm.FindBySlug` returns nil | `fmt.Errorf("roadmap item %q not found", slug)` |
| `ui.DefaultPicker.Show` | Wrap: `"picker cancelled or error: %w"` (covers ESC/Ctrl+C) |
| `prompt.StageSkills` | Check `errors.Is(err, prompt.ErrTemplateRender)` for template errors |
| `exec.Command.Run` | Check `exec.ErrNotFound` for missing tool, wrap exit errors |
| Post-invocation verification | `"planning completed but implementation-plan.md was not created"` |

### 4.8 Output Formatting (Req 11)

**Table mode** (default interactive):
```
Implementation plan created: .muster/work/{slug}/plan/implementation-plan.md
```

**JSON mode** (`--format json`):
```json
{"slug": "my-feature", "plan_path": ".muster/work/my-feature/plan/implementation-plan.md"}
```

The plan path in JSON output uses the relative path (from project root) for portability. The `ui.GetOutputMode()` function determines which format to use, consistent with how other commands handle this.

---

## 5. Data Flow Diagram

```
User invokes: muster plan [slug]

  cmd/plan.go
    |
    +-- config.LoadUserConfig("")
    +-- config.LoadProjectConfig(".")
    +-- config.ResolveStep("plan", projectCfg, userCfg)
    |     \-- stepDefaultTiers["plan"] = "muster-deep"
    |
    +-- roadmap.LoadRoadmap(".")
    +-- resolveSlug(args, rm)
    |     +-- [arg mode] rm.FindBySlug(slug)
    |     +-- [picker mode] ui.DefaultPicker.Show(filteredItems)
    |
    +-- ensurePlanDir(cwd, slug)
    |     \-- os.MkdirAll(".muster/work/{slug}/plan/{research,synthesis}")
    |
    +-- [check existing plan, prompt if needed]
    |
    +-- prompt.NewPromptContext(resolved, ..., slug, cwd, cwd, planDir)
    +-- prompt.StageSkills(ctx)
    |     \-- tmpDir/skills/roadmap-plan-feature/{SKILL.md, *.md}
    |
    +-- exec.Command("claude", "--print", "--plugin-dir", tmpDir, "--model", model)
    |     +-- stdout -> os.Stdout (user sees progress)
    |     +-- stderr -> os.Stderr
    |     +-- env: ToolEnvOverrides applied
    |     |
    |     \-- Claude Code reads skill, executes planning pipeline:
    |           research-runner -> researcher-prompt (codebase analysis)
    |           synthesis-runner -> consolidate findings
    |           planning-runner -> planner-prompt -> write implementation-plan.md
    |
    +-- Verify: .muster/work/{slug}/plan/implementation-plan.md exists
    +-- Output summary (table or JSON)
```

---

## 6. Flag and Argument Specification

```go
var planCmd = &cobra.Command{
    Use:   "plan [slug]",
    Short: "Create an implementation plan for a roadmap item",
    Long:  `...`,
    Args:  cobra.MaximumNArgs(1),
    RunE:  runPlan,
}

func init() {
    rootCmd.AddCommand(planCmd)
    planCmd.Flags().Bool("force", false, "Overwrite existing plan without confirmation")
}
```

The `--verbose` and `--format` flags are inherited from `rootCmd.PersistentFlags()`. No additional flags are needed for the initial implementation (Req 20 defers `--research-only`, `--skip-research`; Req 14 defers `--output`).

---

## 7. Testing Strategy

### 7.1 Unit Tests: `cmd/plan_test.go`

**Slug resolution tests:**
- `TestResolveSlug_FromArgument` -- valid slug returns item
- `TestResolveSlug_ArgumentNotFound` -- invalid slug returns descriptive error
- `TestResolveSlug_CompletedItemWarning` -- completed item via arg prints warning to stderr
- `TestResolveSlug_PickerFiltersCompleted` -- completed items excluded from picker options
- `TestResolveSlug_PickerSortsPriority` -- high before medium before low before lower
- `TestResolveSlug_EmptyRoadmap` -- helpful error message
- `TestResolveSlug_AllCompleted` -- "no eligible items" error
- `TestResolveSlug_NonInteractiveNoArg` -- error requiring slug argument

**Picker option building tests:**
- `TestBuildPickerOpts_Format` -- verifies `"{slug} - {title} [{priority}, {status}]"` format
- `TestBuildPickerOpts_SortOrder` -- verifies priority-then-alpha ordering

**Directory creation tests:**
- `TestEnsurePlanDir_CreatesStructure` -- verifies plan/, research/, synthesis/ exist
- `TestEnsurePlanDir_Idempotent` -- calling twice doesn't error
- `TestEnsurePlanDir_AbsolutePath` -- returned path is absolute

**Existing plan detection tests:**
- `TestExistingPlanDetection_Warns` -- detects existing plan file
- `TestExistingPlanDetection_ForceSkips` -- `--force` bypasses confirmation

**Output tests:**
- `TestPlanOutput_TableMode` -- verifies text summary format
- `TestPlanOutput_JSONMode` -- verifies JSON structure with `slug` and `plan_path` fields

### 7.2 Unit Tests: `internal/config/resolve_test.go`

- `TestResolveStep_PlanDefaultsTier` -- `ResolveStep("plan", nil, nil)` uses `muster-deep` tier
- `TestResolveStep_PlanOverrideModel` -- project config model overrides tier default

### 7.3 Integration-Style Tests

Tests that verify the full `runPlan` function require mocking the Claude invocation. Following the `ai.InvokeAI` pattern from `cmd/out.go`, the exec.Command call should be wrapped in a package-level variable for test replacement:

```go
// planInvoker is the function that invokes Claude Code for planning.
// Replaceable for testing.
var planInvoker = invokePlanClaude

func invokePlanClaude(executable string, args []string, env map[string]string) error {
    cmd := exec.Command(executable, args...)
    // ... setup stdout/stderr/env ...
    return cmd.Run()
}
```

Tests replace `planInvoker` with a function that writes a mock `implementation-plan.md` to the expected location, allowing end-to-end verification of the command flow without requiring Claude to be installed.

### 7.4 Cross-Platform Considerations

- All paths use `filepath.Join` (never string concatenation with `/`)
- `os.MkdirAll` permissions use `0755` with `//nolint:gosec` annotation
- `os.Getwd()` returns platform-native absolute paths
- Tests use `t.TempDir()` for isolation

---

## 8. Technology Choices and Rationale

### 8.1 Skill Staging over `ai.InvokeAI()` (Req 3, Decision 1)

The planning workflow is multi-phase (research, synthesis, plan writing). `ai.InvokeAI()` stages a single SKILL.md and captures JSON output -- it is designed for simple request-response interactions. The plan command needs Claude to orchestrate sub-agents via the `plan-feature` skill's runner files (research-runner.md, synthesis-runner.md, planning-runner.md). `StageSkills` + `exec.Command` is the correct pattern, matching `muster code`.

### 8.2 `--print` Mode for Non-Interactive Execution

Unlike `muster code` which launches an interactive Claude session, `muster plan` runs Claude in `--print` mode. The skill templates contain complete instructions; no user interaction is needed during planning. This makes the command scriptable and allows reliable `--format json` output.

### 8.3 Absolute Paths via `os.Getwd()` (Decision Q4)

Templates reference `{{.PlanDir}}`, `{{.WorktreePath}}`, and `{{.MainRepoPath}}`. Using absolute paths prevents ambiguity when Claude Code's working directory may differ from the user's. `os.Getwd()` is called once at the start of `runPlan` and used to construct all path values.

### 8.4 `muster-deep` Default Tier (Req 12)

Planning requires the highest reasoning capability for codebase analysis and architectural decision-making. The `stepDefaultTiers` entry ensures this without requiring user configuration, while still allowing overrides through the standard 5-step resolution chain.

### 8.5 No Partial Plan Resumption (Decision Q1)

The initial implementation always runs the full pipeline. If interrupted, the user re-runs `muster plan {slug}` and the `--force` flag (or confirmation prompt) handles the overwrite. Partial resumption adds significant complexity (tracking which phases completed, validating intermediate artifacts) for minimal benefit at this stage.

### 8.6 `planInvoker` Variable for Testability

Following the established pattern from `cmd/out.go` (`vcsFactory`) and `internal/ai/invoke.go` (`InvokeAI`), the Claude invocation is wrapped in a replaceable function variable. This enables comprehensive testing of the command flow without requiring external tool installation.

---

## 9. Integration Points

### 9.1 With Existing Skills

The `plan-feature` skill templates already exist at `internal/prompt/prompts/plan-feature/` and reference `{{.Slug}}`, `{{.PlanDir}}`, `{{.Models.Deep}}`, etc. These templates are already staged by `StageSkills()`. The plan command simply needs to provide the correct `PromptContext` values, and the existing skill infrastructure handles the rest.

### 9.2 With `muster code`

After `muster plan` writes the implementation plan, `muster code` can reference it. The `execute-plan` skill (already staged by `StageSkills`) reads from the same `.muster/work/{slug}/plan/` directory. No coupling is needed between the commands -- they communicate through the filesystem.

### 9.3 With Roadmap

The plan command is read-only with respect to the roadmap (Req 16). It does not update item status. The `muster code` command handles the transition to `in_progress`.

### 9.4 With Config System

The plan command participates fully in the 5-step config resolution chain. Users can customize the tool, provider, and model for planning via:
- `.muster/config.yml` pipeline section: `pipeline: { plan: { model: "claude-opus-4" } }`
- `.muster/config.yml` defaults section
- `~/.config/muster/config.yml` user defaults
- Tool-specific tier mappings

---

## 10. File Inventory

**New files:**
- `cmd/plan.go` -- Command definition and execution logic
- `cmd/plan_test.go` -- Unit and integration tests

**Modified files:**
- `internal/config/resolve.go` -- Add `"plan": "muster-deep"` to `stepDefaultTiers`
- `internal/config/resolve_test.go` -- Add test for plan tier default

**Unchanged but consumed:**
- `internal/prompt/stage.go` -- `StageSkills()`
- `internal/prompt/context.go` -- `NewPromptContext()`
- `internal/prompt/prompts/plan-feature/*.md.tmpl` -- Skill templates
- `internal/config/config.go` -- `ToolExecutable()`, `ToolEnvOverrides()`
- `internal/roadmap/roadmap.go` -- Data model
- `internal/roadmap/validate.go` -- `FindBySlug()`
- `internal/roadmap/load.go` -- `LoadRoadmap()`
- `internal/ui/picker.go` -- `DefaultPicker`, `PickerOption`
- `internal/ui/output.go` -- `IsInteractive()`, `GetOutputMode()`
