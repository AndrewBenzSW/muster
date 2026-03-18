# Roadmap Management: Plan Review

*Reviewed: 2026-03-18*

## Summary

**Overall assessment**: This plan has significant gaps and contradictions that make it **NOT READY** for execution without major fixes. While the architecture is sound and follows established patterns well, the implementation plan has critical issues in AI integration design, missing implementation details, and underspecified acceptance criteria.

**Biggest risk**: The AI integration approach is fundamentally broken. The plan specifies using stdin to pipe prompts to AI tools but contradicts itself by showing command-line flags (`--prompt`) that don't match how claude-code/opencode actually work. The sync fuzzy matching algorithm is completely underspecified — there's no concrete implementation of how AI responses are parsed, how confidence thresholds work, or how the picker integrates with AI output.

## Findings

### BLOCKER

**B-1: AI tool invocation pattern is incorrect and contradictory**
- **Location**: Phase 4 (Task 4.2), Phase 5 (Task 5.2), Architecture section on stdin piping (line 878)
- **Problem**: The plan shows `exec.Command(resolved.Tool, "--prompt", "-")` but this doesn't match the actual pattern from `cmd/code.go` which uses `--plugin-dir` for staging. The research (ai-integration.md) states muster uses "orchestration pattern" but doesn't clarify whether add/sync operations stage prompts as files or pipe via stdin. The architecture says "stdin piping" but shows command-line flags.
- **Evidence**: `cmd/code.go:124` shows `cmdArgs = append(cmdArgs, "--plugin-dir", tmpDir)` — there's no stdin piping pattern in the codebase. The plan invents a new pattern without validation.
- **Impact**: Tasks 4.2 and 5.2 cannot be implemented as specified. AI integration will fail.
- **Fix**: Either (a) use the existing `prompt.StageSkills()` pattern and create skill files for add/sync operations, or (b) explicitly research how claude-code accepts ad-hoc prompts (if it does) and document the exact command-line interface. The research file ai-integration.md must be updated with the actual invocation pattern before implementation begins.

**B-2: Sync fuzzy matching algorithm is completely underspecified**
- **Location**: Phase 5 (Task 5.2), Architecture section sync algorithm (line 686-691)
- **Problem**: The plan says "AI fuzzy match for unmatched items" but provides zero implementation details. How is the match confidence threshold determined? What value? How does the picker show "low-confidence matches" — what's the UI? How are match pairs with multiple candidates handled? What if AI returns an empty array or malformed JSON?
- **Evidence**: Implementation plan line 119 says "AI fuzzy match for unmatched items" and "show picker for low-confidence matches unless `--yes`" but provides no concrete algorithm, threshold values, or error handling.
- **Impact**: Task 5.2 is unimplementable. Developer will have to design the algorithm during implementation, which defeats the purpose of planning.
- **Fix**: Add a detailed subsection to Task 5.2 specifying: (1) exact confidence threshold (e.g., 0.7), (2) how picker options are constructed from match candidates (show both source and target titles?), (3) fallback behavior when AI returns no matches or parse fails, (4) whether multiple candidates for a single source item are supported, (5) explicit error handling for malformed AI response.

**B-3: Requirement 13 not implemented in any task**
- **Location**: Synthesis Req 13 (line 58), missing from all phases
- **Problem**: Requirement states "Config resolution per operation using `config.ResolveStep("add"|"sync", ...)`" but no task creates helper functions like `config.ResolveAdd()` or documents where config loading happens in each command. Tasks 4.2 and 5.2 mention config resolution in passing but don't specify error handling for config load failures.
- **Evidence**: `cmd/code.go:38-54` shows explicit config loading with error categorization using `errors.Is(err, config.ErrConfigParse)`. The plan's Task 4.2 pseudocode shows config resolution but skips all error handling that Req 22 requires.
- **Impact**: Commands will have inconsistent error handling compared to `muster code`. User experience degrades.
- **Fix**: Add explicit subtasks in Phase 4 and Phase 5: "Implement config loading with error categorization following cmd/code.go pattern (lines 38-54). Handle ErrConfigParse separately from other errors with user guidance." Add acceptance criteria AC-20: "Config load errors are categorized and provide helpful messages."

**B-4: PromptContext extension for sync is incompatible with template rendering**
- **Location**: Architecture section "Prompt Context Extension" (line 805-818)
- **Problem**: The plan proposes embedding `*prompt.PromptContext` in a `SyncPromptData` struct but `internal/prompt/template.go` (which uses `template.Execute()`) expects the exact `PromptContext` type. Go templates don't support inheritance/embedding the way this design assumes.
- **Evidence**: Reading `internal/prompt/context.go` shows `PromptContext` is a concrete struct. The plan says "template is rendered by creating a new `template.Template`" (line 816) which bypasses the existing `prompt.RenderTemplate()` function entirely, creating a second template rendering system.
- **Impact**: Either templates won't render (type mismatch) or developers will create duplicate template rendering code, violating DRY principle and making future template changes harder.
- **Fix**: Extend `PromptContext` with an `Extra map[string]interface{}` field (as mentioned in line 762 as an alternative). Update `sync-match-prompt.md.tmpl` to use `{{range .Extra.SourceItems}}`. Document this pattern in the architecture section. Update `internal/prompt/context.go` to add the field.

**B-5: Picker cannot be tested as specified in Phase 2**
- **Location**: Phase 2, Task 2.4
- **Problem**: Task 2.4 says "interactive selection tests may need to be skipped with TODO for huh mocking" but this contradicts the QA strategy requirement that "All MUST requirements have corresponding tests" (qa-strategy.md line 413). Requirement 16 (fuzzy-filterable picker) is MUST but is explicitly marked as untestable.
- **Evidence**: QA strategy acknowledges "picker doesn't expose programmatic testing API" (line 195) but still requires comprehensive test coverage. This is a fundamental contradiction.
- **Impact**: Req 16 cannot be verified automatically. Feature might ship broken.
- **Fix**: Either (1) add manual testing procedures to QA strategy for picker functionality with explicit checklist, or (2) create a picker interface (`type Picker interface { Show(...) (string, error) }`) with huh implementation and mock for tests. Add this as Task 2.2.5 before Task 2.3.

### SHOULD FIX

**S-1: Missing verbose flag implementation in add and sync commands**
- **Location**: Synthesis Req 21, Architecture line 796-801, not in Phase 4 or 5 tasks
- **Problem**: Architecture section says "When `--verbose` is set (inherited from root), `add` and `sync` print to stderr" with specific examples, but no task implements this. Req 21 is marked SHOULD.
- **Evidence**: `cmd/code.go:72-75` shows the pattern but implementation plan omits it.
- **Impact**: Debugging AI operations becomes harder. Users can't see resolved config without modifying code.
- **Fix**: Add subtasks to 4.2 and 5.2: "Implement verbose logging following code.go pattern — print resolved triple (tool, provider, model), template path, and tool invocation command to stderr when root's --verbose flag is set."

**S-2: Batch mode slug generation algorithm not specified**
- **Location**: Phase 4, Task 4.2, line 619
- **Problem**: Task says "generate slug from title (lowercase, hyphens, strip non-alphanumeric)" but doesn't specify: What about unicode? Leading/trailing hyphens? Multiple consecutive hyphens? Max length enforcement (architecture says 40 chars, line 714)?
- **Evidence**: No existing slug generation utility in codebase. This is new code that needs precise specification.
- **Impact**: Inconsistent slug format. Potential bugs with unicode titles, very long titles, or titles with special characters.
- **Fix**: Add helper function spec to Task 4.2: "Create `internal/roadmap/slug.go` with `GenerateSlug(title string) string` — lowercase via `strings.ToLower()`, replace spaces/underscores with single hyphen, strip all chars not matching `[a-z0-9-]`, collapse consecutive hyphens, trim leading/trailing hyphens, truncate to 40 chars. Add unit tests for unicode, empty string, very long titles."

**S-3: Context stdin input (`--context -`) not tested**
- **Location**: Phase 4, Task 4.3, line 108; Synthesis Req Q5 (line 260-261)
- **Problem**: Task 4.3 test list includes "context from stdin (`--context -`)" but no implementation details in Task 4.2 for how stdin is read and validated. The requirement says "follows `kubectl apply -f -` pattern" but doesn't specify buffering, max size, or EOF handling.
- **Evidence**: No stdin reading examples in existing codebase.
- **Impact**: Implementation may be inconsistent. Edge cases (empty stdin, very large input, binary data) not handled.
- **Fix**: Add to Task 4.2 implementation details: "When `--context` is `-`, read from `os.Stdin` using `io.ReadAll()` with 1MB size limit. Return error if stdin is empty or exceeds limit. Trim whitespace from result."

**S-4: Sync --delete behavior unclear for matched-but-modified items**
- **Location**: Phase 5, Task 5.2, line 119; Synthesis Q8 (line 269-270)
- **Problem**: Plan says "--delete removes target items not found in source" but doesn't clarify: Does a matched item (same slug) get its fields updated, or deleted and re-added? If title/priority/status differ, is that an update or a conflict?
- **Evidence**: Architecture says "update matched items" (line 690) but "delete unmatched" is ambiguous when item exists in both but with different data.
- **Impact**: Ambiguous behavior specification leads to implementation confusion and potential data loss.
- **Fix**: Clarify in Task 5.2: "Matched items (by slug or AI fuzzy match) are updated in-place — target slug is preserved, all other fields (title, priority, status, context, pr_url, branch) are overwritten from source. Unmatched target items are preserved unless --delete is set. --delete removes target items that exist in target but not in source AND were not matched by AI."

**S-5: Empty roadmap JSON format inconsistency**
- **Location**: AC-8 (line 155), Architecture FormatRoadmapTable (line 413)
- **Problem**: AC-8 says "outputs valid JSON array (or empty `[]` for no items)" but FormatRoadmapTable in architecture returns `"No roadmap items found..."` string for empty in TableMode. What about the wrapper format? Should it be `[]` or `{"items": []}`?
- **Evidence**: Synthesis Q4 (line 258) says "JSONMode: output empty array `[]`" but SaveRoadmap always writes wrapper format. Inconsistency between read and list output formats.
- **Impact**: Piping `muster status --format json` to `jq` might break if wrapper format is used instead of array.
- **Fix**: Specify in Task 2.3: "FormatRoadmapTable() JSONMode outputs unwrapped array `[]` for empty and `[{item}, {item}, ...]` for populated list — not wrapper format. This differs from SaveRoadmap which uses wrapper. Document this decision in architecture."

**S-6: Roadmap validation is called automatically on load but not on add**
- **Location**: Phase 1, Task 1.2 (line 151-153), Task 1.3 AddItem (line 69)
- **Problem**: LoadRoadmap calls `Validate()` automatically but AddItem only checks duplicate slug. If a caller constructs an invalid RoadmapItem (missing required field, invalid enum), AddItem doesn't catch it.
- **Evidence**: Architecture loader.go line 151 "Validate() called automatically" but validate.go AddItem (line 270-275) only checks slug uniqueness.
- **Impact**: Invalid items can be added to roadmap and saved, bypassing validation. Later load will fail.
- **Fix**: Update Task 1.3: "AddItem() validates the incoming item before append — check required fields, enum values using Priority.IsValid() and Status.IsValid(). Return descriptive error for invalid item. Add unit test for adding invalid item returns error."

**S-7: No specification for how sync handles optional fields (pr_url, branch)**
- **Location**: Phase 5, Task 5.2, entire sync algorithm
- **Problem**: RoadmapItem has optional fields PRUrl and Branch (pointers). When syncing, if source has `pr_url: null` and target has `pr_url: "https://..."`, what happens? Is null preserved (clearing the field) or skipped (keeping target value)?
- **Evidence**: Architecture RoadmapItem (line 81-89) defines optional fields but sync algorithm doesn't mention them.
- **Impact**: Undefined behavior. Different developers will implement different semantics.
- **Fix**: Add to Task 5.2 sync algorithm: "Optional fields: if source field is non-nil, overwrite target (including nil values to clear fields). This ensures source is source of truth for all present fields."

**S-8: Test strategy requires "race detector clean" but no configuration provided**
- **Location**: AC-18 (line 165), QA strategy (line 402), Makefile line 44
- **Problem**: Plan requires `go test -race ./...` passes but doesn't identify race-prone code patterns to avoid. The picker uses huh which might have internal goroutines; formatters use mutex on OutputMode. Are these race-safe?
- **Evidence**: `internal/ui/output.go:32-34` shows `sync.Mutex` for mode switching which is good, but no analysis of whether huh is race-safe or whether new roadmap code needs any synchronization.
- **Impact**: Developers might introduce data races without realizing it until CI fails.
- **Fix**: Add to QA strategy: "Race detector considerations: roadmap package is synchronous and race-free by design (no goroutines). Picker wraps huh which is assumed race-safe (no concurrent access). OutputMode is protected by mutex. New code should avoid goroutines unless explicitly required and documented."

### NICE TO HAVE

**N-1: Phase dependency graph could be more specific about task-level dependencies**
- **Location**: Implementation plan line 125-140
- **Problem**: Graph shows phases 3, 4, 5 are independent but tasks within phases might have dependencies. For example, can 4.1 (template) be done in parallel with 4.2 (command)? Can 5.1 (template) be done before 4.3 tests?
- **Impact**: Minor — developers can figure this out, but explicit task-level dependencies would improve planning.
- **Fix**: Add note after Phase Dependency Graph: "Within Phase 4, Task 4.1 must complete before 4.2 (command needs template). Task 4.3 can proceed after 4.2 completes. Phase 5 Task 5.1 can be done in parallel with Phase 4."

**N-2: No specification for sync progress output during long operations**
- **Location**: Phase 5, Task 5.2
- **Problem**: If source has 100 items and target has 100 items, AI fuzzy matching might take 10+ seconds. No user feedback specified.
- **Impact**: Poor UX. User doesn't know if command is hung or working.
- **Fix**: Add to Task 5.2: "During AI fuzzy matching, print to stderr: 'Matching items with AI (this may take a moment)...' before tool invocation. After completion, print: 'Matched N items, added M items, updated K items'."

**N-3: Default values for --priority and --status flags should use enum constants**
- **Location**: Phase 4, Task 4.2, line 613-615
- **Problem**: Default values are hardcoded strings "medium" and "planned" but should reference `roadmap.PriorityMedium` and `roadmap.StatusPlanned` for type safety.
- **Impact**: Very minor — if enum values change, flag defaults won't update. But enums are unlikely to change.
- **Fix**: Update Task 4.2 flag registration: `addCmd.Flags().String("priority", string(roadmap.PriorityMedium), ...)` to ensure consistency.

**N-4: Should consider adding --no-confirm flag as alias for --yes**
- **Location**: Phase 5, Task 5.2, line 119
- **Problem**: Other CLI tools use `--no-confirm` or `--force` for skipping prompts. Muster uses `--yes` which is less discoverable.
- **Impact**: Negligible — `--yes` is fine, just less common than `--no-confirm`.
- **Fix**: Optional — add `--no-confirm` as hidden alias for `--yes` in sync command flags.

**N-5: Architecture document should clarify why roadmap package doesn't have repository pattern**
- **Location**: Architecture overview (line 7-11)
- **Problem**: Modern Go often uses repository pattern (interface for persistence) but plan uses concrete LoadRoadmap/SaveRoadmap functions. This is fine but not justified.
- **Impact**: None — concrete functions are simpler and appropriate for this use case. Just worth documenting the decision.
- **Fix**: Add to architecture "Data Models" section: "The roadmap package uses concrete functions (LoadRoadmap, SaveRoadmap) rather than a repository interface because (1) only one persistence mechanism (JSON files) is needed, (2) testing uses t.TempDir() with real files which is simple and reliable, (3) avoiding over-engineering."

**N-6: FormatRoadmapItem function seems redundant**
- **Location**: Architecture roadmap_format.go (line 461-479), Task 2.3
- **Problem**: FormatRoadmapItem outputs "Added: {slug} ({priority}, {status})" in TableMode but FormatRoadmapDetail already provides detailed output. Is brief confirmation needed?
- **Evidence**: Usage in add.go (line 580-581) and sync.go implied but not shown explicitly.
- **Impact**: None — having a brief confirmation formatter is fine, just seems like it might not be used much.
- **Fix**: Optional — consider removing FormatRoadmapItem and using FormatRoadmapDetail after add/sync operations instead. If keeping it, ensure all command tests verify its output.

## Requirements Coverage

**Verified coverage for all MUST requirements (1-17):**

| Req | Coverage | Notes |
|-----|----------|-------|
| 1 | ✅ Phase 1, Task 1.2, AC-1 | Fallback loading covered |
| 2 | ✅ Phase 1, Task 1.2, AC-3 | Format detection covered |
| 3 | ✅ Phase 1, Task 1.2, AC-4 | Save format covered |
| 4 | ✅ Phase 1, Task 1.2, AC-2 | Parse error isolation covered |
| 5 | ✅ Phase 1, Task 1.3, AC-5 | Validation covered |
| 6 | ✅ Phase 3, Task 3.1, AC-6, AC-7, AC-8 | Status command covered |
| 7 | ⚠️ Phase 4, Task 4.2, AC-10, AC-11 | **Underspecified** (see B-1) |
| 8 | ⚠️ Phase 5, Task 5.2, AC-12, AC-13 | **Underspecified** (see B-2) |
| 9 | ✅ Phase 3, Task 3.1, AC-6 | Command registration covered |
| 10 | ✅ Phase 2, Task 2.3, Architecture | Output formatting covered |
| 11 | ✅ Phase 1, Task 1.1, AC-5 | Data structures covered |
| 12 | ✅ Phase 1, Task 1.1, AC-5 | Enums covered |
| 13 | ❌ **Missing** | **Config resolution not in any task** (see B-3) |
| 14 | ✅ Phase 4, Task 4.1; Phase 5, Task 5.1 | Templates covered |
| 15 | ✅ Phase 4, Task 4.1; Phase 5, Task 5.1 | Fast tier covered |
| 16 | ⚠️ Phase 2, Task 2.2, AC-16 | **Untestable** (see B-5) |
| 17 | ✅ Phase 2, Task 2.2, AC-17 | Picker API covered |

**3 MUST requirements have issues** (Req 7, 8, 13) and **1 is untestable** (Req 16). This is blocking.

**SHOULD requirements (18-29) mostly covered**, notable gap: Req 21 (verbose flag) not in tasks (see S-1).

## Feasibility Assessment

### Phase 1: Data Models and File I/O — **FEASIBLE**
All tasks are well-specified with clear inputs/outputs. File operations follow established patterns from `internal/config/project.go`. Testing approach is solid with `t.TempDir()`. No blockers.

**Estimated effort**: 4-6 hours for implementation + tests

### Phase 2: Output Formatting and Picker — **FEASIBLE WITH CAVEATS**
Formatters are straightforward. Picker wrapping huh is simple but testing is problematic (see B-5). The untestable interactive behavior is a risk but acceptable if manual testing is documented.

**Estimated effort**: 3-4 hours for implementation + tests

**Risk**: Picker bugs won't be caught by automated tests.

### Phase 3: Status Command — **FEASIBLE**
Simplest command, read-only, well-specified. Tests can use `t.TempDir()` and output capture as in `version_test.go`. No external dependencies. This is the lowest-risk phase.

**Estimated effort**: 2-3 hours for implementation + tests

### Phase 4: Add Command — **NOT FEASIBLE AS SPECIFIED**
**Blocker B-1** makes AI integration unimplementable. Batch mode is fine. Interactive mode requires complete redesign of AI invocation pattern. Template rendering might conflict with existing prompt system (B-4).

**Estimated effort**: Cannot estimate until AI invocation pattern is fixed. After fix: 6-10 hours.

**Risk**: High. AI integration is the core value of this feature and it's broken.

### Phase 5: Sync Command — **NOT FEASIBLE AS SPECIFIED**
**Blocker B-2** makes fuzzy matching unimplementable. Sync algorithm needs concrete specification with threshold values, error handling, and picker integration details. Without these, developer will invent their own algorithm which defeats the planning process.

**Estimated effort**: Cannot estimate until algorithm is specified. After fix: 10-15 hours (most complex command).

**Risk**: Critical. Sync is the most valuable command (handles migration from legacy files) and it's completely underspecified.

## Conclusion

**This plan requires major revisions before implementation can begin.** The architecture is sound and follows project conventions well, but the implementation tasks have critical gaps:

1. **AI integration pattern is broken** (B-1) — must be fixed by researching actual claude-code CLI or redesigning to use skill staging
2. **Sync algorithm is missing** (B-2) — must be specified with concrete thresholds, error handling, and picker integration
3. **Config resolution implementation is missing** (B-3) — must add explicit tasks for error handling
4. **PromptContext extension is incompatible** (B-4) — must use Extra field or fix template rendering
5. **Picker testing strategy is contradictory** (B-5) — must add manual testing procedures or create interface for mocking

**Recommendation**: Do NOT proceed to implementation. Fix all BLOCKER issues first. Consider creating a spike/prototype for Phase 4 to validate the AI integration approach before finalizing the plan.

**Estimated fix time**: 4-8 hours to research AI invocation, specify sync algorithm, and update plan. This is time well spent — attempting implementation with current plan will waste far more time.
