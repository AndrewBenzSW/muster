# muster CLI — Consolidated Roadmap Workflow Tool

## Context

The abenz-workflow plugin has grown into a capable but fragmented system: ~1900 lines of shell scripts orchestrating Docker containers, 9 Claude Code skills (some maintaining two variants for interactive vs container use), Dockerfiles, proxy configs, and lifecycle hooks. The experience of using it feels incoherent — you invoke skills for some things, run shell scripts for others, and the boundary between "workflow session" and "general coding session" is blurry.

**muster** consolidates everything into a single Go CLI that owns the full workflow lifecycle. Shell scripts become Go code. Skill prompts become embedded templates staged at runtime. The two-variant maintenance burden (interactive teams vs container foreground agents) collapses into a single template set with conditional blocks. The result is one tool, one install, one coherent interface.

**Reference implementation**: The existing abenz-workflow plugin at `~/work/claude/abenz-workflow` is the source of truth for all behavior being ported. Shell scripts, prompt files, Docker configs, and skill variants in that repo should be consulted when implementing any feature in muster.

## Why "muster"?

The name **muster** maps naturally to every phase of the workflow:

- **"Muster" as a verb** — gather, assemble, dispatch. The workflow gathers roadmap items, assembles a plan, and dispatches agents to execute it.
- **"Pass muster"** — the verify and review gates are literally about whether work passes muster. This idiom is baked into the tool's core loop.
- **Military muster** — roll call before a mission. Fits the batched, sortie-like execution model where items are selected, checked, and sent out.
- **Ergonomics** — 6 characters, no ambiguity, easy tab-completion. Reads well as a CLI prefix.

### Why "muster in" and "muster out"?

"Muster in" and "muster out" are real terms:

- **Muster in** = enlist, bring into active service. `muster in <slug>` brings a roadmap item into active development — creating its workspace, planning it, building it, reviewing it, and shipping it. The full lifecycle of "mustering in" a feature.
- **Muster out** = discharge, complete service. `muster out <slug>` discharges a completed item — merging it, tagging the release, cleaning up the worktree. The feature has served its purpose and is released.

This creates an intuitive mental model: items enter the pipeline with `muster in` and leave it with `muster out`. The `--next` flag on `muster in` makes the pipeline a state machine — each call advances the item one step closer to being ready to muster out.

## Command Surface

```
# The Pipeline
muster in                         # Pick items interactively, run full pipeline
muster in <slug>                  # Full pipeline for one item
muster in --all                   # Batch: all eligible items
muster in <slug> --next           # Just the next step, then stop
muster in <slug> --step <phase>   # Run a specific phase (plan, execute, verify, review, prepare, finish)
muster in --resume                # Resume in-progress items
muster in --retry                 # Retry failed steps

# Completing Work (post-PR/MR lifecycle)
muster out [slug]                 # Monitor CI, ensure merge, pull latest, cleanup worktree
muster out [slug] --no-fix        # Don't auto-fix CI failures
muster out [slug] --wait          # Block until PR/MR is merged (poll CI status)

# Standalone Planning
muster plan [slug]                # Research -> synthesis -> implementation plan (pre-planning supported)

# Roadmap Management
muster add <description>          # AI-assisted: generates title, slug, priority; adds to README + .muster/roadmap.json
muster sync                       # Sync README <-> .muster/roadmap.json (AI-assisted fuzzy matching)
muster status                     # Show all items (includes container status per item)
muster status <slug>              # Detailed status for one item

# Container Management
muster down [slug]                # Tear down dev-agent container for a slug (defaults to current project)
muster down --all                 # Tear down all containers for current project
muster down --orphans             # Find and tear down containers with no active work item
muster down --project <name>      # Target a different project

# Tools
muster code                       # Spawn interactive coding agent with workflow skills staged
muster code --yolo                # ...in a sandboxed dev-agent container
muster code --tool opencode       # Use OpenCode instead of Claude Code
muster code --no-plugin           # Start bare (no workflow skills)
muster init                       # Auto-detect tooling, generate .muster/ structure
muster doctor [--fix]             # Health check (and auto-fix) project setup
muster doctor --config            # Show fully resolved (tool, provider, model) per step with source layer
muster doctor --models            # Validate provider/model combinations are sensible
muster help [topic]               # Reference docs + pipeline diagnostics
muster update                     # Self-update from GitHub Releases
```

### Key Behaviors

- **`muster in --next`**: State machine. Reads `.muster/work/{slug}/checkpoint.json` to determine current position, runs the next step, writes checkpoint, stops. Calling `--next` repeatedly walks through the full pipeline one step at a time.
- **`muster plan`**: Top-level because planning can happen before `muster in` (pre-planning). When `muster in` detects `.muster/work/{slug}/plan/implementation-plan.md` exists, it skips the plan step.
- **`muster code`**: Always stages embedded skill templates to a temp dir and passes `--plugin-dir` to the agent. This fully replaces the plugin for interactive use.
- **`muster in --all`**: Replaces `roadmap-loop.sh`. Supports parallel execution with claim locks. Picks items by priority (high > medium > lower) and status (in_progress if --resume > planned > pending).
- **Interactive detection**: Auto-detect TTY. Interactive mode shows tables, spinners, color. Non-interactive mode outputs JSON to stdout.
- **`muster out`**: Only meaningful when `merge_strategy` is `github-pr` or `gitlab-mr`. Handles the post-PR lifecycle: monitors CI checks, optionally pushes fixes for failures, waits for merge, pulls latest main, cleans up the worktree and roadmap entry. When `merge_strategy: direct`, the pipeline's `finish` step handles the merge directly and `muster out` is unnecessary.
- **`muster doctor`**: Dependency tiers — **hard requirements**: `git` and at least one of `claude`/`opencode`. **Soft requirements**: `docker` (only for `--yolo` container mode), `gh` (only for `merge_strategy: github-pr`), `glab` (only for `merge_strategy: gitlab-mr`). Doctor reports all tiers but only errors on hard requirements.

## Configuration

Two config layers:

### User config (`~/.config/muster/config.yml`)

Defines available tools, their providers, model tiers, and the user's defaults. Each step in the pipeline is fully described by a triple: **tool + provider + model**.

```yaml
default:
  tool: claude
  provider: max           # which provider to use when not specified
  model: deep             # which tier to use when no model is specified

tools:
  claude:
    models:
      fast: haiku
      standard: sonnet
      deep: opus
    providers:
      anthropic:                        # Anthropic API (direct)
        api_key_env: ANTHROPIC_API_KEY
      bedrock: {}                       # auto-detected from AWS config
      max: {}                           # auto-detected from ~/.claude/.credentials.json
      openrouter:
        api_key_env: OPENROUTER_API_KEY
      local:
        base_url: http://localhost:1234/v1

  opencode:
    models:
      fast: gemma3:4b
      standard: qwen3:32b
      deep: qwen3:235b
    providers:
      openrouter:
        api_key_env: OPENROUTER_API_KEY
      local:
        base_url: http://localhost:11434
```

Providers with `{}` are auto-detected from existing host configuration. Providers with `api_key_env` reference an environment variable name (not the key itself). Providers with `base_url` point to a locally running inference server.

### Project config (`.muster/config.yml` + `.muster/dev-agent/config.yml`)

Project config is split into two concerns: **workflow** (how the pipeline runs) and **container environment** (how the sandbox is configured). Each file supports a `.local.yml` override that is gitignored. Both pairs are deep-merged: local values override at the field level, missing fields fall through. **Lists are replaced, not appended** — if a local file specifies `allowed_domains`, it replaces the entire list.

Models can reference tiers via `muster-fast`, `muster-standard`, `muster-deep` (resolved from the user config for the step's tool) or use literal model names.

**`.muster/config.yml`** — Pipeline and workflow:

```yaml
merge_strategy: direct  # or: github-pr, gitlab-mr

lifecycle:
  setup: .muster/dev-agent/scripts/setup.sh
  check: .muster/dev-agent/scripts/check.sh
  verify: .muster/dev-agent/scripts/verify.sh
  teardown: .muster/dev-agent/scripts/teardown.sh

# Per-step tool, provider, and model configuration
# Each field inherits from `defaults` unless overridden
pipeline:
  defaults:
    tool: claude
    provider: max
    model: muster-standard        # resolves via user config for the tool
  plan:
    provider: anthropic           # same tool (claude), different provider
    model: muster-deep            # resolves to opus (claude's deep tier)
  execute:
    provider: local               # claude + local provider
    model: qwen3:32b              # literal model name for local inference
  review:
    provider: anthropic
    model: muster-deep
  add:
    model: muster-fast            # inherits tool + provider from defaults
  # verify, prepare, finish don't invoke AI — no tool/model/provider needed
```

**`.muster/dev-agent/config.yml`** — Container/sandbox environment:

```yaml
allowed_domains:
  - .npmjs.org
env:
  DATABASE_URL: "postgres://..."
volumes:
  - "path:/container/path:ro"
networks:
  - external-network
```

### Step Resolution

Each step resolves a `(tool, provider, model)` triple. Project config is the deep-merge of `.muster/config.local.yml` over `.muster/config.yml` — local overrides win at the field level.

1. **Determine tool**: step > `pipeline.defaults` > `user.default.tool`
2. **Determine provider**: step > `pipeline.defaults` > `user.default.provider`
3. **Determine model string**: step > `pipeline.defaults` > `user.default.model`
4. **Resolve model string**:
   - If it starts with `muster-` (e.g., `muster-deep`): look up the tier (`deep`) in `user.tools[tool].models` to get the concrete model name
   - Otherwise: use the literal model name as-is (e.g., `opus`, `qwen3:32b`)
5. **Resolve provider auth**: look up the provider in `user.tools[tool].providers` to get auth config (API key env var, base URL, or auto-detect)

This enables powerful combinations: Claude Code with Anthropic's hosted Opus for planning, Claude Code with a locally-running Qwen for implementation, and Claude Code with OpenRouter for review — all in the same pipeline, all configured per-project.

## Architecture

### Project Structure

```
muster/
├── main.go
├── cmd/                          # Cobra command definitions
│   ├── root.go                   # Global flags, TTY detection
│   ├── in.go                     # Pipeline command
│   ├── out.go                    # Complete/discharge command
│   ├── plan.go                   # Standalone planning
│   ├── add.go, sync.go, status.go  # Roadmap management
│   ├── code.go                   # Spawn interactive agent
│   ├── init.go, doctor.go        # Project setup/health
│   ├── help.go, update.go        # Reference + self-update
│   └── version.go                # Print CLI version
├── internal/
│   ├── config/                   # Parse both user + project config
│   ├── roadmap/                  # Load/save .muster/roadmap.json, item selection, priority sorting
│   ├── pipeline/                 # State machine, checkpoint, step implementations, locks, timing
│   ├── docker/                   # Compose generation, auth detection, container lifecycle
│   ├── prompt/                   # go:embed, template resolution, staging to temp dir
│   ├── git/                      # Worktree, merge, tag, PR/MR creation
│   ├── version/                  # Semver parsing, CHANGELOG promotion, bump logic
│   └── ui/                       # TTY detection, tables, spinners, color, JSON mode, interactive picker
├── prompts/                      # Source .md.tmpl files (embedded at compile time)
│   ├── plan-feature/             # SKILL.md.tmpl + 5 runner/prompt templates
│   ├── execute-plan/             # SKILL.md.tmpl + 2 runner/prompt templates
│   ├── review-implementation/    # SKILL.md.tmpl + 4 runner/prompt templates
│   └── simple/                   # add-item, sync, loop-setup, help templates
├── docker/                       # Embedded Docker assets
│   ├── agent.Dockerfile
│   ├── proxy.Dockerfile
│   ├── docker-compose.yml
│   ├── entrypoint.sh, init-firewall.sh
│   ├── squid.conf, allowed-domains.txt
│   └── settings.json, settings.max.json
├── scripts/
│   └── install.sh                # curl-pipe-bash installer
├── .goreleaser.yml
└── .github/workflows/
    ├── ci.yml                    # Test + lint on PR
    └── release.yml               # Build + release on tag
```

### Runtime Data Layout

All project-level muster data lives under `.muster/` at the repository root:

```
.muster/
├── config.yml                    # Pipeline/workflow config (merge strategy, step config, lifecycle)
├── config.local.yml              # Per-user overrides (gitignored)
├── roadmap.json                  # Roadmap items with status, PR/MR URLs
├── work/                         # Per-item working data
│   └── {slug}/
│       ├── plan/                 # Plan artifacts
│       │   ├── research/
│       │   ├── synthesis/
│       │   └── implementation-plan.md
│       ├── checkpoint.json       # Pipeline state
│       └── pipeline.log          # Pipeline run log
└── dev-agent/                    # Container/sandbox environment config
    ├── config.yml                # Allowed domains, env vars, volumes, networks
    ├── config.local.yml          # Per-user container overrides (gitignored)
    └── scripts/
        ├── setup.sh
        ├── check.sh
        ├── verify.sh
        └── teardown.sh
```

**What's committed vs gitignored**: `muster init` adds `config.local.yml` and `dev-agent/config.local.yml` to `.gitignore`. Everything else under `.muster/` is committed — config, roadmap, plans, and lifecycle scripts are shared with the team. Checkpoints and logs are ephemeral but harmless to commit; projects can gitignore them if preferred.

**Backward compatibility**: Muster checks for `.muster/` paths first, then falls back to legacy locations (`.roadmap.json`, `.plan/{slug}/`, `.dev-agent/config.yml`, `.pipeline-checkpoint`). `muster init --migrate` moves legacy files into the `.muster/` structure (e.g., `.plan/my-feature/` becomes `.muster/work/my-feature/plan/`). This fallback will be removed in a future major version.

**Roadmap item schema**:

```json
{
  "slug": "add-retry-logic",
  "title": "Add retry logic to HTTP client",
  "priority": "medium",
  "status": "in_progress",
  "context": "Description of why this item exists and what it should accomplish.",
  "pr_url": "https://github.com/org/repo/pull/42",
  "branch": "abenz/add-retry-logic"
}
```

`pr_url` is written by the pipeline's `finish` step when `merge_strategy` is `github-pr` or `gitlab-mr`. `muster out` reads it to know which PR/MR to monitor. `muster status` displays it. `branch` is written when the worktree is created.

### Prompt Template System

All prompt `.md` files from both skill variants are unified into `.md.tmpl` templates using Go's `text/template`. The CLI resolves them at runtime based on execution context.

**Template variables:**

```go
type PromptContext struct {
    Interactive  bool   // true = teams (TeamCreate/SendMessage), false = foreground agents
    Tool         string // "claude" or "opencode" — resolved for this step
    Provider     string // "anthropic", "bedrock", "max", "openrouter", "local"
    Model        string // concrete model name after tier resolution (e.g., "opus", "qwen3:32b")
    Slug         string // concrete slug, not {slug} placeholder
    WorktreePath string // concrete path
    MainRepoPath string // concrete path
    PlanDir      string // e.g., .muster/work/add-retry-logic/plan
    Models       struct {
        Fast     string // resolved concrete name for this step's tool's fast tier
        Standard string // resolved concrete name for this step's tool's standard tier
        Deep     string // resolved concrete name for this step's tool's deep tier
    }
}
```

**Example template block (from research-runner.md.tmpl):**

```
{{if .Interactive}}
Create a team named `plan-research`:
Use TeamCreate with team_name "plan-research". Spawn each researcher
with team_name "plan-research" and mode "bypassPermissions".
Monitor completion via SendMessage. When all agents complete,
use shutdown_request to close the team.
{{else}}
Spawn all research agents in a **single parallel tool call** using
the Agent tool. Each agent runs as a foreground agent (no team_name).
Wait for all to complete before proceeding.
{{end}}
```

**Staging flow:**

1. CLI resolves all templates for the target skill with `PromptContext`
2. Writes resolved `.md` files to temp dir structured as a Claude Code skill directory
3. For `muster code`: passes `--plugin-dir <tempdir>` to claude
4. For `muster in` (container): mounts temp dir into container at `/home/node/.claude/skills`
5. For `muster in` (local): passes `--plugin-dir <tempdir>` and `--prompt "Run /roadmap-plan-feature"`
6. Temp dir cleaned up after invocation completes

### Docker Integration

Replaces `run.sh` (540 lines of bash) with Go in `internal/docker/`:

- **Single multi-tool image**: One container image with both Claude Code and OpenCode installed. The CLI `exec`s whichever tool is configured for each pipeline step. Container stays up across steps — no restart between tool switches.
- **Auth detection**: The CLI scans all providers referenced by pipeline steps (across both tools) and injects the necessary auth into the container. Since the container has both tools installed and steps may use different tool+provider combos, all required auth is mounted at startup:
  - **Anthropic API**: Pass through the env var named in `api_key_env`.
  - **Bedrock**: Mount `~/.aws` read-only, set `AWS_PROFILE`/`AWS_REGION`/`CLAUDE_CODE_USE_BEDROCK=1`.
  - **Max**: Mount `~/.claude/.credentials.json` read-only.
  - **OpenRouter** (either tool): Pass through the env var named in `api_key_env`.
  - **Local provider** (either tool): Rewrite `base_url` for container context — `localhost` becomes `host.docker.internal` so the container can reach the host's LM Studio/Ollama/vLLM. The proxy allowlist and firewall rules are updated to permit host gateway access.
  - The CLI validates at startup that all referenced providers have valid auth config (e.g., the env var actually exists, the local URL is reachable). `muster doctor` also checks this.
- **Compose generation**: Start with embedded base `docker-compose.yml`. Layer overrides programmatically (auth, workspace config, proxy domains, worktree mounts, user's override file). Write single merged compose to `~/.cache/muster/{project}/docker-compose.yml`.
- **Asset extraction**: Docker assets embedded via `go:embed` are extracted to `~/.cache/muster/docker-assets/` on first use, versioned by content hash (auto-refreshes when binary updates).
- **Container lifecycle**: Start/stop/exec via `docker compose` CLI (not Docker SDK — compose handles service ordering, health checks, overlay merging).
- **Container labeling**: Every container started by muster gets Docker labels for discovery and management:
  ```yaml
  labels:
    muster.managed: "true"
    muster.project: "my-api"       # derived from project directory name
    muster.slug: "add-retry-logic" # omitted for interactive `muster code` sessions
  ```
  `muster down` queries `docker ps --filter label=muster.managed=true` to find containers, then filters by project/slug. `--orphans` cross-references running containers against `.muster/roadmap.json` to find containers whose slug is no longer in_progress. `muster status` uses these labels to show container state alongside each item.
- **Step invocation**: Each pipeline step resolves its tool + model from config, then execs the appropriate command in the running container (e.g., `claude --prompt "..." --model opus` or `opencode --prompt "..."`).

### Key Libraries

| Purpose | Library |
|---------|---------|
| CLI framework | `github.com/spf13/cobra` |
| YAML parsing | `gopkg.in/yaml.v3` |
| Terminal UI | `github.com/charmbracelet/lipgloss` + `huh` (picker) + `bubbles` (spinner) |
| Semver | `github.com/Masterminds/semver/v3` |
| Self-update | `github.com/creativeprojects/go-selfupdate` |
| Templates | `text/template` (stdlib) |
| Git, Docker | `os/exec` shelling out to `git` and `docker compose` |
| Testing | `github.com/stretchr/testify` (assertions + require) |

### Cross-Platform Support

Windows is explicitly in scope. The CLI must build and run correctly on macOS, Linux, and Windows. Key considerations:

- **Path handling**: Use `filepath.Join` and `filepath.Abs` everywhere. Never hardcode `/` as a separator or assume Unix-style paths. Template output that includes filesystem paths should use `filepath.ToSlash`/`filepath.FromSlash` as needed.
- **Shell execution**: `os/exec.Command` calls must not assume a Unix shell. For `git` and `docker compose`, invoke the binary directly with argument arrays — never wrap commands in `sh -c`. The `internal/docker/` and `internal/git/` packages must build command args as `[]string`, not shell strings.
- **TTY detection**: Use `golang.org/x/term.IsTerminal()` rather than checking for `/dev/tty`. This works across platforms.
- **Temp directories**: Use `os.MkdirTemp` — never hardcode `/tmp`. Prompt staging writes to the OS-appropriate temp location.
- **Docker Desktop**: On Windows, Docker Desktop provides `docker compose` as a CLI plugin the same way it does on Mac. The compose generation and container lifecycle code should work without modification, but CI should verify this.
- **Line endings**: Embedded prompt templates use LF (they're in the Go binary). Staged files written to disk should also use LF since Claude and OpenCode expect it, but this should be explicitly handled in the staging code.
- **File locking**: `internal/pipeline/lock.go` should use `os.OpenFile` with `os.O_CREATE|os.O_EXCL` for atomic lock creation (works cross-platform). PID-based stale lock detection should use `os.FindProcess` + signal 0 on Unix and `syscall.OpenProcess` on Windows, behind a build-tag abstraction.
- **CI**: GitHub Actions should run tests on `ubuntu-latest`, `macos-latest`, and `windows-latest`.

## Testing Strategy

Every phase includes automated tests as part of its deliverable. The project uses `testify` for assertions (`assert` for soft checks, `require` for hard stops) — it reads naturally and is the standard in the Go ecosystem.

### What to test

| Layer | Approach | Examples |
|-------|----------|----------|
| Config resolution | Table-driven unit tests | Step triple resolution, tier lookup, local override merging, missing fields fallback |
| Roadmap parsing | Table-driven unit tests | Load both array and wrapper `.muster/roadmap.json` formats, priority sorting, status transitions, PR URL field handling |
| Template rendering | Unit tests with golden files | Render each `.md.tmpl` with known `PromptContext`, compare output to checked-in `.golden` files |
| Version/changelog | Table-driven unit tests | Semver bump, CHANGELOG section promotion, edge cases (pre-release, no changelog) |
| Checkpoint | Unit tests | Write/read round-trip, resume from each step, corrupt checkpoint handling |
| Compose generation | Unit tests | Given config inputs, assert the generated YAML contains expected services, labels, volumes, env vars. No Docker required. |
| Git operations | Integration tests (real git repos) | Create temp repos with `t.TempDir()`, test worktree creation, merge, conflict detection. Real git, no mocks. |
| Docker lifecycle | Integration tests (optional) | Guarded by `if testing.Short() { t.Skip() }`. Only run in CI or with `-short=false`. Test container start/exec/stop with real Docker. |
| CLI commands | Integration tests | Use `exec.Command` to run the built binary with known fixtures. Assert exit codes and stdout/stderr. |

### Patterns

- **Table-driven tests** are the default. Each test case is a struct with a name, inputs, and expected output. Easy to add cases without writing new test functions.
- **Golden files** for template output: run tests with `-update` flag to regenerate `.golden` files, then review diffs in version control. Avoids brittle string assertions on multi-line template output.
- **`t.TempDir()`** for any test that touches the filesystem. Go cleans these up automatically.
- **No mocks for `git` and `docker compose`** — these are the core integrations and should be tested against real binaries where possible. Use temp repos and short-mode guards instead.
- **`testdata/` directories** in each package for fixtures (config files, roadmap JSON, template inputs).

### Phase test deliverables

- **Phase 0**: CI runs `go test ./...` on all three platforms (Linux, macOS, Windows). Test helpers and fixtures scaffolded.
- **Phase 1**: Config resolution tests (table-driven), template rendering tests (golden files).
- **Phase 2**: Compose generation tests (assert YAML output), auth detection tests (given env/files, assert injected config).
- **Phase 3**: Roadmap parsing + sorting tests, status transition tests.
- **Phase 4**: Git operation tests (temp repos), version bump tests, changelog tests.
- **Phase 5**: Plan command integration test (mock Claude invocation by stubbing the exec call, verify plan directory structure).
- **Phase 6**: Checkpoint round-trip tests, pipeline state machine tests, lock acquisition/release/stale detection tests.
- **Phase 7**: Doctor output tests (given available/missing tools, assert output and exit code).

## Error Handling & Logging

### Logging

Every `muster in` pipeline run writes a structured log to `.muster/work/{slug}/pipeline.log`. Each entry includes a timestamp, step name, and outcome. Steps that invoke external tools (Claude, git, docker) capture stdout/stderr to this log. The log is append-only — retries and resumes add to the same file.

```
2025-03-17T10:04:12Z [plan]     started  provider=anthropic model=opus
2025-03-17T10:06:45Z [plan]     completed  duration=2m33s
2025-03-17T10:06:46Z [execute]  started  provider=local model=qwen3:32b
2025-03-17T10:12:01Z [execute]  failed   exit_code=1 duration=5m15s
2025-03-17T10:12:01Z [execute]  stderr: "Error: connection refused to localhost:1234"
```

`muster status <slug>` reads this log to show the last step's outcome, duration, and error (if any). `--verbose` on any command increases log detail to include full command invocations.

### Error categories and behaviors

| Category | Example | Behavior |
|----------|---------|----------|
| **Missing hard dependency** | `git` not found | Exit immediately with install instructions. Don't attempt partial execution. |
| **Missing soft dependency** | `docker` not found, but running `muster in` (non-yolo) | Continue normally. Only error when a command actually needs the missing tool. |
| **Container failure** | Docker not running, image build fails, exec fails | Log the error, leave container running for debugging (`muster down` cleans up explicitly). Write checkpoint so `--retry` can resume. |
| **AI produced no output** | `muster plan` ran but no `implementation-plan.md` appeared | Retry up to 2 times with an explicit "your previous attempt produced no output" nudge. Then fail with the log path for manual inspection. |
| **External CLI failure** | `gh pr create` fails (auth, network, etc.) | Log full stderr, suggest `muster doctor` for auth checks. Don't retry automatically — these are usually config issues. |
| **Stale lock** | `muster in --all` finds a lock with a dead PID | Detect via `os.FindProcess` + signal 0 (Unix) / `syscall.OpenProcess` (Windows). Remove stale lock, log a warning, and proceed. |
| **Checkpoint corruption** | `.muster/work/{slug}/checkpoint.json` has invalid JSON | Log warning, ask user to confirm reset to the last known-good step. Don't silently restart from the beginning. |
| **Auth failure** | API key env var missing, credentials file not found | Fail before starting the step, not mid-execution. `muster doctor --config` helps diagnose which layer is misconfigured. |

### Principles

- **Fail early, fail clearly.** Validate prerequisites (auth, tools, config) before starting work, not after 5 minutes of execution.
- **Never swallow errors.** Every error either recovers (with a logged warning) or halts (with a clear message and the log path).
- **Actionable messages.** Every error message should tell the user what to do next: run `muster doctor`, check the log at a specific path, or retry with a specific flag.
- **Leave state inspectable.** On failure, don't clean up the container or worktree. The user (or `--retry`) needs that state. `muster down` is the explicit cleanup command.

## Implementation Phases

Depth-first: each phase produces a working, testable increment.

### Phase 0: Project Bootstrap
- Go module, directory structure, Makefile (`build`, `test`, `lint`, `install`)
- Cobra root command with global flags (`--format`, `--verbose`)
- TTY detection + JSON/table output mode in `internal/ui/`
- `go:embed` all prompt templates and Docker assets
- GitHub Actions CI (test + lint on Linux, macOS, and Windows)
- GoReleaser config for cross-platform builds (macOS, Linux, Windows)
- Test helpers and `testdata/` fixture scaffolding
- **Deliverable**: `muster version` works, builds and passes tests on all three platforms

### Phase 1: `muster code` — Validate prompt staging
- `internal/prompt/`: embed, template resolution, stage to temp dir
- `internal/config/`: parse user config (`~/.config/muster/config.yml`) for model tiers and default tool
- `cmd/code.go`: stage skills, invoke `claude --plugin-dir <tempdir>` (or opencode)
- Flags: `--tool`, `--no-plugin`
- `--yolo` stubbed with helpful error pointing to Phase 2
- Table-driven tests for config resolution (step triple lookup, tier resolution, fallback chains)
- Golden file tests for template rendering (render each `.md.tmpl`, compare to checked-in `.golden` files)
- **Deliverable**: `muster code` launches Claude with all workflow skills available. Config and template systems are tested.

### Phase 2: `muster code --yolo` + `muster down` — Docker container orchestration
- `internal/docker/auth.go`: Bedrock/Max detection, OpenCode provider auth resolution
- `internal/docker/compose.go`: compose file generation from embedded base + overrides, container labeling (`muster.managed`, `muster.project`, `muster.slug`)
- `internal/docker/worktree.go`: git worktree detection for volume mounts
- `internal/docker/container.go`: start/stop/exec/isRunning/list (query by labels)
- `internal/config/`: parse project config (`.muster/config.yml` + `config.local.yml` merge, `.muster/dev-agent/config.yml` + its local override)
- `cmd/code.go`: wire `--yolo`, `--main-branch`, `--current-path`
- `cmd/down.go`: tear down containers by slug, `--all`, `--orphans`, `--project`
- **Deliverable**: `muster code --yolo` boots a labeled container. `muster down` manages container teardown. Full replacement of `run.sh` for interactive use.

### Phase 3: `muster status` + `muster add` + `muster sync` — Roadmap management
- `internal/roadmap/`: load/save `.muster/roadmap.json` (both array and wrapper formats, with backward compat for `.roadmap.json`), item struct with `pr_url` and `branch` fields, priority sorting, status transitions
- `cmd/status.go`: table view (all items, showing PR URL when present) + detail view (one slug) + JSON mode
- `cmd/add.go`: invoke AI (fast model) to generate title/slug/priority, add to README + `.muster/roadmap.json`, commit
- `cmd/sync.go`: stage sync prompt, invoke Claude for fuzzy matching, commit results
- `internal/ui/picker.go`: interactive item selector (reused by `in`, `out`, `plan`)
- **Deliverable**: Full roadmap CRUD without needing the pipeline.

### Phase 4: `muster out` — Post-PR/MR lifecycle + git operations
- `internal/git/`: worktree create/remove/list, squash merge, conflict detection, version tagging
- `internal/git/pr.go`: GitHub PR / GitLab MR creation (shells to `gh`/`glab`), CI status polling, merge confirmation
- `internal/version/`: CHANGELOG parsing, semver bump, release section promotion
- `cmd/out.go`: post-PR lifecycle — monitor CI checks, optionally push fixes for CI failures (with Claude), wait for merge, pull latest main, clean up worktree + roadmap entry. Only meaningful when `merge_strategy` is `github-pr` or `gitlab-mr`.
- Flags: `--no-fix`, `--wait`
- Git operation tests against real temp repos, version bump and changelog tests (table-driven)
- **Deliverable**: `muster out` handles the full post-PR lifecycle. Git and version packages are tested.

### Phase 5: `muster plan` — Standalone planning
- `cmd/plan.go`: resolve slug (from args or interactive picker), stage plan-feature templates, invoke Claude, verify plan output exists
- Handle pre-planning (no worktree yet — work in a temp dir or main branch)
- **Deliverable**: `muster plan [slug]` runs research/synthesis/planning, produces `.muster/work/{slug}/plan/implementation-plan.md`

### Phase 6: `muster in` — The full pipeline
- `internal/pipeline/`: state machine, step definitions, checkpoint read/write (`.muster/work/{slug}/checkpoint.json`), step implementations, timing, summary generation
- `internal/pipeline/lock.go`: filesystem locks with stale PID detection
- `cmd/in.go`: full orchestration with all flags
- Pipeline steps ported from `roadmap-implement.sh`:
  - **worktree**: `git worktree add`, write `branch` to roadmap item, copy pre-existing plan from main
  - **setup**: boot container, run lifecycle.setup hook
  - **plan**: invoke Claude with plan-feature templates (skip if `.muster/work/{slug}/plan/implementation-plan.md` exists)
  - **execute**: invoke Claude with execute-plan templates
  - **verify**: run lifecycle.verify, up to 3 Claude fix attempts
  - **review**: invoke Claude with review-implementation templates
  - **post-review-verify**: verify again after review fixes
  - **prepare**: merge main, version bump, changelog, roadmap cleanup, commit
  - **finish**: when `merge_strategy: direct`, squash merge into main. When `github-pr` or `gitlab-mr`, create the PR/MR and write `pr_url` to the roadmap item (post-PR monitoring is handled by `muster out`).
- `--all` mode: parallel batch execution with claim locks (replaces `roadmap-loop.sh`)
- `--next`: run one step and stop
- `--step <phase>`: run specific phase
- `--resume` / `--retry`: checkpoint-based recovery
- **Deliverable**: Complete pipeline. Full replacement of `roadmap-implement.sh` and `roadmap-loop.sh`.

### Phase 7: `muster init` + `muster doctor` + `muster help` + `muster update`
- `cmd/init.go`: stage loop-setup prompt, invoke Claude to auto-detect tooling and generate `.muster/` structure. `--migrate` moves legacy files (`.roadmap.json`, `.plan/`, `.dev-agent/`, `.pipeline-checkpoint`) into `.muster/`.
- `cmd/doctor.go`: tiered health checks — **hard requirements** (`git`, at least one of `claude`/`opencode`) cause exit 1 if missing; **soft requirements** (`docker`, `gh`, `glab`) show warnings but pass. `--fix` auto-remediates what it can. `--config` shows the fully resolved step config with source annotations (which layer each value came from). `--models` validates provider/model combinations and reports mismatches (e.g., local provider with a cloud-only model name).
- `cmd/help.go`: embedded reference docs, topic-based lookup, pipeline failure diagnostics
- `cmd/update.go`: self-update from GitHub Releases via go-selfupdate
- **Deliverable**: Full CLI surface complete.

### Phase 8: Polish + Release
- Shell completion generation (cobra built-in: bash, zsh, fish, powershell)
- `scripts/install.sh`: detect OS/arch, download latest release, install to `~/.local/bin` (Mac/Linux)
- `scripts/install.ps1`: PowerShell installer for Windows
- README with usage examples
- First tagged release (v0.1.0)
- Parity testing against existing shell scripts
- Manual verification on Windows (Docker Desktop, Git for Windows, Claude Code)

## Verification

Each phase has both automated tests (see Testing Strategy) and manual smoke tests:

- **Phase 0**: `go build ./...`, `go test ./...` green on Linux, macOS, Windows. CI green.
- **Phase 1**: Automated: config resolution and template golden file tests pass. Manual: `muster code` launches Claude with skills; verify skills appear via `/help` in the Claude session.
- **Phase 2**: Automated: compose generation and auth detection tests. Manual: `muster code --yolo` boots container; verify firewall blocks direct internet; verify proxy works; verify skills are available inside container; verify `docker ps --filter label=muster.managed=true` shows the container; `muster down` tears it down cleanly.
- **Phase 3**: Automated: roadmap parsing, sorting, status transition tests. Manual: `muster status` shows items; `muster add "test feature"` adds to both files; `muster sync` reconciles differences.
- **Phase 4**: Automated: git operations against temp repos, version bump, changelog promotion. Manual: create a test branch with changes, run `muster out`, verify CI monitoring and cleanup.
- **Phase 5**: Automated: plan command integration test with stubbed AI invocation. Manual: `muster plan test-slug` produces `.muster/work/test-slug/plan/implementation-plan.md`.
- **Phase 6**: Automated: checkpoint round-trip, state machine transitions, lock acquisition/stale detection. Manual: `muster in test-slug` runs full pipeline end-to-end on a test project with `.muster/` config.
- **Phase 7**: Automated: doctor output tests given available/missing tools. Manual: `muster doctor` reports health; `muster update` checks for new version.
- **Phase 8**: Install script works on fresh Mac, Linux, and Windows; `muster` shows help with all commands.

## Migration Strategy

During development, the existing abenz-workflow plugin continues to work. After Phase 6 is complete and the pipeline is verified:

1. Run both systems in parallel on a real project, comparing outcomes
2. Once confidence is high, stop maintaining the plugin's container skill variants
3. Archive the abenz-workflow repository after muster reaches v1.0

## Future: Muster Station

Out of scope for this CLI work, but noted for architectural awareness: the plan is to build **Muster Station**, a web service that acts as a "vibe kanban" — a visual dashboard for managing and monitoring muster workflows across projects. The CLI should be designed with the understanding that a service layer will eventually sit on top of it, which reinforces decisions like JSON output mode, structured state files, and clean separation of concerns in the internal packages.
