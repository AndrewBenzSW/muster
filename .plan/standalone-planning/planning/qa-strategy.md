# QA Strategy: muster plan Command

**Role**: QA Strategist
**Date**: 2026-03-23
**Feature**: `muster plan [slug]` command for AI-assisted implementation planning

---

## Executive Summary

The `muster plan` command introduces a new invocation pattern (skill staging with long-running Claude Code orchestration) and critical path operations (directory creation, multi-phase AI workflow, file I/O). Testing must validate both the standard command execution flow and the unique skill staging mechanics. This strategy defines three test layers—unit, integration, and command functional tests—with 42 specific test scenarios covering happy paths, edge cases, error conditions, and cross-platform compatibility. Quality gates ensure the command meets reliability standards before deployment.

---

## Test Layers

### Layer 1: Unit Tests

**Scope**: Individual functions and methods in isolation
**Location**: `internal/prompt/stage_test.go`, `internal/roadmap/roadmap_test.go`, `internal/config/resolve_test.go`
**Coverage Target**: 80% line coverage for new code paths
**Run Frequency**: On every commit via `make test`

**Key Areas**:
- Template rendering with PromptContext (Req #5, #12)
- StageSkills() directory creation and cleanup (Req #3, #6)
- Config resolution for "plan" step (Req #1, #12)
- Roadmap FindBySlug() validation (Req #2, #6)
- Path construction for `.muster/work/{slug}/plan/` (Req #4)

**Existing Coverage**: StageSkills() has comprehensive tests in `/Users/andrew.benz/work/muster/muster-main/internal/prompt/stage_test.go` covering temp directory creation, cleanup idempotency, LF line endings, cross-platform paths, concurrent calls, and error handling. These tests serve as the foundation for plan command validation.

### Layer 2: Integration Tests

**Scope**: Multi-component interactions with real filesystem and temp directories
**Location**: `cmd/plan_test.go` (to be created)
**Coverage Target**: All critical paths exercised with realistic fixtures
**Run Frequency**: On every commit via `make test`, with race detector on CI

**Key Areas**:
- Config loading (user + project) → step resolution → skill staging (Req #1, #3)
- Roadmap loading → slug resolution → directory creation (Req #2, #4)
- PromptContext construction with absolute paths (Req #5, Q4)
- Environment variable overrides application (Req #3, #7)
- Cleanup on success and partial failure (Req #3, #6)

### Layer 3: Command Functional Tests

**Scope**: End-to-end command execution with mocked AI invocation
**Location**: `cmd/plan_test.go`
**Coverage Target**: All command flags, argument patterns, and output modes
**Run Frequency**: On every commit via `make test`

**Key Areas**:
- Cobra command structure and flag definitions (Req #1)
- Argument handling (optional slug vs interactive picker) (Req #2)
- Dry-run simulation without AI invocation (implied from patterns)
- Verbose logging to stderr (Req #1, Q3)
- Error message clarity and actionability (Req #6)

**Pattern Reference**: `/Users/andrew.benz/work/muster/muster-main/cmd/out_test.go` provides the structural testing pattern—test command existence, flag definitions, argument validation, config/roadmap error handling, and full lifecycle integration tests.

---

## Test Scenarios

### Structural Tests (5 scenarios)

Validate command definition and Cobra integration. Based on `TestOutCommand_Exists`, `TestOutCommand_HasRunE`, `TestOutCommand_RequiresExactlyOneArg` patterns.

1. **TestPlanCommand_Exists**
   - **Requirement**: Req #1 (8-step execution pattern)
   - **Assert**: `planCmd` is registered with rootCmd
   - **Assert**: Command use is `"plan [slug]"`
   - **Assert**: Command has short and long descriptions

2. **TestPlanCommand_HasRunE**
   - **Requirement**: Req #1
   - **Assert**: `planCmd.RunE` is not nil
   - **Assert**: `planCmd.Args` is `cobra.MaximumNArgs(1)`

3. **TestPlanCommand_AllFlagsExist**
   - **Requirement**: Req #8, #11
   - **Assert**: `--force` flag exists (bool, default false)
   - **Assert**: `--format` flag exists (inherited from rootCmd)
   - **Assert**: `--verbose` flag exists (inherited from rootCmd)

4. **TestPlanCommand_WithHelpFlag_Succeeds**
   - **Requirement**: Standard CLI behavior
   - **Execute**: `muster plan --help`
   - **Assert**: No error, help text mentions `--force` and interactive picker

5. **TestPlanCommand_IsAddedToRootCommand**
   - **Requirement**: Cobra registration pattern
   - **Assert**: `rootCmd.Commands()` includes command named "plan"

### Config and Roadmap Loading Tests (8 scenarios)

Validate error handling for malformed configs and missing roadmap items. Based on `TestOutCommand_ConfigMalformedYAML`, `TestOutCommand_RoadmapMalformedJSON`, `TestOutCommand_SlugNotFound` patterns.

6. **TestPlanCommand_ConfigMalformedYAML_ReturnsError**
   - **Requirement**: Req #6 (`errors.Is(err, config.ErrConfigParse)`)
   - **Setup**: Create `.muster/config.yml` with invalid YAML
   - **Execute**: `muster plan test-slug`
   - **Assert**: Error contains "config file malformed"

7. **TestPlanCommand_RoadmapMalformedJSON_ReturnsError**
   - **Requirement**: Req #6 (`errors.Is(err, roadmap.ErrRoadmapParse)`)
   - **Setup**: Create `.muster/roadmap.json` with invalid JSON
   - **Execute**: `muster plan test-slug`
   - **Assert**: Error contains "roadmap file is malformed"

8. **TestPlanCommand_SlugNotFound_ReturnsError**
   - **Requirement**: Req #2 (validate slug with FindBySlug)
   - **Setup**: Valid roadmap without target slug
   - **Execute**: `muster plan nonexistent-slug`
   - **Assert**: Error is `fmt.Errorf("roadmap item %q not found", slug)`

9. **TestPlanCommand_EmptyRoadmap_ReturnsError**
   - **Requirement**: Req #13 (handle empty roadmap gracefully)
   - **Setup**: Valid config, empty roadmap `{"items":[]}`
   - **Execute**: `muster plan` (interactive mode)
   - **Assert**: Error contains "No roadmap items found. Run 'muster add' to create one."

10. **TestPlanCommand_ConfigMissing_UsesDefaults**
    - **Requirement**: Req #1 (config resolution chain)
    - **Setup**: No config files, valid roadmap
    - **Execute**: `muster plan test-slug --verbose`
    - **Assert**: Succeeds with hard-coded defaults (claude-code/anthropic/sonnet)
    - **Assert**: Stderr shows resolved triple

11. **TestPlanCommand_ProjectConfigOverridesUser**
    - **Requirement**: Req #1 (config resolution precedence)
    - **Setup**: User config with tool=opencode, project config with tool=claude-code
    - **Execute**: `muster plan test-slug --verbose`
    - **Assert**: Uses claude-code (project wins)

12. **TestPlanCommand_StepConfigOverridesDefaults**
    - **Requirement**: Req #12 (muster-deep tier for plan step)
    - **Setup**: Project config with `pipeline.steps.plan.model_tier: muster-standard`
    - **Execute**: `muster plan test-slug --verbose`
    - **Assert**: Uses standard tier model, not deep

13. **TestPlanCommand_ModelTierResolution**
    - **Requirement**: Req #12 (use muster-deep by default)
    - **Setup**: Valid config with model tiers defined
    - **Execute**: `muster plan test-slug --verbose`
    - **Assert**: Stderr shows model resolved to deep tier (e.g., "claude-opus-4")

### Slug Resolution Tests (7 scenarios)

Validate dual-mode slug handling (argument vs picker). Based on `TestStatusCommand_TableOutputWithItems`, picker patterns from ui package.

14. **TestPlanCommand_SlugArgument_Succeeds**
    - **Requirement**: Req #2 (accept optional positional argument)
    - **Setup**: Valid roadmap with slug "feature-x"
    - **Execute**: `muster plan feature-x`
    - **Assert**: No error, plans "feature-x"

15. **TestPlanCommand_NoSlugNonTTY_Errors**
    - **Requirement**: Req #2 (`ui.IsInteractive()` check)
    - **Setup**: Valid roadmap, non-TTY environment
    - **Execute**: `muster plan` (no arg, no TTY)
    - **Assert**: Error mentions "interactive mode requires a terminal"

16. **TestPlanCommand_PickerFiltersCompleted**
    - **Requirement**: Req #9 (exclude completed items from picker)
    - **Setup**: Roadmap with 3 items: 1 completed, 2 planned
    - **Execute**: `muster plan` (interactive, mock picker)
    - **Assert**: Picker options count is 2 (excludes completed)

17. **TestPlanCommand_PickerSortsByPriority**
    - **Requirement**: Req #10 (sort by priority in picker)
    - **Setup**: Roadmap with items: lower, high, medium, low
    - **Execute**: `muster plan` (interactive, mock picker)
    - **Assert**: Options order is high → medium → low → lower

18. **TestPlanCommand_PickerCancelled_ReturnsError**
    - **Requirement**: Req #6 (handle picker cancellation)
    - **Setup**: Mock picker returns error (ESC pressed)
    - **Execute**: `muster plan` (interactive)
    - **Assert**: Error propagated, no panic

19. **TestPlanCommand_AllItemsCompleted_ShowsError**
    - **Requirement**: Req #9 (handle all-completed case)
    - **Setup**: Roadmap with only completed items
    - **Execute**: `muster plan` (interactive)
    - **Assert**: Error is "No eligible items to plan. All items are completed."

20. **TestPlanCommand_CompletedItemExplicitSlug_ShowsWarning**
    - **Requirement**: Req #9 (warn when planning completed item)
    - **Setup**: Roadmap with completed item "done-feature"
    - **Execute**: `muster plan done-feature`
    - **Assert**: Stderr contains "Warning: planning an already-completed item"
    - **Assert**: Command proceeds (no error)

### Skill Staging and Invocation Tests (10 scenarios)

Validate skill staging, cleanup, and command construction. Leverages existing `stage_test.go` coverage.

21. **TestPlanCommand_StageSkillsCreatesCorrectStructure**
    - **Requirement**: Req #3 (stage skills to tmpDir)
    - **Setup**: Valid config and roadmap
    - **Execute**: Run command with mocked exec.Command
    - **Assert**: Staged directory contains `skills/roadmap-plan-feature/SKILL.md`
    - **Assert**: `.tmpl` extensions removed

22. **TestPlanCommand_CleanupCalledOnSuccess**
    - **Requirement**: Req #3 (defer cleanup immediately)
    - **Setup**: Mock successful AI invocation
    - **Execute**: `muster plan test-slug`
    - **Assert**: Temp directory deleted after command completes

23. **TestPlanCommand_CleanupCalledOnError**
    - **Requirement**: Req #3 (cleanup even on error)
    - **Setup**: Mock AI invocation returns error
    - **Execute**: `muster plan test-slug`
    - **Assert**: Temp directory deleted despite error
    - **Assert**: Error propagated to caller

24. **TestPlanCommand_CommandConstruction**
    - **Requirement**: Req #7 (invoke claude --plugin-dir --model)
    - **Setup**: Valid config with resolved model "claude-sonnet-4.5"
    - **Execute**: Run with mocked exec.Command
    - **Assert**: Command is `["claude", "--plugin-dir", tmpDir, "--model", "claude-sonnet-4.5"]`

25. **TestPlanCommand_EnvironmentOverrides**
    - **Requirement**: Req #3, #7 (apply env overrides)
    - **Setup**: Project config with provider base_url = "http://localhost:8080"
    - **Execute**: Run with mocked exec.Command
    - **Assert**: Env includes `ANTHROPIC_BASE_URL=http://localhost:8080`

26. **TestPlanCommand_PromptContextPaths**
    - **Requirement**: Req #5, Q4 (absolute paths via os.Getwd)
    - **Setup**: Valid config, run from `/Users/test/project`
    - **Execute**: Run with mocked staging
    - **Assert**: PromptContext.MainRepoPath is `/Users/test/project`
    - **Assert**: PromptContext.WorktreePath is `/Users/test/project`
    - **Assert**: PromptContext.PlanDir is `/Users/test/project/.muster/work/test-slug/plan`

27. **TestPlanCommand_PromptContextModelTiers**
    - **Requirement**: Req #5, #12 (Models.Fast/Standard/Deep populated)
    - **Setup**: Config with tool-specific model tiers
    - **Execute**: Run with mocked staging
    - **Assert**: PromptContext.Models.Deep is "claude-opus-4"
    - **Assert**: PromptContext.Models.Standard is "claude-sonnet-4.5"
    - **Assert**: PromptContext.Models.Fast is "claude-haiku-4"

28. **TestPlanCommand_PromptContextInteractive**
    - **Requirement**: Req #5 (Interactive=true for plan command)
    - **Execute**: Run plan command
    - **Assert**: PromptContext.Interactive is true (planning always interactive)

29. **TestPlanCommand_PromptContextNoExtra**
    - **Requirement**: Req #5 (don't populate Extra for skills)
    - **Execute**: Run with captured PromptContext
    - **Assert**: PromptContext.Extra is nil or empty map

30. **TestPlanCommand_StagingError_PropagatesCleanly**
    - **Requirement**: Req #6 (wrapped errors with context)
    - **Setup**: Mock StageSkills to return error
    - **Execute**: `muster plan test-slug`
    - **Assert**: Error is `fmt.Errorf("failed to stage skills: %w", err)`

### Directory Creation Tests (5 scenarios)

Validate `.muster/work/{slug}/plan/` structure creation. Tests trace to Req #4 and Decision #2.

31. **TestPlanCommand_CreatesPlanDirectory**
    - **Requirement**: Req #4 (create `.muster/work/{slug}/plan/`)
    - **Setup**: No existing directories
    - **Execute**: `muster plan test-slug`
    - **Assert**: Directory `.muster/work/test-slug/plan/` exists
    - **Assert**: Subdirectories `research/` and `synthesis/` exist

32. **TestPlanCommand_DirectoryPermissions**
    - **Requirement**: Req #4 (use os.MkdirAll with 0755)
    - **Setup**: No existing directories
    - **Execute**: `muster plan test-slug`
    - **Assert**: Directories have mode 0755

33. **TestPlanCommand_ExistingPlanNoForce_PromptsConfirmation**
    - **Requirement**: Req #8 (detect and warn about existing plans)
    - **Setup**: `.muster/work/test-slug/plan/implementation-plan.md` exists
    - **Execute**: `muster plan test-slug` (interactive)
    - **Assert**: Prompts "Plan already exists. Overwrite? (y/N)"
    - **Assert**: Waits for user input

34. **TestPlanCommand_ExistingPlanWithForce_Overwrites**
    - **Requirement**: Req #8 (skip confirmation with --force)
    - **Setup**: `.muster/work/test-slug/plan/implementation-plan.md` exists
    - **Execute**: `muster plan test-slug --force`
    - **Assert**: No confirmation prompt
    - **Assert**: Proceeds with planning

35. **TestPlanCommand_BlockedItem_ShowsWarning**
    - **Requirement**: Req #15 (warn if item is blocked)
    - **Setup**: Roadmap item with status="blocked"
    - **Execute**: `muster plan blocked-slug`
    - **Assert**: Stderr contains "Warning: item status is 'blocked'"
    - **Assert**: Command proceeds (non-fatal)

### Output and Reporting Tests (4 scenarios)

Validate completion messages and JSON output. Based on `ui.FormatRoadmap*` patterns.

36. **TestPlanCommand_CompletionMessage**
    - **Requirement**: Req #11 (output completion summary)
    - **Setup**: Mock successful planning
    - **Execute**: `muster plan test-slug`
    - **Assert**: Stdout contains "Implementation plan created: .muster/work/test-slug/plan/implementation-plan.md"

37. **TestPlanCommand_JSONOutput**
    - **Requirement**: Req #11, Q2 (basic --format json)
    - **Setup**: Mock successful planning
    - **Execute**: `muster plan test-slug --format json`
    - **Assert**: Stdout is valid JSON: `{"slug": "test-slug", "plan_path": "..."}`

38. **TestPlanCommand_VerboseLogging**
    - **Requirement**: Req #1, Q3 (verbose to stderr)
    - **Setup**: Valid config
    - **Execute**: `muster plan test-slug --verbose`
    - **Assert**: Stderr contains "Using: tool=claude-code, provider=anthropic, model=..."
    - **Assert**: Stderr contains "Staging skills to..."

39. **TestPlanCommand_NoRoadmapModification**
    - **Requirement**: Req #16 (plan is read-only)
    - **Setup**: Capture roadmap before and after
    - **Execute**: `muster plan test-slug`
    - **Assert**: Roadmap file content unchanged
    - **Assert**: No call to `roadmap.SaveRoadmap()`

### Cross-Platform Tests (3 scenarios)

Validate path handling on Windows, macOS, Linux. Based on `TestStageSkillsCrossPlatformPaths` pattern.

40. **TestPlanCommand_WindowsPathSeparators**
    - **Requirement**: Req #4 (use filepath.Join for portability)
    - **Platform**: Windows only (`runtime.GOOS == "windows"`)
    - **Execute**: `muster plan test-slug`
    - **Assert**: Plan directory path contains backslashes
    - **Assert**: Directory created successfully

41. **TestPlanCommand_UnixPathSeparators**
    - **Requirement**: Req #4
    - **Platform**: macOS/Linux (`runtime.GOOS != "windows"`)
    - **Execute**: `muster plan test-slug`
    - **Assert**: Plan directory path contains forward slashes
    - **Assert**: Directory created successfully

42. **TestPlanCommand_AbsolutePathResolution**
    - **Requirement**: Req #5, Q4 (absolute paths via os.Getwd)
    - **Platform**: All
    - **Execute**: `muster plan test-slug --verbose`
    - **Assert**: Stderr shows absolute paths (no relative `./` or `../`)
    - **Assert**: `filepath.IsAbs(planDir)` is true

---

## Quality Gates

### Pre-Commit Gates

**Enforced by**: Local developer workflow
**Failure action**: Fix before commit

1. **All tests pass**: `make test` exits with code 0
2. **No race conditions**: `go test -race ./...` passes
3. **Linting clean**: `make lint` exits with code 0
4. **Build succeeds**: `make build` produces working binary

### CI Gates

**Enforced by**: GitHub Actions on PR
**Failure action**: Fix before merge

1. **Cross-platform tests pass**: Linux, macOS, Windows runners all succeed
2. **Coverage maintained**: New code has ≥80% line coverage
3. **No flaky tests**: All tests pass consistently (3 consecutive runs)
4. **Binary size reasonable**: `dist/muster` ≤ previous release + 5%

### Acceptance Criteria

**Verified by**: Manual testing before release
**Failure action**: Block release, create bug ticket

1. **Happy path works**: `muster plan` on sample project completes without errors
2. **Error messages actionable**: User can fix config/roadmap issues from error text alone
3. **Performance acceptable**: Planning completes in <5min for typical roadmap item
4. **Cleanup reliable**: No temp directory leaks after 10 consecutive runs
5. **Documentation accurate**: `--help` text matches actual behavior

---

## Success Criteria

The `muster plan` command is ready for deployment when:

1. **All 42 test scenarios pass** on Linux, macOS, and Windows
2. **Code coverage ≥80%** for new files (`cmd/plan.go`, related helpers)
3. **No known P0 bugs** (data loss, crashes, security issues)
4. **Performance within budget** (planning <5min, temp cleanup <100ms)
5. **User experience validated** (manual acceptance testing complete)

### Testable Assertions per Requirement

- **Req #1** (8-step pattern): Tests 1-13 validate config loading, flag parsing, verbose logging
- **Req #2** (slug resolution): Tests 14-20 validate argument handling, picker integration
- **Req #3** (skill staging): Tests 21-30 validate StageSkills, cleanup, command construction
- **Req #4** (output paths): Tests 31-35 validate directory creation, permissions, overwrite behavior
- **Req #5** (PromptContext): Tests 26-29 validate path resolution, model tiers, interactive flag
- **Req #6** (error handling): Tests 6-8, 18, 30 validate error wrapping, descriptive messages
- **Req #7** (direct invocation): Tests 24-25 validate command construction, env overrides
- **Req #8** (existing plans): Tests 33-34 validate confirmation prompts, --force flag
- **Req #9** (filter completed): Tests 16, 19-20 validate picker filtering, warnings
- **Req #10** (sort picker): Test 17 validates priority-based sorting
- **Req #11** (output summary): Tests 36-38 validate completion messages, JSON output
- **Req #12** (muster-deep tier): Tests 12-13, 27 validate model tier resolution
- **Req #13** (empty roadmap): Test 9 validates graceful error message
- **Req #15** (validate suitability): Test 35 validates blocked item warning
- **Req #16** (no roadmap modification): Test 39 validates read-only behavior

### Risk Mitigation

**High Risk: Temp directory leaks**
- **Mitigation**: Tests 22-23 validate cleanup on success and error
- **Detection**: Manual check for `muster-prompts-*` dirs in system temp after test runs
- **Fallback**: Document cleanup command in troubleshooting guide

**Medium Risk: Cross-platform path issues**
- **Mitigation**: Tests 40-42 validate Windows/Unix path handling
- **Detection**: CI runs on all three platforms (Linux, macOS, Windows)
- **Fallback**: Block release if any platform fails

**Medium Risk: Picker hangs in non-TTY**
- **Mitigation**: Test 15 validates early error on non-TTY
- **Detection**: CI runs in non-TTY environment
- **Fallback**: Document batch mode requirement in help text

**Low Risk: Config precedence confusion**
- **Mitigation**: Tests 11-12 validate resolution chain
- **Detection**: Verbose logging shows resolved config
- **Fallback**: Improve error messages with resolution trace

---

## Notes

- **Existing test coverage**: StageSkills() in `/Users/andrew.benz/work/muster/muster-main/internal/prompt/stage_test.go` provides comprehensive baseline (18 tests covering concurrency, cleanup, cross-platform paths, LF line endings)
- **Pattern source**: `/Users/andrew.benz/work/muster/muster-main/cmd/out_test.go` demonstrates structural testing pattern for Cobra commands (120+ tests)
- **Roadmap validation**: `/Users/andrew.benz/work/muster/muster-main/internal/roadmap/roadmap_test.go` shows FindBySlug and AddItem test patterns
- **AI invocation mocking**: `cmd/out_test.go` lines 666-883 show pattern for mocking `ai.InvokeAI` during tests
- **Golden files**: Not applicable for plan command (no template rendering output to verify)
- **Race detector**: Existing concurrent tests in `stage_test.go` (lines 440-488) use `go test -race` pattern
