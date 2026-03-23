# Standalone Planning Command: Implementation Plan

*Version: 2026-03-23*

The `muster plan [slug]` command enables AI-assisted implementation planning for roadmap items. It resolves a roadmap slug (from args or interactive picker), stages plan-feature skill templates, invokes Claude Code with `--plugin-dir`, and verifies that an implementation plan was produced at `.muster/work/{slug}/plan/implementation-plan.md`.

This command is architecturally distinct from the single-shot `add` and `sync` commands (which use `ai.InvokeAI()`). Like `muster code`, it uses `prompt.StageSkills()` + `exec.Command` to launch Claude Code with staged skills. The plan-feature skill executes automatically in the foreground when loaded via `--plugin-dir` — it reads the codebase, synthesizes findings, writes the plan, and exits. No `--print` flag is needed because skills handle their own orchestration.

Only two new files are created (`cmd/plan.go`, `cmd/plan_test.go`) and one existing file is modified (`internal/config/resolve.go`). All other functionality composes existing packages without modification.

---

## Technology Stack

| Layer | Technology |
|-------|-----------|
| Language | Go 1.23+ |
| CLI framework | Cobra |
| Config | internal/config (5-step resolution chain) |
| Roadmap | internal/roadmap (JSON load, FindBySlug) |
| Templates | internal/prompt (StageSkills, PromptContext) |
| TUI | internal/ui (HuhPicker via charmbracelet/huh) |
| AI invocation | exec.Command → claude --plugin-dir |
| Testing | testify/assert, testify/require |

---

## Solution Structure

```
cmd/
  plan.go              NEW  — Cobra command definition and execution logic
  plan_test.go         NEW  — Unit and integration tests (42 scenarios)
internal/
  config/
    resolve.go         MOD  — Add "plan": "muster-deep" to stepDefaultTiers
    resolve_test.go    MOD  — Add test for plan tier default
```

**Unchanged but consumed**: `internal/prompt/stage.go`, `internal/prompt/context.go`, `internal/config/config.go`, `internal/roadmap/roadmap.go`, `internal/roadmap/validate.go`, `internal/roadmap/load.go`, `internal/ui/picker.go`, `internal/ui/output.go`

---

## Implementation Phases

### Phase 1: Config Foundation and Command Scaffold

Create the command structure with slug resolution and directory creation. Wire up config resolution with the `muster-deep` default tier.

| ID | Task | Key Details |
|----|------|-------------|
| 1.1 | Add plan tier default | Add `"plan": "muster-deep"` to `stepDefaultTiers` in `internal/config/resolve.go`. Add `TestStepDefaultTiers_ContainsPlanKey` (direct map verification), `TestResolveStep_PlanDefaultsTier`, `TestResolveStep_PlanOverrideModel`, and `TestResolveStep_PlanInvalidTierOverride_Errors` to `internal/config/resolve_test.go`. |
| 1.2 | Create `cmd/plan.go` scaffold | Define `planCmd` with `Use: "plan [slug]"`, `Args: cobra.MaximumNArgs(1)`, `RunE: runPlan`. Register `--force` flag. Add `init()` to register with `rootCmd`. Implement 8-step execution pattern through step 5 (load roadmap). |
| 1.3 | Implement slug resolution | Create `resolveSlug(args []string, rm *roadmap.Roadmap, interactive bool, w io.Writer, picker ui.Picker) (string, *roadmap.RoadmapItem, error)`. Argument mode: `FindBySlug` with "not found" error; print completed-item warning to `w` if interactive. Picker mode: filter completed items, sort by priority then alpha, build `PickerOption` with `"{slug} - {title} [{priority}, {status}]"` format. Guard non-interactive mode. Guard empty roadmap. Explicit parameters enable test mocking without global state. |
| 1.4 | Implement directory creation | Create `ensurePlanDir(projectRoot, slug)` returning absolute path. Use `os.MkdirAll` for `.muster/work/{slug}/plan/`, `research/`, `synthesis/` with `0755` permissions and `//nolint:gosec // G301: Standard directory permissions for plan storage` annotation. Resolve absolute path via `os.Getwd()`. |
| 1.5 | Add structural and slug resolution tests | Tests 1-5 (command structure), 6-9 (config/roadmap errors), 14-20 (slug resolution). Use patterns from `cmd/out_test.go`. Mock picker for interactive tests. |

**Dependencies:** None

### Phase 2: Skill Staging, Invocation, and Verification

Wire up PromptContext construction, skill staging, Claude Code invocation, and post-invocation verification.

| ID | Task | Key Details |
|----|------|-------------|
| 2.1 | Implement PromptContext construction | Build context with `prompt.NewPromptContext(resolved, projectCfg, userCfg, true, slug, cwd, cwd, planDir)`. Both WorktreePath and MainRepoPath set to absolute `cwd`. PlanDir is absolute. Do not populate Extra map. |
| 2.2 | Implement skill staging and cleanup | Call `prompt.StageSkills(ctx)`, immediately `defer cleanup()`. Handle staging errors with wrapped message: `"failed to stage skills: %w"`. |
| 2.3 | Implement Claude Code invocation | Create `planInvoker` function variable (following `vcsFactory` pattern from `cmd/out.go` for testability). Build command: `exec.Command(config.ToolExecutable(resolved.Tool), "--plugin-dir", tmpDir, "--model", resolved.Model)`. No `--print` flag — skills execute automatically in foreground when loaded via `--plugin-dir`. Connect stdin/stdout/stderr (stdin for skill interaction). Apply `config.ToolEnvOverrides`. Run and check exit code. |
| 2.4 | Implement output verification | After successful invocation, `os.Stat` the expected `implementation-plan.md`. Error if missing: `"planning completed but implementation-plan.md was not created at %s"`. Also check `info.Size() == 0` and error: `"plan file exists but is empty at %s"`. Handle stat errors: `"failed to verify plan file: %w"`. |
| 2.5 | Add invocation and staging tests | Tests 21-30 (staging, cleanup, command construction, env overrides, PromptContext validation). Mock `planInvoker` to write stub plan file. Verify cleanup on both success and error paths. |

**Dependencies:** Phase 1

### Phase 3: UX Polish, Output, and Full Test Coverage

Add existing plan detection, warnings, output formatting, and remaining test scenarios.

| ID | Task | Key Details |
|----|------|-------------|
| 3.1 | Implement existing plan detection and `--force` | Before invocation, check if `implementation-plan.md` exists. Create `confirmOverwrite(interactive bool, stdin io.Reader, w io.Writer) (bool, error)` function for testability. If interactive and no `--force`: prompt "Plan already exists. Overwrite? (y/N)" via `bufio.NewReader(stdin).ReadString('\n')`, accept "y"/"Y". If non-interactive without `--force`: return error `"plan exists at %s; use --force to overwrite in non-interactive mode"`. If `--force`: skip prompt, log "Overwriting existing plan" to stderr. |
| 3.2 | Implement warnings | Print to stderr when `interactive` is true: completed-item warning in `resolveSlug` after finding item, blocked-item warning in `runPlan` after slug resolution before directory creation. Format: `fmt.Fprintf(w, "Warning: %s\n", condition)`. Two warnings only: `"Warning: planning an already-completed item"` and `"Warning: item status is 'blocked'"`. Suppress in non-interactive mode by checking the `interactive` parameter. |
| 3.3 | Implement output formatting | Table mode: `"Implementation plan created: {relative_path}"`. JSON mode: `{"slug": "...", "plan_path": "..."}` using relative path from project root. Use `ui.GetOutputMode()` for format detection. Verbose mode: log resolved config triple and staging path to stderr. |
| 3.4 | Add remaining tests | Tests 31-35 (directory creation, permissions, overwrite behavior), 36-39 (output, JSON, verbose, read-only), 40-42 (cross-platform paths). Additional tests: `TestPlanCommand_ExistingPlanNonInteractiveNoForce_Errors`, `TestPlanCommand_EmptyPlanFile_Errors`. Verify no roadmap modification after planning. |
| 3.5 | Run full validation | `make build && make test && make lint`. Verify all 42 tests pass. Verify cross-platform path handling. Manual smoke test with real roadmap. |

**Dependencies:** Phase 2

### Phase Dependency Graph

```
Phase 1: Config Foundation and Command Scaffold
    |
    v
Phase 2: Skill Staging, Invocation, and Verification
    |
    v
Phase 3: UX Polish, Output, and Full Test Coverage
```

Linear dependency chain — each phase builds on the previous. Within each phase, test tasks (1.5, 2.5, 3.4) can be written first (TDD style) but will fail until implementation tasks in the same phase are complete. Alternatively, implement code and tests together incrementally within each phase.

---

## Acceptance Criteria

| # | Criterion |
|---|-----------|
| AC-1 | `muster plan my-slug` with valid roadmap resolves slug and invokes Claude Code with `--plugin-dir --model` flags |
| AC-2 | `muster plan` (no arg, interactive) shows picker with completed items filtered and items sorted by priority |
| AC-3 | `muster plan` in non-TTY without slug argument returns descriptive error |
| AC-4 | Plan output is written to `.muster/work/{slug}/plan/implementation-plan.md` with research/ and synthesis/ subdirectories created |
| AC-5 | `config.ResolveStep("plan", nil, nil)` defaults to `muster-deep` model tier |
| AC-6 | Existing plan file triggers confirmation prompt (skippable with `--force`) |
| AC-7 | `--format json` outputs `{"slug": "...", "plan_path": "..."}` |
| AC-8 | Malformed config returns `config.ErrConfigParse`, malformed roadmap returns `roadmap.ErrRoadmapParse` |
| AC-9 | Staging cleanup runs on both success and error paths (no temp directory leaks) |
| AC-10 | Command is read-only with respect to roadmap file |
| AC-11 | All paths use `filepath.Join` for cross-platform compatibility |
| AC-12 | `make build && make test && make lint` all pass on Linux, macOS, and Windows |

---

## Test Strategy

**3 test layers, 42 scenarios** (see QA strategy doc for full details):

- **Structural tests** (5): Command definition, flags, registration
- **Config/roadmap tests** (8): Error handling, config precedence, model tier resolution
- **Slug resolution tests** (7): Argument vs picker, filtering, sorting, edge cases
- **Staging/invocation tests** (10): StageSkills, cleanup, command construction, env overrides, PromptContext
- **Directory creation tests** (5): Permissions, overwrite behavior, warnings
- **Output tests** (4): Completion messages, JSON format, verbose logging, read-only verification
- **Cross-platform tests** (3): Windows/Unix path separators, absolute path resolution

**Quality gates**: `make test` (with `-race`), `make lint`, `make build` before every commit. CI runs on Linux, macOS, Windows. Coverage ≥80% for new code.

**Testability**: `planInvoker` function variable (following `vcsFactory` pattern from `cmd/out.go`) allows mocking Claude invocation. Tests write stub `implementation-plan.md` to verify the full flow without requiring Claude.

---

## Known Limitations

- **No partial plan resumption**: If interrupted, user must re-run `muster plan` from scratch. Deferred to future iteration.
- **No `--research-only` or `--skip-research` flags**: Full pipeline execution only. Phase control deferred.
- **No `--output` flag**: Output always goes to `.muster/work/{slug}/plan/`. Custom output location deferred.
- **No backward compatibility with `.plan/` paths**: Legacy path structure is not supported. Migration via `muster init --migrate` deferred to Phase 7.
- **No Docker/yolo mode**: Planning runs locally only (safe operation, no need for isolation).
- **Test fixtures using legacy paths**: 13 existing test contexts reference `.plan/{slug}/` and should be updated during implementation, but this is not blocking.
- **No concurrency protection**: Multiple simultaneous `muster plan` invocations for the same slug will overwrite each other's outputs. Use branch-based workflows to avoid this.
- **No slug format validation**: Invalid slug characters (spaces, slashes) are caught by `FindBySlug` returning nil, not by explicit format validation. Future improvement could add pre-validation with descriptive error.
