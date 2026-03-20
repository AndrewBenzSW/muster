# Post-PR Lifecycle (muster out): Architecture

*Version: 2026-03-20*

## Overview

The `muster out` command closes the development loop by managing everything that happens after a PR is created: monitoring CI checks, optionally invoking AI to fix CI failures, waiting for merge, pulling latest changes, and cleaning up the worktree and roadmap entry. It is the counterpart to `muster code` -- where `code` starts work on an item, `out` finishes it.

The implementation introduces two new packages -- `internal/git/` for low-level git operations and `internal/vcs/` for platform-specific PR/MR lifecycle management -- plus a new `cmd/out.go` Cobra command that orchestrates them. The `internal/vcs/` package defines a `VCS` interface with GitHub and GitLab implementations that shell out to `gh` and `glab` respectively, following the codebase's established `exec.Command` convention. The command also extends `internal/config/` with a `merge_strategy` field on `ProjectConfig` and registers `"out"` in `stepDefaultTiers` with `muster-standard` as the default tier.

The core execution flow is a state machine: discover the PR from the roadmap item, authenticate with the VCS platform, poll CI status in a loop (with optional AI fix attempts on failure), wait for merge (if `--wait`), then clean up. Cleanup is fault-tolerant: all steps are attempted and errors are collected rather than failing fast, because the merge is the critical state transition and partial cleanup is better than none.

## Package Design

### `internal/git/`

**File: `internal/git/git.go`**

Core helper that all other files in the package use. Unlike `docker/worktree.go:runGitCommand` which relies on cwd, every function here takes an explicit directory parameter and sets `cmd.Dir`.

```go
package git

import (
    "bytes"
    "fmt"
    "os/exec"
    "strings"
)

// RunGit executes a git command in the specified directory and returns trimmed stdout.
// The dir parameter is set as cmd.Dir to avoid reliance on the working directory.
func RunGit(dir string, args ...string) (string, error) {
    //nolint:gosec // G204: Arguments are validated git subcommands, not arbitrary user input
    cmd := exec.Command("git", args...)
    cmd.Dir = dir
    var stdout, stderr bytes.Buffer
    cmd.Stdout = &stdout
    cmd.Stderr = &stderr

    if err := cmd.Run(); err != nil {
        return "", fmt.Errorf("git %s failed: %w\nstderr: %s",
            strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
    }

    return strings.TrimSpace(stdout.String()), nil
}
```

**File: `internal/git/branch.go`**

```go
// CurrentBranch returns the current branch name for the repo at dir.
func CurrentBranch(dir string) (string, error)

// PullLatest pulls the latest changes from remote/branch into dir.
func PullLatest(dir, remote, branch string) error

// DeleteBranch deletes a local branch. Uses -d (not -D) to ensure it has been merged.
func DeleteBranch(dir, branch string) error
```

**File: `internal/git/worktree.go`**

```go
// RemoveWorktree removes a git worktree. Calls git worktree remove --force.
func RemoveWorktree(repoDir, worktreePath string) error
```

**Responsibilities**: Pure git operations only. No VCS platform logic. No config awareness. Every function accepts a directory parameter. All use `RunGit` internally.

**Testing**: Real git repos created in `t.TempDir()` with `testing.Short()` guards. Table-driven tests. A shared `testutil.InitGitRepo(t, dir)` helper initializes a repo with an initial commit (SHOULD-5 from synthesis).

### `internal/vcs/`

**File: `internal/vcs/vcs.go`** -- Interface and data models.

```go
package vcs

// PRState represents the state of a pull request / merge request.
type PRState string

const (
    PRStateOpen   PRState = "open"
    PRStateClosed PRState = "closed"
    PRStateMerged PRState = "merged"
)

// CIStatus represents the aggregate CI status.
type CIStatus string

const (
    CIStatusPending CIStatus = "pending"
    CIStatusPassing CIStatus = "passing"
    CIStatusFailing CIStatus = "failing"
    CIStatusNone    CIStatus = "none"  // No CI checks configured
)

// ReviewStatus represents the review decision.
type ReviewStatus string

const (
    ReviewApproved        ReviewStatus = "approved"
    ReviewChangesRequired ReviewStatus = "changes_requested"
    ReviewPending         ReviewStatus = "review_required"
    ReviewNone            ReviewStatus = "none"
)

// PRStatus holds the current state of a PR/MR.
type PRStatus struct {
    Number   int
    URL      string
    State    PRState
    Branch   string
    Draft    bool
    CI       CIStatus
    Review   ReviewStatus
}

// CheckResult represents a single CI check's status.
type CheckResult struct {
    Name   string // e.g. "CI / test (ubuntu-latest)"
    Status string // "pass", "fail", "pending", "skipping", "cancel"
    URL    string // Link to the check run
}

// FailedCheckLog holds the name and log output of a failed CI check.
type FailedCheckLog struct {
    Name string
    Log  string
}

// VCS defines the interface for VCS platform operations.
// Implementations shell out to platform CLIs (gh, glab) via exec.Command.
// The ref parameter accepts either a PR/MR number or a full URL.
type VCS interface {
    // CheckAuth verifies that the CLI is authenticated.
    // Returns an actionable error message if not (e.g., "run 'gh auth login'").
    CheckAuth() error

    // ViewPR returns the current status of a PR/MR.
    ViewPR(ref string) (*PRStatus, error)

    // ListChecks returns individual CI check results for a PR/MR.
    ListChecks(ref string) ([]CheckResult, error)

    // GetFailedLogs retrieves log output for failed CI checks.
    // Used to provide context to the AI fix loop.
    GetFailedLogs(ref string) ([]FailedCheckLog, error)

    // MergePR merges the PR/MR. Does not pass merge method flags --
    // lets branch protection rules decide.
    MergePR(ref string) error
}
```

**File: `internal/vcs/github.go`** -- GitHub implementation.

```go
// GitHubVCS implements VCS by shelling out to the gh CLI.
// All queries use --json for structured output, never table parsing.
type GitHubVCS struct {
    Dir string // Working directory for gh commands (sets cmd.Dir)
}

func NewGitHub(dir string) *GitHubVCS
```

Key implementation details:
- `CheckAuth()`: runs `gh auth status`, exit code 0 = authenticated.
- `ViewPR(ref)`: runs `gh pr view <ref> --json state,number,url,headRefName,isDraft,reviewDecision`, maps GitHub field names (camelCase, `OPEN`/`MERGED`) to common types (lowercase `open`/`merged`).
- `ListChecks(ref)`: runs `gh pr checks <ref> --json bucket,name,state,link`, maps `bucket` field to check status.
- `GetFailedLogs(ref)`: runs `gh run view <runID> --log-failed` for each failed check. The run ID is extracted from the check's link URL.
- `MergePR(ref)`: runs `gh pr merge <ref> --delete-branch`. No `--squash`/`--rebase`/`--merge` flag -- let branch protection decide (Key Decision 4).

**File: `internal/vcs/gitlab.go`** -- GitLab implementation.

```go
// GitLabVCS implements VCS by shelling out to the glab CLI.
type GitLabVCS struct {
    Dir string
}

func NewGitLab(dir string) *GitLabVCS
```

Key implementation details:
- `CheckAuth()`: runs `glab auth status`.
- `ViewPR(ref)`: runs `glab mr view <ref> --output json`, maps GitLab field names (snake_case, `opened`/`merged`) to common types.
- `ListChecks(ref)`: runs `glab ci status -b <branch> -F json`, maps pipeline job statuses.
- `GetFailedLogs(ref)`: runs `glab ci view <jobID> --output json` for failed jobs.
- `MergePR(ref)`: runs `glab mr merge <ref> --remove-source-branch --yes`. No merge method flags.

**File: `internal/vcs/factory.go`** -- Construction from merge strategy.

```go
// New returns the appropriate VCS implementation for the given merge strategy.
// Returns an error for "direct" (no VCS needed) or unknown strategies.
func New(strategy string, dir string) (VCS, error) {
    switch strategy {
    case config.MergeStrategyGitHubPR:
        return NewGitHub(dir), nil
    case config.MergeStrategyGitLabMR:
        return NewGitLab(dir), nil
    case config.MergeStrategyDirect:
        return nil, fmt.Errorf("VCS not needed for merge_strategy %q", strategy)
    default:
        return nil, fmt.Errorf("unknown merge_strategy %q", strategy)
    }
}
```

**Testing**: Unit tests with mock `exec.Command` outputs. The VCS interface itself enables mock implementations for `cmd/out_test.go`. Integration tests (guarded by build tags or env vars) can test against real GitHub/GitLab repos, but are not required for v1.

### `cmd/out.go`

```go
package cmd

var outCmd = &cobra.Command{
    Use:   "out [slug]",
    Short: "Complete post-PR lifecycle: monitor CI, merge, cleanup",
    Long: `Complete the post-PR lifecycle for a roadmap item.

This command:
  1. Discovers the PR/MR from the roadmap item's pr_url or branch
  2. Verifies VCS authentication (gh or glab)
  3. Monitors CI check status with configurable polling
  4. Optionally invokes AI to fix CI failures (up to 3 retries)
  5. Waits for merge (with --wait flag)
  6. Pulls latest changes to main branch
  7. Cleans up worktree, branch, and marks roadmap item completed

Only meaningful when merge_strategy is github-pr or gitlab-mr.
When merge_strategy is direct, exits with a message.`,
    Args: cobra.ExactArgs(1),
    RunE: runOut,
}

func init() {
    rootCmd.AddCommand(outCmd)
    outCmd.Flags().Bool("no-fix", false, "Skip AI-assisted CI failure repair")
    outCmd.Flags().Bool("wait", false, "Block until PR/MR is merged")
    outCmd.Flags().Duration("timeout", 30*time.Minute, "Maximum wait duration for CI/merge")
    outCmd.Flags().Bool("dry-run", false, "Preview actions without executing")
}
```

**Flags**:
| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--no-fix` | bool | false | Skip AI CI repair loop |
| `--wait` | bool | false | Block until merged (polls CI + merge status) |
| `--timeout` | duration | 30m | Maximum time for `--wait` mode |
| `--dry-run` | bool | false | Print what would happen, do nothing |

### Config Changes

**File: `internal/config/config.go`** -- Add to `ProjectConfig`:

```go
type ProjectConfig struct {
    // ... existing fields ...

    // MergeStrategy determines how completed work is merged.
    // Valid values: "direct", "github-pr", "gitlab-mr".
    // Default: "github-pr".
    MergeStrategy *string `yaml:"merge_strategy"`
}
```

**File: `internal/config/config.go`** -- Add constants:

```go
const (
    MergeStrategyDirect   = "direct"
    MergeStrategyGitHubPR = "github-pr"
    MergeStrategyGitLabMR = "gitlab-mr"
    DefaultMergeStrategy  = MergeStrategyGitHubPR
)
```

**File: `internal/config/config.go`** -- Add resolution helper:

```go
// ResolveMergeStrategy returns the configured merge strategy, falling back
// to DefaultMergeStrategy ("github-pr") when not set. Merge strategy is
// project-level only -- not per-step or per-user (Key Decision 6).
func ResolveMergeStrategy(projectCfg *ProjectConfig) string {
    if projectCfg != nil && projectCfg.MergeStrategy != nil {
        return *projectCfg.MergeStrategy
    }
    return DefaultMergeStrategy
}
```

**File: `internal/config/config.go`** -- Add to `Validate()`:

```go
// Inside Config.Validate(), after existing checks:
if projectCfg.MergeStrategy != nil {
    switch *projectCfg.MergeStrategy {
    case MergeStrategyDirect, MergeStrategyGitHubPR, MergeStrategyGitLabMR:
        // valid
    default:
        errs = append(errs, fmt.Errorf(
            "invalid merge_strategy %q; valid values: direct, github-pr, gitlab-mr",
            *projectCfg.MergeStrategy))
    }
}
```

**File: `internal/config/project.go`** -- Add to `mergeProjectConfigs()`:

```go
// After existing field merges, before return:
if override.MergeStrategy != nil {
    result.MergeStrategy = override.MergeStrategy
}
```

Also copy base value in the initial result construction:

```go
result := &ProjectConfig{
    // ... existing fields ...
    MergeStrategy: base.MergeStrategy,
}
```

**File: `internal/config/resolve.go`** -- Register step default tier:

```go
var stepDefaultTiers = map[string]string{
    "add":  "muster-fast",
    "sync": "muster-fast",
    "out":  "muster-standard",  // CI fixing is complex; needs standard tier
}
```

## Data Models

All key data models are defined above in the package design sections. Here is a consolidated summary of the new types:

```go
// internal/vcs/vcs.go
type PRState string      // "open", "closed", "merged"
type CIStatus string     // "pending", "passing", "failing", "none"
type ReviewStatus string // "approved", "changes_requested", "review_required", "none"

type PRStatus struct {
    Number int
    URL    string
    State  PRState
    Branch string
    Draft  bool
    CI     CIStatus
    Review ReviewStatus
}

type CheckResult struct {
    Name   string
    Status string // "pass", "fail", "pending", "skipping", "cancel"
    URL    string
}

type FailedCheckLog struct {
    Name string
    Log  string
}

// internal/config/config.go (additions)
const MergeStrategyDirect   = "direct"
const MergeStrategyGitHubPR = "github-pr"
const MergeStrategyGitLabMR = "gitlab-mr"
const DefaultMergeStrategy  = MergeStrategyGitHubPR
```

## Command Lifecycle

The `runOut` function orchestrates the full lifecycle. Each numbered step below maps to a helper function in `cmd/out.go`.

### Step 1: Setup and Validation

```
runOut(cmd, args)
  slug := args[0]
  Load config (LoadUserConfig, LoadProjectConfig, ResolveStep("out", ...))
  strategy := config.ResolveMergeStrategy(projectCfg)
  if strategy == "direct" → print message, return nil  [MUST-8]
  Load roadmap → FindBySlug(slug)
  if item == nil → error "roadmap item not found: <slug>"
  if item.Status == "completed" → error "item already completed"
```

### Step 2: PR Discovery

```
discoverPR(item, vcsClient)
  if item.PRUrl != nil → return *item.PRUrl  [MUST-6]
  if item.Branch != nil → attempt vcsClient.ViewPR(*item.Branch)
  error "no PR URL or branch found for item <slug>; set pr_url in roadmap"
```

Both `gh` and `glab` accept branch names or URLs as refs, so passing either works. The function tries `pr_url` first (canonical), then falls back to branch-based lookup.

### Step 3: Authentication Pre-check

```
vcsClient.CheckAuth()  [MUST-9]
  on error → "gh not authenticated: run 'gh auth login'" (or glab equivalent)
```

This runs before any VCS operation to fail fast with an actionable message.

### Step 4: CI Status Monitoring

```
monitorCI(ctx, prRef, vcsClient, noFix, resolved, projectCfg, userCfg, verbose)
  poll loop (interval: 30s, configurable):
    checks := vcsClient.ListChecks(prRef)
    display check status on stderr
    aggregate status:
      all pass → return nil (CI passed)
      any fail → enter fix loop or return error
      all pending → continue polling
      timeout → return error "CI checks did not complete within <timeout>"
```

The poll interval is 30 seconds by default. The loop respects the `--timeout` duration. Each iteration prints a summary line to stderr showing the number of passing/failing/pending checks.

**CI Status State Machine**:

```
                    ┌─────────┐
              ┌────►│ PENDING │◄────┐
              │     └────┬────┘     │
              │          │          │
         poll again   checks     fix pushed
              │      complete       │
              │          │          │
              │     ┌────▼────┐     │
              │     │ PASSING │     │
              │     └─────────┘     │
              │                     │
              │     ┌─────────┐     │
              └─────│ FAILING │─────┘
                    └────┬────┘
                         │
                    no-fix or
                    max retries
                         │
                    ┌────▼────┐
                    │  ERROR  │
                    └─────────┘
```

### Step 5: AI CI Fix Loop (SHOULD-1)

Triggered when CI fails and `--no-fix` is not set. Limited to 3 retries (configurable via constant, not flag).

```
fixCI(prRef, vcsClient, resolved, projectCfg, userCfg, verbose, attempt)
  if attempt > maxCIFixRetries (3) → return error "CI fix failed after 3 attempts"

  logs := vcsClient.GetFailedLogs(prRef)

  // Build prompt context with failure details
  ctx := prompt.NewPromptContext(resolved, projectCfg, userCfg, false, slug, ".", ".", "")
  ctx.Extra["FailedChecks"] = logs
  ctx.Extra["Attempt"] = attempt

  promptContent := prompt.RenderTemplate("prompts/out/ci-fix-prompt.md.tmpl", ctx)

  result := ai.InvokeAI(ai.InvokeConfig{
      Tool:    config.ToolExecutable(resolved.Tool),
      Model:   resolved.Model,
      Prompt:  promptContent,
      Verbose: verbose,
      Env:     config.ToolEnvOverrides(resolved, projectCfg, userCfg),
  })

  // AI is expected to commit and push fixes directly
  // After AI returns, resume CI polling
```

**Template: `internal/prompt/prompts/out/ci-fix-prompt.md.tmpl`**

This template receives the failed check logs and attempt number via `PromptContext.Extra`. It instructs the AI to analyze the failures, make targeted fixes, commit them with a descriptive message, and push. The template does NOT have `name:` frontmatter (per CLAUDE.md skill convention).

### Step 6: Wait for Merge (optional, with `--wait`)

```
waitForMerge(ctx, prRef, vcsClient, timeout)
  poll loop (interval: 30s):
    pr := vcsClient.ViewPR(prRef)

    if pr.State == "merged" → return nil
    if pr.State == "closed" → return error "PR was closed without merging"

    // Review awareness (SHOULD-2)
    if pr.Review == "changes_requested" → return error "review changes requested; address feedback and re-run"

    display status: "Waiting for merge... CI: <status>, Review: <review>"

    timeout → return error "timed out waiting for merge after <duration>"
```

Without `--wait`, the command checks the current state once: if merged, proceed to cleanup; if still open with CI passing, print a message and exit (the PR is ready but not yet merged).

### Step 7: Cleanup

```
cleanup(slug, item, roadmap, projectDir, verbose, dryRun)
  var cleanupErrors []error

  // 7a. Pull latest main
  if err := git.PullLatest(projectDir, "origin", "main"); err != nil {
      cleanupErrors = append(cleanupErrors, fmt.Errorf("pull latest: %w", err))
  }

  // 7b. Remove worktree (if item.Branch is set and worktree exists)
  if item.Branch != nil {
      if err := git.RemoveWorktree(projectDir, worktreePath); err != nil {
          cleanupErrors = append(cleanupErrors, fmt.Errorf("remove worktree: %w", err))
      }
  }

  // 7c. Delete local feature branch
  if item.Branch != nil {
      if err := git.DeleteBranch(projectDir, *item.Branch); err != nil {
          cleanupErrors = append(cleanupErrors, fmt.Errorf("delete branch: %w", err))
      }
  }

  // 7d. Update roadmap item status to completed (always, regardless of cleanup errors)
  item.Status = roadmap.StatusCompleted
  if err := roadmap.SaveRoadmap(projectDir, rm); err != nil {
      cleanupErrors = append(cleanupErrors, fmt.Errorf("save roadmap: %w", err))
  }

  // Report all errors together
  if len(cleanupErrors) > 0 {
      // Print each error to stderr
      for _, e := range cleanupErrors {
          fmt.Fprintf(os.Stderr, "Cleanup warning: %v\n", e)
      }
      return fmt.Errorf("cleanup completed with %d warning(s)", len(cleanupErrors))
  }

  return nil
```

Key behavior: the roadmap item is marked `completed` regardless of whether worktree/branch cleanup succeeds (Key Decision from synthesis: "marks item completed regardless"). The `--dry-run` flag prints each step without executing it.

## Error Handling

### Error Categories and Responses

| Failure Mode | Detection | Response |
|---|---|---|
| Config parse error | `errors.Is(err, config.ErrConfigParse)` | `"config file malformed: %w"` |
| Roadmap parse error | `errors.Is(err, roadmap.ErrRoadmapParse)` | `"roadmap file is malformed: %w"` |
| Item not found | `rm.FindBySlug(slug) == nil` | `"roadmap item %q not found"` |
| No PR URL or branch | Both `item.PRUrl` and `item.Branch` are nil | `"no PR URL or branch for item %q; ensure muster code set these fields"` |
| VCS CLI not installed | `errors.As(err, &exec.Error{})` | `"gh not found: install from https://cli.github.com"` (or glab equivalent) |
| VCS not authenticated | `vcs.CheckAuth()` returns error | `"gh not authenticated: run 'gh auth login'"` |
| CI timeout | Context deadline exceeded | `"CI checks did not complete within %v"` |
| CI fix exhausted | `attempt > maxRetries` | `"CI fix failed after %d attempts; fix manually and re-run"` |
| PR closed (not merged) | `pr.State == "closed"` | `"PR %s was closed without merging"` |
| Review changes requested | `pr.Review == "changes_requested"` | `"review changes requested on PR %s; address feedback and re-run"` |
| Merge timeout | Context deadline exceeded in wait loop | `"timed out waiting for merge after %v"` |
| Cleanup failures | Collected in `[]error` | Printed as warnings; non-zero exit code but item still marked completed |
| `merge_strategy: direct` | Strategy check at start | Print message, exit 0 (not an error) |

### Error Wrapping Convention

All errors follow the codebase's `fmt.Errorf("context: %w", err)` pattern. Sentinel errors are checked with `errors.Is()`. Type assertions use `errors.As()`. This matches `cmd/code.go`, `cmd/add.go`, and `cmd/sync.go` exactly.

## Integration Points

### Config System (`internal/config/`)

- **New field**: `MergeStrategy *string` on `ProjectConfig` with `ResolveMergeStrategy()` helper.
- **Step registration**: `"out": "muster-standard"` in `stepDefaultTiers`.
- **Merge function**: `mergeProjectConfigs()` extended to handle `MergeStrategy`.
- **Validation**: `Validate()` extended to check `merge_strategy` values.
- **Resolution**: `cmd/out.go` calls `config.ResolveStep("out", projectCfg, userCfg)` for the tool/provider/model triple (needed for AI fix loop).
- **Env overrides**: Passed to `ai.InvokeAI` via `config.ToolEnvOverrides(resolved, projectCfg, userCfg)`.

### Roadmap (`internal/roadmap/`)

- **Read**: `roadmap.LoadRoadmap(".")` then `rm.FindBySlug(slug)` to get the item with `PRUrl` and `Branch`.
- **Write**: Mutate `item.Status = roadmap.StatusCompleted` then `roadmap.SaveRoadmap(".", rm)`.
- **No new fields needed**: `PRUrl` and `Branch` already exist on `RoadmapItem`.

### AI Invocation (`internal/ai/`)

- **CI fix loop**: Uses `ai.InvokeAI(ai.InvokeConfig{...})` with the same pattern as `cmd/add.go` and `cmd/sync.go`.
- **Mockable**: `ai.InvokeAI` is a package-level variable, enabling `testutil.MockInvokeAI()` in tests.
- **Response parsing**: The CI fix AI does not return structured JSON -- it commits and pushes directly. The `InvokeResult.RawOutput` is logged if verbose but not parsed.

### Prompt Templates (`internal/prompt/`)

- **New template**: `internal/prompt/prompts/out/ci-fix-prompt.md.tmpl` -- rendered with `PromptContext` where `Extra["FailedChecks"]` contains the failed check logs and `Extra["Attempt"]` contains the retry count.
- **No SKILL.md needed**: The `out` command uses `ai.InvokeAI` (single-shot `--print` mode), not interactive skill staging. The template is rendered and passed directly as the prompt.
- **Golden file test**: `internal/prompt/testdata/out-ci-fix-prompt.golden` verifies rendered output.

### Git Operations (`internal/git/`)

- **Used by cleanup**: `git.PullLatest()`, `git.RemoveWorktree()`, `git.DeleteBranch()`.
- **No dependency on VCS**: Pure git operations, no platform awareness.
- **Future reuse**: The `git.RunGit()` helper and worktree operations will be used by `muster code` (worktree creation) and `muster in` (version tagging) in later phases.

### VCS Operations (`internal/vcs/`)

- **Factory**: `vcs.New(strategy, dir)` creates the appropriate implementation from `merge_strategy`.
- **Used by CI monitoring**: `vcs.ListChecks()`, `vcs.ViewPR()`, `vcs.GetFailedLogs()`.
- **Used by merge wait**: `vcs.ViewPR()` for state polling.
- **Not used for merge**: In v1, `muster out` does not call `vcs.MergePR()` -- it monitors and waits. The `MergePR` method is on the interface for future use by `--auto-merge` (NICE-1) or `muster in`.

## File Summary

New files to create:

| File | Purpose |
|---|---|
| `internal/git/git.go` | `RunGit(dir, args...)` helper |
| `internal/git/branch.go` | `CurrentBranch`, `PullLatest`, `DeleteBranch` |
| `internal/git/worktree.go` | `RemoveWorktree` |
| `internal/git/git_test.go` | Tests for RunGit |
| `internal/git/branch_test.go` | Tests for branch operations |
| `internal/git/worktree_test.go` | Tests for worktree operations |
| `internal/vcs/vcs.go` | VCS interface, data models, constants |
| `internal/vcs/github.go` | GitHubVCS implementation (gh CLI) |
| `internal/vcs/gitlab.go` | GitLabVCS implementation (glab CLI) |
| `internal/vcs/factory.go` | `New(strategy, dir)` factory function |
| `internal/vcs/github_test.go` | GitHub implementation tests |
| `internal/vcs/gitlab_test.go` | GitLab implementation tests |
| `internal/vcs/factory_test.go` | Factory tests |
| `cmd/out.go` | Cobra command, lifecycle orchestration |
| `cmd/out_test.go` | Command tests (registration, flags, integration) |
| `internal/prompt/prompts/out/ci-fix-prompt.md.tmpl` | CI fix prompt template |
| `internal/prompt/testdata/out-ci-fix-prompt.golden` | Golden file for template test |

Files to modify:

| File | Change |
|---|---|
| `internal/config/config.go` | Add `MergeStrategy` field, constants, `ResolveMergeStrategy()`, validation |
| `internal/config/project.go` | Add `MergeStrategy` to `mergeProjectConfigs()` |
| `internal/config/resolve.go` | Add `"out": "muster-standard"` to `stepDefaultTiers` |
| `internal/config/config_test.go` | Tests for merge strategy validation |
| `internal/config/project_test.go` | Tests for merge strategy merging |
| `internal/config/resolve_test.go` | Tests for out step tier resolution |
| `internal/testutil/helpers.go` | Add `InitGitRepo(t, dir)` shared helper |
