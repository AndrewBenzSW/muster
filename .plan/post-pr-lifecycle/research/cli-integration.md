# gh/glab CLI Integration

*Researched: 2026-03-20*
*Scope: GitHub and GitLab CLI integration for PR/MR lifecycle*

---

## Key Findings

1. **No existing gh/glab usage in codebase.** The muster codebase currently shells out to `git` and `docker compose` via `exec.Command` but has zero integration with `gh` or `glab`. The `pr_url` field on `RoadmapItem` is defined but only used for display in `muster status`.

2. **gh and glab have parallel but non-identical interfaces.** Both support PR/MR creation, CI status checking, and merge operations, but flag names, JSON output formats, and authentication mechanisms differ significantly. A thin abstraction layer is warranted.

3. **gh has superior JSON output support.** `gh` supports `--json <fields>` with fine-grained field selection and `--jq` filtering. `glab` uses `--output json` (or `-F json`) which dumps the full object. Both return structured JSON suitable for Go's `encoding/json`.

4. **CI status polling is well-supported by both CLIs.** `gh pr checks --watch` and `glab ci status --live` both provide blocking poll modes, but for programmatic use, polling with `--json` output and custom intervals is more controllable.

5. **Authentication is handled by the CLIs themselves.** Both `gh` and `glab` manage their own credential storage. Muster should not manage tokens — it should detect whether auth is configured and fail early with guidance if not.

## Detailed Analysis

### PR/MR Creation

#### GitHub (`gh pr create`)

```bash
gh pr create --title "Add retry logic" \
  --body "Implements exponential backoff for HTTP client" \
  --base main \
  --head abenz/add-retry-logic
```

Key flags:
- `--title`, `--body`, `--base`, `--head` — core PR fields
- `--draft` — create as draft
- `--fill` — auto-populate title/body from commit messages
- `--label`, `--assignee`, `--reviewer` — metadata
- `--dry-run` — print details without creating

Output: prints the PR URL to stdout on success. No `--json` flag on create, but the URL can be captured directly.

#### GitLab (`glab mr create`)

```bash
glab mr create --title "Add retry logic" \
  --description "Implements exponential backoff for HTTP client" \
  --target-branch main \
  --source-branch abenz/add-retry-logic \
  --yes
```

Key flags:
- `--title`, `--description` (not `--body`), `--target-branch` (not `--base`), `--source-branch` (not `--head`)
- `--draft` — create as draft
- `--fill` — auto-populate from commits
- `--label`, `--assignee`, `--reviewer` — metadata
- `--push` — auto-push before creating
- `--auto-merge` — enable auto-merge on creation
- `--yes` — skip confirmation prompt

Output: prints the MR URL to stdout on success.

**Notable differences:**
| Concept | gh flag | glab flag |
|---------|---------|-----------|
| PR/MR body | `--body` | `--description` |
| Target branch | `--base` | `--target-branch` |
| Source branch | `--head` | `--source-branch` |
| Skip prompts | (not needed w/ flags) | `--yes` |

### CI Status Polling

#### GitHub (`gh pr checks`)

```bash
# One-shot JSON check
gh pr checks 42 --json name,state,bucket,workflow,completedAt

# Blocking watch mode
gh pr checks 42 --watch --fail-fast --interval 30
```

JSON fields: `bucket`, `completedAt`, `description`, `event`, `link`, `name`, `startedAt`, `state`, `workflow`

The `bucket` field categorizes raw `state` into: `pass`, `fail`, `pending`, `skipping`, `cancel`.

Exit codes:
- `0` — all checks passed
- `1` — one or more checks failed
- `8` — checks still pending

Example JSON output:
```json
[
  {
    "name": "CI / test (ubuntu-latest)",
    "state": "SUCCESS",
    "bucket": "pass",
    "workflow": "CI",
    "completedAt": "2026-03-20T10:15:00Z",
    "link": "https://github.com/org/repo/actions/runs/12345"
  },
  {
    "name": "CI / lint",
    "state": "FAILURE",
    "bucket": "fail",
    "workflow": "CI",
    "completedAt": "2026-03-20T10:14:30Z",
    "link": "https://github.com/org/repo/actions/runs/12345"
  }
]
```

**For muster:** Use `gh pr checks <number> --json bucket,name,state,link` for programmatic polling. Check exit code 0 (pass), 1 (fail), 8 (pending). The `--watch` mode is useful but muster should control its own poll loop for better UX integration (spinners, status updates).

#### GitLab (`glab ci status`)

```bash
# One-shot JSON
glab ci status --branch abenz/add-retry-logic --output json

# Live watch mode
glab ci status --branch abenz/add-retry-logic --live
```

Key flags:
- `--branch` / `-b` — target branch
- `--output json` / `-F json` — JSON output
- `--live` — blocking watch mode (incompatible with `--output json`)
- `--compact` — condensed display

Note: `--live` and `--output json` are mutually exclusive. For programmatic polling, use repeated `--output json` calls.

**For muster:** Poll with `glab ci status -b <branch> -F json` at intervals. Parse pipeline status from JSON. The JSON schema is less documented than gh's — test empirically.

### PR/MR Status Checking

#### GitHub (`gh pr view`)

```bash
gh pr view 42 --json state,mergeStateStatus,mergeable,isDraft,statusCheckRollup,url,number,headRefName,reviewDecision
```

Key JSON fields for lifecycle monitoring:
- `state`: `OPEN`, `CLOSED`, `MERGED`
- `mergeStateStatus`: `BEHIND`, `BLOCKED`, `CLEAN`, `DIRTY`, `DRAFT`, `HAS_HOOKS`, `UNKNOWN`, `UNSTABLE`
- `mergeable`: `MERGEABLE`, `CONFLICTING`, `UNKNOWN`
- `isDraft`: boolean
- `reviewDecision`: `APPROVED`, `CHANGES_REQUESTED`, `REVIEW_REQUIRED`, `null`
- `statusCheckRollup`: array of check status objects
- `url`: PR URL
- `number`: PR number
- `headRefName`: branch name

Example JSON:
```json
{
  "state": "OPEN",
  "mergeStateStatus": "CLEAN",
  "mergeable": "MERGEABLE",
  "isDraft": false,
  "reviewDecision": "APPROVED",
  "number": 42,
  "url": "https://github.com/org/repo/pull/42",
  "headRefName": "abenz/add-retry-logic"
}
```

#### GitLab (`glab mr view`)

```bash
glab mr view 42 --output json
```

Key fields (from GitLab API, exposed through glab):
- `state`: `opened`, `closed`, `merged`, `locked`
- `merge_status`: `can_be_merged`, `cannot_be_merged`, `checking`, `unchecked`
- `draft`: boolean
- `web_url`: MR URL
- `iid`: MR number
- `source_branch`: branch name
- `pipeline.status`: `running`, `pending`, `success`, `failed`, `canceled`, `skipped`

Note: glab's `--output json` returns the full GitLab API MR object. Field names use snake_case (GitLab API convention) vs camelCase (GitHub GraphQL convention).

### Merge Confirmation

#### GitHub (`gh pr merge`)

```bash
# Squash merge with branch cleanup
gh pr merge 42 --squash --delete-branch --body "Merge add-retry-logic"

# Enable auto-merge (merges when checks pass)
gh pr merge 42 --squash --auto

# Disable auto-merge
gh pr merge 42 --disable-auto
```

Key flags:
- `--merge`, `--rebase`, `--squash` — merge strategy (mutually exclusive)
- `--delete-branch` / `-d` — delete local and remote branch after merge
- `--auto` — enable auto-merge once requirements met
- `--admin` — bypass branch protection rules
- `--match-head-commit <SHA>` — safety check before merge
- `--body`, `--subject` — merge commit message

Merge queue support: if the branch requires a merge queue, `gh pr merge` automatically handles queueing. If checks haven't passed, `--auto` is enabled; if they have, the PR joins the queue.

#### GitLab (`glab mr merge`)

```bash
# Squash merge with branch cleanup
glab mr merge 42 --squash --remove-source-branch --yes

# With auto-merge
glab mr merge 42 --auto-merge --squash --yes
```

Key flags:
- `--squash` / `-s` — squash commits
- `--rebase` / `-r` — rebase commits
- `--remove-source-branch` / `-d` — delete source branch
- `--auto-merge` — merge when pipeline succeeds (enabled by default)
- `--sha` — verify HEAD matches before merge
- `--message` / `-m` — custom merge commit message
- `--yes` / `-y` — skip confirmation

**Notable differences:**
| Concept | gh flag | glab flag |
|---------|---------|-----------|
| Delete branch | `--delete-branch` | `--remove-source-branch` |
| Auto-merge | `--auto` | `--auto-merge` |
| Skip prompt | (non-interactive by default) | `--yes` |
| Merge commit msg | `--body` + `--subject` | `--message` |

### Authentication

#### GitHub (`gh auth`)

Authentication methods (in precedence order):
1. `GH_TOKEN` or `GITHUB_TOKEN` environment variable — takes precedence over stored credentials
2. `gh auth login` — stores credentials in system keyring or config file
3. `GH_ENTERPRISE_TOKEN` / `GITHUB_ENTERPRISE_TOKEN` — for GitHub Enterprise

Detection: `gh auth status` exits 0 if authenticated, 1 if not. Supports `--json` for structured output.

```bash
# Check auth (exit code based)
gh auth status
# Structured check
gh auth status --json user,token --hostname github.com
```

Config location: `~/.config/gh/hosts.yml` (or system keyring on macOS/Windows).

#### GitLab (`glab auth`)

Authentication methods:
1. `GITLAB_TOKEN`, `GITLAB_ACCESS_TOKEN`, or `OAUTH_TOKEN` environment variable
2. `glab auth login` — interactive login, stores in config
3. `GITLAB_HOST` — specifies target GitLab instance

Detection: `glab auth status` checks auth for the current instance.

```bash
# Check auth
glab auth status
# Show token
glab auth status --show-token
```

Config location: `~/.config/glab-cli/config.yml`.

### Error Handling Patterns

Both CLIs follow standard conventions:

**gh exit codes:**
- `0` — success
- `1` — general failure
- `2` — cancelled
- `4` — authentication required
- `8` — checks pending (gh pr checks only)

**glab exit codes:**
- `0` — success
- `1` — general failure
- Less documented than gh; rely on stderr parsing for specific errors

**Common failure modes to handle:**
1. **Not authenticated** — exit code 4 (gh) or stderr contains "auth" / "token" (glab)
2. **PR/MR not found** — exit code 1 with "not found" in stderr
3. **Network error** — exit code 1 with connection error in stderr
4. **Branch protection / merge conflict** — merge commands fail with descriptive stderr
5. **No CI checks configured** — `gh pr checks` returns empty with exit 0; handle as "no checks to wait for"

**Recommendation:** Capture both stdout and stderr from all CLI invocations. Parse stderr for error classification. Use exit codes as primary signal, stderr content as secondary.

### Detecting Which CLI to Use

The `merge_strategy` field in `.muster/config.yml` determines which CLI:
- `github-pr` → use `gh`
- `gitlab-mr` → use `glab`
- `direct` → no CLI needed (muster out is unnecessary)

For auto-detection (useful for `muster doctor`):
```bash
# Check if in a GitHub repo
gh repo view --json url 2>/dev/null

# Check if in a GitLab repo
glab repo view 2>/dev/null
```

Or inspect the git remote URL:
```bash
git remote get-url origin
# github.com → gh
# gitlab.com or self-hosted GitLab → glab
```

## Recommendations

### 1. Create an internal/vcs package with a common interface

```go
// internal/vcs/vcs.go
type PRStatus struct {
    Number    int
    URL       string
    State     string    // "open", "closed", "merged"
    Branch    string
    Mergeable bool
    Draft     bool
    CIStatus  CIStatus
    Review    ReviewStatus
}

type CIStatus string
const (
    CIStatusPending CIStatus = "pending"
    CIStatusPassing CIStatus = "passing"
    CIStatusFailing CIStatus = "failing"
    CIStatusNone    CIStatus = "none"
)

type ReviewStatus string
const (
    ReviewApproved        ReviewStatus = "approved"
    ReviewChangesRequired ReviewStatus = "changes_requested"
    ReviewPending         ReviewStatus = "review_required"
    ReviewNone            ReviewStatus = "none"
)

type CheckResult struct {
    Name   string
    Status string // pass, fail, pending, skipping, cancel
    URL    string
}

type VCS interface {
    CreatePR(opts PRCreateOpts) (url string, err error)
    ViewPR(ref string) (*PRStatus, error)
    ListChecks(ref string) ([]CheckResult, error)
    MergePR(ref string, opts MergeOpts) error
    CheckAuth() error
}
```

### 2. Implement GitHub and GitLab backends

Each backend wraps the respective CLI with `exec.Command`, parses JSON output, and maps to the common interface. This follows the same pattern as `internal/docker/` which shells out to `docker compose`.

### 3. Use JSON output for all queries, never parse table output

Both CLIs support structured JSON. Always use:
- `gh ... --json field1,field2` (selective fields)
- `glab ... --output json` (full object, filter in Go)

### 4. Implement muster's own poll loop rather than using --watch/--live

`gh pr checks --watch` and `glab ci status --live` are designed for human consumption. For muster:
- Poll at configurable intervals (default 30s, configurable in project config)
- Parse JSON on each poll
- Update TUI with current status (spinner + check list)
- Exit when all checks pass, fail, or timeout

### 5. Auth detection in muster doctor

```go
// For merge_strategy: github-pr
func checkGHAuth() error {
    cmd := exec.Command("gh", "auth", "status")
    if err := cmd.Run(); err != nil {
        return fmt.Errorf("gh not authenticated: run 'gh auth login'")
    }
    return nil
}
```

`muster doctor` should check:
- `gh`/`glab` binary exists on PATH
- Auth is configured for the current repo's host
- Report as soft requirement (warning, not error)

### 6. PR number extraction from URL

The `pr_url` field in roadmap items stores the full URL. To extract the PR/MR number:
```go
// "https://github.com/org/repo/pull/42" → "42"
// "https://gitlab.com/org/repo/-/merge_requests/42" → "42"
func extractPRNumber(url string) (string, error) {
    parts := strings.Split(strings.TrimRight(url, "/"), "/")
    return parts[len(parts)-1], nil
}
```

Both `gh` and `glab` accept either a number or URL as the PR/MR identifier, so passing the full URL directly also works.

### 7. Cross-platform considerations

Both `gh` and `glab` are cross-platform Go binaries. No platform-specific handling needed beyond `exec.Command` patterns already established in the codebase. Use `exec.LookPath` to check binary availability (same as how the codebase checks for `git`).

## Open Questions

### 1. Should muster support merge queue workflows?
- **Why it matters:** GitHub merge queues change the merge flow — `gh pr merge` may enqueue rather than merge directly. Muster needs to decide if it monitors queue status or just enables auto-merge and exits.
- **What we found:** `gh pr merge` handles queues transparently. If checks haven't passed, auto-merge is enabled; if they have, the PR joins the queue. The `--admin` flag bypasses the queue.
- **Recommendation:** Support merge queues by default via `gh pr merge --auto`. Don't try to monitor queue position — it's too complex and GitHub handles it automatically.

### 2. What is glab's exact JSON schema for ci status and mr view?
- **Why it matters:** The glab JSON schema is less documented than gh's. Field names and structure may vary between glab versions.
- **What we tried:** GitLab documentation site serves JS-rendered pages that couldn't be fetched. The raw docs describe flags but not JSON schemas.
- **Recommendation:** Test empirically against a real GitLab repo and pin to glab version >= 1.36 (latest stable). The JSON output mirrors the GitLab REST API responses, so the GitLab API docs can serve as a reference.

### 3. How should muster handle PR reviews in the out workflow?
- **Why it matters:** `muster out --wait` blocks until merge, but human review approval is outside muster's control. The design doc says "wait for merge approval" but doesn't specify if muster should prompt or just poll.
- **What we found:** `gh pr view --json reviewDecision` returns `APPROVED`, `CHANGES_REQUESTED`, `REVIEW_REQUIRED`, or `null`. `glab mr view --output json` includes `approved_by` array.
- **Recommendation:** In `--wait` mode, display review status alongside CI status. When review is `CHANGES_REQUESTED`, exit the poll loop and report (the AI fix loop only applies to CI failures, not review feedback). When `REVIEW_REQUIRED`, continue polling.

### 4. Should the poll interval be configurable per-project?
- **Why it matters:** Some CI pipelines take 2 minutes, others take 30+. A 10s interval is wasteful for long pipelines.
- **Recommendation:** Default 30s, configurable in `.muster/config.yml` under a new `out:` section. The `gh pr checks` default is 10s which is aggressive for programmatic use.

## References

- **gh CLI manual:** https://cli.github.com/manual (authoritative, all commands documented)
- **gh exit codes:** `gh help exit-codes` — 0 success, 1 failure, 2 cancelled, 4 auth required, 8 checks pending
- **gh environment variables:** `gh help environment` — `GH_TOKEN`, `GITHUB_TOKEN`, `GH_HOST`, `GH_ENTERPRISE_TOKEN`
- **glab CLI docs:** https://gitlab.com/gitlab-org/cli/-/tree/main/docs/source (raw markdown, JS-rendered site)
- **glab auth:** `GITLAB_TOKEN`, `GITLAB_ACCESS_TOKEN`, `OAUTH_TOKEN` environment variables
- **Existing exec.Command patterns:** `internal/docker/worktree.go:58`, `internal/ai/invoke.go:103`, `cmd/code.go:139`
- **Roadmap item PR URL field:** `internal/roadmap/roadmap.go:94`, `internal/ui/roadmap_format.go:27`
- **Design doc Phase 4:** `docs/design.md` lines 501-508 — `internal/git/pr.go` for gh/glab integration
- **Merge strategy config:** `docs/design.md` lines 137, 81-82 — `merge_strategy: github-pr | gitlab-mr | direct`
