# Git Operations Audit

*Researched: 2026-03-20*
*Scope: Existing git operations patterns in muster codebase*

---

## Key Findings

1. **No `internal/git/` package exists yet.** The design doc (`docs/design.md:216`) specifies `internal/git/` for "Worktree, merge, tag, PR/MR creation" but it has not been created. This is the primary package to build for Phase 4.

2. **Existing git code lives in `internal/docker/worktree.go`** and provides only workspace detection (worktree vs main repo) for Docker volume mounting. It is not a general-purpose git package.

3. **The codebase shells out to git via `exec.Command`** -- no Go git library (like go-git) is used. The `go.mod` has no git-related dependencies. The design doc (`docs/design.md:370`) explicitly specifies `os/exec` shelling out to `git`.

4. **`merge_strategy` is referenced in the design doc but not yet in the config structs.** The `ProjectConfig` type in `internal/config/config.go` has no `MergeStrategy` field. This must be added as part of Phase 4.

5. **Roadmap items already have `Branch` and `PRUrl` optional fields** (`internal/roadmap/roadmap.go:94-97`), ready for the `muster out` command to read PR URLs and the pipeline to write branch names.

6. **The `muster out` command does not exist yet.** There is no `cmd/out.go`. The roadmap item `post-pr-lifecycle` describes it.

## Detailed Analysis

### Existing Git Operations in `internal/docker/worktree.go`

The only git code in the codebase is the `DetectWorkspace()` function and its helper `runGitCommand()`.

**`DetectWorkspace()` (lines 18-53):**
- Uses `git rev-parse --show-toplevel` to get the worktree directory
- Uses `git rev-parse --git-common-dir` to find the main .git directory
- Uses `git rev-parse --absolute-git-dir` as fallback for relative paths
- Returns `(worktreeDir, mainRepoDir, error)` where `mainRepoDir` is empty for main repos
- Purpose: Determine Docker volume mount strategy (worktree RW + main .git RO vs single mount)

**`runGitCommand()` (lines 56-68):**
- Generic helper: `exec.Command("git", args...)` with stdout/stderr capture
- Returns trimmed stdout, wraps errors with context
- Has `//nolint:gosec` for G204 with justification comment
- **Important: This helper runs git in the current working directory.** No `cmd.Dir` is set. The new `internal/git/` package should accept explicit directory parameters for all operations.

**Test patterns in `internal/docker/worktree_test.go`:**
- Creates real temp git repos using `t.TempDir()` and `exec.Command`
- Uses `testing.Short()` guard for integration tests
- Has a `runCmd(dir, name, args...)` helper (lines 267-279) that sets `cmd.Dir`
- Tests cover: main repo detection, worktree detection, non-git directory error
- Tests verify paths are absolute

### How External Commands Are Invoked

The codebase has a consistent pattern for `exec.Command` usage across 11 files:

| Package | File | Usage |
|---------|------|-------|
| `docker` | `worktree.go` | `git rev-parse` commands |
| `docker` | `container.go` | `docker compose` commands |
| `docker` | `compose.go` | `docker compose -f path config` for validation |
| `docker` | `auth.go` | `aws configure export-credentials` |
| `ai` | `invoke.go` | AI tool execution (`claude --print`) |
| `cmd` | `code.go` | AI tool execution (`claude`) |
| `testutil` | `mockai.go` | Mock AI tool binary (builds test helper) |

**Common patterns observed:**
- `//nolint:gosec // G204:` with justification comment on every `exec.Command`
- `bytes.Buffer` for capturing stdout/stderr
- `context.WithTimeout` for external command timeouts
- Error wrapping with `fmt.Errorf("description: %w", err)`

### Roadmap Data Model and Git Fields

`internal/roadmap/roadmap.go:77-98` defines `RoadmapItem` with:
```go
PRUrl  *string `json:"pr_url,omitempty"`  // line 94
Branch *string `json:"branch,omitempty"`  // line 97
```

Both are optional pointer fields. The sync command (`cmd/sync.go:349-353`) preserves these during item updates. The UI formatting (`internal/ui/roadmap_format.go:136-140`) displays them when present. Tests verify round-trip serialization of these fields.

The design doc (`docs/design.md:284`) states:
- `pr_url` is written by the pipeline's `finish` step when `merge_strategy` is `github-pr` or `gitlab-mr`
- `muster out` reads it to know which PR/MR to monitor
- `branch` is written when the worktree is created

### What `muster out` Needs (from design doc)

From `docs/design.md:42-44` and `docs/design.md:501-507`:

**Command surface:**
```
muster out [slug]             # Monitor CI, ensure merge, pull latest, cleanup worktree
muster out [slug] --no-fix    # Don't auto-fix CI failures
muster out [slug] --wait      # Block until PR/MR is merged (poll CI status)
```

**Behavior:**
- Only meaningful when `merge_strategy` is `github-pr` or `gitlab-mr`
- Monitors CI checks on the PR
- Optionally pushes fixes for CI failures (with Claude)
- Waits for merge
- Pulls latest main
- Cleans up worktree + roadmap entry

**What this requires in `internal/git/`:**
- Worktree CRUD: create, remove, list
- Squash merge (for `merge_strategy: direct`)
- Conflict detection
- Version tagging

**What this requires in `internal/git/pr.go`:**
- GitHub PR creation (shells to `gh`)
- GitLab MR creation (shells to `glab`)
- CI status polling
- Merge confirmation

### Config Gaps: `merge_strategy`

The design doc defines `merge_strategy` as a project config field (`docs/design.md:137`):
```yaml
merge_strategy: direct  # or: github-pr, gitlab-mr
```

But `ProjectConfig` in `internal/config/config.go:109-127` has no `MergeStrategy` field. This needs to be added:
```go
MergeStrategy *string `yaml:"merge_strategy"`
```

With validation for allowed values: `direct`, `github-pr`, `gitlab-mr`.

### Cross-Platform Considerations

From `docs/design.md:376-384`:
- Use `filepath.Join` and `filepath.Abs` everywhere (already done in existing code)
- Shell execution must not assume Unix shell -- invoke binaries directly with arg arrays
- Use `os.MkdirTemp` -- never hardcode `/tmp` (already followed)
- File locking should use `os.OpenFile` with `os.O_CREATE|os.O_EXCL`

The existing `runGitCommand()` in `docker/worktree.go` does NOT set `cmd.Dir`, relying on the current working directory. The new `internal/git/` package must be more explicit, accepting directory parameters.

### Testing Patterns for Git Operations

From `docs/design.md:400`:
> Git operations: Integration tests (real git repos). Create temp repos with `t.TempDir()`, test worktree creation, merge, conflict detection. Real git, no mocks.

The existing `docker/worktree_test.go` demonstrates the expected pattern:
1. `testing.Short()` guard at the top
2. `t.TempDir()` for temp directories
3. Real `git init`, `git add`, `git commit` via `exec.Command`
4. Helper function `runCmd(dir, name, args...)` for running git in a specific directory
5. Cleanup via `defer os.Chdir(oldDir)` (but the new package shouldn't need chdir if it sets `cmd.Dir`)
6. Verify paths are absolute
7. Table-driven test cases

## Recommendations

### Package Structure

Create `internal/git/` with these files:
- `git.go` -- Core `runGit(dir string, args ...string)` helper, similar to `docker/worktree.go:runGitCommand` but accepting a directory parameter
- `worktree.go` -- Worktree CRUD: `CreateWorktree(repoDir, worktreePath, branch string)`, `RemoveWorktree(repoDir, worktreePath string)`, `ListWorktrees(repoDir string)`
- `merge.go` -- `SquashMerge(dir, sourceBranch string)`, `DetectConflicts(dir string)`, `AbortMerge(dir string)`
- `tag.go` -- `CreateTag(dir, tag, message string)`, `ListTags(dir string)`
- `branch.go` -- `CurrentBranch(dir string)`, `CheckoutBranch(dir, branch string)`, `PullLatest(dir, remote, branch string)`
- `pr.go` -- `CreateGitHubPR(dir, title, body, base, head string)`, `GetPRStatus(dir, prURL string)`, `CreateGitLabMR(...)`, etc.

### Use the Existing `exec.Command` Pattern

Do NOT introduce a Go git library. The codebase consistently uses `exec.Command` for all external tools (git, docker, aws, claude). The design doc explicitly calls this out. Follow the same patterns:
- `//nolint:gosec // G204:` with justification
- `bytes.Buffer` for output capture
- `context.WithTimeout` for safety
- Error wrapping with descriptive messages

### Make Directory an Explicit Parameter

Unlike `docker/worktree.go:runGitCommand` which uses cwd, the new `internal/git/` package must accept a directory for every operation. Set `cmd.Dir` instead of relying on `os.Chdir`. This is critical for:
- Thread safety (multiple worktrees operated on concurrently)
- Testability (no global state mutation)
- Correctness (explicit is better than implicit)

### Move `DetectWorkspace` to `internal/git/`

The `DetectWorkspace()` function in `internal/docker/worktree.go` is a git operation that happens to be used by Docker. Consider either:
1. Moving it to `internal/git/` and having `internal/docker/` import it
2. Wrapping it: `internal/docker/` calls `git.DetectWorkspace()` internally

Option 1 is cleaner. The `runGitCommand` helper should also move (or be duplicated with the directory parameter).

### Add `merge_strategy` to ProjectConfig

Add the field to `internal/config/config.go`:
```go
type ProjectConfig struct {
    MergeStrategy *string `yaml:"merge_strategy"`
    // ... existing fields
}
```
With a validation function that checks for `direct`, `github-pr`, `gitlab-mr`. Default to `direct` when not set.

### PR Operations Should Shell Out to `gh`/`glab`

Per the design doc (`docs/design.md:370, 503`), PR/MR operations shell out to `gh` (GitHub CLI) and `glab` (GitLab CLI). These are soft dependencies -- the `muster doctor` command should check for them but they're only required when `merge_strategy` is `github-pr` or `gitlab-mr`.

### Test Strategy

Follow the pattern from `docker/worktree_test.go`:
- Create real git repos in `t.TempDir()`
- Guard with `testing.Short()`
- Use table-driven tests
- Provide a `runCmd` helper (or reuse from `internal/testutil/`)
- Verify all returned paths are absolute
- Test error cases (non-git directory, missing branch, conflict scenarios)

Consider adding a shared `testutil.InitGitRepo(t, dir)` helper that creates a git repo with an initial commit, since this setup is repeated in `docker/worktree_test.go` and will be needed extensively in `internal/git/` tests.

## Open Questions

1. **Should `internal/docker/worktree.go` be refactored to use `internal/git/`?**
   - The `DetectWorkspace()` function is a git operation used for Docker volume mounting
   - Moving it creates a dependency: `internal/docker/` imports `internal/git/`
   - This is a clean dependency direction, but the refactoring should be confirmed with the user
   - What was tried: Examined import graph -- `internal/docker/` currently has no internal imports except `internal/config/`

2. **What is the default merge strategy when `merge_strategy` is not set in config?**
   - The design doc (`docs/design.md:137`) shows `direct` as the example
   - `muster out` is described as "only meaningful when merge_strategy is github-pr or gitlab-mr"
   - Recommendation: default to `direct`, making `muster out` a no-op with a helpful message
   - Why it matters: determines whether `muster out` should error or silently succeed when no strategy is configured

3. **Should worktree cleanup remove the branch too?**
   - `git worktree remove` removes the worktree directory but not the branch
   - `git branch -d <branch>` can fail if the branch hasn't been merged
   - The design doc says "clean up worktree + roadmap entry" but doesn't specify branch cleanup
   - Why it matters: leftover branches accumulate if not cleaned

4. **How should CI status polling work for `--wait`?**
   - `gh pr checks <number> --watch` blocks until checks complete (built-in to gh CLI)
   - Alternatively, poll with `gh pr view <number> --json statusCheckRollup`
   - The design doc mentions polling but doesn't specify the mechanism
   - Why it matters: `--watch` is simpler but less controllable; polling allows custom retry logic and timeout

## References

| Item | Location |
|------|----------|
| Existing git worktree detection | `internal/docker/worktree.go:18-53` |
| `runGitCommand` helper | `internal/docker/worktree.go:56-68` |
| Worktree test patterns | `internal/docker/worktree_test.go:12-279` |
| `exec.Command` nolint pattern | `internal/docker/worktree.go:57`, `internal/ai/invoke.go:102`, `internal/docker/container.go:152` |
| RoadmapItem Branch/PRUrl fields | `internal/roadmap/roadmap.go:93-97` |
| Branch/PRUrl in sync | `cmd/sync.go:349-353` |
| Branch/PRUrl in UI | `internal/ui/roadmap_format.go:136-140` |
| Design doc: `internal/git/` spec | `docs/design.md:216` |
| Design doc: Phase 4 scope | `docs/design.md:501-508` |
| Design doc: `muster out` behavior | `docs/design.md:42-44, 81` |
| Design doc: merge_strategy config | `docs/design.md:137` |
| Design doc: git testing strategy | `docs/design.md:400` |
| Design doc: cross-platform | `docs/design.md:376-384` |
| Design doc: libraries (exec for git) | `docs/design.md:370` |
| ProjectConfig (missing merge_strategy) | `internal/config/config.go:109-127` |
| Config validation | `internal/config/config.go:298-435` |
| Test helpers | `internal/testutil/helpers.go:1-49` |
| Go module (no git library) | `go.mod:1-70` |
