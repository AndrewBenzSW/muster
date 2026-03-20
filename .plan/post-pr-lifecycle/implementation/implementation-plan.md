# Post-PR Lifecycle (muster out): Implementation Plan

*Version: 2026-03-20*

The `muster out` command closes the development loop after a PR is created. It monitors CI checks on the PR, optionally invokes AI to fix CI failures (up to 3 retries), waits for merge, pulls latest changes, and cleans up the worktree, branch, and roadmap entry. It is the counterpart to `muster code` -- where `code` starts work on an item, `out` finishes it.

The implementation introduces two new packages: `internal/git/` for low-level git operations (worktree removal, branch deletion, pull) and `internal/vcs/` for platform-specific PR/MR lifecycle management behind a common `VCS` interface. Both packages follow the codebase's established `exec.Command` convention -- no Go git libraries or API SDKs. The command itself lives in `cmd/out.go` and orchestrates these packages alongside existing config, roadmap, and AI invocation infrastructure.

Key architectural decisions: the VCS interface wraps `gh` and `glab` CLIs with a `CommandRunner` injection point for testability; muster controls its own CI poll loop rather than delegating to `gh pr checks --watch`; cleanup is fault-tolerant (all steps attempted, errors collected, item marked completed regardless); and the `merge_strategy` config field lives on `ProjectConfig` only, defaulting to `github-pr`.

---

## Technology Stack

| Layer | Technology |
|-------|-----------|
| Language | Go 1.24+ |
| CLI framework | Cobra |
| Git operations | `exec.Command("git", ...)` via `internal/git/` |
| VCS platform ops | `exec.Command("gh"/"glab", ...)` via `internal/vcs/` |
| AI invocation | `ai.InvokeAI` (existing, `claude --print`) |
| Config | Existing 5-step resolution chain (`internal/config/`) |
| Roadmap | Existing JSON model (`internal/roadmap/`) |
| Prompt templates | `text/template` with `//go:embed` (existing `internal/prompt/`) |
| Testing | `testify/assert`, `testify/require`, real temp git repos, `CommandRunner` mocks |
| CI | GitHub Actions on Linux, macOS, Windows |

---

## Solution Structure

```
Files to CREATE:
  internal/git/git.go              RunGit(dir, args...) helper
  internal/git/branch.go           CurrentBranch, PullLatest, DeleteBranch
  internal/git/worktree.go         RemoveWorktree
  internal/git/git_test.go         Tests for RunGit
  internal/git/branch_test.go      Tests for branch operations
  internal/git/worktree_test.go    Tests for worktree operations
  internal/vcs/vcs.go              VCS interface, data models, constants
  internal/vcs/github.go           GitHubVCS implementation (gh CLI)
  internal/vcs/gitlab.go           GitLabVCS implementation (glab CLI)
  internal/vcs/factory.go          New(strategy, dir) factory
  internal/vcs/github_test.go      GitHub implementation tests
  internal/vcs/gitlab_test.go      GitLab implementation tests
  internal/vcs/factory_test.go     Factory tests
  cmd/out.go                       Cobra command, lifecycle orchestration
  cmd/out_test.go                  Command tests
  internal/prompt/prompts/out/ci-fix-prompt.md.tmpl   CI fix prompt template
  internal/prompt/testdata/out-ci-fix-prompt.golden   Golden file for template

Files to MODIFY:
  internal/config/config.go        Add MergeStrategy field, constants, ResolveMergeStrategy(), validation
  internal/config/project.go       Add MergeStrategy to mergeProjectConfigs()
  internal/config/resolve.go       Add "out": "muster-standard" to stepDefaultTiers
  internal/config/config_test.go   Tests for merge strategy validation
  internal/config/project_test.go  Tests for merge strategy merging
  internal/config/resolve_test.go  Tests for out step tier resolution
  internal/testutil/helpers.go     Add InitGitRepo(t, dir) shared helper
```

---

## Implementation Phases

### Phase 1: Foundation (Config, Git Package, Test Helpers)

Establishes the config changes needed by the command and the `internal/git/` package that handles low-level git operations. Also adds the shared `InitGitRepo` test helper. After this phase, `make build`, `make test`, and `make lint` pass with the new packages exercised.

| ID | Task | Key Details |
|----|------|-------------|
| 1.1 | Add `MergeStrategy` to `ProjectConfig` with constants and validation | In `internal/config/config.go`: add `MergeStrategy *string \`yaml:"merge_strategy"\`` field to `ProjectConfig` (after `LocalOverrides`). Add constants `MergeStrategyDirect = "direct"`, `MergeStrategyGitHubPR = "github-pr"`, `MergeStrategyGitLabMR = "gitlab-mr"`, `DefaultMergeStrategy = MergeStrategyGitHubPR`. Add `ResolveMergeStrategy(projectCfg *ProjectConfig) string` function. Add validation in the **existing** `func (c *Config) Validate() []error` method (not on `ProjectConfig` directly) — access `c.Project.MergeStrategy` and reject unknown values via switch statement. |
| 1.2 | Integrate `MergeStrategy` into config merging | In `internal/config/project.go`: copy `base.MergeStrategy` in `mergeProjectConfigs()` initial result construction, then apply `override.MergeStrategy` if non-nil. |
| 1.3 | Register `"out"` step default tier | In `internal/config/resolve.go`: add `"out": "muster-standard"` to `stepDefaultTiers` map. |
| 1.4 | Add config tests for merge strategy | In `internal/config/config_test.go`: test `ResolveMergeStrategy` with nil config (returns `"github-pr"`), set config (returns value). Test `Validate()` accepts `direct`/`github-pr`/`gitlab-mr`, rejects `"foobar"`. In `internal/config/project_test.go`: test `mergeProjectConfigs` propagates and overrides `MergeStrategy`. In `internal/config/resolve_test.go`: test `ResolveStep("out", ...)` defaults to `muster-standard` tier. |
| 1.5 | Add `InitGitRepo(t, dir)` to `internal/testutil/helpers.go` | Create helper that runs `git init -b main`, `git config user.email/name`, creates a file, `git add .`, `git commit -m "initial"`. Returns dir. Fails test on any error. |
| 1.6 | Create `internal/git/git.go` with `RunGit(dir string, args ...string) (string, error)` | Sets `cmd.Dir = dir`, captures stdout/stderr in `bytes.Buffer`, returns trimmed stdout. Uses `//nolint:gosec // G204` with justification. |
| 1.7 | Create `internal/git/branch.go` with `CurrentBranch(dir)`, `PullLatest(dir, remote, branch)`, `DeleteBranch(dir, branch)` | `CurrentBranch`: calls `RunGit(dir, "rev-parse", "--abbrev-ref", "HEAD")`. `PullLatest`: calls `RunGit(dir, "pull", remote, branch)`. `DeleteBranch`: calls `RunGit(dir, "branch", "-d", branch)` (uses `-d` not `-D` for safety). |
| 1.8 | Create `internal/git/worktree.go` with `RemoveWorktree(repoDir, worktreePath)` | Calls `RunGit(repoDir, "worktree", "remove", "--force", worktreePath)`. Uses `--force` because this runs after PR merge, so the worktree is disposable. |
| 1.9 | Create `internal/git/git_test.go`, `branch_test.go`, `worktree_test.go` | Use `testutil.InitGitRepo` for setup. Guard integration tests with `testing.Short()`. Cover: `RunGit` success/invalid command/non-git dir/sets Dir; `CurrentBranch` main/feature/not-git-repo; `PullLatest` success (bare remote + clone)/no-remote; `DeleteBranch` merged/unmerged/current; `RemoveWorktree` success/nonexistent/force-removes-clean-worktree. Add explicit `git worktree remove` in test cleanup before `t.TempDir()` auto-cleanup to avoid Windows lockfile issues. |

**Dependencies:** None

### Phase 2: VCS Abstraction

Implements the `internal/vcs/` package with the `VCS` interface and GitHub/GitLab implementations. Uses `CommandRunner` function injection for testability. After this phase, the VCS layer is fully tested with mocked CLI outputs.

| ID | Task | Key Details |
|----|------|-------------|
| 2.1 | Create `internal/vcs/vcs.go` with interface and data models | Define `VCS` interface with methods: `CheckAuth() error`, `ViewPR(ref string) (*PRStatus, error)`, `ListChecks(ref string) ([]CheckResult, error)`, `GetFailedLogs(ref string) ([]FailedCheckLog, error)`, `MergePR(ref string) error`. Define types: `PRState` (`open`/`closed`/`merged`), `CIStatus` (`pending`/`passing`/`failing`/`none`), `ReviewStatus` (`approved`/`changes_requested`/`review_required`/`none`), `PRStatus` struct, `CheckResult` struct, `FailedCheckLog` struct. Define `CommandRunner func(dir string, name string, args ...string) (stdout string, stderr string, err error)` type and `ExecCommandRunner` default implementation. |
| 2.2 | Create `internal/vcs/github.go` with `GitHubVCS` implementation | Struct has `Dir string` and `Run CommandRunner` fields. `NewGitHub(dir string)` sets `Run` to `ExecCommandRunner`. `CheckAuth`: runs `gh auth status`. `ViewPR(ref)`: runs `gh pr view <ref> --json state,number,url,headRefName,isDraft,reviewDecision`, parses JSON, maps `OPEN`/`MERGED`/`CLOSED` to lowercase `PRState`, maps `reviewDecision` to `ReviewStatus`. `ListChecks(ref)`: runs `gh pr checks <ref> --json bucket,name,state,link`, maps `bucket` (`pass`/`fail`/`pending`) to `CheckResult.Status`. `GetFailedLogs(ref)`: for each failed check, extract GitHub Actions run ID from the check URL (format: `https://github.com/owner/repo/actions/runs/<runID>/job/<jobID>` — split on `/runs/` and take the numeric segment). Run `gh run view <runID> --log-failed`. If a check URL doesn't match the GitHub Actions pattern (e.g., third-party status checks), skip it gracefully and return partial results. `MergePR(ref)`: runs `gh pr merge <ref> --delete-branch` with no merge method flags. |
| 2.3 | Create `internal/vcs/gitlab.go` with `GitLabVCS` implementation | Same structure as GitHub but wrapping `glab`. `CheckAuth`: `glab auth status`. `ViewPR(ref)`: `glab mr view <ref> --output json`, maps `opened`/`merged`/`closed` to `PRState`. `ListChecks(ref)`: `glab ci status -b <branch> -F json`. `GetFailedLogs(ref)`: **best-effort** — run `glab ci list -b <branch> --output json` to get pipeline/job IDs, then `glab ci view <jobID> --output json` for failed jobs. If log retrieval fails or returns unexpected format, return an empty slice and a wrapped error (do not block the fix loop — it can still send a generic "CI failed" prompt). `MergePR(ref)`: `glab mr merge <ref> --remove-source-branch --yes`. |
| 2.4 | Create `internal/vcs/factory.go` with `New(strategy, dir string) (VCS, error)` | Switch on strategy: `MergeStrategyGitHubPR` returns `NewGitHub(dir)`, `MergeStrategyGitLabMR` returns `NewGitLab(dir)`, `MergeStrategyDirect` returns error "VCS not needed", default returns error "unknown merge_strategy". |
| 2.5 | Create `internal/vcs/github_test.go` | Inject mock `CommandRunner` returning canned JSON. Tests: `ViewPR` success/not-found/auth-failure; `ListChecks` all-passing/some-failing/pending/no-checks; `GetFailedLogs` success; `MergePR` success/conflict/review-required; `CheckAuth` success/not-logged-in/command-not-found. Compile-time interface check: `var _ VCS = &GitHubVCS{}`. |
| 2.6 | Create `internal/vcs/gitlab_test.go` | Mirror GitHub tests with GitLab JSON schemas: `ViewMR` success/not-found; `ListPipelines` all-passing/some-failing; `MergeMR` success/pipeline-blocked; `CheckAuth` success/not-logged-in/command-not-found. Compile-time interface check: `var _ VCS = &GitLabVCS{}`. |
| 2.7 | Create `internal/vcs/factory_test.go` | Test `New` returns correct implementation type for each strategy, error for `"direct"`, error for unknown strategy. |

**Dependencies:** Phase 1 (uses `config.MergeStrategy*` constants in factory)

### Phase 3: Command Implementation (Core Lifecycle)

Implements `cmd/out.go` with the full lifecycle: setup/validation, PR discovery, auth check, CI polling, merge waiting, and cleanup. The AI fix loop is wired in but delegates to Phase 4 for the prompt template. After this phase, the command is functional end-to-end with `--no-fix`.

| ID | Task | Key Details |
|----|------|-------------|
| 3.1 | Create `cmd/out.go` with Cobra command registration | Define `outCmd` with `Use: "out [slug]"`, `Args: cobra.ExactArgs(1)`, `RunE: runOut`. Register flags: `--no-fix` (bool, false), `--wait` (bool, false), `--timeout` (duration, 30m), `--dry-run` (bool, false), `--verbose` (bool, false). Add `rootCmd.AddCommand(outCmd)` in `init()`. Add package-level `var vcsFactory = vcs.New` so tests can replace the factory function (matching the `ai.InvokeAI` replaceable variable pattern). |
| 3.2 | Implement `runOut` setup and validation | Load config via `config.LoadUserConfig`/`config.LoadProjectConfig`. Resolve merge strategy via `config.ResolveMergeStrategy`. If `"direct"`, print message to stderr and return nil (exit 0). Load roadmap, find item by slug. Error if not found. Error if `item.Status == "completed"` with descriptive message. Resolve step config via `config.ResolveStep("out", ...)`. Create VCS client via `vcsFactory(strategy, dir)`. **Call `vcsClient.CheckAuth()` immediately** — fail fast with actionable error (e.g., "gh not authenticated: run 'gh auth login'") before any VCS operations (MUST-9). When `--verbose`, log resolved config (tool/provider/model/source), merge strategy, and PR URL to stderr. |
| 3.3 | Implement PR discovery helper | `discoverPR(item *roadmap.RoadmapItem, vcsClient vcs.VCS) (string, error)`: try `item.PRUrl` first, then `item.Branch` via `vcsClient.ViewPR(*item.Branch)`. Return actionable error if neither available. |
| 3.4 | Implement CI status monitoring with poll loop | `monitorCI(ctx context.Context, prRef string, vcsClient vcs.VCS, noFix bool, pollInterval time.Duration, ...) error`: Accept `pollInterval` as a parameter (default 30s) so tests can use 0 for immediate returns. **Without `--wait`**: check CI once. If all pass, proceed. If failing, enter fix loop or return error. If pending, return status message and exit 0 (don't poll). **With `--wait`**: poll at `pollInterval` intervals. Aggregate check statuses. On all-pass, return nil. On failure, enter fix loop or return error. On timeout (from `--timeout`), return descriptive error. Print status summary to stderr each iteration. When `--verbose`, log each poll iteration with timestamp. |
| 3.5 | Implement merge wait loop | `waitForMerge(ctx context.Context, prRef string, vcsClient vcs.VCS, timeout time.Duration) error`: poll `vcsClient.ViewPR(prRef)` at 30s intervals. Return nil on `merged`, error on `closed`, error with review context on `changes_requested`. Without `--wait`, check once and report status. |
| 3.6 | Implement fault-tolerant cleanup | `cleanup(slug string, item *roadmap.RoadmapItem, rm *roadmap.Roadmap, projectDir string, dryRun bool) error`: attempt `git.PullLatest`, `git.RemoveWorktree`, `git.DeleteBranch`, set `item.Status = "completed"`, save roadmap. Collect all errors, print as warnings to stderr, return combined error. Mark item completed regardless of cleanup failures. In `--dry-run` mode, print planned actions without executing. |
| 3.7 | Implement `--dry-run` mode | Thread `dryRun` flag through all operations. When set: print what would happen for each step (auth check, CI polling, cleanup actions) but do not execute VCS calls, git operations, or roadmap writes. |
| 3.8 | Create `cmd/out_test.go` with structural and behavioral tests | Structural: command exists, registered on root, has RunE, requires exactly 1 arg, all flags present with correct defaults and usage text, `--help` succeeds. Config/roadmap: malformed YAML error, malformed JSON error, slug not found, no PR URL or branch, **item already completed returns descriptive error**. Auth: VCS auth failure returns actionable error with login command. Merge strategy: `direct` exits cleanly, `github-pr`/`gitlab-mr` proceed, default is `github-pr`. Dry-run: no side effects, prints planned actions. Cleanup: collects all errors, marks item completed even on partial failure. Use `t.TempDir()` for file I/O, mock VCS via `vcsFactory` variable replacement (matching `ai.InvokeAI` pattern), mock AI via `ai.InvokeAI` variable replacement. |

**Dependencies:** Phase 1 (config, git package), Phase 2 (VCS interface and implementations)

### Phase 4: AI CI Fix Loop

Adds the prompt template for CI failure analysis and wires the AI fix loop into the CI monitoring flow. After this phase, `muster out` can automatically attempt to fix CI failures.

| ID | Task | Key Details |
|----|------|-------------|
| 4.1 | Create `internal/prompt/prompts/out/ci-fix-prompt.md.tmpl` | Template receives `PromptContext` with `Extra["FailedChecks"]` (slice of `vcs.FailedCheckLog`) and `Extra["Attempt"]` (int). Instructs AI to analyze failure logs, make targeted fixes in the worktree, commit with descriptive message, and push. No `name:` frontmatter per CLAUDE.md convention. Include attempt number context ("This is attempt N of 3"). **Registration**: add `prompts/out/ci-fix-prompt.md.tmpl` to the `//go:embed` directive in the prompt package. The template is rendered via `prompt.RenderTemplate()` (or equivalent function that renders an embedded template by path and returns a string). |
| 4.2 | Create golden file test for CI fix prompt | Create `internal/prompt/testdata/out-ci-fix-prompt.golden` with expected rendered output. Add test in prompt package that renders template with sample `FailedCheckLog` data and compares against golden file using `testutil.AssertGoldenFile`. |
| 4.3 | Implement `fixCI` helper in `cmd/out.go` | `fixCI(prRef string, vcsClient vcs.VCS, resolved *config.ResolvedConfig, projectCfg *config.ProjectConfig, userCfg *config.UserConfig, slug string, attempt int) error`: fetch failed logs via `vcsClient.GetFailedLogs(prRef)`. If logs are empty (e.g., GitLab best-effort failed), use a generic "CI checks failed, please investigate and fix" prompt instead. Build `prompt.NewPromptContext(...)` with `Extra["FailedChecks"]` and `Extra["Attempt"]`. Render template via `prompt.RenderTemplate("prompts/out/ci-fix-prompt.md.tmpl", ctx)`. Invoke AI via `ai.InvokeAI` with `muster-standard` tier model and env overrides from `config.ToolEnvOverrides`. Return error if AI invocation fails (logged as warning, does not halt retry loop). |
| 4.4 | Wire fix loop into `monitorCI` | When CI fails and `--no-fix` is not set and `attempt < maxCIFixRetries` (3): call `fixCI`, then resume polling. Increment attempt counter. When max retries exceeded, return error "CI checks still failing after 3 attempts; fix manually and re-run". |
| 4.5 | Add AI fix loop tests to `cmd/out_test.go` | Tests: CI fail triggers AI invocation (`GetInvokeCount() >= 1`); `--no-fix` skips AI (`GetInvokeCount() == 0`); max 3 retries respected; success after retry marks item completed. Use `ai.InvokeAI` mock replacement and mock VCS `CommandRunner`. |

**Dependencies:** Phase 3 (command implementation with CI monitoring)

### Phase 5: Integration and Polish

End-to-end validation, error message quality review, and final CI verification across all platforms.

| ID | Task | Key Details |
|----|------|-------------|
| 5.1 | Add integration test with full lifecycle | In `cmd/out_test.go`: create temp dir with `.muster/config.yml` (merge_strategy: github-pr), `.muster/roadmap.json` (item with pr_url and branch), mock VCS that returns passing CI then merged state, mock AI. Execute `runOut` and verify: roadmap item status is `completed`, no errors returned. Guard with `testing.Short()`. |
| 5.2 | Verify error message quality | Review every error path in `cmd/out.go`, `internal/vcs/`, and `internal/git/`. Ensure all errors include: what went wrong, the context (slug, PR ref, branch name), and what the user should do. Specific checks: `exec.ErrNotFound` includes install URL, auth failure includes login command, timeout includes duration. |
| 5.3 | Run `make build`, `make test`, `make lint` locally | Fix any compilation errors, test failures, or lint warnings. Ensure all `//nolint:gosec` annotations have inline justification comments. Verify `CGO_ENABLED=0 go build` succeeds. |
| 5.4 | Verify cross-platform correctness | Confirm all paths use `filepath.Join`/`filepath.Abs`. Confirm `exec.Command` invokes binaries directly (no shell wrappers). Confirm `testutil.InitGitRepo` uses `git init -b main` for deterministic branch names. Confirm golden file comparisons handle LF correctly. |
| 5.5 | Update CLAUDE.md project structure | Add `internal/git/` and `internal/vcs/` to the Project Structure section. Update "Commands Not Yet Implemented" to remove `muster out`. Add `muster out` to the command list in the first line of the structure section. |

**Dependencies:** Phase 3, Phase 4

### Phase Dependency Graph

```
Phase 1 (Foundation)
  |
  v
Phase 2 (VCS Abstraction)
  |
  +----------+----------+
  |                     |
  v                     v
Phase 3 (Command)    Phase 4.1-4.2 (Template + Golden File)
  |                     |
  +----------+----------+
  |
  v
Phase 4.3-4.5 (Wire Fix Loop)
  |
  v
Phase 5 (Integration & Polish)

Note: Phases 1 and 2 can be done in sequence quickly since
Phase 2 only depends on the constants from Phase 1.1.
Tasks 4.1-4.2 (template + golden file) can run in parallel
with Phase 3 since they only depend on Phase 2 types.
Tasks 4.3-4.5 (wiring) depend on Phase 3.
```

---

## Acceptance Criteria

| # | Criterion |
|---|-----------|
| AC-1 | `muster out my-slug` with `merge_strategy: direct` in `.muster/config.yml` exits 0 and prints "merge_strategy is direct; muster out is only meaningful for PR/MR workflows" to stderr. |
| AC-2 | `muster out my-slug` with `merge_strategy: github-pr` and a roadmap item that has `pr_url` set calls `gh auth status`, then `gh pr checks`, and displays CI status to stderr. |
| AC-3 | `muster out nonexistent-slug` exits non-zero with error message containing "roadmap item \"nonexistent-slug\" not found". |
| AC-4 | `muster out my-slug` where the item has neither `pr_url` nor `branch` exits non-zero with error message containing "no PR URL or branch" and suggests checking `muster status`. |
| AC-5 | `muster out my-slug --no-fix` with failing CI checks exits non-zero with "CI checks failed" without invoking AI. |
| AC-6 | `muster out my-slug` with failing CI checks (and `--no-fix` not set) invokes `ai.InvokeAI` with a prompt containing the failed check logs, up to 3 times. |
| AC-7 | `muster out my-slug --wait --timeout 5m` polls PR state at ~30s intervals and returns nil when PR state becomes `merged`. |
| AC-8 | `muster out my-slug --wait` with a PR that has `changes_requested` review status exits non-zero with error containing "review changes requested". |
| AC-9 | After successful merge detection, cleanup pulls latest, removes worktree, deletes branch, marks roadmap item `completed`, and saves roadmap -- even if worktree removal fails, the item is still marked completed. |
| AC-10 | `muster out my-slug --dry-run` prints planned actions to stderr but does not modify the roadmap file, invoke git cleanup, or call VCS operations. |
| AC-11 | `muster out` with no arguments exits non-zero with Cobra's "accepts 1 arg(s), received 0" error. |
| AC-12 | `config.ResolveMergeStrategy(nil)` returns `"github-pr"`. `config.ResolveMergeStrategy(&ProjectConfig{MergeStrategy: ptr("gitlab-mr")})` returns `"gitlab-mr"`. |
| AC-13 | `config.Validate()` with `merge_strategy: "foobar"` returns error containing "invalid merge_strategy". |
| AC-14 | `config.ResolveStep("out", nil, nil)` resolves model to the concrete model for `muster-standard` tier. |
| AC-15 | `make build`, `make test`, and `make lint` pass on Linux, macOS, and Windows CI. |
| AC-16 | All `internal/git/` exported functions have at least one success and one failure test using real temp git repos. |
| AC-17 | All `VCS` interface methods have tests for success, not-found, and auth failure on both `GitHubVCS` and `GitLabVCS` using injected `CommandRunner` mocks. |
| AC-18 | `muster out completed-slug` where the item has `status: completed` exits non-zero with error containing "already completed". |

---

## Test Strategy

**Unit tests** cover pure logic with no external tool invocation: config validation and merge strategy resolution, VCS JSON parsing and status mapping (via `CommandRunner` mocks), PR discovery logic, CI status aggregation, and cleanup error collection. These run fast on all platforms.

**Integration tests** invoke real `git` commands against temp repos created with `testutil.InitGitRepo`. They cover `internal/git/` operations (branch, worktree, pull) and the `cmd/out.go` RunE function with real config/roadmap files in `t.TempDir()` plus mocked VCS and AI layers. These are guarded with `testing.Short()`.

**Mocking boundaries**: AI is mocked via `ai.InvokeAI` variable replacement (existing pattern). VCS is mocked via `CommandRunner` function injection on the struct. Git operations in command-level tests use real temp repos (matching existing `cmd/sync_test.go` patterns).

**Golden file tests** verify the CI fix prompt template renders correctly with sample `FailedCheckLog` data. Updated via `testutil.AssertGoldenFile`.

**Quality gates**: `make build` (CGO_ENABLED=0), `make test` (`go test -v -race ./...` on Linux/macOS/Windows), `make lint` (`golangci-lint run --timeout=5m`). All three must pass before the feature is considered complete.

---

## Known Limitations

1. **No auto-merge** -- `muster out` monitors CI and merge status but does not call `vcs.MergePR()` automatically in v1. The `MergePR` method exists on the interface for future `--auto-merge` support.
2. **No version tagging or CHANGELOG promotion** -- these are deferred to `muster in` (Phase 6). The `out` command's responsibility ends at merge confirmation and cleanup.
3. **No `internal/version/` package** -- while mentioned in the Phase 4 design doc, it is not needed by `muster out` and is not built here.
4. **GitLab CI log retrieval may be less reliable** -- `glab ci view` output format is less structured than `gh run view --log-failed`. The GitLab `GetFailedLogs` implementation is best-effort.
5. **No merge queue monitoring** -- GitHub merge queues are handled transparently by `gh pr merge --auto` (future). Muster does not track queue position.
6. **No lifecycle scripts** -- `setup`/`check`/`verify`/`teardown` scripts are a `muster in` concern, not `muster out`.
7. **Poll interval is not yet user-configurable** -- the 30-second default is hardcoded as a constant. A `out.poll_interval` config field is deferred (SHOULD-3) but the constant is easy to extract later.
8. **`DetectWorkspace` is not moved** -- moving it from `internal/docker/worktree.go` to `internal/git/` (NICE-2) is deferred to avoid unnecessary churn in this PR.
9. **No progress spinners** -- CI polling output is plain stderr text, not TUI spinners. Spinner support (NICE-4) can be added later using `ui.IsInteractive()`.
10. **Verbose logging is basic** -- `--verbose` flag is included and logs resolved config, merge strategy, PR URL, and poll iterations to stderr (SHOULD-4), but does not include structured logging or log levels.
