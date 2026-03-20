# Post-PR Lifecycle (muster out): Requirements Synthesis

*Synthesized: 2026-03-20*
*Sources: git-ops.md, cli-integration.md, config-merge.md, changelog-semver.md, command-patterns.md*

---

## Executive Summary

The `muster out` command handles the complete post-PR lifecycle: monitoring CI checks on a PR/MR, optionally invoking AI to fix CI failures, waiting for merge, pulling latest changes, and cleaning up the worktree and roadmap entry. Research across five areas confirms the codebase is well-prepared for this feature -- the roadmap data model already has `pr_url` and `branch` fields, the `exec.Command` pattern is consistent and proven, and the Cobra command structure provides a clear template. The two significant gaps are: (1) no `internal/git/` package exists yet, and (2) the `merge_strategy` config field is missing from `ProjectConfig`.

The primary architectural decision is to create a VCS abstraction (`internal/vcs/`) that wraps `gh` and `glab` CLIs behind a common interface, following the same `exec.Command` shelling-out approach used throughout the codebase. This keeps the dependency surface minimal (no Go git library, no GitHub/GitLab API SDKs) while supporting both platforms. The `internal/git/` package provides lower-level git operations (worktree cleanup, branch deletion, pull) that `muster out` orchestrates alongside VCS operations.

Version tagging and CHANGELOG promotion are explicitly deferred to `muster in` (Phase 6). While the `internal/version/` package is part of Phase 4's design doc deliverables, `muster out` itself does not need it -- its job ends after merge confirmation and worktree cleanup. The `internal/version/` package can be built in parallel but is not a blocker for `muster out`.

## Requirements

### MUST Have

1. **Add `merge_strategy` field to `ProjectConfig`** -- a `*string` field with validation for `direct`, `github-pr`, `gitlab-mr`. Include constants, a `ResolveMergeStrategy()` helper defaulting to `github-pr`, and integration into `mergeProjectConfigs()` and `Config.Validate()`. *(config-merge.md: Recommendations 1-5)*

2. **Create `internal/git/` package with core operations** -- at minimum: `RunGit(dir, args...)` helper with explicit `cmd.Dir`, `CurrentBranch(dir)`, `PullLatest(dir, remote, branch)`, `RemoveWorktree(repoDir, worktreePath)`, and `DeleteBranch(repoDir, branch)`. All functions must accept a directory parameter; never rely on cwd. *(git-ops.md: Recommendations)*

3. **Create `internal/vcs/` package with platform abstraction** -- a `VCS` interface with `ViewPR(ref) (*PRStatus, error)`, `ListChecks(ref) ([]CheckResult, error)`, and `MergePR(ref, opts) error`. Implement `GitHubVCS` (wrapping `gh`) and `GitLabVCS` (wrapping `glab`). Always use JSON output (`gh --json`, `glab --output json`), never parse table output. *(cli-integration.md: Recommendations 1-3)*

4. **Implement `cmd/out.go` Cobra command** -- `Use: "out [slug]"`, `Args: cobra.ExactArgs(1)`, `RunE` pattern matching existing commands. Flags: `--no-fix` (skip AI CI repair), `--wait` (block until merged), `--timeout` (max wait duration, default 30m), `--dry-run` (preview actions). Register in `init()` with `rootCmd.AddCommand(outCmd)`. *(command-patterns.md: Recommendations; git-ops.md: design doc surface)*

5. **CI status monitoring with muster-controlled poll loop** -- poll at configurable intervals (default 30s) using `vcs.ListChecks()`, display status on stderr, exit on all-pass, all-fail, or timeout. Do not use `gh pr checks --watch` or `glab ci status --live` -- they are designed for human consumption and prevent custom UX integration. *(cli-integration.md: Recommendation 4)*

6. **PR URL discovery from roadmap item** -- read `pr_url` from the `RoadmapItem` identified by slug. If `pr_url` is nil, attempt fallback via `gh pr view --json url` / `glab mr list --source-branch` using the item's `branch` field. Error with actionable message if neither is available. *(git-ops.md: Roadmap Data Model; cli-integration.md: PR number extraction)*

7. **Worktree and roadmap cleanup after merge** -- after confirming merge, remove the git worktree (`git worktree remove`), delete the feature branch (`git branch -d`), update the roadmap item status to `completed`, and save. *(git-ops.md: design doc behavior)*

8. **Early exit for `merge_strategy: direct`** -- when the resolved merge strategy is `direct`, print a message explaining that `muster out` is only meaningful for PR/MR workflows and exit with code 0. *(config-merge.md: Merge Strategy Values)*

9. **Auth pre-check before any VCS operation** -- run `gh auth status` or `glab auth status` before attempting PR operations. On failure, exit with an actionable error message (e.g., "gh not authenticated: run 'gh auth login'"). *(cli-integration.md: Authentication; Error Handling)*

10. **Follow established error handling patterns** -- use `errors.Is(err, config.ErrConfigParse)` for config errors, `errors.Is(err, roadmap.ErrRoadmapParse)` for roadmap errors, `errors.As(err, &exec.Error{})` with install guidance for missing CLI tools, and descriptive `fmt.Errorf` wrapping throughout. *(command-patterns.md: Error Handling)*

11. **Cross-platform correctness** -- use `filepath.Join`/`filepath.Abs` for all paths, invoke binaries directly via `exec.Command` (no shell assumptions), use `exec.LookPath` for binary detection. Follow existing `//nolint:gosec // G204` pattern with justification comments. *(git-ops.md: Cross-Platform; cli-integration.md: Cross-platform)*

### SHOULD Have

1. **AI-assisted CI fix loop** -- when CI checks fail and `--no-fix` is not set, invoke AI (via `ai.InvokeAI`) to analyze failures and push fixes. Use `muster-standard` as the default model tier for `out` since CI fixing is more complex than add/sync tasks. Limit to a configurable max retry count (default 3) to prevent infinite loops. *(command-patterns.md: AI Invocation pattern; config-merge.md: step tier)*

2. **Review status awareness in `--wait` mode** -- when polling, also check review status via `gh pr view --json reviewDecision` / `glab mr view --output json`. If `CHANGES_REQUESTED`, exit the poll loop and report (AI cannot address review feedback). If `REVIEW_REQUIRED`, continue polling. Display review status alongside CI status. *(cli-integration.md: Open Question 3)*

3. **Configurable poll interval** -- add a project-level config field (e.g., `out.poll_interval`) defaulting to 30 seconds. This accommodates CI pipelines that range from 2 minutes to 30+ minutes. *(cli-integration.md: Open Question 4)*

4. **Verbose logging following existing pattern** -- when `--verbose` is set, log resolved config (tool/provider/model/source), merge strategy, PR URL, each poll iteration with timestamp, and cleanup actions. Use `fmt.Fprintf(os.Stderr, ...)` consistently. *(command-patterns.md: Output Formatting)*

5. **Shared `testutil.InitGitRepo(t, dir)` helper** -- create a reusable helper that initializes a git repo with an initial commit, since this setup is needed in both existing `docker/worktree_test.go` and the new `internal/git/` tests. *(git-ops.md: Testing Patterns)*

6. **Register `out` in `stepDefaultTiers`** -- add `"out": "muster-standard"` to `internal/config/resolve.go` so the command gets an appropriate default model tier for AI-assisted CI fixing. *(command-patterns.md: Config Resolution)*

### NICE TO HAVE

1. **Auto-merge enablement** -- after CI passes and review is approved, use `gh pr merge --squash --auto --delete-branch` / `glab mr merge --squash --auto-merge --remove-source-branch --yes` to enable auto-merge rather than requiring manual merge. This could be gated behind a `--auto-merge` flag. *(cli-integration.md: Merge Confirmation)*

2. **Move `DetectWorkspace()` from `internal/docker/worktree.go` to `internal/git/`** -- it is fundamentally a git operation. The Docker package would then import `internal/git/` (a clean dependency direction). This reduces duplication and centralizes git logic. *(git-ops.md: Recommendation to move DetectWorkspace)*

3. **`CreatePR` in the VCS interface** -- while `muster out` monitors existing PRs, the `finish` pipeline step (Phase 5/6) will need PR creation. Adding `CreatePR(opts) (url, error)` to the interface now avoids a future refactor. *(cli-integration.md: PR/MR Creation)*

4. **Progress spinners for long-running operations** -- use a TUI spinner (leveraging `ui.IsInteractive()`) during CI polling and merge waiting to provide better UX than raw stderr output. *(command-patterns.md: Output Formatting)*

5. **CHANGELOG link references** -- when building `internal/version/`, optionally generate comparison link references at the bottom of CHANGELOG.md (e.g., `[0.5.0]: https://github.com/.../compare/v0.4.1...v0.5.0`). *(changelog-semver.md: Open Question 5)*

### SHOULD NOT Include

1. **Version tagging or CHANGELOG promotion in `muster out`** -- these belong in the `prepare` step of `muster in` (Phase 6). The `out` command's responsibility ends at merge confirmation and cleanup. Mixing release management into post-PR lifecycle creates confusing responsibility boundaries. *(changelog-semver.md: Open Question 1; design doc line 527)*

2. **`internal/version/` package as a blocker for `muster out`** -- while it is a Phase 4 design doc deliverable, `muster out` does not depend on it. It can be built in parallel or deferred. *(changelog-semver.md: detailed analysis)*

3. **Go git library (go-git or similar)** -- the codebase consistently uses `exec.Command` for all external tools. The design doc explicitly specifies `os/exec` for git. Introducing a Go git library would break this convention and add unnecessary dependency surface. *(git-ops.md: Recommendation)*

4. **GitHub/GitLab API SDK usage** -- shelling out to `gh`/`glab` is the design doc's specified approach. These CLIs handle authentication, pagination, and API versioning. Direct API calls would duplicate this and create token management burden. *(cli-integration.md: Authentication)*

5. **Docker/container orchestration in `muster out`** -- unlike `muster code --yolo`, the `out` command operates on the host machine with git/gh/glab. It does not need Docker integration. *(command-patterns.md: Open Question 1)*

6. **Lifecycle config (`setup`, `check`, `verify`, `teardown` scripts)** -- this is a separate concern for `muster in` (Phase 6). `muster out` does not run lifecycle scripts. *(config-merge.md: Open Question 4)*

7. **Merge queue monitoring** -- GitHub merge queues are handled transparently by `gh pr merge --auto`. Muster should not attempt to track queue position or status. *(cli-integration.md: Open Question 1)*

## Key Decisions

### 1. Shell out to gh/glab rather than use API SDKs

- **Decision**: All VCS operations use `exec.Command` to invoke `gh` and `glab` CLIs
- **Rationale**: Consistent with every other external tool invocation in the codebase (git, docker, claude, aws). The design doc explicitly specifies this approach (docs/design.md:370,503). The CLIs handle auth, pagination, and API versioning.
- **Alternatives considered**: GitHub Go SDK (`google/go-github`), GitLab Go SDK (`xanzy/go-gitlab`). Rejected because they add dependency surface, require token management, and break the established `exec.Command` convention.

### 2. Create `internal/vcs/` with a common interface, not just `internal/git/pr.go`

- **Decision**: Separate `internal/vcs/` package with a `VCS` interface and per-platform implementations, rather than putting PR operations in `internal/git/pr.go` as the design doc suggests
- **Rationale**: gh and glab have significantly different flag names, JSON schemas, and error patterns (cli-integration.md). A clean interface allows the command layer to be platform-agnostic. The `internal/git/` package stays focused on pure git operations.
- **Alternatives considered**: Single `internal/git/pr.go` file with if/else branching on merge strategy. Rejected because the platform differences are substantial enough to warrant separate implementations behind an interface.

### 3. Default merge strategy is `github-pr`

- **Decision**: When `merge_strategy` is not configured, default to `github-pr`
- **Rationale**: Muster is designed around PR-based workflows. The `muster out` command is the centerpiece of Phase 4 and would be a no-op with `direct` default. Most teams using muster will be on GitHub. (config-merge.md: Default Value Discussion)
- **Alternatives considered**: `direct` (safer but makes `muster out` useless by default), require explicit config (adds friction for the common case).

### 4. Muster controls its own CI poll loop

- **Decision**: Implement polling with `vcs.ListChecks()` at intervals rather than using `gh pr checks --watch` or `glab ci status --live`
- **Rationale**: Built-in watch modes are designed for human terminal output and cannot integrate with muster's UX (spinners, status formatting, timeout logic, AI fix loop). Custom polling enables configurable intervals and programmatic decision-making. (cli-integration.md: Recommendation 4)
- **Alternatives considered**: `gh pr checks --watch --fail-fast`. Rejected because it blocks the process, provides no structured output, and prevents the AI fix loop from triggering on failure.

### 5. Version tagging is deferred to `muster in`

- **Decision**: `muster out` does not create version tags or promote CHANGELOG sections
- **Rationale**: The design doc places version bump and CHANGELOG promotion in the `prepare` step of `muster in` (docs/design.md:527). `muster out` is about post-PR lifecycle (CI, merge, cleanup), not release management. Mixing concerns would make the command's purpose unclear. (changelog-semver.md: Open Question 1)
- **Alternatives considered**: Tagging after merge in `muster out`. Rejected because not every merged PR is a release, and the `muster in` pipeline has a dedicated prepare step for this.

### 6. Merge strategy is project-level only, not per-step or per-user

- **Decision**: `merge_strategy` lives in `ProjectConfig` only, not in user config or pipeline step config
- **Rationale**: A project is on GitHub or GitLab, not both. This is inherently a project-level setting. Per-step variation has no use case. User-level default adds complexity with no clear benefit. (config-merge.md: Open Questions 1, 3)
- **Alternatives considered**: User-level default, per-step override. Both rejected as over-engineering with no identified use case.

### 7. Worktree cleanup includes branch deletion

- **Decision**: After merge confirmation, `muster out` removes the worktree AND deletes the local feature branch
- **Rationale**: Leftover branches accumulate and cause confusion. After a PR is merged, the feature branch has no purpose. Using `git branch -d` (not `-D`) ensures the branch is only deleted if it has been merged, providing a safety check. (git-ops.md: Open Question 3)
- **Alternatives considered**: Leave branch cleanup to the user. Rejected because muster's purpose is to automate the full lifecycle.

## Resolved Questions

### 1. Slug is required (no implicit discovery)

- **Decision**: `cobra.ExactArgs(1)` — slug is always required
- **Rationale**: Slug is already known from the preceding `muster code` invocation. Implicit branch-to-item discovery adds complexity (multiple items could share a branch) with no clear benefit for v1. Can be added later as a convenience.

### 2. AI CI fix loop included in v1

- **Decision**: Implement AI-assisted CI fixing from the start with `--no-fix` flag and max 3 retries
- **Rationale**: This is a core value proposition of muster — automating the full post-PR lifecycle including fixing CI failures. Fetch failure logs via `gh run view --log-failed` and pass them as context to a `prompts/out/ci-fix-prompt.md.tmpl` template. Use `muster-standard` as the default model tier.

### 3. Collect all cleanup errors, report together

- **Decision**: Attempt every cleanup step, collect errors, report together. Mark roadmap item completed regardless.
- **Rationale**: The merge is the key state transition. Partial cleanup is better than no cleanup. `--dry-run` previews cleanup actions. Exit code reflects cleanup failures (non-zero) but the item is still marked completed.

### 4. Let platform decide merge method

- **Decision**: Do not pass any merge method flags to `gh pr merge` / `glab mr merge`. Let branch protection rules decide.
- **Rationale**: Most teams enforce merge method via branch protection. Passing explicit flags would conflict with repo settings. Can add `--squash`/`--rebase`/`--merge` flags later if users request them.

## References

| Research File | Key Topics | Location |
|---|---|---|
| git-ops.md | Git package design, worktree operations, exec.Command patterns, testing strategy | `.plan/post-pr-lifecycle/research/git-ops.md` |
| cli-integration.md | gh/glab CLI commands, JSON schemas, auth, CI polling, VCS interface design | `.plan/post-pr-lifecycle/research/cli-integration.md` |
| config-merge.md | merge_strategy field, config loading/merge, resolution chain, ProjectConfig gaps | `.plan/post-pr-lifecycle/research/config-merge.md` |
| changelog-semver.md | CHANGELOG format, semver library, version tagging, internal/version/ package scope | `.plan/post-pr-lifecycle/research/changelog-semver.md` |
| command-patterns.md | Cobra patterns, flag handling, error categorization, AI invocation, test structure | `.plan/post-pr-lifecycle/research/command-patterns.md` |

### Key Source Files Referenced

| File | Relevance |
|---|---|
| `internal/config/config.go:108-127` | `ProjectConfig` struct -- needs `MergeStrategy` field |
| `internal/config/project.go:73-163` | `mergeProjectConfigs()` -- needs merge strategy handling |
| `internal/config/resolve.go:10-13` | `stepDefaultTiers` -- needs `out` entry |
| `internal/roadmap/roadmap.go:77-98` | `RoadmapItem` with `PRUrl` and `Branch` fields |
| `internal/docker/worktree.go:18-68` | Existing git operations, candidate for refactor into `internal/git/` |
| `cmd/code.go`, `cmd/add.go`, `cmd/sync.go` | Command pattern templates for `cmd/out.go` |
| `internal/ai/invoke.go` | `InvokeAI` for CI fix loop |
| `docs/design.md:42-44,81,137,501-508` | Design doc specification for `muster out` |
