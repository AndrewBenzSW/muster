# Roadmap Management: Implementation Plan

*Version: 2026-03-18 (rev 2 — incorporates adversarial review findings)*

This plan implements full roadmap CRUD operations for muster through three flat commands (`muster status`, `muster add`, `muster sync`), backed by a new `internal/roadmap/` package for file I/O and validation, output formatters and an interactive picker in `internal/ui/`, and two prompt templates for AI-assisted operations.

The architecture mirrors established patterns: backward-compatible file loading with fallback paths (as in `internal/config/project.go`), centralized output formatting switched on `ui.OutputMode`, config resolution via `config.ResolveStep()` for per-operation model selection, and template-based AI prompting through `internal/prompt/`. The picker uses `charmbracelet/huh` for fuzzy-filterable item selection, reusable by future commands (`in`, `out`, `plan`).

**AI invocation model**: The existing `muster code` command launches an interactive agent with staged skills via `--plugin-dir`. The `add` and `sync` commands use a different pattern — **non-interactive AI invocation** — where a rendered prompt is staged as a single-shot skill and the tool is launched with `--print` mode (or equivalent) to produce structured JSON output. This is implemented via a new `internal/ai/invoke.go` helper that encapsulates tool-specific invocation details.

All commands are thin orchestrators — load config, load roadmap, call business logic, format output — keeping testable logic in `internal/` packages.

---

## Technology Stack

| Layer | Technology |
|-------|-----------|
| CLI framework | Cobra (existing) |
| Config resolution | internal/config with 5-step fallback chain (existing) |
| Roadmap persistence | encoding/json with wrapper format |
| Table formatting | text/tabwriter (stdlib) |
| Interactive picker | charmbracelet/huh |
| AI invocation | internal/ai with tool-specific non-interactive mode |
| AI prompting | internal/prompt template system (existing) |
| Testing | Go testing + testify/assert + testify/require (existing) |

---

## Solution Structure

```
internal/
  roadmap/
    roadmap.go          # Types: RoadmapItem, Roadmap, Priority, Status, sentinel errors
    slug.go             # GenerateSlug() for title-to-slug conversion
    loader.go           # LoadRoadmap(), SaveRoadmap() with fallback + format detection
    validate.go         # Validate(), FindBySlug(), AddItem() with full validation
    roadmap_test.go     # Unit tests for types, validation, enums, slug generation
    loader_test.go      # Integration tests for load/save with fallback, format detection
  ai/
    invoke.go           # InvokeAI() — non-interactive AI tool invocation with JSON response
    invoke_test.go      # Tests with mock tool binary
  ui/
    picker.go           # Picker interface + huh implementation
    picker_test.go      # Tests using mock picker
    roadmap_format.go   # FormatRoadmapTable(), FormatRoadmapDetail(), FormatRoadmapItem()
    roadmap_format_test.go  # Tests for formatters in both output modes
  prompt/
    context.go          # (modified) Add Extra map[string]interface{} to PromptContext
    prompts/
      add-item/
        add-item-prompt.md.tmpl    # AI prompt for item generation
      sync-match/
        sync-match-prompt.md.tmpl  # AI prompt for fuzzy matching
cmd/
  status.go             # muster status [slug]
  status_test.go        # Command tests
  add.go                # muster add (batch + AI-assisted)
  add_test.go           # Command tests
  sync.go               # muster sync (unidirectional with fuzzy matching)
  sync_test.go          # Command tests
```

---

## Implementation Phases

### Phase 1: Data Models and File I/O

Foundation layer — types, persistence, and validation for roadmap data. Everything else depends on this.

| ID | Task | Key Details |
|----|------|-------------|
| 1.1 | Create `internal/roadmap/roadmap.go` | Define `RoadmapItem` struct (Slug, Title, Priority, Status, Context as required; PRUrl, Branch as `*string` with `omitempty`). Define `Priority` and `Status` typed string enums with `IsValid()` methods and `ValidPriorities()`/`ValidStatuses()` helpers. Define `Roadmap` wrapper struct and `ErrRoadmapParse` sentinel error. |
| 1.2 | Create `internal/roadmap/slug.go` | Implement `GenerateSlug(title string) string` — lowercase via `strings.ToLower()`, replace spaces/underscores with single hyphen, strip all chars not matching `[a-z0-9-]`, collapse consecutive hyphens, trim leading/trailing hyphens, truncate to 40 chars. Return empty string for empty input. |
| 1.3 | Create `internal/roadmap/loader.go` | Implement `LoadRoadmap(dir string) (*Roadmap, error)` with fallback: try `.muster/roadmap.json` first, fall back to `.roadmap.json` if not-exists, return empty `Roadmap` if neither exists. Parse error on primary blocks fallback. `loadRoadmapFile()` detects wrapper vs array format (try wrapper first, fall back to array). Implement `SaveRoadmap(dir string, roadmap *Roadmap) error` — always writes wrapper format to `.muster/roadmap.json`, creates directory with `MkdirAll(0755)`, writes with `0644` permissions and trailing newline. |
| 1.4 | Create `internal/roadmap/validate.go` | Implement `Validate()` on `*Roadmap` — checks required fields (slug, title, priority, status, context), unique slugs, valid enum values. Fatal errors return `error`. Implement `FindBySlug(slug string) *RoadmapItem`. Implement `AddItem(item RoadmapItem) error` — validates the incoming item's required fields, enum values via `Priority.IsValid()` and `Status.IsValid()`, AND checks duplicate slug before append. Return descriptive error for any invalid item. |
| 1.5 | Create `internal/roadmap/roadmap_test.go` | Table-driven tests for: enum validation (`IsValid` for all valid/invalid values), `Validate()` (required fields, duplicates, invalid enums, empty roadmap OK), `FindBySlug()` (found, not found), `AddItem()` (success, duplicate, invalid enum, missing field). Slug generation tests: basic title, unicode, empty, very long, special chars, consecutive spaces. |
| 1.6 | Create `internal/roadmap/loader_test.go` | Integration tests using `t.TempDir()`: load from new location, fallback to legacy, both exist (new wins), neither exists (empty), malformed new doesn't fallback, array format parses, save creates directory, save writes wrapper format, save file permissions (0644), save trailing newline, round-trip preserves data, round-trip migrates array to wrapper. |

**Dependencies:** None

### Phase 2: Output Formatting and Picker

UI components — formatters for table/detail/JSON output and testable interactive picker.

| ID | Task | Key Details |
|----|------|-------------|
| 2.1 | Add `charmbracelet/huh` dependency | Run `go get github.com/charmbracelet/huh` and `go mod tidy`. |
| 2.2 | Create `internal/ui/picker.go` with interface | Define `Picker` interface: `Show(title string, options []PickerOption, cfg PickerConfig) (string, error)`. Define `PickerOption` struct (Label, Value string), `PickerConfig` struct (Height int, DefaultValue string, FilteringLabel string), `DefaultPickerConfig()`. Implement `HuhPicker` struct that wraps `charmbracelet/huh` — uses `huh.NewSelect[string]` with `.Filtering(true)`. Returns error on empty options or cancellation (ESC/Ctrl+C). Package-level `DefaultPicker` variable set to `&HuhPicker{}` so commands use `ui.DefaultPicker.Show(...)` and tests can swap in a mock. |
| 2.3 | Create `internal/ui/roadmap_format.go` | Define `RoadmapTableItem` and `RoadmapDetailItem` display structs (decoupled from `roadmap.RoadmapItem`). `FormatRoadmapTable()` — TableMode uses `text/tabwriter` with SLUG/TITLE/PRIORITY/STATUS columns, empty list shows "No roadmap items found. Run 'muster add' to create one." JSONMode outputs unwrapped array `[{...}, ...]` (not wrapper format) via `json.MarshalIndent`, empty outputs `[]`. `FormatRoadmapDetail()` — TableMode shows labeled key-value layout, JSONMode outputs single object. `FormatRoadmapItem()` — brief confirmation after add/sync. |
| 2.4 | Create `internal/ui/picker_test.go` | Test via mock: create `MockPicker` implementing `Picker` interface, test `Show()` returns selected value, empty options error, cancellation error. Test `HuhPicker` option building logic (verifying huh options constructed correctly). |
| 2.5 | Create `internal/ui/roadmap_format_test.go` | Tests for all three formatters in both TableMode and JSONMode. Test empty roadmap: table shows friendly message, JSON outputs `[]`. Verify JSON is valid and parseable (unmarshal back). Use output mode save/restore pattern from existing tests. |

**Dependencies:** Phase 1 (types used by formatters)

### Phase 3: Status Command and AI Infrastructure

First command (read-only) plus the shared AI invocation helper needed by later phases.

| ID | Task | Key Details |
|----|------|-------------|
| 3.1 | Create `cmd/status.go` | Register `statusCmd` via `init()` with `Use: "status [slug]"`, `Args: cobra.MaximumNArgs(1)`. `RunE`: load roadmap via `roadmap.LoadRoadmap(".")`, if slug arg provided find item and format with `ui.FormatRoadmapDetail()`, otherwise format all items with `ui.FormatRoadmapTable()`. Output via `cmd.OutOrStdout()`. Handle `ErrRoadmapParse` with file-specific error message. Include `toTableItems()` and `toDetailItem()` private helpers. |
| 3.2 | Create `cmd/status_test.go` | Tests: command exists, accepts max one arg, table output with items, detail output with slug, empty roadmap friendly message, JSON mode table output, JSON mode detail output, empty roadmap JSON (empty array `[]`), invalid slug error, malformed roadmap error. Follow `version_test.go` pattern. |
| 3.3 | Create `internal/ai/invoke.go` | Implement `InvokeAI(cfg InvokeConfig) (*InvokeResult, error)`. `InvokeConfig` holds: `Tool string`, `Prompt string`, `Verbose bool`. Stages the prompt as a single-shot skill file in a temp directory, executes `exec.Command(tool, "--print", "--plugin-dir", tmpDir)` to get non-interactive JSON output. Captures stdout to buffer, stderr to os.Stderr. Returns `InvokeResult{RawOutput string}`. Error handling: `exec.ErrNotFound` → tool installation guidance, non-zero exit → include stderr, timeout after 60s. Cleanup temp dir on completion. If `Verbose`, prints tool command and resolved config to stderr. |
| 3.4 | Create `internal/ai/invoke_test.go` | Tests with a mock tool binary (small Go program compiled in `TestMain` that reads skill dir and echoes JSON): successful invocation returns output, tool not found error, non-zero exit error, empty output handling. |
| 3.5 | Add `Extra` field to `internal/prompt/context.go` | Add `Extra map[string]interface{}` field to `PromptContext` struct with `json:"-"` tag. Templates access via `{{.Extra.Key}}`. This allows sync templates to pass `SourceItems`/`TargetItems` without modifying the shared struct. Update `NewPromptContext()` to initialize `Extra` as empty map. |

**Dependencies:** Phase 1, Phase 2

### Phase 4: Add Command

Batch mode (flags, no AI) and AI-assisted interactive mode for item creation.

| ID | Task | Key Details |
|----|------|-------------|
| 4.1 | Create `internal/prompt/prompts/add-item/add-item-prompt.md.tmpl` | Template instructs AI to generate JSON with explicit schema: `{"slug": "...", "title": "...", "priority": "high|medium|low|lower", "status": "planned", "context": "..."}`. Rules: slug derived from title (kebab-case, max 40 chars), priority suggested from context (default medium), context expanded from user input. Uses `{{.Models.Fast}}` tier. Output ONLY the JSON object, no fences. |
| 4.2 | Create `cmd/add.go` | Register `addCmd` via `init()` with flags: `--title` (string), `--priority` (string, default `string(roadmap.PriorityMedium)`), `--status` (string, default `string(roadmap.StatusPlanned)`), `--context` (string, supports `-` for stdin). **Config loading**: follow `cmd/code.go:38-54` pattern — load user config, load project config, handle `config.ErrConfigParse` separately with user guidance. Resolve config via `config.ResolveStep("add", projectCfg, userCfg)`. **Verbose logging**: when `--verbose` set, print resolved triple (tool, provider, model), template path, and invocation command to stderr. **Batch mode** (when `--title` provided): generate slug via `roadmap.GenerateSlug(title)`, read context from stdin if `--context` is `-` (use `io.ReadAll(os.Stdin)` with 1MB limit, error if empty or exceeds limit, trim whitespace), create item from flags, validate via `AddItem()`, save, format confirmation. **Interactive/AI mode** (no `--title`): check `ui.IsInteractive()` (error if non-TTY), render `add-item-prompt.md.tmpl` with `PromptContext`, invoke via `ai.InvokeAI()`, parse JSON response into `RoadmapItem`, if parse fails return error with raw response for debugging, show generated item to user via picker (confirm/edit/cancel), on confirm add and save. |
| 4.3 | Create `cmd/add_test.go` | Tests: command exists, has expected flags (title, priority, status, context with correct types), batch mode adds item and generates slug correctly, default priority is "medium" and default status is "planned", duplicate slug error, interactive mode requires terminal (error in non-TTY with helpful message), context from stdin (`--context -`), config load error categorization (ErrConfigParse → "config file malformed"). AI integration tests skipped with TODO noting they need mock tool binary from 3.4. |

**Dependencies:** Phase 1, Phase 2, Phase 3 (for ai.InvokeAI and PromptContext.Extra)

### Phase 5: Sync Command

Unidirectional sync with exact slug matching and AI-assisted fuzzy matching.

| ID | Task | Key Details |
|----|------|-------------|
| 5.1 | Create `internal/prompt/prompts/sync-match/sync-match-prompt.md.tmpl` | Template accepts source and target item lists via `{{range .Extra.SourceItems}}` and `{{range .Extra.TargetItems}}`. Outputs JSON array of match objects: `[{"source_slug": "...", "target_slug": "...", "confidence": 0.95, "reason": "..."}]`. Uses `{{.Models.Fast}}`. Output ONLY the JSON array. |
| 5.2 | Create `cmd/sync.go` | Register `syncCmd` via `init()` with flags: `--source` (string, default ".roadmap.json"), `--target` (string, default ".muster/roadmap.json"), `--yes` (bool), `--dry-run` (bool), `--delete` (bool). **Config loading**: follow `cmd/code.go:38-54` pattern with `config.ResolveStep("sync", ...)`. **Verbose logging**: print resolved config to stderr when `--verbose` set. **Sync algorithm**: (1) Load source file directly via `roadmap.loadRoadmapFile()` (not the fallback chain). Load target via `roadmap.LoadRoadmap()` (with fallback, returns empty if not exists). (2) **Exact slug match**: iterate source items, find matching slug in target. Mark both as matched. For matched items: overwrite all fields from source (title, priority, status, context); for optional fields (pr_url, branch), if source field is non-nil overwrite target (including nil to clear), if source field is nil preserve target value. (3) **AI fuzzy match** for remaining unmatched items: if both unmatched-source and unmatched-target are non-empty, populate `PromptContext.Extra` with `SourceItems` and `TargetItems`, render `sync-match-prompt.md.tmpl`, invoke via `ai.InvokeAI()`, parse JSON response into `[]MatchResult` struct. Print "Matching items with AI (this may take a moment)..." to stderr before invocation. If AI returns empty array or parse fails, treat all remaining as unmatched (add as new). **Confidence threshold**: matches with confidence >= 0.7 are auto-accepted. Matches with confidence < 0.7 are shown to user via picker with options: "Accept match (source-slug → target-slug, confidence: N%)" or "Skip (add as new item)" — unless `--yes` flag, in which case all AI matches regardless of confidence are accepted. (4) **Apply changes**: update matched target items from source, add unmatched source items as new, if `--delete` remove target items not matched by any source item (mark as deletions). (5) **Dry-run**: if `--dry-run`, display summary (N updated, M added, K deleted) with item details and exit without saving. (6) **Save and summarize**: save target roadmap, print summary to stdout. |
| 5.3 | Create `cmd/sync_test.go` | Tests: command exists, has expected flags (source, target, yes, dry-run, delete), dry-run doesn't modify target file, exact slug match updates fields, adds new items when slug doesn't match, `--delete` removes unmatched target items, without `--delete` preserves extra items, source not found error, `--yes` skips confirmation, config load error categorization. Tests use pre-written source/target JSON files in `t.TempDir()`. AI fuzzy matching tests skipped with TODO. |
| 5.4 | Update `internal/prompt/stage.go` | Add `"roadmap-add-item"` and `"roadmap-sync-match"` to `expectedSkills` verification list if the templates are staged alongside existing skills. If add/sync templates are only used via `ai.InvokeAI()` (separate staging), this task is skipped. |

**Dependencies:** Phase 1, Phase 2, Phase 3 (for ai.InvokeAI and PromptContext.Extra)

### Phase Dependency Graph

```
Phase 1: Data Models & File I/O
    |
    v
Phase 2: Output Formatting & Picker
    |
    v
Phase 3: Status Command & AI Infrastructure
    |
    +------+------+
    |             |
    v             v
Phase 4         Phase 5
Add Command     Sync Command
```

Phases 4 and 5 are independent of each other and can be implemented in parallel after Phase 3. Within Phase 4, Task 4.1 (template) must complete before 4.2 (command). Within Phase 5, Task 5.1 (template) must complete before 5.2 (command).

---

## Acceptance Criteria

| # | Criterion |
|---|-----------|
| AC-1 | `roadmap.LoadRoadmap()` loads from `.muster/roadmap.json`, falls back to `.roadmap.json`, returns empty `Roadmap` if neither exists |
| AC-2 | Parse error on `.muster/roadmap.json` returns error immediately without trying fallback |
| AC-3 | Both wrapper `{"items": [...]}` and array `[...]` JSON formats parse correctly |
| AC-4 | `roadmap.SaveRoadmap()` always writes wrapper format to `.muster/roadmap.json` with 0644 permissions |
| AC-5 | `Validate()` rejects missing required fields, duplicate slugs, and invalid enum values |
| AC-6 | `muster status` displays all items in table format with SLUG/TITLE/PRIORITY/STATUS columns |
| AC-7 | `muster status <slug>` displays detailed view for a specific item |
| AC-8 | `muster status --format json` outputs valid JSON array `[...]` (not wrapper format); empty roadmap outputs `[]` |
| AC-9 | Empty roadmap shows "No roadmap items found. Run 'muster add' to create one." in table mode |
| AC-10 | `muster add --title "My Feature" --context "details"` creates item in batch mode without AI |
| AC-11 | `muster add` in interactive mode stages prompt template and invokes AI tool via `ai.InvokeAI()` for item generation |
| AC-12 | `muster sync --dry-run` displays planned changes without modifying target |
| AC-13 | `muster sync` adds new items from source and updates matched items in target |
| AC-14 | `muster sync --delete` removes target items not found in source and not matched by AI |
| AC-15 | `muster sync --yes` skips confirmation prompts for automated workflows |
| AC-16 | Interactive picker displays fuzzy-filterable list via `Picker` interface; testable with mock implementation |
| AC-17 | Picker returns error on cancellation (ESC/Ctrl+C) and empty options |
| AC-18 | All tests pass with `go test -race ./...` |
| AC-19 | `go vet ./...` and `golangci-lint run` report no issues |
| AC-20 | Config load errors in add/sync are categorized (`ErrConfigParse` → "config file malformed") following `cmd/code.go` pattern |
| AC-21 | `AddItem()` validates the incoming item's required fields and enum values before appending |
| AC-22 | Sync fuzzy matches with confidence >= 0.7 are auto-accepted; lower confidence triggers picker confirmation |
| AC-23 | `GenerateSlug()` produces valid kebab-case slugs, handles unicode, max 40 chars |

---

## Test Strategy

**Layers:**
- **Unit tests** for pure logic: enum validation, `Validate()`, `FindBySlug()`, `AddItem()`, `GenerateSlug()`, formatters in both output modes, picker via mock interface
- **Integration tests** for file I/O: load/save with `t.TempDir()`, fallback paths, format detection, round-trip, permissions
- **Command tests** for CLI: flag existence/types, output capture via `cmd.OutOrStdout()`, error messages, output mode switching, config error categorization
- **AI invocation tests**: mock tool binary compiled in `TestMain`, verifies prompt staging, stdout capture, error handling

**Key scenarios per component:**
- `internal/roadmap/`: 12+ load/save scenarios, slug generation edge cases, full validation matrix
- `internal/ai/`: Mock tool binary tests for success, not-found, non-zero exit, timeout
- `internal/ui/`: Mock picker tests, formatter tests in both modes, empty state handling
- `cmd/`: Each command tested for flags, happy path, error cases, both output modes, config error categorization

**Quality gates:**
- Race detector clean (`go test -race`) — roadmap package is synchronous, no goroutines; picker wraps huh (assumed race-safe); OutputMode protected by mutex
- All MUST requirements have tests
- Backward compatibility explicitly tested (legacy location, array format)
- Both TableMode and JSONMode verified for every command
- Config error categorization tested for add and sync

**Conventions:** Table-driven tests, `testify/assert` + `testify/require`, `t.TempDir()` for file tests, output mode save/restore pattern, test naming: `Test<Component>_<Scenario>_<Outcome>`

---

## Known Limitations

- **AI integration tests use mock binary**: Tests compile a small Go program that echoes JSON, not an actual AI tool. Full AI integration is verified manually.
- **Picker interactive selection untestable**: `charmbracelet/huh` doesn't expose programmatic test API. Mitigated by `Picker` interface — all callers tested with mock; huh integration verified manually.
- **Sync is unidirectional only**: Source → target. Bidirectional merge deferred to future enhancement.
- **No automatic legacy file migration**: Loading from `.roadmap.json` doesn't move the file. Migration is implicit on next save.
- **No local roadmap override**: `.muster/roadmap.local.json` deferred — roadmap is typically version-controlled.
- **No multi-select picker**: Single-select only for MVP. Multi-select deferred to future batch operations.
- **AI non-interactive mode assumes `--print` flag**: If the resolved tool doesn't support `--print`, `ai.InvokeAI()` will need tool-specific flag mapping. Currently only validated for claude-code.
