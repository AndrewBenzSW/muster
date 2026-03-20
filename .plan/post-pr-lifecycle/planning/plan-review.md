# Post-PR Lifecycle: Plan Review

*Reviewed: 2026-03-20*

## Summary

The plan is well-structured and covers the vast majority of requirements from the synthesis document. It is nearly ready to execute, but has two issues that would cause confusion or build failures during implementation (the `--force` flag contradiction and the `Validate()` receiver mismatch), plus several underspecified tasks that would slow down an implementer. After addressing the blockers and the higher-priority "should fix" items, the plan can be executed confidently.

## Requirement Coverage

| # | MUST Requirement | Plan Coverage | Notes |
|---|-----------------|---------------|-------|
| MUST-1 | `merge_strategy` on `ProjectConfig` | Tasks 1.1, 1.2, 1.4 | Fully covered |
| MUST-2 | `internal/git/` package | Tasks 1.6, 1.7, 1.8, 1.9 | Fully covered |
| MUST-3 | `internal/vcs/` package with VCS interface | Tasks 2.1-2.7 | Fully covered |
| MUST-4 | `cmd/out.go` Cobra command | Tasks 3.1-3.8 | Fully covered |
| MUST-5 | CI status monitoring with muster-controlled poll | Task 3.4 | Covered |
| MUST-6 | PR URL discovery from roadmap item | Task 3.3 | Covered |
| MUST-7 | Worktree and roadmap cleanup after merge | Task 3.6 | Covered |
| MUST-8 | Early exit for `merge_strategy: direct` | Task 3.2 | Covered |
| MUST-9 | Auth pre-check before VCS operations | Task 3.2 (implied) | See SHOULD FIX #1 |
| MUST-10 | Follow established error handling patterns | Tasks 3.2, 3.8, 5.2 | Covered |
| MUST-11 | Cross-platform correctness | Task 5.4 | Covered |
| SHOULD-1 | AI CI fix loop | Tasks 4.1-4.5 | Covered |
| SHOULD-2 | Review status awareness | Task 3.5 | Covered |
| SHOULD-3 | Configurable poll interval | Known Limitation #7 | Deferred, acceptable |
| SHOULD-4 | Verbose logging | Not explicitly tasked | See SHOULD FIX #6 |
| SHOULD-5 | `testutil.InitGitRepo` helper | Task 1.5 | Covered |
| SHOULD-6 | Register `out` in `stepDefaultTiers` | Task 1.3 | Covered |

## Findings

### BLOCKER

**B1. Contradiction on `--force` flag for `RemoveWorktree` between architecture and plan.**

The architecture document (`architecture.md` line 66) specifies `RemoveWorktree` as "Calls git worktree remove --force", while the implementation plan (task 1.8) says simply "Calls `RunGit(repoDir, "worktree", "remove", worktreePath)`" with no `--force`. The QA strategy then includes `TestRemoveWorktree_DirtyWorktree` expecting an error because "git refuses without `--force`".

This is contradictory and will confuse the implementer. After a successful merge, the worktree should be clean, so `--force` is arguably unnecessary but safer. However, the QA strategy explicitly tests the no-force behavior, while the architecture document says to use force.

**Fix**: Decide one way. Recommended: use `--force` in `RemoveWorktree` (matching architecture) since the PR is already merged and the worktree is disposable. Update the QA strategy to remove the `TestRemoveWorktree_DirtyWorktree` error expectation or change it to test that dirty worktrees ARE removed with force. Alternatively, keep no-force and update the architecture doc.

**B2. `Validate()` is on `*Config`, not `*ProjectConfig` -- task 1.1 adds validation in the wrong place.**

Task 1.1 says "Add validation in `Validate()` that rejects unknown values." The existing `Validate()` method is on `*Config` (the top-level struct containing both `User` and `Project`), not on `ProjectConfig` directly. The plan's description implies adding a check inside `ProjectConfig.Validate()`, which does not exist. The implementer needs to know they are modifying `func (c *Config) Validate() []error` in `config.go` at line 298 and must access `c.Project.MergeStrategy`.

**Fix**: Clarify task 1.1 to specify that the merge strategy validation goes inside the existing `func (c *Config) Validate() []error` method, accessing `projectCfg.MergeStrategy` (which is already extracted at line 306 of the current code).

### SHOULD FIX

**S1. Task 3.2 does not explicitly mention the auth pre-check step (MUST-9).**

The `runOut` setup in task 3.2 mentions creating the VCS client but does not list calling `vcsClient.CheckAuth()`. The architecture document (Step 3) clearly shows this as a separate step after VCS creation and before any VCS operations. The auth pre-check is a MUST requirement (MUST-9) and needs to be called somewhere before `discoverPR` uses the VCS client.

**Fix**: Add an explicit step in task 3.2 (or create a sub-step) that calls `vcsClient.CheckAuth()` after creating the VCS client and before PR discovery. Also add a test case in task 3.8 for auth failure (the QA strategy already includes this).

**S2. Task 2.2 says "extract run ID from check URL" for `GetFailedLogs` but does not specify how.**

The `GetFailedLogs` implementation for GitHub requires extracting a run ID from the check's link URL to call `gh run view <runID> --log-failed`. The plan does not specify the URL format or the extraction logic. GitHub check URLs look like `https://github.com/owner/repo/actions/runs/12345/job/67890` -- the run ID is the path segment after `/runs/`. If the URL format is different (e.g., for third-party checks that don't use GitHub Actions), this parsing will fail silently.

**Fix**: Specify in task 2.2 the expected URL format and the extraction approach (e.g., regex or path splitting). Add a note that non-GitHub-Actions checks (like third-party status checks) may not have extractable run IDs, and `GetFailedLogs` should return a partial result or skip those checks gracefully.

**S3. Task 2.3 (GitLab `GetFailedLogs`) is extremely vague.**

The plan says `GetFailedLogs(ref)` runs `glab ci view <jobID> --output json` for failed jobs, but does not explain how to get the job IDs from the pipeline, what JSON schema to expect, or how to extract the actual log content. The QA strategy (task 2.6) does not even include a `GetFailedLogs` test for GitLab. This contrasts with GitHub where `gh run view --log-failed` is well-documented.

**Fix**: Either (a) specify the two-step process (`glab ci list` to get pipeline/job IDs, then `glab ci view` to get logs), or (b) explicitly mark GitLab `GetFailedLogs` as best-effort/stub that returns an error or empty result, and add a test for this behavior.

**S4. Task 3.4 (`monitorCI`) does not specify behavior when `--wait` is NOT set.**

The architecture document (Step 6: Wait for Merge) says "Without `--wait`, the command checks the current state once: if merged, proceed to cleanup; if still open with CI passing, print a message and exit." However, task 3.4 describes the CI poll loop without clarifying that without `--wait`, CI monitoring should also be a single check rather than a polling loop. The interaction between `--wait`, CI monitoring, and merge waiting is ambiguous.

**Fix**: Clarify task 3.4 to specify: without `--wait`, check CI once. If passing, check merge status once. If merged, proceed to cleanup. If not merged, print status and exit 0. With `--wait`, poll both CI and merge status until completion or timeout.

**S5. No task covers the case where `item.Status == "completed"` (already done).**

Task 3.2 mentions "Error if not found or already completed", but there is no acceptance criterion or test case for this. The QA strategy and AC list both omit testing for the "item already completed" path.

**Fix**: Add a test case to task 3.8 (e.g., `TestOutCommand_AlreadyCompleted_ReturnsError`) and an acceptance criterion verifying that `muster out completed-slug` returns a descriptive error.

**S6. SHOULD-4 (verbose logging) is not covered by any task.**

The synthesis lists verbose logging as a SHOULD requirement, but no task in any phase addresses it. The existing commands use `--verbose` for detailed logging. While the `out` command could omit verbose initially, this should be explicitly deferred or included.

**Fix**: Either add a task to wire `--verbose` flag and logging through the command (following the pattern in `cmd/code.go`), or add a note in Known Limitations that verbose logging is deferred.

**S7. Task 4.3 (`fixCI`) calls `ai.InvokeAI` but the prompt rendering approach is unclear.**

Task 4.3 says "render prompt template with logs and attempt number, invoke AI via `ai.InvokeAI`." However, `ai.InvokeAI` takes a `Prompt` string (the raw prompt content), while the architecture document shows using `prompt.RenderTemplate()`. The plan does not specify which rendering function to use. The existing `internal/prompt/` package uses `//go:embed` and `sync.Once` for template parsing, which means adding a new template requires updating the embed directive and potentially the template registry.

**Fix**: Specify in task 4.1 or 4.3 exactly how the template is registered (added to the `//go:embed` directive in the prompt package), and in task 4.3 which function renders it (e.g., `prompt.RenderTemplate` or a new helper). Check if a `RenderTemplate` function exists that can be called standalone outside of `StageSkills`.

**S8. The plan does not specify how `cmd/out.go` tests mock the VCS layer at the command level.**

Task 3.8 says "mock VCS via interface" but does not explain the mechanism. Since `cmd/out.go` creates the VCS client via `vcs.New(strategy, dir)`, the test needs a way to inject a mock. Unlike `ai.InvokeAI` which is a replaceable package-level variable, `vcs.New` is a regular function. The test would need either: (a) a package-level `var NewVCS = vcs.New` to replace in tests, (b) dependency injection via a struct, or (c) passing a factory function to `runOut`.

**Fix**: Specify the injection mechanism in task 3.1 or 3.2. Recommended: add a package-level `var vcsFactory = vcs.New` in `cmd/out.go` that tests can replace, matching the `ai.InvokeAI` pattern.

### NICE TO HAVE

**N1. Tasks 1.7 and the architecture disagree on `PullLatest` return type.**

Task 1.7 defines `PullLatest(dir, remote, branch)` with no explicit return type listed, while the architecture shows it returning just `error`. The implementation plan says `PullLatest`: calls `RunGit(dir, "pull", remote, branch)` -- but `RunGit` returns `(string, error)`. The stdout from `git pull` is typically informational. The task should clarify whether `PullLatest` returns `(string, error)` or just `error` (discarding stdout).

**N2. Task 5.5 (update CLAUDE.md) could be done earlier.**

Updating CLAUDE.md is listed in Phase 5 but could be done at the end of Phase 1 or 2 since the package structure is finalized at that point. This would help any parallel work that references CLAUDE.md.

**N3. Integration test in task 5.1 could test the full lifecycle more thoroughly.**

The integration test mocks VCS to return "passing CI then merged state" but does not verify that the correct VCS methods were called in the expected order (auth check, list checks, view PR). Adding call-order assertions would catch orchestration bugs.

**N4. No task creates a `SaveRoadmap` test for the cleanup path.**

The cleanup function calls `roadmap.SaveRoadmap(projectDir, rm)`, but no test verifies the roadmap file is actually written correctly after status mutation. The command-level test checks `item.Status == "completed"` in memory, but not on disk.

## Dependency Analysis

The phase dependency chain (1 -> 2 -> 3 -> 4 -> 5) is correct and the plan correctly notes that Phases 1 and 2 could be done quickly in sequence since Phase 2 only needs constants from 1.1.

However, there is a missed parallelization opportunity: Phase 4 (AI CI Fix Loop) tasks 4.1 and 4.2 (template creation and golden file test) have no dependency on Phase 3. They only depend on having the `PromptContext` and `FailedCheckLog` types available, which are defined in Phase 2 (task 2.1). Tasks 4.1 and 4.2 could run in parallel with Phase 3, leaving only tasks 4.3-4.5 (wiring the fix loop into the command) dependent on Phase 3.

The `internal/vcs/` factory (task 2.4) imports `config.MergeStrategy*` constants from Phase 1, which is correctly noted. No circular dependency issues.

## Risk Assessment

**Risk 1: `gh pr checks --json` field names and schema may vary across `gh` versions.**
The plan relies on `bucket`, `name`, `state`, `link` fields from `gh pr checks --json`. If `gh` changes field names between versions (as has happened historically), the parsing will break silently. Mitigation: pin to a minimum `gh` version in docs, or detect and report schema mismatches gracefully.

**Risk 2: GitLab CI log retrieval is acknowledged as "best-effort" but no fallback is defined.**
If `glab ci view` fails or returns unexpected output, the AI fix loop for GitLab users would be completely non-functional. Mitigation: define the fallback behavior explicitly -- either skip AI fixing and report the limitation, or pass a generic "CI failed, please investigate" prompt without logs.

**Risk 3: The 30-second poll interval could cause test slowness if not mocked.**
Task 3.4 describes a 30s poll interval. If any test accidentally uses real timing instead of mocked immediate responses, CI will be extremely slow. Mitigation: ensure the poll interval is a parameter (or constant that can be overridden) in the `monitorCI` function signature, not buried in the implementation.

**Risk 4: `testing.Short()` guard direction may be inverted.**
The plan says integration tests are "guarded with `testing.Short()`" to skip in short mode. However, `go test -v -race ./...` (the Makefile command) does NOT pass `-short`, so these tests will always run in CI. This is fine for correctness, but the plan's language "to allow CI to skip them when speed matters" in the QA doc is misleading. The tests will always run in the standard `make test` flow.

**Risk 5: Windows worktree cleanup race with `t.TempDir()`.**
The QA strategy acknowledges this (line 252) but provides no concrete mitigation strategy for the plan. If worktree tests fail intermittently on Windows CI due to lockfiles, it could block the entire feature. Mitigation: add explicit `git worktree remove` calls in test cleanup (before `t.TempDir()` auto-cleanup) as the QA strategy suggests, and note this in the relevant test tasks (1.9).
