# AI Integration Patterns

*Researched: 2026-03-18*
*Scope: Prompt staging system, template rendering, variable substitution, config resolution for tool/provider/model selection, AI invocation patterns via Claude Agent SDK*

---

## Key Findings

**The muster codebase does not directly invoke AI models**. Instead, it orchestrates external AI coding tools (claude-code, opencode) by:

1. **Staging prompt templates** as Claude Agent SDK skill files in a temporary directory
2. **Resolving configuration** (tool/provider/model) through a 5-step fallback chain
3. **Executing the tool** with `--plugin-dir` pointing to staged skills
4. **Delegating AI invocation** to the tool itself (Claude Code or OpenCode)

For AI-assisted commands like `add` and `sync`, the pattern is:
- Render prompt templates with `internal/prompt/template.go:RenderTemplate()`
- Pass resolved model names to the AI tool via command execution
- Let the tool handle actual AI API calls (Anthropic, OpenAI, etc.)

There is **no direct HTTP client** or API integration in the muster codebase. The tool acts as an orchestration layer.

---

## Detailed Analysis

### Prompt Template System

**Location**: `internal/prompt/template.go`

The template system uses Go's `text/template` with the following characteristics:

- **Embedded templates**: All prompts live in `internal/prompt/prompts/` as `.md.tmpl` files and are embedded via `go:embed` directive (`internal/prompt/embed.go:5-6`)
- **Singleton parsing**: Templates are parsed once via `sync.Once` pattern (`template.go:16-17`)
- **Error handling**: `missingkey=error` option enforces that all template variables must be defined (`template.go:26`)
- **Template naming**: Full path within embedded FS is used as template name (e.g., `prompts/plan-feature/SKILL.md.tmpl`)

**Core functions**:

```go
// template.go:24-56
func ParseTemplates() error
// Parses all .tmpl files from embedded FS, thread-safe via sync.Once

// template.go:58-84
func RenderTemplate(name string, ctx *PromptContext) (string, error)
// Renders a template by full path name with provided context
// Returns error if context is nil or template not found
```

**Template structure example** (`prompts/plan-feature/researcher-prompt.md.tmpl:1-35`):

```markdown
# Researcher Prompt

You are a code research agent analyzing the codebase to inform feature planning.

## Context

- **Plan Directory**: {{.PlanDir}}
- **Worktree Path**: {{.WorktreePath}}
- **Model**: {{.Models.Deep}}

{{if .Interactive}}
You are working in a team. Report findings via SendMessage when complete.
{{else}}
You are working in foreground mode. Write findings directly to the plan directory.
{{end}}
```

Templates use conditional logic (`{{if .Interactive}}...{{else}}...{{end}}`) to adapt prompts for interactive vs. batch modes.

### Variable Substitution

**Context struct**: `internal/prompt/context.go:15-55`

The `PromptContext` struct provides all template variables:

```go
type PromptContext struct {
    Interactive  bool    // Whether this is an interactive session
    Tool         string  // Resolved tool name (e.g., "claude-code", "opencode")
    Provider     string  // Resolved provider name (e.g., "anthropic", "openai")
    Model        string  // Resolved model name (e.g., "claude-sonnet-4.5")
    Slug         string  // Roadmap item identifier
    WorktreePath string  // Absolute path to git worktree
    MainRepoPath string  // Absolute path to main repository
    PlanDir      string  // Absolute path to plan directory
    Models       struct {
        Fast     string  // Model name for fast/lightweight tier
        Standard string  // Model name for standard/balanced tier
        Deep     string  // Model name for deep/capable tier
    }
}
```

**Variable population** (`context.go:57-109`):

The `NewPromptContext()` function populates the context from:
1. Resolved configuration (`ResolvedConfig`)
2. User configuration (`UserConfig`)
3. Runtime parameters (slug, paths, interactive mode)

**Model tier population** follows tool-specific precedence:
1. Tool-specific tiers from `userCfg.Tools[resolved.Tool].ModelTiers` (lines 84-95)
2. User-level tiers from `userCfg.ModelTiers` (lines 98-106)

This ensures templates like `{{.Models.Fast}}` render the correct model name for the active tool (e.g., `claude-haiku-4` for claude-code, `gemma3:4b` for opencode).

### Config Resolution for Tool/Provider/Model Selection

**Location**: `internal/config/resolve.go`

**5-step resolution chain** (`resolve.go:18-103`):

```go
func ResolveStep(stepName string, projectCfg *ProjectConfig, userCfg *UserConfig) (*ResolvedConfig, error)
```

Resolution order for each field (tool, provider, model):

1. **Step config**: `projectCfg.Pipeline[stepName]` (step-specific overrides)
2. **Project defaults**: `projectCfg.Defaults` (project-level defaults)
3. **User defaults**: `userCfg.Defaults` (user-level defaults)
4. **Tool defaults**: Not used for tool/provider, only for model tier resolution
5. **Hard-coded defaults**: Constants (`DefaultTool = "claude-code"`, `DefaultProvider = "anthropic"`, `DefaultModel = "claude-sonnet-4.5"`) at lines 32-36

**Model tier resolution** (`resolve.go:105-159`):

The `resolveModelTier()` function handles special "muster-" prefixed model names:

- `"muster-fast"` → lookup `userCfg.Tools[tool].ModelTiers.Fast` or `userCfg.ModelTiers.Fast`
- `"muster-standard"` → lookup standard tier
- `"muster-deep"` → lookup deep tier
- Literal model names (no `"muster-"` prefix) pass through unchanged (line 120)

**Tool override rule** (line 15-17 comment): When a step overrides the tool but not the model, model string resolution continues through the fallback chain, then tier resolution uses the newly selected tool's tier mapping.

**ResolvedConfig result**:

```go
type ResolvedConfig struct {
    Tool     string  // Final resolved tool name
    Provider string  // Final resolved provider name
    Model    string  // Final concrete model name (tier resolved)
}
```

### AI Invocation Patterns

**Direct invocation does not exist**. The muster codebase invokes external tools via `os/exec`:

**Pattern from `cmd/code.go:127-140`**:

```go
// Execute the tool
execCmd := exec.Command(resolved.Tool, cmdArgs...)
execCmd.Stdin = os.Stdin
execCmd.Stdout = os.Stdout
execCmd.Stderr = os.Stderr

if err := execCmd.Run(); err != nil {
    // Error handling
}
```

The `cmdArgs` includes `--plugin-dir` pointing to staged skills directory.

**AI-assisted command workflow** (inferred from prompt templates and Phase 2 architecture):

1. **Resolve configuration**: Call `config.ResolveStep("add", projectCfg, userCfg)` to get (tool, provider, model)
2. **Create prompt context**: Call `prompt.NewPromptContext(resolved, userCfg, ...)` with operation-specific parameters
3. **Render prompt template**: Call `prompt.RenderTemplate("path/to/template.md.tmpl", ctx)` to generate the prompt
4. **Execute tool with prompt**: Write rendered prompt to temp file, invoke tool with prompt file path or pipe via stdin

**For `add` command** (AI-assisted item creation):
- Render a prompt that describes the task: "Create a roadmap item with title X, description Y"
- Pass user input (title, description, tags) as template variables
- Tool receives prompt, invokes AI, streams response back
- Parse AI response to extract structured data (JSON or markdown)
- Write to `roadmap.json`

**For `sync` command** (AI-assisted fuzzy matching):
- Render a prompt with existing roadmap items and work items to match
- Tool invokes AI with context about items
- AI returns matching pairs or suggestions
- Parse response and update roadmap status

### Response Handling and Parsing

**No response parsing exists yet** in the codebase. The current `code` command (`cmd/code.go`) launches an interactive session and doesn't parse responses.

For AI-assisted commands, response handling will need to:

1. **Capture stdout**: Use `exec.Cmd.StdoutPipe()` instead of `execCmd.Stdout = os.Stdout`
2. **Stream or buffer**: Read response incrementally or buffer fully
3. **Parse structure**: Extract JSON or structured markdown from AI response
4. **Validate**: Check for expected fields, handle malformed responses

**Recommended parsing approach** (based on Phase 1 patterns):

```go
cmd := exec.Command(resolved.Tool, args...)
var stdout bytes.Buffer
cmd.Stdout = &stdout
cmd.Stderr = os.Stderr
cmd.Stdin = strings.NewReader(renderedPrompt)

if err := cmd.Run(); err != nil {
    return fmt.Errorf("AI invocation failed: %w", err)
}

// Parse response
var result ResponseStruct
if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
    // Fallback: parse as markdown or plain text
}
```

### Error Handling for AI Calls

**Existing error patterns** from `cmd/code.go:134-140`:

```go
var execErr *exec.Error
if errors.As(err, &execErr) && execErr.Err == exec.ErrNotFound {
    return fmt.Errorf("tool %q not found: %w\n\nPlease install %s...",
        resolved.Tool, err, resolved.Tool)
}
return fmt.Errorf("failed to execute %s: %w", resolved.Tool, err)
```

**Error categories to handle**:

1. **Tool not found**: Exec returns `exec.ErrNotFound` (handled at line 136)
2. **Template render failure**: `prompt.ErrTemplateRender` (defined at `stage.go:16`, checked at `code.go:102`)
3. **Config parse failure**: `config.ErrConfigParse` (defined at `config.go:28`, checked at `code.go:41,50`)
4. **Non-zero exit**: Tool exits with error status (caught by `cmd.Run()` returning error)
5. **AI response parse failure**: Not yet implemented, needs custom error type

**Recommended error handling for AI commands**:

```go
if err := cmd.Run(); err != nil {
    var exitErr *exec.ExitError
    if errors.As(err, &exitErr) {
        stderr := string(exitErr.Stderr)
        // Parse stderr for known AI error patterns
        if strings.Contains(stderr, "rate limit") {
            return fmt.Errorf("AI rate limit exceeded: %w", err)
        }
        if strings.Contains(stderr, "authentication") {
            return fmt.Errorf("AI authentication failed: check API key: %w", err)
        }
    }
    return fmt.Errorf("AI invocation failed: %w", err)
}
```

### Prompt Staging Process

**Function**: `internal/prompt/stage.go:100-226`

```go
func StageSkills(ctx *PromptContext) (tmpDir string, cleanup func(), err error)
```

**Staging flow**:

1. Clean up stale temp directories older than 24 hours (`stage.go:38-76`)
2. Create temp directory with `os.MkdirTemp("", "muster-prompts-")` (line 115)
3. Walk embedded `Prompts` filesystem, find all `.md.tmpl` files (line 134)
4. For each template:
   - Render with `RenderTemplate(path, ctx)` (line 150)
   - Transform path: `prompts/plan-feature/SKILL.md.tmpl` → `tmpDir/skills/roadmap-plan-feature/SKILL.md` (lines 158-185)
   - Write rendered content to file (line 200)
5. Verify expected skill files exist (lines 212-223)
6. Return temp directory path and cleanup function

**Cleanup pattern**:

The cleanup function is always returned (even on error) and should be deferred immediately:

```go
tmpDir, cleanup, err := prompt.StageSkills(ctx)
defer cleanup()
if err != nil {
    return err
}
```

**Directory structure** (`stage.go:92-99`):

```
tmpDir/
  skills/
    roadmap-plan-feature/
      SKILL.md
      planner-prompt.md
    roadmap-execute-plan/
      SKILL.md
      worker-prompt.md
    roadmap-review-implementation/
      SKILL.md
      reviewer-prompt.md
```

---

## Recommendations

### For AI-Assisted Commands (add, sync)

**1. Use internal/prompt for template rendering**

```go
// In cmd/add.go
import "github.com/abenz1267/muster/internal/prompt"
import "github.com/abenz1267/muster/internal/config"

func runAdd(cmd *cobra.Command, args []string) error {
    // 1. Load and resolve config
    userCfg, _ := config.LoadUserConfig("")
    projectCfg, _ := config.LoadProjectConfig(".")
    resolved, _ := config.ResolveStep("add", projectCfg, userCfg)

    // 2. Create prompt context
    ctx := prompt.NewPromptContext(
        resolved,
        userCfg,
        true,  // interactive
        "",    // no slug yet
        ".",   // worktreePath
        ".",   // mainRepoPath
        "",    // no planDir
    )

    // 3. Render prompt template
    // Create add-item-prompt.md.tmpl in internal/prompt/prompts/
    rendered, err := prompt.RenderTemplate("prompts/add-item-prompt.md.tmpl", ctx)
    if err != nil {
        return fmt.Errorf("failed to render add prompt: %w", err)
    }

    // 4. Execute tool with prompt
    cmd := exec.Command(resolved.Tool, "--prompt", "-")
    cmd.Stdin = strings.NewReader(rendered)
    var stdout bytes.Buffer
    cmd.Stdout = &stdout
    cmd.Stderr = os.Stderr

    if err := cmd.Run(); err != nil {
        return fmt.Errorf("AI invocation failed: %w", err)
    }

    // 5. Parse response
    response := stdout.String()
    // Parse JSON or markdown from response

    return nil
}
```

**2. Use fast model tier for lightweight operations**

For `add` and `sync` commands, use `{{.Models.Fast}}` in prompt templates. These operations don't require deep reasoning:

```markdown
# Add Item Prompt

You are assisting with roadmap item creation.

**Model**: {{.Models.Fast}}

Given the following input:
- Title: {{.Title}}
- Description: {{.Description}}

Generate a JSON object...
```

**3. Create dedicated prompt templates**

Add these to `internal/prompt/prompts/`:

- `add-item-prompt.md.tmpl` - For AI-assisted item creation
- `sync-match-prompt.md.tmpl` - For AI-assisted fuzzy matching
- `format-item-prompt.md.tmpl` - For formatting/validation

**4. Handle both interactive and non-interactive modes**

Use `{{if .Interactive}}` blocks to adjust prompts:

```markdown
{{if .Interactive}}
Please provide a roadmap item in JSON format. Ask clarifying questions if needed.
{{else}}
Generate a roadmap item in strict JSON format. No questions, just output.
{{end}}
```

**5. Response parsing strategy**

For structured output, instruct the AI to use JSON and parse accordingly:

```go
type AddItemResponse struct {
    Slug        string   `json:"slug"`
    Title       string   `json:"title"`
    Description string   `json:"description"`
    Tags        []string `json:"tags"`
}

var result AddItemResponse
if err := json.Unmarshal([]byte(response), &result); err != nil {
    // Fallback: use regex or markdown parsing
    return fmt.Errorf("failed to parse AI response: expected JSON: %w", err)
}
```

**6. Config resolution for each command**

Each AI-assisted command should resolve its own step config:

- `muster add` → resolve step `"add"`
- `muster sync` → resolve step `"sync"`

This allows users to configure different models per operation:

```yaml
# .muster/config.yml
pipeline:
  defaults:
    tool: claude-code
    model: muster-standard
  add:
    model: muster-fast  # Use fast model for simple operations
  sync:
    model: muster-standard  # Use standard model for matching
```

### For Error Handling

**1. Define sentinel errors**

```go
// In internal/ai/errors.go
var (
    ErrAIInvocationFailed = errors.New("AI invocation failed")
    ErrAIResponseParse    = errors.New("AI response parse error")
    ErrAIRateLimit        = errors.New("AI rate limit exceeded")
    ErrAIAuth             = errors.New("AI authentication failed")
)
```

**2. Wrap errors with context**

```go
if err := cmd.Run(); err != nil {
    return fmt.Errorf("%w: tool %s exited with error: %v",
        ErrAIInvocationFailed, resolved.Tool, err)
}
```

**3. Provide actionable messages**

```go
if errors.Is(err, ErrAIAuth) {
    return fmt.Errorf("authentication failed for provider %s: "+
        "check ANTHROPIC_API_KEY environment variable or credentials file: %w",
        resolved.Provider, err)
}
```

### Model Tier Usage Guidelines

Based on Phase 1 template patterns:

| Tier | Model Examples | Use Cases |
|------|---------------|-----------|
| Fast (`{{.Models.Fast}}`) | claude-haiku-4, gemma3:4b | Item creation, formatting, simple validation |
| Standard (`{{.Models.Standard}}`) | claude-sonnet-4.5, qwen3:14b | Fuzzy matching, coordination, synthesis |
| Deep (`{{.Models.Deep}}`) | claude-opus-4, qwen3:235b | Deep research, complex analysis, planning |

For `add` command: Use Fast tier (simple structured output)
For `sync` command: Use Standard tier (matching requires some reasoning)

---

## Open Questions

### 1. Should AI-assisted commands use stdin piping or temp files?

**Question**: Should rendered prompts be piped via stdin (`cmd.Stdin = strings.NewReader(prompt)`) or written to temp files and passed as arguments?

**Why it matters**:
- Stdin piping is simpler but limits debugging (can't inspect prompt)
- Temp files allow inspection but require cleanup and path handling

**What was tried**: Checked existing code - `cmd/code.go` uses `--plugin-dir` flag for directory-based plugins, no prompt piping

**Recommendation**: Use stdin piping for prompts (simpler), but add `--debug-prompt` flag that writes prompt to stderr before sending

### 2. How should response streaming be handled for interactive commands?

**Question**: Should AI responses be buffered fully before parsing, or streamed token-by-token to the user?

**Why it matters**:
- Buffering enables full parsing but delays first output
- Streaming provides better UX but complicates parsing

**What was tried**: Current `code` command connects tool's stdout directly to user's stdout (line 130), no intermediate handling

**Recommendation**: For `add`/`sync`, buffer fully (these need parsing). For future interactive commands, stream directly.

### 3. What is the expected response format from AI tools?

**Question**: Do claude-code/opencode support structured output flags (JSON mode, schema validation)?

**Why it matters**: Without structured output support, responses require robust parsing with fallback logic

**What was tried**: Searched codebase and templates - no examples of parsing tool output, only launching interactive sessions

**Recommendation**: Human input needed - test whether `claude-code --prompt -` returns structured output, or requires prompt engineering like "Output only JSON"

### 4. Should AI invocation be abstracted into a shared package?

**Question**: Should there be an `internal/ai/` package that wraps tool execution, or should each command handle it directly?

**Why it matters**:
- Abstraction reduces duplication but adds indirection
- Direct handling keeps commands simple but risks inconsistency

**What was tried**: No existing abstraction - `cmd/code.go` calls `exec.Command` directly

**Recommendation**: Start with direct execution in `cmd/add.go` and `cmd/sync.go`. Extract to `internal/ai/invoke.go` if duplication emerges (>3 commands doing the same pattern)

### 5. How should AI-assisted commands handle partial failures?

**Question**: If AI generates malformed JSON, should the command fail completely or prompt user to edit?

**Why it matters**:
- Strict failure prevents bad data but frustrates users
- Interactive editing improves UX but complicates implementation

**What was tried**: No examples in codebase - `code` command is all-or-nothing (launches session or fails)

**Recommendation**: For Phase 1 (roadmap-management), fail with clear error message. For Phase 2, consider interactive editing if users report frequent parse failures.

---

## References

### File Paths and Line Numbers

**Template System**:
- `internal/prompt/template.go:24-56` - `ParseTemplates()` function
- `internal/prompt/template.go:58-84` - `RenderTemplate()` function
- `internal/prompt/embed.go:5-6` - Embedded prompts filesystem
- `internal/prompt/stage.go:100-226` - `StageSkills()` function

**Context and Variable Substitution**:
- `internal/prompt/context.go:15-55` - `PromptContext` struct definition
- `internal/prompt/context.go:57-109` - `NewPromptContext()` function with tier population

**Config Resolution**:
- `internal/config/resolve.go:18-103` - `ResolveStep()` function (5-step chain)
- `internal/config/resolve.go:105-159` - `resolveModelTier()` function
- `internal/config/config.go:32-36` - Default configuration constants
- `internal/config/user.go:24-78` - User config loading

**AI Invocation**:
- `cmd/code.go:127-140` - Tool execution via `os/exec`
- `cmd/code.go:88-99` - Prompt context creation
- `cmd/code.go:134-140` - Error handling for exec failures

**Template Examples**:
- `internal/prompt/prompts/plan-feature/researcher-prompt.md.tmpl:1-35` - Conditional logic example
- `internal/prompt/prompts/plan-feature/research-runner.md.tmpl:1-48` - Model tier usage example

**Test Files** (for usage patterns):
- `internal/prompt/template_test.go:74-86` - Rendering example
- `internal/prompt/template_test.go:158-200` - Model tier population verification
- `internal/prompt/stage_test.go` - Staging flow examples

### Config Examples

**User Config Structure** (`~/.config/muster/config.yml`):
```yaml
defaults:
  tool: claude-code
  provider: anthropic
  model: muster-standard

tools:
  claude-code:
    model_tiers:
      fast: claude-haiku-4
      standard: claude-sonnet-4.5
      deep: claude-opus-4
  opencode:
    model_tiers:
      fast: gemma3:4b
      standard: qwen3:14b
      deep: qwen3:235b
```

**Project Config Structure** (`.muster/config.yml`):
```yaml
defaults:
  tool: claude-code
  model: muster-standard

pipeline:
  add:
    model: muster-fast
  sync:
    model: muster-standard
  plan:
    model: muster-deep
```
