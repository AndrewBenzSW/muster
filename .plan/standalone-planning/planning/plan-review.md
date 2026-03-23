# Plan Review: muster plan [slug] Command

**Role**: Plan Reviewer (Adversarial)
**Date**: 2026-03-23
**Reviewed Documents**: implementation-plan.md, architecture.md, qa-strategy.md, synthesis.md, CLAUDE.md
**Codebase Verification**: Completed against `/Users/andrew.benz/work/muster/muster-main`

---

## Executive Summary

This plan is **mostly solid but has 2 BLOCKER issues and 7 SHOULD FIX issues** that must be addressed before execution. The architecture correctly identifies skill staging as the right pattern, and the phased approach is sensible. However, critical gaps exist around the `--print` flag usage, missing flag implementations, and incomplete test scenarios.

**Key Strengths:**
- Correct architectural choice: skill staging over `ai.InvokeAI()`
- Minimal changes (2 new files, 1 modification to `resolve.go`)
- Comprehensive test strategy with 42 scenarios
- Proper use of existing patterns (`vcsFactory`, `stepDefaultTiers`)

**Key Weaknesses:**
- **BLOCKER**: Implementation assumes `--print` flag exists but it doesn't in Claude Code
- **BLOCKER**: Missing `--force` flag implementation details
- Multiple missing implementation details for non-fatal warnings
- Incomplete verification logic for plan output
- Test scenarios don't cover all edge cases properly

---

## BLOCKER Findings (Must Fix Before Execution)

### B1: `--print` Flag Does Not Exist in Claude Code

**Location**: Architecture doc lines 209-212, Implementation plan Phase 2 Task 2.3

**Issue**: The plan states:
> "Unlike `muster code` which launches an interactive Claude session, `muster plan` runs Claude in `--print` mode."

And Task 2.3 specifies:
> "Build command: `exec.Command(config.ToolExecutable(resolved.Tool), "--print", "--plugin-dir", tmpDir, "--model", resolved.Model)`"

**Actual Codebase Reality**: Searching the codebase for `--print` usage reveals it's ONLY used with `ai.InvokeAI()` for single-shot prompts where stdin is piped (`cmd.Stdin = strings.NewReader(cfg.Prompt)`). The `muster code` command does NOT use `--print` — it launches Claude Code interactively without any special flags (see `cmd/code.go:139`).

**Evidence from code.go:**
```go
cmdArgs := []string{"--plugin-dir", tmpDir, "--model", resolved.Model}
execCmd := exec.Command(config.ToolExecutable(resolved.Tool), cmdArgs...)
execCmd.Stdin = os.Stdin  // Interactive
execCmd.Stdout = os.Stdout
execCmd.Stderr = os.Stderr
```

**Why This Is a Blocker**: The plan's core invocation strategy is invalid. Claude Code skills execute automatically when loaded via `--plugin-dir` — there is no separate "print mode" vs "interactive mode" for skills. The `--print` flag is specifically for single-shot prompts piped via stdin (the `ai.InvokeAI()` pattern).

**Correct Approach**: The plan command should invoke Claude Code with:
```go
cmdArgs := []string{"--plugin-dir", tmpDir, "--model", resolved.Model}
```

The `plan-feature` skill's SKILL.md will execute automatically in the foreground (not as an interactive session). The skill writes files to `{{.PlanDir}}` and exits. This is ALREADY the correct behavior based on how skills work — the architecture doc is wrong about needing `--print`.

**Fix Required**:
1. Remove all references to `--print` flag from implementation plan Phase 2 Task 2.3
2. Update Architecture doc section 4.5 to clarify: "Skills execute in foreground when loaded, writing outputs to PlanDir"
3. Update QA Strategy test 24 (TestPlanCommand_CommandConstruction) to verify command is `["claude", "--plugin-dir", tmpDir, "--model", "..."]` WITHOUT `--print`

---

### B2: `--force` Flag Implementation Completely Missing

**Location**: Implementation plan Phase 3 Task 3.1, Architecture doc section 4.3

**Issue**: The plan states `--force` flag should skip overwrite confirmation (Phase 3 Task 3.1):
> "If so and `--force` not set: prompt 'Plan already exists. Overwrite? (y/N)'"

But there is NO implementation guidance for:
1. How to read from stdin in non-interactive mode (task says "in interactive mode" but what about piped input?)
2. What happens in non-interactive mode without `--force`? (Architecture line 158 says "defaults to 'no'" but this isn't in the implementation plan)
3. How to actually implement the confirmation prompt (which library? raw stdin read?)

**Why This Is a Blocker**: This is acceptance criteria AC-6 and is cited as a MUST requirement (Req 8). The implementation plan gives NO concrete guidance on how to build this. Looking at the codebase, there's no existing pattern for confirmation prompts — all other commands use `ui.DefaultPicker.Show()` for choices.

**Missing Details**:
1. Should we use `bufio.NewReader(os.Stdin).ReadString('\n')` for the prompt?
2. How do we detect if stdin is a TTY vs piped? (Use `ui.IsInteractive()` or `term.IsTerminal()`?)
3. What's the exact error message when non-interactive without `--force`?
4. Should the prompt be in a separate function for testability?

**Fix Required**:
1. Add new task in Phase 3: "Implement confirmation prompt logic"
2. Specify: Use `bufio.NewReader(os.Stdin).ReadString('\n')` for reading confirmation
3. Specify: Check `ui.IsInteractive()` — if false and no `--force`, return error: `"plan exists; use --force to overwrite in non-interactive mode"`
4. Add test scenario: "TestPlanCommand_ExistingPlanNonInteractiveNoForce_Errors"

---

## SHOULD FIX Findings (Strongly Recommended)

### SF1: Missing `stepDefaultTiers` Test Won't Catch Typos

**Location**: Implementation plan Phase 1 Task 1.1, `internal/config/resolve_test.go`

**Issue**: The plan says:
> "Add `TestResolveStep_PlanDefaultsTier` and `TestResolveStep_PlanOverrideModel` to `internal/config/resolve_test.go`"

But looking at the actual `resolve.go:10-14`, the `stepDefaultTiers` map is package-private. If someone makes a typo (e.g., `"plann": "muster-deep"`), the test won't catch it because it only verifies the BEHAVIOR, not the map entry itself.

**Why This Matters**: The test should verify:
1. The map key is exactly `"plan"` (not `"planning"` or `"planner"`)
2. The tier string is exactly `"muster-deep"` (not `"deep"` or `"muster-deeper"`)

**Current Test Gap**: The specified tests would do:
```go
resolved := ResolveStep("plan", nil, nil)
assert.Equal(t, "claude-opus-4", resolved.Model) // Indirect check
```

This tests the END RESULT, not the map entry. If someone changes the map to `"planning": "muster-deep"`, the test still passes as long as they update the test to call `ResolveStep("planning", ...)`.

**Fix**: Add direct verification:
```go
func TestStepDefaultTiers_ContainsPlanKey(t *testing.T) {
    tier, exists := stepDefaultTiers["plan"]
    require.True(t, exists, "stepDefaultTiers must contain 'plan' key")
    assert.Equal(t, "muster-deep", tier, "plan step must default to muster-deep tier")
}
```

---

### SF2: Phase Dependency Graph Doesn't Account for Test-Driven Development

**Location**: Implementation plan lines 88-100

**Issue**: The plan shows a linear dependency graph:
```
Phase 1 → Phase 2 → Phase 3
```

But the tasks within each phase are described as "ordered but can be developed incrementally (write code + test together)". This is contradictory.

**Why This Matters**: Task 1.5 says "Add structural and slug resolution tests" but it depends on Task 1.2-1.4 code existing. If a developer writes Task 1.2 code and Task 1.5 tests together (true TDD), they can't run tests until Task 1.3 and 1.4 are also done.

**Better Approach**: Specify test-first workflow explicitly:
- Phase 1: Write failing tests (1.5), then implement code (1.2-1.4)
- Phase 2: Write failing tests (2.5), then implement code (2.1-2.4)
- Phase 3: Write failing tests (3.4), then implement code (3.1-3.3)

Or clarify that "incremental" means "write small pieces and test immediately" but tests can't run until the phase is complete.

**Fix**: Add note to Phase 1 description:
> "Tests in Task 1.5 can be written first (TDD) but will fail until Tasks 1.2-1.4 are complete. Alternatively, implement Tasks 1.2-1.4 then add tests in 1.5."

---

### SF3: `resolveSlug` Function Signature Is Underspecified

**Location**: Implementation plan Phase 1 Task 1.3, Architecture doc section 4.1

**Issue**: Architecture line 110 shows:
```go
func resolveSlug(cmd *cobra.Command, args []string, rm *roadmap.Roadmap) (string, *roadmap.RoadmapItem, error)
```

But the implementation plan says (Task 1.3):
> "Create `resolveSlug(cmd, args, rm)` function."

This implies the function takes a `*cobra.Command` for `cmd.ErrOrStderr()` (to print warnings), but then the implementation plan talks about "interactive mode" and "non-interactive mode" WITHOUT specifying how to detect this.

**Missing Details**:
1. Should `resolveSlug` call `ui.IsInteractive()` internally?
2. Should it take an `io.Writer` parameter for warnings instead of `cmd`?
3. How does it access `ui.DefaultPicker.Show()` — directly or via dependency injection?

**Why This Matters**: Testability. If `resolveSlug` calls `ui.DefaultPicker.Show()` directly, you can't test picker behavior without mocking the global `DefaultPicker` variable (which the QA strategy mentions but doesn't detail HOW).

**Fix**: Specify exact signature with io.Writer:
```go
func resolveSlug(args []string, rm *roadmap.Roadmap, interactive bool, w io.Writer, picker ui.Picker) (string, *roadmap.RoadmapItem, error)
```

Pass `ui.IsInteractive()` result from `runPlan`, pass `cmd.ErrOrStderr()` for `w`, and pass `ui.DefaultPicker` (mockable in tests).

---

### SF4: Directory Creation Doesn't Handle Parent Directory Permissions

**Location**: Implementation plan Phase 1 Task 1.4

**Issue**: Task 1.4 says:
> "Use `os.MkdirAll` for `.muster/work/{slug}/plan/`, `research/`, `synthesis/` with `0755` permissions."

But `os.MkdirAll` creates ALL intermediate directories with the SAME permissions. If `.muster/` or `.muster/work/` don't exist, they'll also get `0755`, which might not match the user's umask expectations.

**Why This Matters**: Consistency with existing project structure. If the user's umask is `022` (default), they expect `0755`. But if their umask is `027`, they expect `0750`. Forcing `0755` might create a security issue in restrictive environments.

**Evidence from Codebase**: Looking at `internal/prompt/stage.go:129`, it uses `0755` with a nolint comment:
```go
if err := os.MkdirAll(skillsDir, 0755); err != nil { //nolint:gosec // G301: Standard directory permissions for skill staging
```

So the pattern exists, but the comment justifies it for TEMP directories. Plan directories are PERSISTENT.

**Fix**: Either:
1. Use `os.MkdirAll(planDir, 0755)` and add nolint comment with justification
2. OR create parent directories separately with default permissions, then plan directory with explicit `0755`

Update Task 1.4 to specify: "Use `0755` with `//nolint:gosec // G301: Standard directory permissions for plan storage`"

---

### SF5: Verification Logic Doesn't Check File Contents or Size

**Location**: Implementation plan Phase 2 Task 2.4, Architecture doc section 4.6

**Issue**: Task 2.4 says:
> "After successful invocation, `os.Stat` the expected `implementation-plan.md`. Error if missing."

But this only checks that the FILE EXISTS. It doesn't verify:
1. The file is non-empty (size > 0)
2. The file is readable
3. The file contains valid markdown (at least has `#` or some content)

**Why This Matters**: If the skill crashes or writes a 0-byte file, `os.Stat` will succeed and the command will report success. The user won't discover the problem until they try to read the plan.

**Real-World Scenario**: Skill has a template rendering error, writes empty file, exits 0. Command says "Implementation plan created" but the file is useless.

**Fix**: Update Task 2.4:
```go
planFile := filepath.Join(planDir, "implementation-plan.md")
info, err := os.Stat(planFile)
if os.IsNotExist(err) {
    return fmt.Errorf("planning completed but implementation-plan.md was not created at %s", planFile)
}
if err != nil {
    return fmt.Errorf("failed to verify plan file: %w", err)
}
if info.Size() == 0 {
    return fmt.Errorf("plan file exists but is empty at %s", planFile)
}
```

Add test scenario: "TestPlanCommand_EmptyPlanFile_Errors"

---

### SF6: Warnings Implementation Is Incomplete

**Location**: Implementation plan Phase 3 Task 3.2, Architecture doc section 4.3

**Issue**: Task 3.2 says:
> "Print to stderr in interactive mode: completed item warning, blocked item warning, empty context warning. Format: `"Warning: {condition}"`. Suppress in non-interactive mode."

But this is underspecified:
1. WHERE in the code flow do these warnings print? Before slug resolution? After?
2. The "empty context warning" isn't mentioned anywhere else in the plan (no requirement for it)
3. "Suppress in non-interactive mode" — does this mean check `ui.IsInteractive()` for EACH warning?

**Traceability Gap**: The synthesis doc mentions completed item warning (Req 9) and blocked item warning (Req 15), but "empty context warning" appears ONLY in Task 3.2. Is this a requirement or not?

**Fix Required**:
1. Remove "empty context warning" from Task 3.2 (not a requirement)
2. Specify warning locations:
   - Completed item warning: Print in `resolveSlug` after finding item, before returning
   - Blocked item warning: Print in `runPlan` after slug resolution, before directory creation
3. Specify: Each warning checks `interactive` boolean (passed to function) before printing

---

### SF7: Model Tier Resolution Error Handling Is Missing

**Location**: Implementation plan Phase 1 Task 1.1, Architecture doc section 2.2

**Issue**: The plan says to add `"plan": "muster-deep"` to `stepDefaultTiers`, and Architecture doc lines 128-137 show the resolution logic:

```go
resolvedTier, err := resolveModelTier(tier, *tool, projectCfg, userCfg)
if err == nil {
    model = &resolvedTier
    modelSrc = fmt.Sprintf("step default tier (%s) via user config", tier)
} else {
    // No user tier config — use built-in default for this tier
    defaultModel := concreteModelForTier(tier)
    model = &defaultModel
    modelSrc = fmt.Sprintf("step default tier (%s)", tier)
}
```

But what if `concreteModelForTier` returns empty string because the tier name is invalid? Looking at `resolve.go:19-29`, it has a default case that returns `DefaultModel`, which is fine. But the NEXT call to `resolveModelTier` at line 146 can STILL fail if the model string is a tier reference that doesn't resolve.

**Missing Test Scenario**: What happens if:
1. User sets `pipeline.plan.model_tier: muster-nonexistent`
2. No tier mappings are defined anywhere

The code will call `resolveModelTier("muster-nonexistent", ...)` and get an error at line 212: `return "", fmt.Errorf("unknown model tier %q for tool %q", modelStr, tool)`.

**Current Plan**: Task 1.1 adds tests `TestResolveStep_PlanDefaultsTier` and `TestResolveStep_PlanOverrideModel`, but neither tests the error path for invalid tier override.

**Fix**: Add test scenario:
```go
func TestResolveStep_PlanInvalidTierOverride_Errors(t *testing.T) {
    projectCfg := &config.ProjectConfig{
        Pipeline: map[string]*config.PipelineStepConfig{
            "plan": {ModelTier: strPtr("muster-invalid")},
        },
    }
    _, err := config.ResolveStep("plan", projectCfg, nil)
    require.Error(t, err)
    assert.Contains(t, err.Error(), "unknown model tier")
}
```

---

## NICE TO HAVE Findings (Would Improve Plan)

### N1: Missing Test for Concurrent Plan Invocations

**Location**: QA Strategy doc, missing from 42 scenarios

**Issue**: The implementation plan doesn't test what happens if two users run `muster plan same-slug` simultaneously. Will they:
1. Both create temp directories (fine, different tmpDir names)
2. Both write to `.muster/work/same-slug/plan/implementation-plan.md` (race condition!)
3. Overwrite each other's work?

**Why This Matters**: In a team setting, two developers might run planning for the same item. The command should either:
- Detect another process is running (lock file?)
- Succeed but warn about overwrite
- Fail with clear error

**Recommendation**: Add warning to Known Limitations:
> "No concurrency protection: Multiple simultaneous `muster plan` invocations for the same slug will overwrite each other's outputs. Use branch-based workflows to avoid this."

---

### N2: Verbose Logging Doesn't Show Skill Staging Path

**Location**: Implementation plan Phase 3 Task 3.3, cmd/code.go comparison

**Issue**: Task 3.3 says verbose mode should log:
> "Verbose mode: log resolved config triple and staging path to stderr."

But looking at `cmd/code.go:76-82`, it only logs the config triple, NOT the staging path. The staging path is only shown when `--keep-staged` is used (line 129).

**Inconsistency**: The plan says to show staging path in verbose mode, but the existing pattern in `code` command only shows it with `--keep-staged`. Which is correct?

**Recommendation**: Match the `code` command behavior:
- Verbose: Show config triple only
- If `--keep-staged` flag exists (it doesn't in the plan), show staging path

OR clarify that `plan` command is different because skills run in foreground, so showing tmpDir is useful for debugging.

---

### N3: JSON Output Doesn't Include Absolute Path

**Location**: Implementation plan Phase 3 Task 3.3, Architecture doc section 4.8

**Issue**: Architecture line 249 specifies:
> "The plan path in JSON output uses the relative path (from project root) for portability."

But the PromptContext uses absolute paths everywhere (Req Q4 resolution). Why is JSON output different?

**Potential Issue**: If the user runs `muster plan slug --format json` from a subdirectory, the relative path might be ambiguous. Should it be relative to:
1. Current working directory?
2. Project root?

**Recommendation**: Add both to JSON output:
```json
{
  "slug": "my-feature",
  "plan_path": ".muster/work/my-feature/plan/implementation-plan.md",
  "plan_path_absolute": "/Users/user/project/.muster/work/my-feature/plan/implementation-plan.md"
}
```

This matches patterns in other tools (like `go list -json`) that provide both relative and absolute paths.

---

### N4: Test Scenario 20 (Completed Item Warning) Has Wrong Assertion

**Location**: QA Strategy lines 190-195

**Issue**: Test 20 says:
> "TestPlanCommand_CompletedItemExplicitSlug_ShowsWarning"
> - Assert: Stderr contains "Warning: planning an already-completed item"
> - Assert: Command proceeds (no error)

But Synthesis doc line 87 (Req 9) says:
> "Allow explicit slug argument to bypass filter (user intentionally planning completed item)"
> "Log warning if planning a completed item: 'Warning: planning an already-completed item'"

The test is correct! But the synthesis doc uses "Log warning" which is ambiguous (stdout? stderr?). The test correctly specifies stderr.

**Recommendation**: Update Synthesis doc Req 9 to specify "Print warning to stderr" for clarity.

---

### N5: Missing Test for Invalid Slug Characters

**Location**: QA Strategy, missing from 42 scenarios

**Issue**: None of the test scenarios verify what happens if the user provides a slug with invalid characters (e.g., `muster plan "my slug with spaces"` or `muster plan my/feature`).

**Why This Matters**: The roadmap validation requires valid slug format (kebab-case, max 40 chars), but the plan command doesn't validate the input slug before passing to `FindBySlug`. If the slug contains filesystem-unsafe characters, the directory creation in Task 1.4 might fail with cryptic errors.

**Recommendation**: Add test scenario:
```
43. TestPlanCommand_InvalidSlugCharacters_Errors
   - Setup: Valid roadmap
   - Execute: muster plan "my slug" (with space)
   - Assert: Error mentions invalid slug format
```

And add validation before `FindBySlug`:
```go
if !isValidSlug(args[0]) {
    return fmt.Errorf("invalid slug format: %q (must be kebab-case, max 40 chars)", args[0])
}
```

---

## Traceability Verification

I verified that every MUST requirement from synthesis.md is covered:

| Requirement | Implementation Plan Coverage | Status |
|-------------|------------------------------|--------|
| Req 1 (8-step pattern) | Phase 1 Tasks 1.2-1.5, Phase 2-3 | ✅ Complete |
| Req 2 (dual-mode slug) | Phase 1 Task 1.3 | ✅ Complete |
| Req 3 (skill staging) | Phase 2 Tasks 2.1-2.3 | ✅ Complete |
| Req 4 (output paths) | Phase 1 Task 1.4, Phase 2 Task 2.4 | ✅ Complete |
| Req 5 (PromptContext) | Phase 2 Task 2.1 | ✅ Complete |
| Req 6 (error handling) | All phases, Task 4.7 | ✅ Complete |
| Req 7 (Claude invocation) | Phase 2 Task 2.3 | ⚠️ BLOCKER B1 (--print flag) |
| Req 8 (existing plans) | Phase 3 Task 3.1 | ⚠️ BLOCKER B2 (missing impl) |
| Req 9 (filter completed) | Phase 1 Task 1.3, Phase 3 Task 3.2 | ✅ Complete |
| Req 10 (sort picker) | Phase 1 Task 1.3 | ✅ Complete |
| Req 11 (output summary) | Phase 3 Task 3.3 | ✅ Complete |
| Req 12 (muster-deep) | Phase 1 Task 1.1 | ✅ Complete |

---

## Acceptance Criteria Verification

| AC# | Criterion | Plan Coverage | Status |
|-----|-----------|---------------|--------|
| AC-1 | Invoke with --plugin-dir --model | Phase 2 Task 2.3 | ⚠️ BLOCKER B1 |
| AC-2 | Picker filters completed | Phase 1 Task 1.3 | ✅ Complete |
| AC-3 | Non-TTY without arg errors | Phase 1 Task 1.3 | ✅ Complete |
| AC-4 | Output to .muster/work/{slug}/plan/ | Phase 1 Task 1.4 | ✅ Complete |
| AC-5 | Plan defaults to muster-deep | Phase 1 Task 1.1 | ✅ Complete |
| AC-6 | Existing plan confirmation | Phase 3 Task 3.1 | ⚠️ BLOCKER B2 |
| AC-7 | JSON output format | Phase 3 Task 3.3 | ✅ Complete |
| AC-8 | Error categorization | All phases | ✅ Complete |
| AC-9 | Cleanup on success and error | Phase 2 Task 2.2 | ✅ Complete |
| AC-10 | Read-only roadmap | Phase 1 Task 1.2 | ✅ Complete |
| AC-11 | Cross-platform paths | All phases | ✅ Complete |
| AC-12 | All gates pass | Phase 3 Task 3.5 | ✅ Complete |

---

## Code Pattern Verification

I verified implementation patterns against actual codebase:

✅ **Correct Patterns**:
- `stepDefaultTiers` map usage matches `resolve.go:10-14`
- `vcsFactory` pattern matches `cmd/out.go:19-21` for testability
- `StageSkills` + `defer cleanup()` matches `cmd/code.go:107-120`
- `PromptContext` construction matches `cmd/code.go:95-104`
- `ToolEnvOverrides` application matches `cmd/code.go:145-151`
- `FindBySlug` usage matches `cmd/out.go:103`
- `ui.DefaultPicker.Show()` matches `internal/ui/picker.go:41`

⚠️ **Incorrect Patterns**:
- `--print` flag usage does NOT match existing patterns (BLOCKER B1)
- Confirmation prompt has NO existing pattern to follow (BLOCKER B2)

---

## Recommendations for Plan Fixes

### Critical (Must Fix):
1. **Remove `--print` flag** from all invocation logic (BLOCKER B1)
2. **Add concrete implementation** for `--force` flag and confirmation prompt (BLOCKER B2)
3. **Add file size check** to verification logic (SF5)
4. **Clarify warning locations** and remove "empty context warning" (SF6)

### Important (Should Fix):
5. **Add direct `stepDefaultTiers` map test** to catch typos (SF1)
6. **Specify test-driven development workflow** in phase descriptions (SF2)
7. **Define `resolveSlug` signature** with explicit parameters (SF3)
8. **Add nolint comment** for directory permissions (SF4)
9. **Add error path test** for invalid tier override (SF7)

### Nice to Have:
10. **Add concurrency warning** to Known Limitations (N1)
11. **Clarify verbose logging** behavior vs `code` command (N2)
12. **Add absolute path** to JSON output (N3)
13. **Add invalid slug test** scenario (N5)

---

## Final Verdict

**Status**: ⚠️ **NOT READY FOR EXECUTION**

The plan has strong architecture and good test coverage, but **2 BLOCKER issues must be resolved** before implementation can proceed:
1. The `--print` flag assumption is fundamentally wrong
2. The `--force` flag implementation is completely missing

Once these blockers are addressed and the 7 SHOULD FIX issues are resolved, the plan will be ready for execution. The underlying architecture is sound — the issues are in the details.

**Estimated Fix Time**: 2-3 hours to address blockers and critical fixes, 4-6 hours total to address all SHOULD FIX items.
