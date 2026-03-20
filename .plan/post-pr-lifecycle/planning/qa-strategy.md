# Post-PR Lifecycle (muster out): QA Strategy

*Version: 2026-03-20*

## Test Layers

### Unit Tests
Pure logic tests with no external tool invocation. Cover config validation, data transformations, argument parsing, error categorization, and interface contract verification. These run fast, require no `testing.Short()` guard, and execute on every platform without prerequisites.

### Integration Tests
Tests that invoke real `git` commands against temp repos or shell out to mock binaries. Guarded with `testing.Short()` to allow CI to skip them when speed matters. Cover worktree creation/removal, branch operations, and end-to-end command execution with mocked VCS and AI tools.

The codebase does **not** have a separate end-to-end test layer. The integration tests in `cmd/*_test.go` serve that role by exercising `RunE` with temp directories, mock tools, and real file I/O.

## Package Test Plans

### internal/git/

Tests use **real temp git repos** (matching `internal/docker/worktree_test.go` patterns). Every test creates a repo via `t.TempDir()` + `git init` + initial commit. No git mocking -- the design doc mandates real git for this layer.

#### Shared helper: `testutil.InitGitRepo`

```go
// testutil.InitGitRepo initializes a git repo in dir with an initial commit.
// Returns the repo directory (same as dir). Fails the test on error.
func InitGitRepo(t *testing.T, dir string) string
```

This replaces the duplicated 10-line `git init`/`git config`/`git add`/`git commit` setup found in `internal/docker/worktree_test.go` and will be reused extensively.

#### Test functions

| Function | Scenario | Approach |
|---|---|---|
| `TestRunGit_Success` | Runs `git rev-parse --show-toplevel` in a valid repo | Real temp repo, verify non-empty output |
| `TestRunGit_InvalidCommand` | Runs `git not-a-command` | Real temp repo, verify error returned |
| `TestRunGit_NonGitDirectory` | Runs any git command in a plain temp dir | `t.TempDir()` with no init, verify error |
| `TestRunGit_SetsDir` | Verifies `cmd.Dir` is set, not relying on cwd | Create repo in subdir, run from parent, verify success |
| `TestCurrentBranch_MainBranch` | Gets branch name in freshly init'd repo | Real repo, assert `"main"` or `"master"` (platform-dependent default) |
| `TestCurrentBranch_FeatureBranch` | Creates and checks out a feature branch | Real repo, `git checkout -b feature`, assert `"feature"` |
| `TestCurrentBranch_NotGitRepo` | Calls `CurrentBranch` on a non-git dir | Verify descriptive error |
| `TestPullLatest_Success` | Pulls from a local bare remote | Create bare repo + clone, push a commit, pull, verify file exists |
| `TestPullLatest_NoRemote` | Pulls with invalid remote name | Verify error includes remote name |
| `TestRemoveWorktree_Success` | Removes a worktree created via `git worktree add` | Create main repo + worktree, remove, verify directory gone |
| `TestRemoveWorktree_NonExistent` | Removes a worktree path that doesn't exist | Verify error |
| `TestRemoveWorktree_DirtyWorktree` | Removes a worktree with uncommitted changes | Verify error (git refuses without `--force`) |
| `TestDeleteBranch_MergedBranch` | Deletes a branch that has been merged | Create branch, merge to main, delete, verify `git branch` output |
| `TestDeleteBranch_UnmergedBranch` | Deletes an unmerged branch with `git branch -d` | Verify error (safety check: `-d` not `-D`) |
| `TestDeleteBranch_CurrentBranch` | Deletes the currently checked-out branch | Verify error |

All integration tests are guarded with:
```go
if testing.Short() {
    t.Skip("skipping integration test in short mode")
}
```

### internal/vcs/

Tests use **mocked exec.Command** -- no real `gh` or `glab` invocation. The VCS package wraps CLI tools; we test the wrapping logic (argument construction, JSON parsing, error handling) not the tools themselves.

#### Mocking strategy: command runner injection

The VCS implementations accept a `CommandRunner` function (or use a package-level variable like `ai.InvokeAI`):

```go
// CommandRunner executes a command and returns stdout, stderr, and error.
type CommandRunner func(dir string, name string, args ...string) (stdout string, stderr string, err error)
```

In production, this calls `exec.Command`. In tests, it returns canned JSON responses or errors. This avoids requiring `gh`/`glab` to be installed in CI.

#### Test functions -- GitHub VCS

| Function | Scenario | Mock returns |
|---|---|---|
| `TestGitHubVCS_ViewPR_Success` | Parse `gh pr view --json` output | Valid JSON with `state`, `mergeable`, `reviewDecision` fields |
| `TestGitHubVCS_ViewPR_NotFound` | PR does not exist | stderr: `"Could not resolve to a PullRequest"`, exit 1 |
| `TestGitHubVCS_ViewPR_AuthFailure` | Not authenticated | stderr: `"gh auth login"`, exit 4 |
| `TestGitHubVCS_ListChecks_AllPassing` | All CI checks pass | JSON array with `state: "SUCCESS"` for each check |
| `TestGitHubVCS_ListChecks_SomeFailing` | Mixed pass/fail | JSON array with mixed states, verify correct `CheckResult` mapping |
| `TestGitHubVCS_ListChecks_Pending` | Checks still running | JSON with `state: "PENDING"` entries |
| `TestGitHubVCS_ListChecks_NoChecks` | PR has no CI checks | Empty JSON array, verify graceful handling |
| `TestGitHubVCS_MergePR_Success` | Merge succeeds | Empty stdout, exit 0 |
| `TestGitHubVCS_MergePR_ConflictFailure` | Merge blocked by conflicts | stderr with merge conflict message, exit 1 |
| `TestGitHubVCS_MergePR_ReviewRequired` | Merge blocked by review | stderr with review required message, exit 1 |
| `TestGitHubVCS_CheckAuth_Success` | `gh auth status` exits 0 | exit 0 |
| `TestGitHubVCS_CheckAuth_NotLoggedIn` | `gh auth status` exits non-zero | exit 1, verify actionable error message |
| `TestGitHubVCS_CommandNotFound` | `gh` binary not in PATH | `exec.ErrNotFound`, verify install guidance in error |

#### Test functions -- GitLab VCS

Mirror the GitHub tests above with GitLab-specific JSON schemas and flag names:

| Function | Scenario |
|---|---|
| `TestGitLabVCS_ViewMR_Success` | Parse `glab mr view --output json` |
| `TestGitLabVCS_ViewMR_NotFound` | MR does not exist |
| `TestGitLabVCS_ListPipelines_AllPassing` | All pipeline jobs pass |
| `TestGitLabVCS_ListPipelines_SomeFailing` | Mixed job results |
| `TestGitLabVCS_MergeMR_Success` | Merge succeeds |
| `TestGitLabVCS_MergeMR_PipelineBlocked` | Merge blocked by failing pipeline |
| `TestGitLabVCS_CheckAuth_NotLoggedIn` | `glab auth status` fails |
| `TestGitLabVCS_CommandNotFound` | `glab` binary not in PATH |

#### Interface contract tests

```go
func TestVCSInterface_Implementations(t *testing.T) {
    // Verify both GitHubVCS and GitLabVCS satisfy the VCS interface at compile time.
    var _ VCS = &GitHubVCS{}
    var _ VCS = &GitLabVCS{}
}
```

### cmd/out.go

Tests follow the exact patterns from `cmd/code_test.go` and `cmd/sync_test.go`: fresh `cobra.Command` instances, `t.TempDir()` working directories, `os.Chdir` with deferred restore, and `testutil.MockInvokeAI` for AI mock.

#### Structural tests

| Function | What it verifies |
|---|---|
| `TestOutCommand_Exists` | `outCmd` is non-nil, `Use` is `"out [slug]"` |
| `TestOutCommand_IsAddedToRootCommand` | `rootCmd.Commands()` contains `"out"` |
| `TestOutCommand_HasRunEFunction` | `outCmd.RunE` is non-nil |
| `TestOutCommand_RequiresExactlyOneArg` | `cobra.ExactArgs(1)` -- test with 0 args (error) and 2 args (error) |
| `TestOutCommand_HasExpectedFlags` | `--no-fix` (bool, default false), `--wait` (bool), `--timeout` (duration, default 30m), `--dry-run` (bool) |
| `TestOutCommand_AllFlags_HaveUsageText` | Every flag has non-empty `Usage` |
| `TestOutCommand_ShortDescription_IsConcise` | `len(Short) < 100` |
| `TestOutCommand_WithHelpFlag_Succeeds` | `--help` returns no error, output contains flag names |

#### Config and roadmap loading tests

| Function | Scenario |
|---|---|
| `TestOutCommand_ConfigLoadError_MalformedYAML` | Malformed `.muster/config.yml`, verify `"config file malformed"` in error |
| `TestOutCommand_RoadmapLoadError_MalformedJSON` | Malformed `.muster/roadmap.json`, verify `"roadmap file is malformed"` in error |
| `TestOutCommand_SlugNotFound` | Valid roadmap but slug doesn't match any item, verify descriptive error |
| `TestOutCommand_NoPRUrl_NoBranch` | Roadmap item has neither `pr_url` nor `branch`, verify actionable error |

#### Merge strategy tests

| Function | Scenario |
|---|---|
| `TestOutCommand_DirectMergeStrategy_ExitsCleanly` | Config has `merge_strategy: direct`, verify exit 0 with explanatory message |
| `TestOutCommand_GitHubPR_Strategy` | Config has `merge_strategy: github-pr`, verify VCS operations are attempted |
| `TestOutCommand_GitLabMR_Strategy` | Config has `merge_strategy: gitlab-mr`, verify VCS operations are attempted |
| `TestOutCommand_DefaultMergeStrategy` | No `merge_strategy` in config, verify defaults to `github-pr` |

#### Dry-run tests

| Function | Scenario |
|---|---|
| `TestOutCommand_DryRun_NoSideEffects` | `--dry-run` flag set, verify roadmap file unchanged, no git operations, no VCS calls |
| `TestOutCommand_DryRun_PrintsPlannedActions` | Verify output describes what would happen |

#### AI CI fix loop tests

| Function | Scenario |
|---|---|
| `TestOutCommand_CIFix_InvokedOnFailure` | CI checks fail, AI is invoked, verify `testutil.GetInvokeCount() >= 1` |
| `TestOutCommand_CIFix_NoFixFlag_SkipsAI` | `--no-fix` set, CI checks fail, verify `GetInvokeCount() == 0` |
| `TestOutCommand_CIFix_MaxRetriesRespected` | CI keeps failing, verify AI invoked at most 3 times (default max) |
| `TestOutCommand_CIFix_SuccessAfterRetry` | First CI fails, AI fixes, second CI passes, verify item marked completed |

#### Cleanup tests

| Function | Scenario |
|---|---|
| `TestOutCommand_Cleanup_CollectsAllErrors` | Multiple cleanup steps fail (worktree remove, branch delete), verify all errors reported together |
| `TestOutCommand_Cleanup_MarksCompleted` | After merge, roadmap item status is `"completed"` even if cleanup partially fails |
| `TestOutCommand_Cleanup_DryRunSkipsCleanup` | `--dry-run` does not modify roadmap or call git cleanup |

### Config Changes (merge_strategy)

Tests go in `internal/config/config_test.go` and `internal/config/project_test.go`, following existing validation patterns.

| Function | Scenario |
|---|---|
| `TestProjectConfig_MergeStrategy_ValidValues` | Each of `direct`, `github-pr`, `gitlab-mr` passes validation |
| `TestProjectConfig_MergeStrategy_InvalidValue` | `merge_strategy: foobar` fails validation with descriptive error |
| `TestProjectConfig_MergeStrategy_NilDefault` | When omitted, `ResolveMergeStrategy()` returns `"github-pr"` |
| `TestProjectConfig_MergeStrategy_YAMLRoundTrip` | Marshal/unmarshal preserves the value |
| `TestProjectConfig_MergeStrategy_MergeOverride` | `config.local.yml` overrides `config.yml` merge strategy |
| `TestProjectConfig_MergeStrategy_NotInUserConfig` | Verify user config cannot set `merge_strategy` (it's project-only) |
| `TestResolveStep_OutDefaultTier` | `ResolveStep("out", ...)` defaults to `muster-standard` tier |

## Mocking Strategy

### External tool mocking

There are three categories of external tools to mock, each with a different approach:

**1. AI tool (claude)** -- Use existing `testutil.MockInvokeAI()` for unit tests and `testutil.NewMockAITool()` for integration tests. Both are already established patterns. The CI fix loop will invoke `ai.InvokeAI`, which is a replaceable package-level variable.

**2. VCS tools (gh, glab)** -- Use `CommandRunner` function injection on the VCS struct:

```go
type GitHubVCS struct {
    Run CommandRunner // injected; defaults to execCommandRunner in production
    Dir string
}
```

In tests, replace `Run` with a closure that returns canned JSON/errors:

```go
vcs := &GitHubVCS{
    Run: func(dir, name string, args ...string) (string, string, error) {
        return `{"state":"MERGED"}`, "", nil
    },
}
```

This avoids needing `gh`/`glab` installed in CI. It also avoids the fragility of `exec.Command` test process patterns (TestMain/subprocess approach) which the codebase does not use.

**3. Git operations (internal/git/)** -- At the `cmd/out.go` level, the git package functions are called directly. To mock them in command tests, inject the git operations as an interface or function fields on a struct passed to the RunE helpers. Alternatively, since the existing command tests (`cmd/sync_test.go`, `cmd/add_test.go`) use real file I/O with temp dirs, command-level tests for `out` can use the same approach with real temp git repos for the git layer and mocked VCS/AI layers.

### Mock boundary diagram

```
cmd/out.go (integration tests)
  |
  |-- config loading: real files in t.TempDir()
  |-- roadmap loading: real files in t.TempDir()
  |-- internal/git/*: real temp git repos OR injected mock
  |-- internal/vcs/*: injected CommandRunner mock (no real gh/glab)
  |-- ai.InvokeAI: testutil.MockInvokeAI() or MockInvokeAIWithQueue()
```

## Cross-Platform Considerations

### Path handling
- All `internal/git/` functions accept `dir string` and use `filepath.Join`/`filepath.Abs`. Never concatenate paths with `/`.
- Tests verify returned paths with `filepath.IsAbs()` (matching `docker/worktree_test.go` pattern).
- Roadmap file paths use `filepath.Join(dir, ".muster", "roadmap.json")`.

### Line endings
- `.gitattributes` enforces LF. Tests that compare file content should not be sensitive to `\r\n` vs `\n`. Use `strings.TrimSpace()` on git command output (already done in existing `runGitCommand`).
- Golden file tests (if any are added for prompt templates) use `AssertGoldenFile` which reads raw bytes.

### Git default branch name
- `git init` creates `master` on older git versions and `main` on newer ones. Tests that check branch names should accept either, or explicitly set the branch: `git init -b main`.
- Recommendation: Use `git init -b main` in `testutil.InitGitRepo` so tests are deterministic across platforms.

### Binary names
- `exec.LookPath("gh")` works on all platforms. On Windows, `gh.exe` is found automatically.
- The `exec.ErrNotFound` check in error handling works cross-platform.

### Temp directory cleanup
- `t.TempDir()` handles cleanup automatically. Git worktrees created inside temp dirs may leave `.git` lockfiles on Windows; tests should use `git worktree remove` before `t.TempDir()` cleanup runs, or accept that Windows may need a short delay.

### exec.Command without shell
- All commands invoke binaries directly (`exec.Command("git", ...)`, `exec.Command("gh", ...)`). No `sh -c` or `cmd /c` wrappers. This is already the codebase convention and works cross-platform.

## Error Scenarios

### internal/git/ error matrix

| Operation | Error Condition | Expected Behavior |
|---|---|---|
| `RunGit` | Not a git repo | Return error with "not a git repository" context |
| `RunGit` | Invalid git subcommand | Return error with git stderr |
| `RunGit` | Dir does not exist | Return error before exec |
| `CurrentBranch` | Detached HEAD | Return error with "detached HEAD" context |
| `PullLatest` | Remote unreachable | Return error with remote name |
| `PullLatest` | Merge conflict | Return error with conflict indication |
| `RemoveWorktree` | Path not a worktree | Return error from git |
| `RemoveWorktree` | Uncommitted changes | Return error (no `--force`) |
| `DeleteBranch` | Branch not merged | Return error (uses `-d` not `-D`) |
| `DeleteBranch` | Branch is current | Return error |
| `DeleteBranch` | Branch does not exist | Return error |

### internal/vcs/ error matrix

| Operation | Error Condition | Expected Behavior |
|---|---|---|
| `CheckAuth` | CLI not installed | Return error with install URL |
| `CheckAuth` | Not authenticated | Return error with `"run 'gh auth login'"` / `"run 'glab auth login'"` |
| `ViewPR` | PR/MR not found | Return error with PR ref |
| `ViewPR` | JSON parse failure | Return error wrapping parse error + raw output |
| `ViewPR` | Network timeout | Return error from exec timeout |
| `ListChecks` | No checks configured | Return empty slice, no error |
| `ListChecks` | Partial JSON response | Return error wrapping parse error |
| `MergePR` | Merge conflicts | Return error with conflict context |
| `MergePR` | Review not approved | Return error indicating review status |
| `MergePR` | Branch protection violation | Return error with protection rule context |

### cmd/out.go error matrix

| Phase | Error Condition | Expected Behavior |
|---|---|---|
| Config loading | Malformed YAML | `"config file malformed: ..."` |
| Config loading | Missing config file | Proceed with defaults |
| Roadmap loading | Malformed JSON | `"roadmap file is malformed: ..."` |
| Roadmap loading | File not found | `"failed to load roadmap: ..."` |
| Slug lookup | Slug not in roadmap | `"roadmap item not found: <slug>"` |
| PR discovery | No `pr_url` or `branch` | Actionable error suggesting `muster status` |
| Auth check | gh/glab not installed | Error with install URL |
| Auth check | Not authenticated | Error with login command |
| CI polling | Timeout exceeded | Exit with timeout error, no cleanup |
| CI polling | Network failure mid-poll | Retry with backoff, then error |
| AI CI fix | AI invocation fails | Log warning, continue without fix |
| AI CI fix | Max retries exceeded | Exit with "CI checks still failing after 3 attempts" |
| Merge | Review changes requested | Exit with review status message |
| Cleanup: worktree remove | Git error | Collect error, continue to next step |
| Cleanup: branch delete | Branch not merged | Collect error, continue |
| Cleanup: roadmap save | File write failure | Collect error, report |
| Cleanup | Multiple failures | Report all collected errors, non-zero exit |

## Quality Gates

All of the following must pass before the feature is considered done:

1. **`make test` passes on Linux, macOS, and Windows** -- CI runs `go test -v -race ./...` on all three. The `-race` flag catches data races in mock replacement and concurrent access.

2. **`make lint` passes** -- `golangci-lint run --timeout=5m` with the project's existing configuration. All `//nolint:gosec` annotations have inline justification comments.

3. **`make build` produces a working binary** -- `CGO_ENABLED=0 go build` succeeds. The binary runs `muster out --help` without error.

4. **Unit test coverage for new packages**:
   - `internal/git/`: every exported function has at least one success and one failure test.
   - `internal/vcs/`: every `VCS` interface method has tests for success, not-found, auth failure, and parse error on both GitHub and GitLab implementations.
   - `internal/config/`: merge_strategy validation covers valid values, invalid values, nil default, and merge behavior.

5. **Command-level test coverage for cmd/out.go**:
   - Structural tests: exists, registered, flags, args validation.
   - Config/roadmap error categorization tests.
   - Merge strategy routing tests (direct exits cleanly, github-pr/gitlab-mr proceed).
   - Dry-run does not mutate state.
   - AI CI fix loop respects `--no-fix` and max retries.
   - Cleanup collects errors and marks item completed.

6. **No flaky tests** -- Tests that depend on timing (polling, timeouts) use controlled mocks with deterministic behavior, not real sleeps. Integration tests with real git repos use `t.TempDir()` for isolation.

7. **Cross-platform CI green** -- The GitHub Actions workflow runs tests on `ubuntu-latest`, `macos-latest`, and `windows-latest`. All three must pass.

8. **Error messages are actionable** -- Every error path includes context about what went wrong and, where applicable, what the user should do (e.g., "run 'gh auth login'", "install gh from https://cli.github.com").
