# Output Path Convention Clarification

*Researched: 2026-03-23*
*Scope: Investigation of muster's directory structure conventions, comparing .plan/ vs .muster/work/ paths, and analyzing the intent behind the specified output location*

---

## Key Findings

### The New Standard: `.muster/work/{slug}/plan/`

The canonical output location for implementation plans in muster is **`.muster/work/{slug}/plan/implementation-plan.md`**. This is explicitly documented in multiple authoritative sources:

1. **docs/design.md line 268**: "Backward compatibility: Muster checks for `.muster/` paths first, then falls back to legacy locations (`.roadmap.json`, `.plan/{slug}/`, `.dev-agent/config.yml`, `.pipeline-checkpoint`). `muster init --migrate` moves legacy files into the `.muster/` structure (e.g., `.plan/my-feature/` becomes `.muster/work/my-feature/plan/`)."

2. **docs/design.md lines 248-255**: Runtime data layout explicitly shows:
   ```
   .muster/
   ├── work/                         # Per-item working data
   │   └── {slug}/
   │       ├── plan/                 # Plan artifacts
   │       │   ├── research/
   │       │   ├── synthesis/
   │       │   └── implementation-plan.md
   ```

3. **Roadmap item context** (both `.roadmap.json` and `.muster/roadmap.json` line 8/22): "cmd/plan.go resolves slug (args or picker), stages plan-feature templates, invokes Claude, verifies plan output exists at `.muster/work/{slug}/plan/implementation-plan.md`."

4. **docs/design.md line 513**: Phase 5 deliverable explicitly states: "produces `.muster/work/{slug}/plan/implementation-plan.md`"

### Legacy Path: `.plan/{slug}/`

The `.plan/{slug}/` structure is a **legacy location** that is deprecated but still supported for backward compatibility. Key evidence:

1. **Test fixtures use legacy paths**: In `internal/prompt/template_test.go` and `internal/prompt/stage_test.go`, all test contexts construct paths like `/workspace/.plan/test-feature` (lines 113, 143, 173, 213, etc.)

2. **Template references use legacy paths**: The template file `internal/prompt/prompts/execute-plan/SKILL.md.tmpl` line 29 references `.plan/<slug>/implementation/implementation-plan.md`

3. **Backward compatibility commitment**: docs/design.md explicitly states this fallback will be "removed in a future major version"

### Current Working Environment

The current execution environment uses **`.plan/standalone-planning/`** as the working directory. This appears to be:

1. A **temporary workspace** for the current roadmap item (slug: `standalone-planning`)
2. Located at `/Users/andrew.benz/work/muster/muster-main/.plan/standalone-planning/`
3. Contains subdirectories: `research/`, `synthesis/`, `planning/`, `implementation/`
4. Listed in environment context as "Additional working directories: /Users/andrew.benz/work/muster/muster-main/.plan/standalone-planning"

This suggests the orchestrator created a legacy-format workspace for this planning session.

---

## Detailed Analysis

### Directory Structure Conventions

**Modern convention (`.muster/` tree)**:
- **Location**: `.muster/work/{slug}/plan/`
- **Purpose**: Centralized project data structure owned by muster
- **Contents**: `research/`, `synthesis/`, `implementation-plan.md`
- **Version control**: Committed to git (shared with team)
- **Lifecycle**: Created by `muster plan` or during `muster in` pipeline
- **Related files**: `.muster/work/{slug}/checkpoint.json`, `.muster/work/{slug}/pipeline.log`

**Legacy convention (`.plan/` tree)**:
- **Location**: `.plan/{slug}/`
- **Purpose**: Original structure before consolidation
- **Status**: Deprecated, backward-compatible fallback
- **Migration path**: `muster init --migrate` moves to `.muster/work/{slug}/plan/`
- **Removal timeline**: Future major version

### Path Resolution in Code

The `PromptContext` struct (internal/prompt/context.go:39-40) has a `PlanDir` field described as "the absolute path to the plan directory for this item". This field is:

1. **Passed to templates**: Available as `{{.PlanDir}}` in all prompt templates
2. **Used in 8+ templates**: All plan-feature, execute-plan, and review-implementation templates reference it
3. **Documented in design.md line 301**: Example shows `.muster/work/add-retry-logic/plan`
4. **Test usage inconsistent**: Tests construct paths like `/workspace/.plan/test-feature` (legacy format)

The code does **not** contain an explicit migration or path resolver yet. The current implementation assumes callers pass the correct path. The backward compatibility logic mentioned in design.md (checking `.muster/` first, falling back to `.plan/`) is documented but not yet implemented.

### Test Fixtures Are Out of Date

All 13 test contexts in `internal/prompt/template_test.go` and `internal/prompt/stage_test.go` construct `PlanDir` values using the legacy `.plan/` format:

```go
"/workspace/.plan/test-feature"
"/workspace/.plan/test-staging"
"/workspace/.plan/test-execution"
"/workspace/.plan/test-review"
"/workspace/.plan/test-pr"
"/workspace/.plan/test-all"
```

These should be updated to use `.muster/work/` format when the `muster plan` command is implemented.

### Golden File References

Two golden test files (`internal/prompt/testdata/execute-plan-*.golden`) contain template output with the legacy reference:

```
1. Load implementation plan from `.plan/<slug>/implementation/implementation-plan.md`
```

This is rendered from `internal/prompt/prompts/execute-plan/SKILL.md.tmpl` line 29. The template itself uses a hardcoded path fragment rather than `{{.PlanDir}}`, which means it will need manual updating during migration.

---

## Recommendations

### For the `muster plan` Command Implementation

1. **Use `.muster/work/{slug}/plan/` as the output path** — this is the documented standard and the intended future path.

2. **Construct the path as**: `filepath.Join(mainRepoPath, ".muster", "work", slug, "plan")`

3. **Create the directory structure**:
   ```go
   planDir := filepath.Join(mainRepoPath, ".muster", "work", slug, "plan")
   os.MkdirAll(filepath.Join(planDir, "research"), 0755)
   os.MkdirAll(filepath.Join(planDir, "synthesis"), 0755)
   // implementation-plan.md written directly to planDir
   ```

4. **Pass `planDir` to `NewPromptContext`** so templates have the correct `{{.PlanDir}}` value

5. **Verify output at**: `filepath.Join(planDir, "implementation-plan.md")`

### For Backward Compatibility (Future Work)

When implementing path resolution with fallback (not in Phase 5 scope):

1. Check `.muster/work/{slug}/plan/implementation-plan.md` first
2. Fall back to `.plan/{slug}/implementation/implementation-plan.md` if not found
3. Apply this logic in:
   - `muster in` plan step (check for existing plan)
   - `muster plan` (warn if legacy location exists)
   - Template rendering (so execute-plan can find the plan)

### For Test Updates (Follow-up Work)

1. Update all test fixtures in `internal/prompt/*_test.go` to use `.muster/work/` paths
2. Update golden files to reflect the new path convention
3. Update `internal/prompt/prompts/execute-plan/SKILL.md.tmpl` line 29 to use `{{.PlanDir}}/implementation-plan.md` instead of hardcoded `.plan/<slug>/implementation/implementation-plan.md`

### For Cross-Platform Compatibility

Always use `filepath.Join()` when constructing paths. Never hardcode `/` or `\\` separators. The `PlanDir` field should always contain an absolute path with platform-appropriate separators.

---

## Open Questions

### Why does the current environment use `.plan/standalone-planning/`?

The environment shows "Additional working directories: /Users/andrew.benz/work/muster/muster-main/.plan/standalone-planning". This suggests:

- **Possible explanation 1**: The orchestrator was written before the `.muster/work/` convention was established and uses legacy paths
- **Possible explanation 2**: This is a temporary workspace for the planning session itself, not the final output location
- **Impact**: The `muster plan` command should ignore this and use the documented `.muster/work/` path regardless

**Resolution**: The feature specification explicitly states the output should be at `.muster/work/{slug}/plan/implementation-plan.md`, so the command should write there even if the current workspace uses a different structure.

### Should `muster plan` create `.muster/` if it doesn't exist?

The design doc suggests `muster init` creates the `.muster/` structure. However, Phase 5 context mentions "handles pre-planning where no worktree exists yet."

- **Recommendation**: `muster plan` should create `.muster/work/{slug}/plan/` if it doesn't exist (using `os.MkdirAll`)
- **Rationale**: Users should be able to run `muster plan` before `muster init` for quick planning
- **Alternative**: Make `muster init` a prerequisite (fails with helpful error if `.muster/` doesn't exist)

**This decision should be clarified with the team lead or in the planning phase.**

### What happens if both paths exist?

If both `.muster/work/{slug}/plan/implementation-plan.md` and `.plan/{slug}/implementation/implementation-plan.md` exist:

- **For `muster plan`**: Always write to `.muster/work/` location (new standard wins)
- **For `muster in` checking existing plan**: Use `.muster/work/` if it exists, otherwise fall back to `.plan/`
- **For user awareness**: Consider a warning if both exist with different content

---

## References

### Documentation
- `/Users/andrew.benz/work/muster/muster-main/docs/design.md` lines 240-269 (Runtime Data Layout)
- `/Users/andrew.benz/work/muster/muster-main/docs/design.md` lines 510-513 (Phase 5 spec)
- `/Users/andrew.benz/work/muster/muster-main/docs/design.md` line 76-77 (Key Behaviors)
- `/Users/andrew.benz/work/muster/muster-main/CLAUDE.md` lines 38-39 (Roadmap files section)

### Roadmap Data
- `/Users/andrew.benz/work/muster/muster-main/.roadmap.json` line 8
- `/Users/andrew.benz/work/muster/muster-main/.muster/roadmap.json` line 22

### Code References
- `/Users/andrew.benz/work/muster/muster-main/internal/prompt/context.go` (PromptContext struct and PlanDir field)
- `/Users/andrew.benz/work/muster/muster-main/internal/prompt/template_test.go` (13 test contexts using legacy paths)
- `/Users/andrew.benz/work/muster/muster-main/internal/prompt/stage_test.go` (stage test using legacy path)
- `/Users/andrew.benz/work/muster/muster-main/internal/prompt/prompts/execute-plan/SKILL.md.tmpl` line 29

### Golden Files
- `/Users/andrew.benz/work/muster/muster-main/internal/prompt/testdata/execute-plan-interactive.golden` line 21
- `/Users/andrew.benz/work/muster/muster-main/internal/prompt/testdata/execute-plan-non-interactive.golden` line 21

### Quantified Findings
- **12 template files** reference `{{.PlanDir}}` in their rendered output
- **13 test contexts** use legacy `.plan/` format paths (need updating)
- **2 golden files** contain legacy path references (need updating)
- **1 template file** has hardcoded legacy path (needs to use `{{.PlanDir}}`)
- **0 implementations** of the backward-compatible path resolver (documented but not yet coded)
