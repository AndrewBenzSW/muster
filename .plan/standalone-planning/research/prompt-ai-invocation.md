# Prompt Staging and AI Invocation Flow

*Researched: 2026-03-23*
*Scope: Audit internal/prompt and internal/ai to understand template rendering, skill staging, Claude Code plugin integration, and AI invocation mechanics*

---

## Key Findings

### Template System Architecture

The prompt system uses Go's `text/template` with embedded templates from `internal/prompt/prompts/`. Templates are parsed once using `sync.Once` and cached in a module-level variable. Templates receive a `PromptContext` struct containing resolved configuration, paths, and model tier mappings.

**Key files:**
- `/Users/andrew.benz/work/muster/muster-main/internal/prompt/template.go` - Template parsing and rendering
- `/Users/andrew.benz/work/muster/muster-main/internal/prompt/context.go` - PromptContext definition
- `/Users/andrew.benz/work/muster/muster-main/internal/prompt/embed.go` - Embedded template filesystem

### Skill Staging Mechanism

`StageSkills()` creates a temporary directory structure that mirrors Claude Code's plugin system:
- Creates `muster-prompts-*` temp dir in OS temp location
- Mirrors embedded `prompts/` structure under `tmpDir/skills/`
- Transforms directory names: `plan-feature` → `roadmap-plan-feature`
- Strips `.tmpl` extension: `SKILL.md.tmpl` → `SKILL.md`
- Returns temp dir path, cleanup function, and error

**Directory structure:**
```
tmpDir/
  skills/
    roadmap-plan-feature/
      SKILL.md
      planner-prompt.md
      research-runner.md
      ...
    roadmap-execute-plan/
      SKILL.md
      worker-prompt.md
      ...
```

**Key file:** `/Users/andrew.benz/work/muster/muster-main/internal/prompt/stage.go`

### AI Invocation Flow

`ai.InvokeAI()` provides single-shot AI invocation by:
1. Creating temp directory `muster-ai-invoke-*`
2. Writing prompt content to `tmpDir/skills/SKILL.md`
3. Executing: `tool --print --plugin-dir tmpDir [--model model]`
4. Capturing stdout as JSON response
5. Cleaning up temp directory

**Configuration:**
- `InvokeConfig` struct with Tool, Model, Prompt, Verbose, Timeout, Env fields
- Default timeout: 120 seconds
- Supports env overrides (e.g., `ANTHROPIC_BASE_URL` for local models)

**Key file:** `/Users/andrew.benz/work/muster/muster-main/internal/ai/invoke.go`

### Two Invocation Patterns

**Pattern 1: Direct AI invocation (add/sync commands)**
- Renders a single template (e.g., `add-item-prompt.md.tmpl`)
- Calls `ai.InvokeAI()` with rendered prompt
- Gets JSON response, parses, and uses result
- Used by `/Users/andrew.benz/work/muster/muster-main/cmd/add.go` and `/Users/andrew.benz/work/muster/muster-main/cmd/sync.go`

**Pattern 2: Skill staging (future plan/execute commands)**
- Calls `prompt.StageSkills()` to stage entire skill directories
- Invokes Claude Code with `--plugin-dir` pointing to staged skills
- Skills contain multi-file workflows (SKILL.md + runner files + prompts)
- Used for complex multi-phase operations

### PromptContext Structure

`PromptContext` is the data model passed to all templates:

```go
type PromptContext struct {
    Interactive  bool   // TTY vs batch mode
    Tool         string // "claude-code", "opencode", etc.
    Provider     string // "anthropic", "openai", etc.
    Model        string // Resolved model name
    Slug         string // Roadmap item identifier
    WorktreePath string // Git worktree absolute path
    MainRepoPath string // Main repo absolute path
    PlanDir      string // Plan directory path
    Models       struct {
        Fast     string // Fast tier model
        Standard string // Standard tier model
        Deep     string // Deep tier model
    }
    Extra        map[string]interface{} // Template-specific data
}
```

**Creation:** `NewPromptContext()` accepts resolved config, project config, user config, and populates Models from tier mappings with 4-level precedence: project tool tiers > project model tiers > user tool tiers > user model tiers.

**Key file:** `/Users/andrew.benz/work/muster/muster-main/internal/prompt/context.go:62-124`

### Environment Variable Overrides

`config.ToolEnvOverrides()` maps provider config to tool-specific env vars:
- For `claude-code`: `provider.base_url` → `ANTHROPIC_BASE_URL`
- Enables local model servers (LM Studio, Ollama)
- Applied to `ai.InvokeConfig.Env` in add/sync commands

**Key file:** `/Users/andrew.benz/work/muster/muster-main/internal/config/config.go:260-281`

### Testing Patterns

**Template tests:**
- Golden file tests compare rendered output against `.golden` files in `testdata/`
- Use `go test -update` flag to regenerate golden files
- Normalize line endings for cross-platform comparison
- Test both `Interactive=true` and `Interactive=false` branches

**Staging tests:**
- Verify temp directory creation and cleanup
- Check file permissions (0755 dirs, 0644 files)
- Test concurrent calls with race detector
- Verify LF line endings preserved across platforms
- Test cleanup idempotency

**AI invocation tests:**
- Compile mock binary once using `sync.Once`
- Mock accepts env vars to control response (`MOCK_RESPONSE`, `MOCK_EXIT_CODE`, `MOCK_DELAY_MS`)
- Test timeout, concurrent calls, error handling

**Key files:**
- `/Users/andrew.benz/work/muster/muster-main/internal/prompt/template_test.go`
- `/Users/andrew.benz/work/muster/muster-main/internal/prompt/stage_test.go`
- `/Users/andrew.benz/work/muster/muster-main/internal/ai/invoke_test.go`

---

## Detailed Analysis

### Template Parsing and Caching

Templates are embedded at compile time via `//go:embed all:prompts` and parsed lazily on first use. The `sync.Once` pattern ensures thread-safe initialization:

```go
var parsedTemplates *template.Template
var parseOnce sync.Once

func ParseTemplates() error {
    parseOnce.Do(func() {
        parsedTemplates = template.New("").Option("missingkey=error")
        // Walk embedded FS and parse each .tmpl file
    })
    return parseErr
}
```

Templates are identified by their full path: `prompts/plan-feature/SKILL.md.tmpl`. The `missingkey=error` option ensures undefined variables cause clear errors.

**Rendering:** `RenderTemplate(name, ctx)` looks up the parsed template and executes it with the given context, returning the rendered string.

### Skill Staging Details

**Exclusions:** `StageSkills()` skips templates in `add-item/`, `sync-match/`, `out/`, and `test/` directories. These are used via direct AI invocation, not as staged skills.

**Naming transformation:**
- Input: `prompts/plan-feature/SKILL.md.tmpl`
- Output: `tmpDir/skills/roadmap-plan-feature/SKILL.md`
- Rationale: `roadmap-` prefix avoids collisions with user-installed skills

**Cleanup:** Always returns a cleanup function, even on error. Cleanup is idempotent - can be called multiple times safely. Stale directories (>24h old) are cleaned up automatically.

**Verification:** After staging, checks for expected SKILL.md files in `roadmap-plan-feature`, `roadmap-execute-plan`, and `roadmap-review-implementation`.

### Cross-Platform Compatibility

**Path handling:**
- Uses `filepath.Join()` for OS-appropriate separators
- `os.TempDir()` returns platform-specific temp locations
- Windows: `C:\Users\...\AppData\Local\Temp`
- macOS: `/var/folders/...`
- Linux: `/tmp`

**Line endings:**
- Template rendering produces LF (`\n`) consistently
- `os.WriteFile()` writes bytes as-is (no CRLF conversion)
- Tests verify LF preservation: `/Users/andrew.benz/work/muster/muster-main/internal/prompt/stage_test.go:170-204`
- `.gitattributes` enforces LF for source files

### AI Invocation Configuration

**InvokeConfig fields:**
- `Tool`: Executable path or name (resolved via `config.ToolExecutable()`)
- `Model`: Passed as `--model` flag (empty means tool default)
- `Prompt`: Content staged as `SKILL.md`
- `Verbose`: If true, prints command and env to stderr
- `Timeout`: Execution timeout (default 120s)
- `Env`: Additional env vars (e.g., provider base_url)

**Command construction:**
```
tool --print --plugin-dir <tmpDir> [--model <model>]
```

The `--print` flag tells Claude Code to output JSON to stdout instead of interactive mode.

### JSON Response Handling

`ai.ExtractJSON()` strips markdown code fences from responses:
- Input: `` ```json\n{"key": "value"}\n``` ``
- Output: `{"key": "value"}`
- Handles both `` ```json `` and `` ``` `` fences
- Returns input unchanged if no fences found

Used by add/sync commands before `json.Unmarshal()`.

### Template Extra Data Pattern

Templates needing dynamic data use `ctx.Extra`:
- `add-item-prompt.md.tmpl`: Uses `{{.Extra.UserInput}}`
- `sync-match-prompt.md.tmpl`: Uses `{{.Extra.SourceItems}}` and `{{.Extra.TargetItems}}`

This avoids modifying the shared `PromptContext` struct for template-specific needs.

### Model Tier Resolution

Model tiers allow abstract tier names (fast/standard/deep) to map to tool-specific models:

**Resolution order (highest precedence first):**
1. Project tool tiers: `.muster/config.yml tools.claude-code.model_tiers.fast`
2. Project model tiers: `.muster/config.yml model_tiers.fast`
3. User tool tiers: `~/.config/muster/config.yml tools.claude-code.model_tiers.fast`
4. User model tiers: `~/.config/muster/config.yml model_tiers.fast`

**Fallback defaults:** If all sources are nil, falls back to hard-coded defaults (haiku/sonnet/sonnet).

**Purpose:** Enables templates to reference `{{.Models.Fast}}` and get the correct model for the active tool, supporting multi-tool environments.

---

## Recommendations

### For `muster plan` Implementation

**1. Use staged skills pattern, not direct invocation**

The plan command should use `prompt.StageSkills()` and pass the staged directory to Claude Code, not `ai.InvokeAI()`. This enables multi-file skill workflows with research/synthesis/planning phases.

```go
tmpDir, cleanup, err := prompt.StageSkills(ctx)
defer cleanup()

// Invoke claude with --plugin-dir tmpDir
// Claude will load roadmap-plan-feature skill
```

**2. Create PromptContext with proper paths**

```go
ctx := prompt.NewPromptContext(
    resolved,           // From config.ResolveStep("plan", ...)
    projectCfg,
    userCfg,
    interactive,        // From TTY detection
    slug,               // From args or picker
    worktreePath,       // .worktrees/<slug> or main repo
    mainRepoPath,       // Absolute path to main repo
    planDir,            // .muster/work/<slug>/plan
)
```

**3. Don't populate Extra for skill staging**

Skills access context via their own mechanisms. Only populate Extra for direct AI invocation templates (add-item, sync-match).

**4. Handle cleanup properly**

Always defer cleanup immediately after StageSkills:

```go
tmpDir, cleanup, err := prompt.StageSkills(ctx)
defer cleanup() // Even if err != nil
if err != nil {
    return fmt.Errorf("failed to stage skills: %w", err)
}
```

**5. Apply environment overrides**

Pass provider-specific env vars to Claude Code invocation:

```go
cmd := exec.Command("claude", ...)
cmd.Env = append(os.Environ(), toEnvSlice(config.ToolEnvOverrides(resolved, projectCfg, userCfg))...)
```

**6. Test with golden files**

Create golden file tests for plan-feature templates:
- Test Interactive=true/false branches
- Test with different tools (claude-code, opencode)
- Verify Models.Fast/Standard/Deep populate correctly
- Use `-update` flag to regenerate

**7. Don't reuse ai.InvokeAI() for plan**

`ai.InvokeAI()` is for single-shot prompts. Plan needs full skill orchestration with multiple phases and file outputs. Invoke Claude Code directly with the staged plugin-dir.

---

## Open Questions

### 1. Should plan command output go to .plan/ or .muster/work/?

**Context:** Existing patterns show:
- `.plan/<slug>/` is used as temp workspace (see teammate-message context)
- `.muster/work/<slug>/plan/implementation-plan.md` is mentioned as output location
- CLAUDE.md references both locations

**What was tried:** Read CLAUDE.md, teammate message, and existing code. No definitive answer found.

**Why it matters:** Need to know where to write implementation-plan.md and research outputs.

### 2. How should plan command invoke Claude Code?

**Context:** Skill staging creates tmpDir with staged skills, but actual invocation mechanism unclear:
- Should we use exec.Command directly?
- Should we extend ai.InvokeAI() to support --plugin-dir without creating its own tmpDir?
- Should plan command have its own invocation function?

**What was tried:** Reviewed ai.InvokeAI() and cmd/add.go patterns. InvokeAI creates its own tmpDir for single prompts, which conflicts with StageSkills.

**Why it matters:** Need to avoid duplicate temp dir creation and ensure proper plugin-dir usage.

### 3. Should plan command support both interactive (teams) and foreground modes?

**Context:** SKILL.md templates have `{{if .Interactive}}` branches suggesting two modes:
- Interactive mode: Uses teams for parallel research
- Foreground mode: Sequential phase execution

**What was tried:** Read plan-feature/SKILL.md.tmpl and execute-plan/SKILL.md.tmpl. Both have mode-specific instructions.

**Why it matters:** Affects command flag design and invocation strategy. May need `--interactive` flag or auto-detect from TTY.

### 4. How should research/synthesis/planning phases write outputs?

**Context:** Templates reference runner files (research-runner.md, synthesis-runner.md, planning-runner.md) but actual phase orchestration is unclear:
- Do phases run as sub-agents?
- Do they write to specific paths?
- Does the team lead coordinate?

**What was tried:** Read runner templates (not included in this research scope).

**Why it matters:** Need to understand data flow between phases to implement plan command correctly.

---

## References

### Source Files Examined

**Prompt package:**
- `/Users/andrew.benz/work/muster/muster-main/internal/prompt/context.go` - PromptContext definition and tier resolution
- `/Users/andrew.benz/work/muster/muster-main/internal/prompt/template.go` - Template parsing and rendering
- `/Users/andrew.benz/work/muster/muster-main/internal/prompt/stage.go` - Skill staging mechanism
- `/Users/andrew.benz/work/muster/muster-main/internal/prompt/embed.go` - Embedded template filesystem

**AI package:**
- `/Users/andrew.benz/work/muster/muster-main/internal/ai/invoke.go` - AI invocation implementation

**Config package:**
- `/Users/andrew.benz/work/muster/muster-main/internal/config/config.go:260-281` - ToolEnvOverrides function

**Command implementations:**
- `/Users/andrew.benz/work/muster/muster-main/cmd/add.go:156-257` - Interactive AI mode with direct invocation
- `/Users/andrew.benz/work/muster/muster-main/cmd/sync.go:208-308` - AI fuzzy matching with template Extra data

**Templates examined:**
- `/Users/andrew.benz/work/muster/muster-main/internal/prompt/prompts/plan-feature/SKILL.md.tmpl` - Skill metadata and capabilities
- `/Users/andrew.benz/work/muster/muster-main/internal/prompt/prompts/add-item/add-item-prompt.md.tmpl` - Direct invocation example
- `/Users/andrew.benz/work/muster/muster-main/internal/prompt/prompts/sync-match/sync-match-prompt.md.tmpl` - Extra data usage example

**Test files:**
- `/Users/andrew.benz/work/muster/muster-main/internal/prompt/template_test.go` - Golden file tests
- `/Users/andrew.benz/work/muster/muster-main/internal/prompt/stage_test.go` - Staging and cleanup tests
- `/Users/andrew.benz/work/muster/muster-main/internal/ai/invoke_test.go` - Mock binary and invocation tests

### Documentation

- `/Users/andrew.benz/work/muster/muster-main/CLAUDE.md` - Project patterns and conventions
