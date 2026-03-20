# Config & Merge Strategies

*Researched: 2026-03-20*
*Scope: Config system and merge strategy resolution*

---

## Key Findings

1. **`merge_strategy` does not exist in the codebase yet.** The design doc (`docs/design.md:137`) specifies it as a top-level field in `.muster/config.yml` with values `direct`, `github-pr`, or `gitlab-mr`, but the `ProjectConfig` Go struct and all test fixtures have no `MergeStrategy` field. It must be added.

2. **The config system is well-established with clear patterns.** Two-layer config (project base + `.local.yml` overlay) with 5-step resolution chain is fully implemented and tested. Adding `merge_strategy` follows an existing, predictable pattern.

3. **`lifecycle` config is also missing from the struct.** The design doc shows a `lifecycle` section in `.muster/config.yml` with fields like `setup`, `check`, `verify`, `teardown` pointing to scripts. This is not yet in `ProjectConfig` either.

4. **`muster out` needs config from two sources**: the `merge_strategy` from `ProjectConfig` to determine its behavior, and the `pr_url`/`branch` fields from `RoadmapItem` to know which PR to monitor. Both data types exist (roadmap fields are implemented), only `merge_strategy` is missing.

## Detailed Analysis

### Config Structs and YAML Schema

The `ProjectConfig` struct (`internal/config/config.go:108-127`) currently has:
- `Defaults *DefaultsConfig` (tool/provider/model)
- `Pipeline map[string]*PipelineStepConfig` (per-step overrides)
- `Tools map[string]*ToolConfig` (project-level tool overrides)
- `Providers map[string]*ProviderConfig` (project-level provider overrides)
- `ModelTiers *ModelTiersConfig` (project-level tier mappings)
- `LocalOverrides map[string]interface{}` (arbitrary local overrides)

**Missing fields needed for `muster out`:**
- `MergeStrategy *string` (yaml: `merge_strategy`) -- top-level field per design doc
- `Lifecycle *LifecycleConfig` (yaml: `lifecycle`) -- optional, contains script paths

### Config Loading and Merge

Project config loading (`internal/config/project.go:15-52`) follows this flow:
1. Load `.muster/config.yml` (fallback to `.dev-agent/config.yml`)
2. Load `.muster/config.local.yml` if it exists
3. Deep-merge local over base via `mergeProjectConfigs()`

The merge function (`internal/config/project.go:73-163`) does:
- Pointer fields: override replaces if non-nil
- Maps: key-by-key override (base keys preserved if not overridden)
- Lists: replaced entirely (not appended)

This means `merge_strategy` as a `*string` would naturally merge correctly -- a `.local.yml` can override it and absence means "use base value."

### Resolution Chain

The 5-step resolution chain (`internal/config/resolve.go:42-158`) resolves tool/provider/model triples per pipeline step:
1. `projectCfg.Pipeline[stepName]` -- step-level override
2. `projectCfg.Defaults` -- project defaults
3. `userCfg.Defaults` -- user defaults
4. Tool-specific tier resolution
5. Hard-coded defaults (claude-code, anthropic, sonnet)

**`merge_strategy` does not participate in this chain.** It's not a per-step value -- it's a project-level setting that determines behavior of `muster out` and the pipeline's `finish` step. It should be resolved directly from `ProjectConfig.MergeStrategy` with a sensible default.

### How Commands Access Config

All existing commands follow a consistent pattern (`cmd/code.go`, `cmd/add.go`, `cmd/sync.go`):
1. `config.LoadUserConfig("")` -- load user config
2. `config.LoadProjectConfig(".")` -- load project config
3. `config.ResolveStep("stepName", projectCfg, userCfg)` -- resolve triple
4. Use `resolved.Tool`, `resolved.Provider`, `resolved.Model`

For `muster out`, the command would additionally read `projectCfg.MergeStrategy` directly (not through `ResolveStep`) to determine which flow to execute.

Some commands (like `code --yolo`) use the higher-level `config.LoadAll("", ".")` which returns a `*Config` containing all three layers (User, Project, DevAgent) and call `cfg.Resolve("stepName")`.

### Merge Strategy Values per Design Doc

From `docs/design.md:81,137,284,505,528`:
- `direct` -- squash merge into main directly (pipeline's `finish` step handles it)
- `github-pr` -- create GitHub PR via `gh`, `muster out` monitors CI and merge
- `gitlab-mr` -- create GitLab MR via `glab`, `muster out` monitors CI and merge

When `merge_strategy` is `direct`, `muster out` should exit early with a message that it's not needed. The design doc explicitly states: "Only meaningful when `merge_strategy` is `github-pr` or `gitlab-mr`."

### Doctor Command Dependencies

From `docs/design.md:82`:
- `gh` is a **soft requirement** only needed when `merge_strategy: github-pr`
- `glab` is a **soft requirement** only needed when `merge_strategy: gitlab-mr`
- `muster doctor` should check these conditionally based on the configured merge strategy

### Roadmap Item Fields

The `RoadmapItem` struct (`internal/roadmap/roadmap.go:77-98`) already has:
- `PRUrl *string` (json: `pr_url`) -- written by `finish` step for PR/MR strategies
- `Branch *string` (json: `branch`) -- written when worktree is created

Both are optional pointer fields. `muster out` reads `PRUrl` to know which PR/MR to monitor.

## Recommendations

### 1. Add `MergeStrategy` to `ProjectConfig`

```go
// In internal/config/config.go, inside ProjectConfig struct:

// MergeStrategy determines how completed work is merged.
// Valid values: "direct", "github-pr", "gitlab-mr".
// Default: "github-pr" (most common workflow).
MergeStrategy *string `yaml:"merge_strategy"`
```

Use a `*string` to distinguish "not set" (nil, use default) from "explicitly set." This is consistent with all other optional config fields.

### 2. Add Merge Strategy Constants and Default

```go
const (
    MergeStrategyDirect   = "direct"
    MergeStrategyGitHubPR = "github-pr"
    MergeStrategyGitLabMR = "gitlab-mr"
    DefaultMergeStrategy  = MergeStrategyGitHubPR
)
```

### 3. Add Resolution Helper

```go
// ResolveMergeStrategy returns the merge strategy from project config,
// falling back to the default if not configured.
func ResolveMergeStrategy(projectCfg *ProjectConfig) string {
    if projectCfg != nil && projectCfg.MergeStrategy != nil {
        return *projectCfg.MergeStrategy
    }
    return DefaultMergeStrategy
}
```

This is simpler than the 5-step chain because merge strategy is project-level only, not per-step or per-user. A user-level default could be added later if needed.

### 4. Add Validation

```go
func ValidateMergeStrategy(strategy string) error {
    switch strategy {
    case MergeStrategyDirect, MergeStrategyGitHubPR, MergeStrategyGitLabMR:
        return nil
    default:
        return fmt.Errorf("invalid merge_strategy %q; valid values: direct, github-pr, gitlab-mr", strategy)
    }
}
```

Add this to `Config.Validate()` alongside existing checks.

### 5. Update `mergeProjectConfigs`

Add merge handling in `internal/config/project.go:73`:
```go
// After existing field copies, before return:
if override.MergeStrategy != nil {
    result.MergeStrategy = override.MergeStrategy
}
```

### 6. Default Value Discussion

The design doc shows `merge_strategy: direct` in the example config, but `github-pr` is the most common real-world workflow. Consider:
- `github-pr` as default: matches most users' git workflows, makes `muster out` immediately useful
- `direct` as default: safer/simpler for new projects, but makes `muster out` a no-op
- No default (require explicit config): most explicit, but adds friction

Recommendation: default to `github-pr` since muster is designed around PR-based workflows.

## Open Questions

### 1. Should merge strategy be configurable at the user level?
- **Question**: Should `~/.config/muster/config.yml` allow a default `merge_strategy`?
- **Why it matters**: A developer who always uses GitHub might want to set it once globally rather than per-project.
- **What I found**: The design doc only shows it in project config. All current user config fields relate to tool/provider/model. Merge strategy is inherently project-specific (a project is on GitHub OR GitLab, not both).
- **Recommendation**: Keep it project-level only for now. If demand arises, it can be added to user config later.

### 2. What should `muster out` do when `merge_strategy` is not set and no `.muster/config.yml` exists?
- **Question**: Should it fail, assume a default, or prompt?
- **Why it matters**: The command needs to know whether to use `gh` or `glab` for CI monitoring.
- **What I found**: Other commands (code, add, sync) work fine with empty/missing project config by using defaults. If the default is `github-pr`, `muster out` would assume GitHub.
- **Recommendation**: Use default (`github-pr`) and let it fail gracefully if `gh` is not available with an actionable error message.

### 3. Should merge strategy support per-step override?
- **Question**: Could a pipeline have `finish` use `github-pr` but some future step use a different strategy?
- **Why it matters**: Architecture decision that affects where the field lives.
- **What I found**: The design doc treats it as a single project-level value. The `finish` pipeline step checks it, and `muster out` checks it. There's no use case for per-step variation.
- **Recommendation**: Keep it project-level. Per-step would be over-engineering.

### 4. Lifecycle config section
- **Question**: The design doc shows a `lifecycle` section in `.muster/config.yml` -- is it needed for `muster out`?
- **Why it matters**: `muster out` might need to run teardown scripts after cleanup.
- **What I found**: The lifecycle scripts (setup, check, verify, teardown) are referenced in the design doc under `.muster/config.yml` but none are implemented. They appear to be used by `muster in` pipeline steps, not `muster out` specifically.
- **Recommendation**: Lifecycle config is a separate concern. `muster out` doesn't need it. It can be added when implementing `muster in` (Phase 6).

## References

| Item | Location |
|------|----------|
| `ProjectConfig` struct | `internal/config/config.go:108-127` |
| `PipelineStepConfig` struct | `internal/config/config.go:153-166` |
| `ResolvedConfig` struct | `internal/config/config.go:229-244` |
| `DefaultsConfig` struct | `internal/config/config.go:62-67` |
| `Config.Validate()` | `internal/config/config.go:298-435` |
| `ResolveStep()` | `internal/config/resolve.go:42-158` |
| `mergeProjectConfigs()` | `internal/config/project.go:73-163` |
| `LoadProjectConfig()` | `internal/config/project.go:15-52` |
| `LoadAll()` | `internal/config/config.go:203-227` |
| `RoadmapItem` (PRUrl, Branch) | `internal/roadmap/roadmap.go:77-98` |
| Design doc merge_strategy | `docs/design.md:81,137,284,505,528` |
| Design doc lifecycle section | `docs/design.md:139-143` |
| Design doc doctor dependencies | `docs/design.md:82` |
| Project config testdata | `internal/config/testdata/project-full.yml` |
| Resolve tests | `internal/config/resolve_test.go` |
| Project config tests | `internal/config/project_test.go` |
| Config round-trip tests | `internal/config/config_test.go` |
| cmd/code.go config pattern | `cmd/code.go:42-64` |
| cmd/add.go config pattern | `cmd/add.go:42-59` |
