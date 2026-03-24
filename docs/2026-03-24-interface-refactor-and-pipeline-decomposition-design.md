# Interface Refactor and Pipeline Decomposition

## Problem

Phase 6 ("full pipeline") is too large to build, test, and debug as a single unit. Additionally, muster's current architecture couples AI tool invocation and Docker orchestration to concrete implementations, making it impossible for a Claude agent (or any automated test harness) to run the full pipeline without real Claude Code and Docker dependencies.

The goal is twofold:
1. Introduce proper interfaces for coding tools and container runtimes so that a test harness can exercise the full pipeline with deterministic, file-based mock implementations.
2. Decompose Phase 6 into independently buildable and testable roadmap items.

## CodingTool Interface

### Package: `internal/tool/`

```go
type CodingTool interface {
    // Invoke runs a single-shot AI call: stage prompt, run tool, capture output.
    // Used by pipeline steps (plan, execute, review) and commands (add, sync).
    Invoke(ctx context.Context, cfg InvokeConfig) (*InvokeResult, error)

    // Launch starts an interactive session with skills staged.
    // Used by `muster code`. The session blocks until the user exits.
    Launch(ctx context.Context, cfg LaunchConfig) error

    // Name returns the tool identifier (e.g., "claude-code", "opencode", "testcode").
    Name() string

    // ContainerSetup returns the container requirements for this tool given a provider.
    // Used by compose generation to produce tool-specific Docker configuration.
    ContainerSetup(provider ProviderConfig) ContainerRequirements
}

type InvokeConfig struct {
    Model     string
    Prompt    string
    SkillsDir string            // pre-staged skills directory
    Env       map[string]string // provider env overrides (e.g., ANTHROPIC_BASE_URL)
    Timeout   time.Duration
    Verbose   bool
}

type InvokeResult struct {
    Output string
}

type LaunchConfig struct {
    Model     string
    SkillsDir string
    Env       map[string]string
    Verbose   bool
    NoPlugin  bool // --no-plugin flag for bare sessions
}

type ContainerRequirements struct {
    InstallCommands []string          // e.g., "npm install -g @anthropic-ai/claude-code"
    AuthMounts      []Mount           // e.g., ~/.claude/.credentials.json -> /home/node/.claude/
    AuthEnvVars     map[string]string // e.g., ANTHROPIC_API_KEY, CLAUDE_CODE_USE_BEDROCK
    ConfigFiles     []Mount           // e.g., settings.json -> appropriate location
    EntrypointCmd   string            // what the container runs to start the tool
}

type Mount struct {
    Source   string
    Target   string
    ReadOnly bool
}

type ProviderConfig struct {
    Name      string // "anthropic", "bedrock", "max", "openrouter", "local"
    APIKeyEnv string // env var name for API key (if applicable)
    BaseURL   string // for local providers
}
```

### Implementations

**`internal/tool/claudecode/`** - Translates `InvokeConfig` to `claude --print --plugin-dir <skillsDir> --model <model>` with stdin prompt. Translates `LaunchConfig` to `claude --plugin-dir <skillsDir>`. `ContainerSetup` returns Claude-specific auth mounts (Max credentials, AWS config for Bedrock), env vars (`CLAUDE_CODE_USE_BEDROCK`), and install commands (`npm install -g @anthropic-ai/claude-code`). This replaces the current `internal/ai/invoke.go` logic.

**`internal/tool/opencode/`** - Translates to opencode-specific CLI flags. `ContainerSetup` returns OpenCode's config paths and auth requirements.

**`internal/tool/testcode/`** - File-based mock driven entirely by files on disk. The calling code is identical to using any other tool; the only difference is `tool: testcode` in config.

### TestCode File Protocol

```
.muster/testcode/
  responses/          # Pre-populated by test harness before running
    001.txt           # Response for 1st Invoke call
    002.txt           # Response for 2nd Invoke call
    003.txt           # Response for 3rd Invoke call (or Launch)
  calls/              # Written by TestCode as it runs
    001-invoke.json   # Records what Invoke received (model, prompt, skillsDir, env)
    002-invoke.json
    003-launch.json   # Records what Launch received
```

**Workflow:**
1. Test harness writes numbered response files to `responses/`.
2. Muster runs normally with `tool: testcode` in config.
3. Each `Invoke` call consumes the next response file in order and writes the actual inputs it received to `calls/`.
4. Each `Launch` call writes its inputs to `calls/` and exits immediately (no response file consumed, or an optional one for exit code).
5. After the run, test harness inspects `calls/` to verify inputs were correct.

### Migration

`cmd/code.go`, `cmd/add.go`, and `cmd/sync.go` currently call `ai.InvokeAI` directly. These will be migrated to receive a `CodingTool` instance from command setup. The `internal/ai/` package is replaced by the `claudecode` implementation. The `var InvokeAI = invokeAI` pattern is no longer needed since the interface provides a natural test seam.

Note: the existing `ai.ExtractJSON` utility (strips markdown code fences from AI output) should be relocated to a shared location (e.g., `internal/tool/extract.go`) since callers of `Invoke` still need it.

Note: the current `ai.invokeAI` creates its own `context.Context` internally. The new interface adds `context.Context` as a parameter, which means calling commands (`add.go`, `sync.go`, `code.go`) will need to thread context through — this is the correct pattern but requires updating those call sites.

## ContainerRuntime Interface

### Package: `internal/docker/`

```go
type ContainerRuntime interface {
    Up(ctx context.Context, composeFile, projectName string) error
    Down(ctx context.Context, composeFile, projectName string) error
    Exec(ctx context.Context, composeFile, projectName, service string, cmd []string) error
    List(ctx context.Context, project, slug string) ([]ContainerInfo, error)
    Ping(ctx context.Context) error
}
```

The existing `docker.Client` is refactored to satisfy this interface. It keeps its Docker SDK and `docker compose` CLI internals.

### What stays outside the interface

Config generation code remains as pure functions, directly testable without Docker:
- `compose.go` - compose file generation from config inputs. Updated to accept `ContainerRequirements` from the CodingTool rather than hardcoding Claude-specific knowledge.
- `auth.go` - provider-specific auth resolution moves into each tool's `ContainerSetup`. Generic "translate requirements into compose overrides" stays here.
- `allowlist.go` - proxy domain allowlist building.
- `assets.go` / `embed.go` - asset extraction (filesystem only).

### MockRuntime

File-based, same pattern as TestCode:

```
.muster/mockdocker/
  responses/          # Optional: pre-configured responses (e.g., container listings)
    001-list.json
  calls/              # Written by MockRuntime as it runs
    001-up.json       # Records composeFile, projectName
    002-exec.json     # Records service, cmd
    003-down.json
```

For most operations (`Up`, `Down`, `Exec`), the mock records the call and succeeds. For `List`, it reads from a pre-populated response file so the pipeline can see "containers" it expects. `Ping` always succeeds.

### Tool-Container Coupling

When running in Docker (`--yolo` mode), the container must be configured specifically for the coding tool being used. Each `CodingTool.ContainerSetup()` returns the tool's requirements (install commands, auth mounts, env vars, config files). The compose generation code translates these requirements into Docker config without knowing tool internals. This means adding a new tool doesn't require touching Docker code.

## VCS Layer

The VCS package (`internal/vcs/`) already has a proper `VCS` interface with `CommandRunner` function injection for testability. No changes needed. Pipeline steps that need VCS (finish step for PR creation) get a `VCS` instance from the existing factory.

## Phase 6 Decomposition

The existing single "full-pipeline" roadmap item is replaced by 6 ordered items:

### 1. `interface-refactor` (HIGH priority)

Prerequisite for everything else. Delivers:
- `CodingTool` interface and `InvokeConfig`/`LaunchConfig`/`ContainerRequirements` types in `internal/tool/`
- `claudecode` implementation (extracted from `internal/ai/invoke.go` and `cmd/code.go`)
- `opencode` implementation (stub that returns "not yet implemented" is acceptable if full opencode CLI flags aren't known yet; the interface contract is what matters)
- `testcode` implementation with file-based protocol
- `ContainerRuntime` interface in `internal/docker/`
- Refactor `docker.Client` to satisfy `ContainerRuntime`
- `MockRuntime` with file-based protocol
- Migrate `cmd/code.go`, `cmd/add.go`, `cmd/sync.go` to use `CodingTool`
- Refactor compose generation to use `ContainerRequirements` instead of hardcoded Claude knowledge
- All existing tests continue to pass; new tests exercise TestCode and MockRuntime

### 2. `pipeline-skeleton` (HIGH priority)

The pipeline infrastructure without real step implementations. Delivers:
- `internal/pipeline/` package with state machine and step registry
- Checkpoint read/write (`.muster/work/{slug}/checkpoint.json`)
- `cmd/in.go` with `--next` and `--step` flags
- Steps are no-op stubs that can be individually replaced
- Checkpoint round-trip tests, state machine transition tests
- Testable: `muster in <slug> --next` advances through stub steps

### 3. `pipeline-git-steps` (MEDIUM priority)

Steps that are pure git/config operations, no AI or Docker. Delivers:
- **worktree** step: `git worktree add`, write `branch` to roadmap item, copy pre-existing plan from main
- **prepare** step: merge main, version bump, changelog, roadmap cleanup, commit
- **finish** step: squash merge (direct strategy) or create PR/MR (github-pr/gitlab-mr via VCS interface)
- Integration tests against real temp git repos

### 4. `pipeline-ai-steps` (MEDIUM priority)

Steps that invoke AI through the CodingTool interface. Delivers:
- **plan** step: stage plan-feature templates, call `CodingTool.Invoke`, skip if plan exists
- **execute** step: stage execute-plan templates, call `CodingTool.Invoke`
- **review** step: stage review-implementation templates, call `CodingTool.Invoke`
- Fully testable with TestCode: write expected responses, run pipeline, verify calls files

### 5. `pipeline-docker-steps` (MEDIUM priority)

Steps that involve Docker through the ContainerRuntime interface. Delivers:
- **setup** step: boot container via `ContainerRuntime.Up`, run lifecycle.setup hook via `Exec`
- **verify** step: run lifecycle.verify via `Exec`, up to 3 AI fix attempts (combines ContainerRuntime and CodingTool)
- **post-review-verify** step: verify again after review fixes
- Testable with MockRuntime + TestCode together

### 6. `pipeline-batch-mode` (MEDIUM priority)

Orchestration on top of the working pipeline. Delivers:
- `--all` flag: parallel batch execution with claim locks (replaces `roadmap-loop.sh`)
- `--resume` / `--retry`: checkpoint-based recovery
- `internal/pipeline/lock.go`: filesystem locks with stale PID detection (cross-platform)
- Lock acquisition/release/stale detection tests

### Dependencies

```
interface-refactor
  -> pipeline-skeleton
       -> pipeline-git-steps    \
       -> pipeline-ai-steps      }-> pipeline-batch-mode
       -> pipeline-docker-steps /
```

Items 3, 4, and 5 can be worked in parallel once the skeleton is in place. Item 6 depends on all three step items since it orchestrates the full working pipeline.

## Testing Philosophy

The primary purpose of these abstractions is to let a Claude agent have full control over the testing environment. With TestCode and MockRuntime:
- An agent can set up expected responses, run the pipeline, and verify the tool was called with correct inputs.
- No real Claude Code, Docker, or external services needed for pipeline logic testing.
- The agent can iterate until all primary logic works, with manual testing only needed for the final integration with real tools.
- Each roadmap item produces independently testable increments that can be verified before moving on.
