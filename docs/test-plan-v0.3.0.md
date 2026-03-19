# Manual Test Plan — muster v0.3.0

All commands assume you're in the project root (`muster-main/`) and have built the binary:

```bash
make build
```

The binary is at `~/work/muster/muster-main/dist/muster`. On macOS, the user config path is
`~/Library/Application Support/muster/config.yml` (not `~/.config/`).
No user config file is required — muster falls back to defaults
(tool=claude-code, provider=anthropic, model=claude-sonnet-4.5).

---

## 1. Version and Help

### 1.1 Version output (table)

**What:** Confirm version displays build metadata in human-readable format.

```bash
~/work/muster/muster-main/dist/muster version --format table
```

**Expected:**

```
muster v0.3.0  (commit: xxxxxxx, built: 2026-03-18T..., go1.25.5, darwin/arm64)
```

The commit hash, build date, Go version, and platform should all be present.

### 1.2 Version output (JSON)

**What:** Confirm JSON output is well-formed and contains all fields.

```bash
~/work/muster/muster-main/dist/muster version --format json
```

**Expected:** JSON object with keys: `version`, `commit`, `date`, `goVersion`, `platform`.

### 1.3 Root help

**What:** All implemented commands appear in help.

```bash
~/work/muster/muster-main/dist/muster --help
```

**Expected:** Available Commands list includes: `add`, `code`, `completion`, `down`, `help`, `status`, `sync`, `version`.

### 1.4 Unknown command

**What:** Typo or wrong command gives a useful error.

```bash
~/work/muster/muster-main/dist/muster foobar
```

**Expected:** `Error: unknown command "foobar" for "muster"` plus a hint to run `--help`. Exit code 1.

---

## 2. Status Command

### 2.1 Table output

**What:** Items display as an aligned table in a TTY.

```bash
~/work/muster/muster-main/dist/muster status --format table
```

**Expected:**

```
SLUG                 TITLE                                             PRIORITY  STATUS
mock-agent-tool      Mock Agent Tool                                   high      planned
post-pr-lifecycle    Phase 4: muster out and git operations            medium    planned
...
```

Columns should be aligned. There are currently 6 items.

### 2.2 Auto-detect output mode

**What:** Without `--format`, a TTY should get table output and a pipe should get JSON.

```bash
# In your terminal (should show table):
~/work/muster/muster-main/dist/muster status

# Through a pipe (should show JSON):
~/work/muster/muster-main/dist/muster status | cat
```

**Expected:** First shows a table, second shows a JSON array.

### 2.3 Detail view for a specific slug

**What:** Passing a slug shows that single item with its context field.

```bash
~/work/muster/muster-main/dist/muster status post-pr-lifecycle
```

**Expected:** JSON (or table if `--format table`) showing one item including a `context` field describing "internal/git/ ..." details.

### 2.4 Invalid slug

**What:** A nonexistent slug produces a clear error.

```bash
~/work/muster/muster-main/dist/muster status does-not-exist
```

**Expected:** `Error: roadmap item with slug "does-not-exist" not found`. Exit code 1.

### 2.5 Empty roadmap

**What:** If the roadmap file is empty, a friendly message appears.

```bash
# Temporarily create an empty roadmap:
cp .muster/roadmap.json .muster/roadmap.json.bak
echo '{"items":[]}' > .muster/roadmap.json

~/work/muster/muster-main/dist/muster status --format table

# Restore:
mv .muster/roadmap.json.bak .muster/roadmap.json
```

**Expected:** A friendly message like `No roadmap items found.` rather than an error or an empty table.

---

## 3. Add Command (Batch Mode)

### 3.1 Add an item with all flags

**What:** Batch mode creates an item from flags, generates a slug, and persists it.

```bash
~/work/muster/muster-main/dist/muster add \
  --title "Widget Dashboard Redesign" \
  --priority high \
  --context "Redesign the widget dashboard to support drag-and-drop layout customization."
```

**Expected:**

```
Added roadmap item: widget-dashboard-redesign
  Title: Widget Dashboard Redesign
  Priority: high
  Status: planned
```

Verify it persisted:

```bash
~/work/muster/muster-main/dist/muster status widget-dashboard-redesign
```

### 3.2 Default priority and status

**What:** Omitting `--priority` defaults to `medium`, `--status` defaults to `planned`.

```bash
~/work/muster/muster-main/dist/muster add \
  --title "Default Priority Test" \
  --context "Testing that defaults are applied correctly."
```

**Expected:** Output shows `Priority: medium` and `Status: planned`.

### 3.3 Context from stdin

**What:** Using `--context -` reads context from a pipe.

```bash
echo "This context came from a pipe via stdin" | ~/work/muster/muster-main/dist/muster add \
  --title "Stdin Context Item" \
  --context -
```

**Expected:** Item added successfully. Verify with:

```bash
~/work/muster/muster-main/dist/muster status stdin-context-item
```

The context field should contain "This context came from a pipe via stdin".

### 3.4 Duplicate slug rejection

**What:** Adding an item with a title that generates the same slug fails.

```bash
~/work/muster/muster-main/dist/muster add \
  --title "Widget Dashboard Redesign" \
  --context "Duplicate test"
```

**Expected:** `Error: failed to add item: duplicate slug: widget-dashboard-redesign`. Exit code 1.

### 3.5 Invalid priority

**What:** An unrecognized priority value is rejected.

```bash
~/work/muster/muster-main/dist/muster add \
  --title "Bad Priority" \
  --priority critical \
  --context "Should fail"
```

**Expected:** `Error: failed to add item: invalid priority: critical`. Exit code 1.

### 3.6 Missing context

**What:** Omitting `--context` in batch mode is an error.

```bash
~/work/muster/muster-main/dist/muster add --title "No Context Item"
```

**Expected:** `Error: context is required`. Exit code 1.

### 3.7 Slug generation from complex titles

**What:** Unicode, special characters, and long titles produce clean slugs.

```bash
~/work/muster/muster-main/dist/muster add \
  --title 'Add OAuth2.0 Support & Token Management!!!' \
  --context 'OAuth integration test'
```

**Expected:** Slug is kebab-case, max 40 chars, no special characters. Something like `add-oauth20-support-token-management`. Verify with `~/work/muster/muster-main/dist/muster status` to see the slug.

### 3.8 Cleanup test items

Reset the roadmap to a clean state by removing the file and recreating it empty:

```bash
rm .muster/roadmap.json
```

Or, if you want to keep the file but clear all items:

```bash
echo '{"items":[]}' > .muster/roadmap.json
```

**Expected:** `~/work/muster/muster-main/dist/muster status --format table` shows either a friendly empty message
or no items.

---

## 4. Add Command (Interactive/AI Mode)

### 4.1 Interactive mode requires a TTY

**What:** Running `add` without `--title` from a pipe fails with a TTY error.

```bash
echo "" | ~/work/muster/muster-main/dist/muster add
```

**Expected:** `Error: interactive mode requires a terminal (TTY). Use --title flag for batch mode`.

### 4.2 Interactive mode with AI

**What:** Without `--title`, the command prompts for a description, invokes Claude Code
to generate a roadmap item, then presents a confirm/cancel picker.

**Prerequisites:** `claude` CLI must be installed and authenticated.

```bash
~/work/muster/muster-main/dist/muster add
```

**Interaction:**

1. You'll see: `Describe the roadmap item you want to add:`
2. Type a description, e.g.: `Add shell completion support for bash, zsh, and fish`
3. Press Ctrl+D (EOF) to submit.
4. Wait a few seconds while the AI generates the item. You may see Claude Code's
   stderr output (e.g., a "thinking" indicator).
5. You'll see the generated item:
   ```
   Generated roadmap item:
     Slug: shell-completion-support
     Title: Add shell completion support for bash, zsh, and fish
     Priority: medium
     Status: planned
     Context: ...expanded context...
   ```
6. A picker appears: `Confirm - Add this item` / `Cancel - Don't add`
7. Select **Cancel** to avoid persisting a test item (or Confirm if you want to keep it).

**Expected:** The AI generates a valid item with a reasonable slug, sensible priority,
and an expanded context. The picker works with arrow keys and enter.

### 4.3 Interactive mode with verbose flag

**What:** Verbose mode shows the resolved tool/provider/model triple and AI invocation details.

```bash
~/work/muster/muster-main/dist/muster add -v
```

**Expected:** Additional stderr output like:

```
Using: tool=claude-code provider=anthropic model=claude-sonnet-4.5
```

Plus AI invocation details before the generated item appears. Then the same flow as 4.2.

---

## 5. Sync Command

Sync operates on two JSON roadmap files: a **source** (default: `.roadmap.json`) and a
**target** (default: `.muster/roadmap.json`). It copies items from source into target.
Use `--source` and `--target` to specify custom paths.

These tests create their own fixture files so they work in any directory.

### 5.0 Setup: create test fixtures

```bash
# Source file with 3 items
cat > /tmp/sync-source.json << 'EOF'
{"items":[
  {"slug":"alpha","title":"Alpha Feature","priority":"high","status":"planned","context":"First feature"},
  {"slug":"beta","title":"Beta Feature","priority":"medium","status":"planned","context":"Second feature"},
  {"slug":"gamma","title":"Gamma Feature","priority":"low","status":"planned","context":"Third feature"}
]}
EOF

# Target file with 2 items (alpha matches, delta is target-only)
cat > /tmp/sync-target.json << 'EOF'
{"items":[
  {"slug":"alpha","title":"Alpha Feature (old)","priority":"low","status":"in_progress","context":"Outdated context"},
  {"slug":"delta","title":"Delta Feature","priority":"medium","status":"planned","context":"Target-only item"}
]}
EOF
```

### 5.1 Dry-run shows planned changes

**What:** `--dry-run` previews what would change without writing anything.

```bash
~/work/muster/muster-main/dist/muster sync \
  --source /tmp/sync-source.json \
  --target /tmp/sync-target.json \
  --dry-run --yes
```

**Expected:**

```
Dry-run mode: No changes will be saved

Summary:
  Updated: 1 items
  Added: 2 items
  Deleted: 0 items

Run without --dry-run to apply changes.
```

`alpha` is updated (exact slug match), `beta` and `gamma` are added (new), `delta`
is preserved (no `--delete`).

### 5.2 Dry-run with --delete

**What:** `--delete` marks target items with no source match for removal.

```bash
~/work/muster/muster-main/dist/muster sync \
  --source /tmp/sync-source.json \
  --target /tmp/sync-target.json \
  --dry-run --delete --yes
```

**Expected:** Same as 5.1 but with `Deleted: 1 items` (delta).

### 5.3 Actual sync with --delete

**What:** Apply the sync and verify the result.

```bash
~/work/muster/muster-main/dist/muster sync \
  --source /tmp/sync-source.json \
  --target /tmp/sync-target.json \
  --delete --yes
```

**Expected:**

```
Sync complete:
  Updated: 1 items
  Added: 2 items
  Deleted: 1 items

Target roadmap saved to: /tmp/sync-target.json
```

Verify that `alpha` was updated (priority now `high`, status now `planned`),
`beta` and `gamma` were added, and `delta` was removed:

```bash
cat /tmp/sync-target.json | python3 -m json.tool
```

### 5.4 Sync adds new source items

**What:** Items in source but not target get added.

Reset the target to just alpha:

```bash
cat > /tmp/sync-target.json << 'EOF'
{"items":[
  {"slug":"alpha","title":"Alpha Feature","priority":"high","status":"planned","context":"First feature"}
]}
EOF

~/work/muster/muster-main/dist/muster sync \
  --source /tmp/sync-source.json \
  --target /tmp/sync-target.json \
  --dry-run --yes
```

**Expected:** `Updated: 1 items`, `Added: 2 items`, `Deleted: 0 items`.

### 5.5 Source file not found

**What:** Specifying a nonexistent source gives a clear error.

```bash
~/work/muster/muster-main/dist/muster sync --source /nonexistent/roadmap.json
```

**Expected:** `Error: source file not found: /nonexistent/roadmap.json`. Exit code 1.

### 5.6 AI fuzzy matching (optional — requires mismatched slugs and `claude` CLI)

**What:** When source and target have items with different slugs but similar titles,
the AI attempts fuzzy matching.

```bash
# Source has "post-pr-lifecycle", target has "post-pr-management" (similar title)
cat > /tmp/fuzzy-source.json << 'EOF'
{"items":[
  {"slug":"post-pr-lifecycle","title":"Post-PR lifecycle management","priority":"medium","status":"planned","context":"Handle CI, merge, cleanup after PR"}
]}
EOF

cat > /tmp/fuzzy-target.json << 'EOF'
{"items":[
  {"slug":"post-pr-management","title":"Post-PR management and cleanup","priority":"medium","status":"planned","context":"Old description"}
]}
EOF

~/work/muster/muster-main/dist/muster sync \
  --source /tmp/fuzzy-source.json \
  --target /tmp/fuzzy-target.json \
  --dry-run -v
```

**Expected:** You should see `Matching items with AI (this may take a moment)...`
followed by the AI attempting to match the two items. If confidence is >= 70%, the
match is auto-accepted. Below 70%, you'll be prompted to confirm or skip.

### 5.7 Cleanup test fixtures

```bash
rm -f /tmp/sync-source.json /tmp/sync-target.json /tmp/fuzzy-source.json /tmp/fuzzy-target.json
```

---

## 6. Code Command

### 6.1 Launch with staged skills

**What:** `muster code` launches Claude Code with workflow skills automatically loaded.

**Prerequisites:** `claude` CLI must be installed and authenticated.

```bash
~/work/muster/muster-main/dist/muster code
```

**Expected:** Claude Code launches in your terminal. Once inside, you should be able to
see the muster workflow skills. Try typing `/skills` or asking Claude about available
skills — you should see roadmap-related skills (plan-feature, execute-plan,
review-implementation).

Exit Claude Code with `/exit` or Ctrl+C.

### 6.2 Launch with --keep-staged

**What:** The `--keep-staged` flag preserves the temp directory and prints its path.

```bash
~/work/muster/muster-main/dist/muster code --keep-staged
```

**Expected:** Before Claude Code launches, you'll see on stderr:

```
Staged skills kept at: /var/folders/.../muster-prompts-XXXXXXXXX
```

After exiting Claude Code, verify the directory still exists:

```bash
ls <the-printed-path>/skills/
```

You should see subdirectories for each skill (e.g., `roadmap-execute-plan/`,
`roadmap-plan-feature/`, `roadmap-review-implementation/`) each containing a `SKILL.md`.

Clean up:

```bash
rm -rf <the-printed-path>
```

### 6.3 Launch with --no-plugin

**What:** Skips skill staging entirely — launches the bare tool.

```bash
~/work/muster/muster-main/dist/muster code --no-plugin
```

**Expected:** Claude Code launches without any muster-specific skills loaded. The
roadmap workflow skills should NOT be available.

### 6.4 Tool override

**What:** `--tool` overrides the resolved tool.

```bash
~/work/muster/muster-main/dist/muster code --tool opencode
```

**Expected:** Attempts to launch `opencode` instead of `claude`. If opencode isn't
installed, you'll get: `Error: tool "opencode" not found: ...`. If it is installed,
it launches opencode with the staged skills.

### 6.5 Verbose mode

**What:** Shows the resolved tool/provider/model triple.

```bash
~/work/muster/muster-main/dist/muster code -v --no-plugin
```

**Expected:** Before the tool launches, you see on stderr:

```
Using: tool=claude-code provider=anthropic model=claude-sonnet-4.5
```

### 6.6 --yolo flag (gated)

**What:** The `--yolo` flag is currently gated as not yet implemented.

```bash
~/work/muster/muster-main/dist/muster code --yolo
```

**Expected:** `Error: --yolo (sandboxed container mode) is not yet implemented`. Exit code 1.

---

## 7. Down Command

### 7.1 No containers running

**What:** Graceful handling when no muster containers exist.

```bash
~/work/muster/muster-main/dist/muster down
```

**Expected:** `No containers to stop.` Exit code 0.

### 7.2 With --orphans flag

**What:** Same graceful handling with the orphan filter.

```bash
~/work/muster/muster-main/dist/muster down --orphans
```

**Expected:** `No containers to stop.` Exit code 0.

### 7.3 With --all flag

**What:** Explicit all-project cleanup.

```bash
~/work/muster/muster-main/dist/muster down --all
```

**Expected:** `No containers to stop.` Exit code 0.

### 7.4 With slug argument

**What:** Target a specific slug's containers.

```bash
~/work/muster/muster-main/dist/muster down some-feature
```

**Expected:** `No containers to stop.` Exit code 0.

### 7.5 Help text

**What:** Help describes all flags and usage examples.

```bash
~/work/muster/muster-main/dist/muster down --help
```

**Expected:** Shows examples for `down`, `down my-feature`, `down --orphans`,
`down --project myproj`, `down --all`.

---

## 8. Roadmap File Handling

These tests verify the load/fallback chain. Start from a clean state:

```bash
rm -rf .muster/ .roadmap.json
```

### 8.1 Neither file exists — empty state

**What:** With no roadmap files at all, status shows a friendly empty message.

```bash
~/work/muster/muster-main/dist/muster status --format table
```

**Expected:** A friendly empty-state message (e.g., `No roadmap items found.`), not an error.

### 8.2 Legacy location fallback

**What:** If `.muster/roadmap.json` doesn't exist but `.roadmap.json` does, status
reads from the legacy location.

```bash
cat > .roadmap.json << 'EOF'
{"items":[
  {"slug":"legacy-item","title":"Legacy Item","priority":"medium","status":"planned","context":"From legacy file"}
]}
EOF

~/work/muster/muster-main/dist/muster status --format table
```

**Expected:** Shows `legacy-item` in the table — it fell back to `.roadmap.json`.

### 8.3 Primary location takes precedence

**What:** When both files exist, `.muster/roadmap.json` wins.

```bash
mkdir -p .muster
cat > .muster/roadmap.json << 'EOF'
{"items":[
  {"slug":"primary-item","title":"Primary Item","priority":"high","status":"in_progress","context":"From primary file"}
]}
EOF

~/work/muster/muster-main/dist/muster status --format table
```

**Expected:** Shows `primary-item` (from `.muster/roadmap.json`), NOT `legacy-item`
(from `.roadmap.json`).

### 8.4 Array vs wrapper format

**What:** Both `[{...}]` (bare array) and `{"items":[{...}]}` (wrapper) formats are accepted.

```bash
# Overwrite with bare array format (no wrapper object)
echo '[{"slug":"array-item","title":"Array Item","priority":"low","status":"planned","context":"Bare array format"}]' > .muster/roadmap.json

~/work/muster/muster-main/dist/muster status --format table
```

**Expected:** Shows `array-item` — the bare array format is parsed correctly, no errors.

### 8.5 Cleanup

```bash
rm -rf .muster/ .roadmap.json
```

---

## 9. Global Flags

### 9.1 --verbose on various commands

**What:** Verbose flag adds diagnostic output to stderr.

```bash
~/work/muster/muster-main/dist/muster status -v
```

**Expected:** Additional diagnostic lines on stderr (e.g., resolved tool/provider/model,
file paths being loaded).

### 9.2 --format flag consistency

**What:** The format flag works the same across commands.

```bash
~/work/muster/muster-main/dist/muster status --format json
~/work/muster/muster-main/dist/muster status --format table
~/work/muster/muster-main/dist/muster version --format json
~/work/muster/muster-main/dist/muster version --format table
```

**Expected:** Each command respects the format flag. No crashes or unexpected output.

---

## Cleanup

If testing in a fresh directory, just delete the `.muster/` directory:

```bash
rm -rf .muster/
```

If testing in the muster repo itself, restore the roadmap from the legacy source:

```bash
~/work/muster/muster-main/dist/muster sync --delete --yes
git diff
```

**Expected:** `git diff` shows no changes.
