# Roadmap Management: Requirements Synthesis

*Synthesized: 2026-03-18*
*Sources: config-loading.md, cli-structure.md, ai-integration.md, picker-libraries.md*

---

## Executive Summary

The research reveals a mature, well-established pattern system across muster's configuration loading, CLI structure, and AI orchestration layers. The roadmap management feature can leverage these patterns directly with minimal new abstractions. Key findings:

**Configuration and file handling** follows a proven backward-compatibility strategy with fallback paths, graceful handling of missing files, and deep-merge for overrides. The roadmap loader should mirror this pattern, supporting both legacy `.roadmap.json` and new `.muster/roadmap.json` locations with automatic format detection for array vs. wrapper structures.

**CLI architecture** uses Cobra with a centralized output formatting system that switches between TableMode and JSONMode based on the `--format` flag. New roadmap commands should integrate seamlessly by using the existing `internal/ui` package's formatters and following established command registration patterns. The flat command structure (`muster status`, `muster add`, `muster sync`) is more appropriate than nested subcommands given CLI brevity goals.

**AI integration** operates through orchestration rather than direct API calls. Muster stages prompt templates, resolves tool/provider/model configuration through a 5-step fallback chain, and delegates actual AI invocation to external tools (claude-code, opencode). The `add` and `sync` commands should follow this pattern, using the fast model tier for lightweight operations and maintaining the template-based approach for consistency.

**Interactive picker implementation** should use `charmbracelet/huh` based on its modern API, active maintenance, built-in fuzzy filtering, type safety, and Bubbletea integration for future extensibility. This provides the best balance of ergonomics and functionality while remaining composable with potential future TUI enhancements.

The most important decisions are: (1) establishing clear file location precedence rules, (2) determining the scope of AI assistance (generation vs. validation), and (3) confirming whether the picker needs multi-select capabilities from day one.

## Requirements

### MUST Have

**File Operations**

1. **Backward-compatible roadmap loading** that tries `.muster/roadmap.json` first, falls back to `.roadmap.json` if not found, and returns an empty roadmap (not an error) if neither exists. *(Source: config-loading.md lines 12-36, 241-275)*

2. **Automatic format detection** that parses both legacy array format (`[{...}]`) and new wrapper format (`{"items": [{...}]}`) transparently during load. *(Source: config-loading.md lines 277-304)*

3. **Consistent save format** that always writes to `.muster/roadmap.json` in wrapper format with proper error wrapping and 0644 permissions. *(Source: config-loading.md lines 306-337, 219)*

4. **Parse error isolation** where malformed primary file (.muster/roadmap.json) returns error immediately without trying fallback, preventing silent data loss. *(Source: config-loading.md lines 117-121, 441-444)*

5. **Post-load validation** that checks required fields (slug, title, priority, status, context), enforces unique slugs, and validates status enum values. *(Source: config-loading.md lines 352-385)*

**CLI Commands**

6. **Status command** (`muster status [slug]`) that displays all roadmap items in table format by default, or detailed view for a specific slug. Must respect `--format` flag for JSON output. *(Source: cli-structure.md lines 296-327)*

7. **Add command** (`muster add`) that creates roadmap items with AI assistance in interactive mode, or accepts `--title`, `--priority`, `--context` flags for non-interactive batch mode. *(Source: cli-structure.md lines 329-363)*

8. **Sync command** (`muster sync`) that reconciles differences between source and target roadmap files using AI-assisted fuzzy matching, with `--yes` flag to skip confirmation and `--dry-run` flag to preview changes. *(Source: cli-structure.md lines 366-403)*

9. **Cobra command registration** via init() functions that adds commands to rootCmd, following the established pattern from version.go and code.go. *(Source: cli-structure.md lines 26-46)*

10. **Centralized output formatting** through `internal/ui` package formatters (FormatRoadmapTable, FormatRoadmapDetail, FormatRoadmapItem) that switch on ui.OutputMode. *(Source: cli-structure.md lines 89-143, 417-421)*

**Data Structures**

11. **RoadmapItem struct** with required fields (Slug, Title, Priority, Status, Context) as strings, and optional fields (PRUrl, Branch) as string pointers with `omitempty` JSON tags. *(Source: config-loading.md lines 389-401, cli-structure.md lines 429-438)*

12. **Priority and status enums** with validation for priority values (high|medium|low|lower) and status values (planned|in_progress|completed|blocked). *(Source: config-loading.md lines 372-379)*

**AI Integration**

13. **Config resolution per operation** using `config.ResolveStep("add"|"sync", projectCfg, userCfg)` to support operation-specific model overrides through the 5-step fallback chain. *(Source: ai-integration.md lines 111-159, 422-441)*

14. **Template-based prompting** by creating operation-specific templates in `internal/prompt/prompts/` (add-item-prompt.md.tmpl, sync-match-prompt.md.tmpl) that use PromptContext variables. *(Source: ai-integration.md lines 315-363, 383-402)*

15. **Fast model tier for lightweight operations** using `{{.Models.Fast}}` in add/sync prompt templates since these operations don't require deep reasoning. *(Source: ai-integration.md lines 365-381, 479-488)*

**Interactive Picker**

16. **Fuzzy-filterable item picker** implemented in `internal/ui/picker.go` using `charmbracelet/huh` with built-in filtering enabled by default. *(Source: picker-libraries.md lines 10-22, 325-378)*

17. **Type-safe picker API** that accepts slice of PickerOption structs (label + value), returns selected value and error for cancellation handling. *(Source: picker-libraries.md lines 336-378)*

### SHOULD Have

**File Operations**

18. **Error context wrapping** that includes file paths in all error messages using `fmt.Errorf(..., %w)` pattern and considers introducing `ErrRoadmapParse` sentinel error. *(Source: config-loading.md lines 88-105, 346-349)*

19. **Directory creation safety** that uses `os.MkdirAll` with 0755 permissions before writing to ensure `.muster/` directory exists. *(Source: config-loading.md lines 310-314)*

**CLI Commands**

20. **Comprehensive test coverage** with dedicated _test.go files for each command covering flag existence/types, output mode switching (TableMode/JSONMode), error conditions, and integration scenarios. *(Source: cli-structure.md lines 242-293, 422-427)*

21. **Verbose flag integration** that outputs debug information to stderr when root's `--verbose` flag is set, showing resolved configuration and AI model selection. *(Source: cli-structure.md lines 186-194)*

22. **Helpful error messages** with user guidance for common failures like malformed roadmap files, missing API keys, or tool not found errors. *(Source: cli-structure.md lines 170-174, ai-integration.md lines 222-259)*

23. **Table formatting with text/tabwriter** for status command table output, starting with stdlib approach before considering external libraries. *(Source: cli-structure.md lines 447-451)*

**AI Integration**

24. **Response parsing with JSON fallback** that attempts to unmarshal AI responses as JSON first, then falls back to regex or markdown parsing if JSON parse fails. *(Source: ai-integration.md lines 190-219, 403-421)*

25. **Stdin prompt piping** that sends rendered prompts via `cmd.Stdin = strings.NewReader(prompt)` with optional `--debug-prompt` flag to write prompt to stderr for debugging. *(Source: ai-integration.md lines 493-503)*

26. **Response buffering for parsing** that captures tool stdout to buffer for structured output extraction rather than streaming directly to user. *(Source: ai-integration.md lines 505-516)*

27. **Error categorization** with checks for tool not found (exec.ErrNotFound), rate limits, authentication failures, and template render errors using errors.As/Is patterns. *(Source: ai-integration.md lines 222-259, 442-474)*

**Interactive Picker**

28. **Picker configuration struct** that supports optional settings like height, default selection, and future dynamic options function. *(Source: picker-libraries.md lines 446-455)*

29. **Cancellation error handling** that returns error when user presses ESC/Ctrl+C, allowing caller to decide whether to exit, retry, or show message. *(Source: picker-libraries.md lines 434-440)*

### NICE TO HAVE

30. **Local roadmap override pattern** (`.muster/roadmap.local.json`) with deep merge for development workflows, though roadmap is typically version controlled. *(Source: config-loading.md lines 405-410, context about local overrides)*

31. **Explicit migration command** (`muster roadmap migrate`) that moves legacy `.roadmap.json` to `.muster/roadmap.json` with user confirmation and optional deletion of old file. *(Source: config-loading.md lines 416-435)*

32. **Multi-select picker support** for batch operations like marking multiple items completed, implementable via `huh.NewMultiSelect()` with minimal API changes. *(Source: picker-libraries.md lines 408-415)*

33. **Theming customization** that exposes huh's five predefined themes (Charm, Dracula, Catppuccin, Base16, Catppuccin Mocha) via config option if users request styling control. *(Source: picker-libraries.md lines 416-424)*

34. **Dynamic picker options** that filter roadmap items by status/priority using `OptionsFunc` for dependent selections in multi-step flows. *(Source: picker-libraries.md lines 441-455)*

35. **Accessibility mode** that enables huh's screen reader/dictation support via environment variable or flag for users with visual impairments. *(Source: picker-libraries.md lines 20, 82)*

36. **Model tier override flags** (`--model`) for add/sync commands following code.go pattern, allowing users to specify different models per invocation. *(Source: cli-structure.md lines 463-467, ai-integration.md lines 422-441)*

37. **Streaming response preview** for interactive mode that shows AI generation in progress, though requires more complex implementation than buffered approach. *(Source: ai-integration.md lines 505-516)*

### SHOULD NOT Include

38. **Direct AI API integration** - Maintain orchestration pattern by delegating to external tools (claude-code, opencode) rather than implementing HTTP clients for Anthropic/OpenAI APIs. *(Source: ai-integration.md lines 10-22, justification: consistency with existing architecture)*

39. **Nested subcommand structure** (`muster roadmap status`) - Use flat command structure for CLI brevity since roadmap operations are core workflows. *(Source: cli-structure.md lines 196-211, justification: user ergonomics)*

40. **Automatic file migration on read** - Don't move files from `.roadmap.json` to `.muster/roadmap.json` during load operations; only migrate during explicit save operations. *(Source: config-loading.md lines 416-424, justification: prevents surprising file modifications)*

41. **Legacy file cleanup** - Don't automatically delete `.roadmap.json` after migrating to `.muster/roadmap.json`; require explicit user action to avoid data loss risk. *(Source: config-loading.md lines 426-435, justification: safety)*

42. **JSON array format on save** - Always use wrapper format (`{"items": [...]}`) when saving; don't support saving in legacy array format even if loaded from it. *(Source: config-loading.md lines 446-454, justification: forward migration path)*

43. **Schema validation before unmarshal** - Rely on json.Unmarshal for structure validation and implement semantic validation post-load rather than JSON schema enforcement. *(Source: config-loading.md lines 149-195, justification: simplicity and established pattern)*

44. **Standalone picker package** - Keep picker integrated in `internal/ui` rather than extracting to separate package since it's UI-related and not general-purpose. *(Source: picker-libraries.md analysis, justification: cohesion)*

45. **Bubbletea direct usage for pickers** - Use huh's higher-level API rather than implementing pickers with raw Bubbletea Model/Update/View, reserving Bubbletea for complex future TUIs. *(Source: picker-libraries.md lines 194-250, justification: avoid boilerplate for simple use cases)*

## Key Decisions

### Decision 1: Use charmbracelet/huh for Interactive Picker

**Rationale:** huh provides optimal balance of API ergonomics, built-in fuzzy filtering, type safety via generics, active maintenance (March 2026), and Bubbletea integration for future extensibility. The library is specifically designed for forms and pickers, avoiding boilerplate while maintaining composability.

**Alternatives considered:**
- pterm: Fewer dependencies but string-only values, less composable
- promptui: Minimal dependencies but unmaintained since 2021
- Bubbletea directly: Too low-level, requires Model/Update/View boilerplate

**Evidence:** picker-libraries.md lines 10-90, 325-378

**Impact:** Adds 10 direct dependencies from Charm ecosystem, moderate binary size increase, but provides best developer experience and future-proofing.

### Decision 2: Prioritize .muster/roadmap.json Over .roadmap.json

**Rationale:** New location takes precedence if exists; legacy is fallback only. This establishes clear migration path and aligns with project's move from `.dev-agent/` to `.muster/` namespace. All save operations write to new location exclusively.

**Alternatives considered:**
- Prompt user when both exist: Too disruptive
- Use .roadmap.json if newer: Confusing precedence
- Fail if both exist: Blocks operations unnecessarily

**Evidence:** config-loading.md lines 241-275, 416-424, cli-structure.md lines 458-462

**Impact:** Users with existing `.roadmap.json` see transparent backward compatibility; new installs use `.muster/roadmap.json` from start.

### Decision 3: Auto-Detect Format but Always Save Wrapper

**Rationale:** Load transparently handles both `[...]` array and `{"items": [...]}` wrapper formats by attempting wrapper parse first, falling back to array. Save always uses wrapper format. This provides forward migration without breaking existing files.

**Alternatives considered:**
- Require manual format migration: Poor UX
- Preserve loaded format on save: Prevents standardization
- Reject array format: Breaks backward compatibility

**Evidence:** config-loading.md lines 277-304, 446-454

**Impact:** Gradual migration to wrapper format as users modify roadmaps; no breaking changes for existing users.

### Decision 4: Parse Errors Block Fallback Attempts

**Rationale:** If `.muster/roadmap.json` exists but is malformed, return error immediately without trying `.roadmap.json`. This prevents masking configuration errors and silently using stale data, following established config loader pattern.

**Alternatives considered:**
- Always try fallback: Masks corruption
- Warn but use fallback: Still risks using wrong file
- Auto-repair malformed files: Dangerous, could lose data

**Evidence:** config-loading.md lines 117-121, 441-444

**Impact:** Users see clear error messages for malformed files rather than mysterious fallback behavior.

### Decision 5: AI Orchestration via External Tools

**Rationale:** Continue pattern of staging templates and invoking claude-code/opencode rather than implementing direct API integration. Maintains architectural consistency and delegates authentication/rate limiting/streaming to specialized tools.

**Alternatives considered:**
- Direct Anthropic/OpenAI API calls: Duplicates tool functionality
- Mix of direct and orchestrated: Inconsistent architecture
- Require users to install libraries: Poor UX

**Evidence:** ai-integration.md lines 10-22, 150-189

**Impact:** Simpler implementation, no API client maintenance, but dependent on external tool behavior.

### Decision 6: Use Fast Model Tier for Add/Sync Operations

**Rationale:** Item creation and fuzzy matching are lightweight operations that don't require deep reasoning. Using fast tier (claude-haiku-4, gemma3:4b) improves response time and reduces costs while maintaining quality for structured outputs.

**Alternatives considered:**
- Standard tier for everything: Slower, more expensive
- Let user choose per operation: Adds complexity
- Deep tier for sync: Overkill for matching

**Evidence:** ai-integration.md lines 365-381, 479-488

**Impact:** Faster AI responses for common operations; users can override via config if needed.

### Decision 7: Flat Command Structure (Not Grouped)

**Rationale:** Use `muster status`, `muster add`, `muster sync` rather than `muster roadmap status`. Roadmap operations are core workflows that benefit from CLI brevity. Follows existing pattern of flat commands (muster code, muster version).

**Alternatives considered:**
- Nested: `muster roadmap [subcommand]`: More typing, less ergonomic
- Mixed: Some flat, some nested: Inconsistent UX
- Single command with verbs: `muster roadmap --add`: Non-standard

**Evidence:** cli-structure.md lines 196-211

**Impact:** Shorter command invocations, consistent with existing CLI structure.

### Decision 8: Centralized Output Formatting via internal/ui

**Rationale:** Reuse existing OutputMode system and FormatX() pattern rather than implementing per-command formatting. Ensures consistent JSON/table output across all commands and respects root's `--format` flag automatically.

**Alternatives considered:**
- Per-command formatting: Code duplication
- Template-based output: Overkill for simple tables
- External table library: Add when proven necessary

**Evidence:** cli-structure.md lines 89-143, 417-421

**Impact:** Consistent user experience, reduced code duplication, simpler testing.

## Resolved Questions

### Q1: Sync direction → Unidirectional MVP
Start with source → target sync. Most use cases are updating `.muster/roadmap.json` from external sources. Add bidirectional later if needed.

### Q2: AI response format → Enforce JSON schema in prompt
Include explicit JSON schema in add-item-prompt.md.tmpl with slug, title, priority, status, context fields. Test with both claude-code and opencode.

### Q3: Picker in sync → Optional
With `--yes` flag, apply sync without picker. Otherwise show picker for manual conflict resolution when AI fuzzy matching has low confidence.

### Q4: Empty roadmap display → Friendly message + empty JSON
TableMode: "No roadmap items found. Run 'muster add' to create one." JSONMode: output empty array `[]` for pipe/jq compatibility.

### Q5: Context input modes → Three modes
Interactive (AI-assisted prompt), flag (`--context 'text'`), and stdin (`--context -` for piping). Follows `kubectl apply -f -` pattern.

### Q6: AI scope in add → Generate + suggest + validate
AI generates slug from title, suggests priority, formats/expands context, validates consistency. User can override all suggestions before saving.

### Q7: Validation severity → Fatal for required, warn for style
Fatal: missing required fields, duplicate slugs, invalid enums. Warnings: unconventional slug format, long titles, empty context.

### Q8: Sync deletions → --delete flag, default false
Add `--delete` flag that removes items from target if not in source. Without flag, preserve existing items. Show deletion count in dry-run.

## References

### Configuration Loading
- **Backward compatibility pattern**: config-loading.md lines 12-36, 241-275 (fallback path logic)
- **Format detection**: config-loading.md lines 277-304 (array vs wrapper parsing)
- **Error handling**: config-loading.md lines 88-105, 340-349 (three-tier classification)
- **Validation pattern**: config-loading.md lines 149-195, 352-385 (post-load semantic checks)
- **Save pattern**: config-loading.md lines 199-220, 306-337 (marshal, write, error wrapping)

### CLI Structure
- **Command registration**: cli-structure.md lines 26-46 (init() pattern)
- **Flag patterns**: cli-structure.md lines 58-88 (PersistentFlags vs Flags)
- **Output formatting**: cli-structure.md lines 89-143 (OutputMode system)
- **Error handling**: cli-structure.md lines 150-194 (sentinel errors, user guidance)
- **Testing conventions**: cli-structure.md lines 242-293 (test organization)
- **Command recommendations**: cli-structure.md lines 296-403 (status/add/sync designs)

### AI Integration
- **Template system**: ai-integration.md lines 28-73 (embedded templates, rendering)
- **Variable substitution**: ai-integration.md lines 75-109 (PromptContext struct)
- **Config resolution**: ai-integration.md lines 111-149 (5-step fallback chain)
- **Model tiers**: ai-integration.md lines 129-159, 479-488 (fast/standard/deep usage)
- **Orchestration pattern**: ai-integration.md lines 150-189 (exec tool, no direct API)
- **Error handling**: ai-integration.md lines 222-259 (categorization, wrapping)
- **Response parsing**: ai-integration.md lines 190-219 (buffer vs stream)

### Interactive Picker
- **Library comparison**: picker-libraries.md lines 28-310 (huh, pterm, promptui analysis)
- **Recommendation rationale**: picker-libraries.md lines 10-22, 325-378 (huh advantages)
- **API design**: picker-libraries.md lines 336-378 (ShowPicker implementation)
- **Feature considerations**: picker-libraries.md lines 408-455 (multi-select, theming, dynamic options)
- **Maintenance status**: picker-libraries.md lines 146-148 (promptui unmaintained), lines 257-260 (survey archived)
