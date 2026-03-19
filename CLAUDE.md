# muster

A Go CLI that orchestrates AI-assisted development workflows. It manages roadmap items through a configurable pipeline (plan, execute, verify, review) using AI coding agents.

## Build, Test, Lint

```bash
make build          # Build to dist/muster (CGO_ENABLED=0)
make test           # go test -v -race ./...
make lint           # golangci-lint run --timeout=5m
```

All three must pass before committing. CI runs tests on Linux, macOS, and Windows.

## Project Structure

```
cmd/                  Cobra command definitions (root, code, add, sync, status, down, version)
internal/
  ai/                 AI tool invocation (stages prompt, runs claude --print, captures output)
  config/             Two-layer config system (user + project) with 5-step resolution chain
  docker/             Docker orchestration for --yolo mode (compose generation, auth, worktree)
  prompt/             Template rendering and skill staging for Claude Code plugins
    prompts/          Embedded Go templates (.md.tmpl) for skills and AI prompts
    testdata/         Golden files for template rendering tests
  roadmap/            Roadmap data model, JSON loading/saving, slug generation
  testutil/           Shared test helpers
  ui/                 Output formatting (table/JSON auto-detect), TUI pickers
```

## Key Patterns

- **Config resolution**: 5-step fallback chain — pipeline step > project defaults > user defaults > tool defaults > hard-coded defaults (`claude-code`/`anthropic`/`sonnet`). See `internal/config/resolve.go`. Project config supports the same `tools`, `providers`, and `model_tiers` fields as user config, with project taking precedence. The `.muster/config.local.yml` override merges on top of `.muster/config.yml`.
- **Model tiers**: `muster-fast`, `muster-standard`, `muster-deep` are tier aliases resolved per-tool to concrete model names. Resolution order: project tool tiers > project model tiers > user tool tiers > user model tiers. Steps can have default tiers (e.g., `add` and `sync` default to `muster-fast`). The `ModelTiersConfig` named type is shared across all config levels.
- **Prompt templates**: `.md.tmpl` files in `internal/prompt/prompts/` are embedded via `//go:embed` and rendered with `text/template`. Templates receive a `PromptContext` with resolved config, paths, and model tiers. Golden file tests verify rendered output.
- **Skill staging**: `prompt.StageSkills()` renders templates to a temp dir (`muster-prompts-*`) passed to Claude Code via `--plugin-dir`. Skill SKILL.md files should NOT have `name:` frontmatter — let Claude Code derive names from directory structure to avoid collisions with user-installed skills.
- **Roadmap files**: Primary location `.muster/roadmap.json`, legacy fallback `.roadmap.json`. Supports both `{"items":[...]}` wrapper and bare `[...]` array formats. Always writes wrapper format.
- **AI invocation**: `ai.InvokeAI()` stages a single-shot prompt, runs `claude --print --plugin-dir`, captures stdout. Used by `add` and `sync` commands. Supports `Env` overrides for provider-specific env vars.
- **Provider env overrides**: `config.ToolEnvOverrides()` maps provider config to tool-specific env vars. For `claude-code`, a provider's `base_url` becomes `ANTHROPIC_BASE_URL`, enabling local model servers (e.g., LM Studio, Ollama). All commands (`code`, `add`, `sync`) apply these overrides and pass `--model` from the resolved config.
- **Cross-platform**: Uses `filepath.Join`, `os.TempDir`, `os.UserConfigDir` for path portability. `.gitattributes` enforces LF line endings on source files. Tests verify LF preservation.
- **Nolint annotations**: Security-related suppressions (`gosec`) are used intentionally for temp dir permissions (0755), file permissions (0644), and exec.Command with config-controlled paths. Each has an inline comment explaining why.

## Testing

- Tests use `testify/assert` and `testify/require`.
- Golden file tests compare rendered template output against `.golden` files in `testdata/`. Update golden files when templates change.
- Template parsing uses `sync.Once` — tests that need fresh parsing must account for this.
- `internal/testutil/helpers.go` provides shared test utilities.

## Roadmap Data

Roadmap items have: `slug` (unique kebab-case ID, max 40 chars), `title`, `priority` (high/medium/low/lower), `status` (planned/in_progress/completed/blocked), `context`, and optional `pr_url`/`branch`.

## Commands Not Yet Implemented

`muster in`, `muster out`, `muster plan`, `muster init`, `muster doctor`, `muster update` are planned for future phases. The `--yolo` flag on `code` is gated. The `down` command exists but is only useful once container orchestration is active.
