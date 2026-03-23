# Standalone Planning Command: Requirements Synthesis

*Synthesized: 2026-03-23*
*Sources: cli-command-patterns.md, prompt-ai-invocation.md, roadmap-slug-resolution.md, output-path-conventions.md*

---

## Executive Summary

Research across four domains reveals a mature, consistent CLI architecture built on clear patterns: an 8-step execution flow, a 5-level config resolution chain, skill staging for complex AI workflows, and dual-mode slug resolution (argument or interactive picker). The `muster plan` command should integrate into these established patterns while introducing a key architectural shift: it will be the first command to use skill staging with Claude Code's plugin system rather than direct AI invocation, enabling multi-phase planning workflows (research → synthesis → plan writing) that operate on the codebase.

The most critical decisions center on path conventions and AI invocation strategy. Research confirmed that `.muster/work/{slug}/plan/` is the canonical output location (legacy `.plan/{slug}/` paths exist only for backward compatibility), and that the command should invoke Claude Code directly with staged skills rather than using the existing `ai.InvokeAI()` helper, which is designed for single-shot prompts. This represents a stepping stone toward the full pipeline orchestration planned for later phases.

Key open questions remain around handling existing plans (overwrite behavior), filtering completed items from the picker, and whether to support partial execution of planning phases. Recommendations favor allowing overwrites with warnings, excluding completed items by default, and implementing full-pipeline execution first with phase control deferred to a future iteration.

---

## Requirements

### MUST Have

1. **Follow the 8-step command execution pattern** (Source: cli-command-patterns.md, lines 10-21)
   - Parse flags using `cmd.Flags().GetString()` and check `rootCmd.PersistentFlags()` for `--verbose`
   - Load user and project config with `config.LoadUserConfig("")` and `config.LoadProjectConfig(".")`
   - Resolve step-specific config using `config.ResolveStep("plan", projectCfg, userCfg)`
   - Enable verbose logging to stderr when `--verbose` flag is set
   - Load roadmap via `roadmap.LoadRoadmap(".")` with fallback chain
   - Execute planning business logic (see invocation requirements below)
   - No roadmap save needed (planning is read-only from roadmap perspective)
   - Output completion summary with plan file path

2. **Implement dual-mode slug resolution** (Source: roadmap-slug-resolution.md, lines 10-14, 106-163)
   - Accept optional positional argument `[slug]` with `Args: cobra.MaximumNArgs(1)`
   - If slug provided in args: validate with `rm.FindBySlug(slug)`, error if not found
   - If no slug and interactive mode: use `ui.DefaultPicker.Show()` to select from roadmap items
   - Build picker options with format: `"{slug} - {title} [{priority}, {status}]"`
   - Check `ui.IsInteractive()` before showing picker, error if non-TTY
   - Return descriptive error: `fmt.Errorf("roadmap item %q not found", slug)` on lookup failure

3. **Use skill staging pattern, not direct AI invocation** (Source: prompt-ai-invocation.md, lines 44-73, 244-254)
   - Call `prompt.StageSkills(ctx)` to stage all templates from `prompts/` directory
   - Skills will be staged to `tmpDir/skills/roadmap-plan-feature/` with `SKILL.md` and supporting files
   - Invoke Claude Code with: `claude --plugin-dir <tmpDir> --model <model>`
   - Do NOT use `ai.InvokeAI()` which creates its own tmpDir for single-shot prompts
   - Apply environment overrides via `config.ToolEnvOverrides(resolved, projectCfg, userCfg)`
   - Always defer cleanup immediately after `StageSkills()`: `defer cleanup()`

4. **Write output to `.muster/work/{slug}/plan/`** (Source: output-path-conventions.md, lines 10-30, 115-130)
   - Base directory: `.muster/work/{slug}/plan/`
   - Subdirectories: `research/`, `synthesis/`
   - Implementation plan: `.muster/work/{slug}/plan/implementation-plan.md`
   - Create directory structure with: `os.MkdirAll(filepath.Join(planDir, "research"), 0755)`
   - Pass constructed `planDir` to `prompt.NewPromptContext()` for template access via `{{.PlanDir}}`
   - Verify plan output exists at expected location after Claude invocation completes

5. **Create PromptContext with correct paths** (Source: prompt-ai-invocation.md, lines 256-268)
   - Call `prompt.NewPromptContext(resolved, projectCfg, userCfg, interactive, slug, worktreePath, mainRepoPath, planDir)`
   - Set `interactive` based on TTY detection (always true for plan command)
   - Set `worktreePath` and `mainRepoPath` to `"."` (planning happens in main repo before worktree creation)
   - Set `planDir` to `.muster/work/{slug}/plan` (absolute path)
   - Do NOT populate `ctx.Extra` map (only used for direct invocation templates)

6. **Handle errors following established patterns** (Source: cli-command-patterns.md, lines 92-108)
   - Check `errors.Is(err, config.ErrConfigParse)` for config file malformation
   - Check `errors.Is(err, roadmap.ErrRoadmapParse)` for roadmap parse errors
   - Return wrapped errors with context: `fmt.Errorf("failed to stage skills: %w", err)`
   - Handle picker cancellation gracefully (ESC/Ctrl+C returns error)
   - Validate slug exists before proceeding with planning

7. **Invoke Claude Code directly with staged plugin-dir** (Source: prompt-ai-invocation.md, lines 318-333)
   - Build command: `exec.Command(config.ToolExecutable(resolved.Tool), "--plugin-dir", tmpDir, "--model", resolved.Model)`
   - Merge environment: `cmd.Env = append(os.Environ(), envOverrides...)`
   - Connect stdout/stderr to command output streams for user visibility
   - Wait for completion and check exit code
   - Parse/validate output files exist at expected locations

### SHOULD Have

8. **Detect and warn about existing plans** (Source: cli-command-patterns.md, lines 347-348, open question; output-path-conventions.md, open question lines 176-183)
   - Check if `.muster/work/{slug}/plan/implementation-plan.md` exists before starting
   - Prompt user for confirmation: "Plan already exists. Overwrite? (y/N)"
   - Skip confirmation if `--force` flag provided
   - Log info message about overwriting existing plan

9. **Filter completed items from picker** (Source: roadmap-slug-resolution.md, lines 167-170, open question lines 210-213)
   - Exclude items with `status == roadmap.StatusCompleted` from picker options
   - Show error if all items are completed: "No eligible items to plan. All items are completed."
   - Allow explicit slug argument to bypass filter (user intentionally planning completed item)
   - Log warning if planning a completed item: "Warning: planning an already-completed item"

10. **Sort picker items by priority** (Source: roadmap-slug-resolution.md, lines 220-223)
    - Order picker options: high → medium → low → lower
    - Within each priority tier, sort alphabetically by slug
    - Use `roadmap.ValidPriorities()` for canonical ordering
    - Improves UX when selecting from many items

11. **Output completion summary** (Source: cli-command-patterns.md, lines 351-353)
    - Print plan file path: `fmt.Fprintf(cmd.OutOrStdout(), "Implementation plan created: %s\n", planPath)`
    - Show brief metadata: number of phases, estimated complexity if available
    - Use consistent output formatting with other commands
    - Support `--format json` for structured output (plan metadata only)

12. **Use `muster-deep` model tier** (Source: roadmap-slug-resolution.md, lines 193-199)
    - Plan command requires highest reasoning capability
    - Default to `muster-deep` tier in step config
    - Allows user override via project/user config
    - Accessed in templates via `{{.Models.Deep}}`

### NICE TO HAVE

13. **Handle empty roadmap gracefully** (Source: roadmap-slug-resolution.md, lines 172-181)
    - Check `len(rm.Items) == 0` before attempting resolution
    - Show helpful message: "No roadmap items found. Run 'muster add' to create one."
    - Exit with error code (not a success case)
    - Matches existing command patterns in status/out

14. **Support `--output` flag to override plan location** (Source: cli-command-patterns.md, line 322)
    - Allow user to specify alternative output directory
    - Validate path is writable before invoking AI
    - Still create `.muster/work/{slug}/` structure under custom base
    - Useful for experimentation or alternative repo layouts

15. **Validate roadmap item is suitable for planning** (Source: roadmap-slug-resolution.md, open question lines 409-414)
    - Warn if item has status "blocked" (may need unblocking first)
    - Warn if item has empty context (planning may be limited)
    - Allow proceeding with confirmation
    - Non-fatal validation (informational only)

### SHOULD NOT Include

16. **Do NOT modify the roadmap file** (Source: cli-command-patterns.md, lines 372-374)
    - Planning is a read-only operation from roadmap perspective
    - Status updates happen via separate commands (`muster code`, `muster out`)
    - No `roadmap.SaveRoadmap()` call needed
    - Keeps plan command safe and idempotent

17. **Do NOT stage skills from add-item/, sync-match/, or out/ directories** (Source: prompt-ai-invocation.md, lines 162-163)
    - These templates are for direct AI invocation, not skill staging
    - `StageSkills()` already excludes these directories
    - Plan command only needs `roadmap-plan-feature` skill
    - Keep staged plugin-dir minimal for performance

18. **Do NOT implement backward compatibility fallback to `.plan/` paths** (Source: output-path-conventions.md, lines 31-39, 131-146)
    - Legacy `.plan/{slug}/` structure is deprecated
    - Fallback logic is documented but not required for Phase 5
    - Always write to `.muster/work/{slug}/plan/` regardless of existing legacy paths
    - Migration support can be added in future phase with `muster init --migrate`

19. **Do NOT implement `--yolo` or Docker orchestration** (Source: cli-command-patterns.md, lines 376-378)
    - Planning is safe to run locally (no destructive operations)
    - Docker orchestration is only for interactive coding sessions
    - Command operates on main repo, not in isolated container
    - Keep plan command simple and fast

20. **Do NOT implement partial phase execution** (Source: cli-command-patterns.md, open question lines 384-390)
    - Start with full-pipeline execution (research → synthesis → plan)
    - Defer `--research-only`, `--skip-research` flags to future iteration
    - Simplifies initial implementation and testing
    - Can detect partial plans and offer resumption later

---

## Key Decisions

### Decision 1: Use Skill Staging, Not Direct AI Invocation
**Rationale**: The research-synthesis-planning workflow is fundamentally different from single-shot prompts used by `add` and `sync` commands. Skill staging enables Claude Code to orchestrate multi-file workflows with the roadmap-plan-feature skill that contains SKILL.md, research-runner.md, synthesis-runner.md, and planning-runner.md. This matches the long-running agent pattern used by `muster code` command.

**Alternatives considered**: Extending `ai.InvokeAI()` to support multi-phase operations, but this would conflate two distinct invocation patterns and make the code harder to understand. Skill staging is the established pattern for complex workflows.

**Source**: prompt-ai-invocation.md lines 61-74, 244-307

### Decision 2: Write to `.muster/work/{slug}/plan/`, Not `.plan/{slug}/`
**Rationale**: Documentation explicitly establishes `.muster/work/` as the canonical location with backward compatibility for `.plan/` being a future migration concern. Writing to the modern location from the start avoids technical debt and aligns with the documented design. Test fixtures using legacy paths are acknowledged as out-of-date.

**Alternatives considered**: Using `.plan/` to match existing test fixtures, but this would perpetuate the deprecated structure. Better to update tests during implementation.

**Source**: output-path-conventions.md lines 10-30, 115-130, 268

### Decision 3: Always Create `.muster/work/{slug}/plan/` Structure
**Rationale**: The plan command should be runnable before `muster init` to enable quick planning. Using `os.MkdirAll()` creates the directory structure if it doesn't exist, making the command more ergonomic. This follows the principle of least surprise — users expect `muster plan` to "just work" without prerequisites.

**Alternatives considered**: Requiring `muster init` first, but this adds friction to the workflow and isn't necessary for a read-only operation that only creates local files.

**Source**: output-path-conventions.md open question lines 166-174

### Decision 4: Exclude Completed Items from Picker by Default
**Rationale**: Planning an already-completed item is likely user error. Filtering improves UX by reducing noise in the picker. Users who genuinely want to plan a completed item (for documentation or re-planning) can edit the status first or pass the slug explicitly as an argument to bypass the filter.

**Alternatives considered**: Showing all items and relying on user judgment, but this increases cognitive load and makes mistakes more likely. Warning on explicit completed-item planning provides a safety net.

**Source**: roadmap-slug-resolution.md lines 210-213, cli-command-patterns.md open question lines 409-414

### Decision 5: Allow Overwriting Existing Plans with Warning
**Rationale**: Users may want to regenerate plans after scope changes or manual edits. Prompting for confirmation (skippable with `--force`) balances safety with flexibility. Unlike modifying version-controlled code, regenerating a plan is non-destructive — the old plan can be recovered from git history.

**Alternatives considered**: Creating versioned backups (e.g., `implementation-plan.md.1`), but this clutters the directory. Erroring on existing plans forces users to manually delete files, adding friction.

**Source**: cli-command-patterns.md open question lines 396-398, 384-390

### Decision 6: Invoke Claude Code Directly, Don't Extend ai.InvokeAI()
**Rationale**: The `ai.InvokeAI()` function is designed for single-shot prompts with JSON responses. Planning requires long-running orchestration with file outputs, not a JSON response. Creating a separate invocation path for plan keeps concerns separated and makes the codebase easier to understand.

**Alternatives considered**: Adding a "skill mode" parameter to `ai.InvokeAI()`, but this creates a confusing dual-mode API. Direct exec.Command invocation is clearer and more maintainable.

**Source**: prompt-ai-invocation.md lines 318-333, open question lines 323-333

---

## Resolved Questions

### Q1: Partial plan resumption → **DEFERRED**
Full-pipeline execution only. If interrupted, user re-runs and overwrites. Resumption logic deferred to future iteration.

### Q2: JSON output → **BASIC**
Implement basic `--format json` with `slug` and `plan_path` fields. Defer rich metadata (phases, timestamps) to later.

### Q3: Non-fatal warnings → **STDERR IN INTERACTIVE MODE**
Print warnings to stderr (e.g., "Warning: item status is 'completed'"). Suppress in non-interactive mode. Use consistent "Warning: {condition}" format.

### Q4: PromptContext paths → **ABSOLUTE**
Use `os.Getwd()` to resolve absolute paths. Set both WorktreePath and MainRepoPath to the same absolute path (main repo).

---

## References

### Research Documents
- **cli-command-patterns.md**: Command structure, flag patterns, argument handling, AI invocation pattern, error handling, interactive mode detection, template rendering
- **prompt-ai-invocation.md**: Template system architecture, skill staging mechanism, PromptContext structure, two invocation patterns, environment overrides, cross-platform compatibility
- **roadmap-slug-resolution.md**: FindBySlug validation, picker infrastructure, LoadRoadmap fallback chain, slug generation, config resolution, filtering recommendations
- **output-path-conventions.md**: Modern `.muster/work/` vs legacy `.plan/` paths, directory structure conventions, test fixture inconsistencies, backward compatibility plan

### Key Code References
- `/Users/andrew.benz/work/muster/muster-main/cmd/add.go` — Batch/interactive modes, AI invocation, picker usage
- `/Users/andrew.benz/work/muster/muster-main/cmd/status.go` — Optional argument handling, slug resolution
- `/Users/andrew.benz/work/muster/muster-main/internal/prompt/stage.go` — StageSkills implementation
- `/Users/andrew.benz/work/muster/muster-main/internal/prompt/context.go` — PromptContext definition
- `/Users/andrew.benz/work/muster/muster-main/internal/roadmap/validate.go` — FindBySlug implementation
- `/Users/andrew.benz/work/muster/muster-main/internal/ui/picker.go` — HuhPicker implementation

### Design Documentation
- `/Users/andrew.benz/work/muster/muster-main/docs/design.md` lines 240-269 — Runtime data layout
- `/Users/andrew.benz/work/muster/muster-main/docs/design.md` lines 510-513 — Phase 5 deliverables
- `/Users/andrew.benz/work/muster/muster-main/CLAUDE.md` — Project patterns and testing conventions

### Pattern Counts
- **8-step execution pattern** used by all commands (add, sync, code, status, out)
- **5-step config resolution chain** (pipeline step → project defaults → user defaults → tool defaults → hard-coded)
- **3 model tiers** (fast, standard, deep) with 4-level precedence for resolution
- **2 invocation patterns** (direct AI for single-shot, skill staging for workflows)
- **13 test contexts** using legacy `.plan/` paths (need updating during implementation)
