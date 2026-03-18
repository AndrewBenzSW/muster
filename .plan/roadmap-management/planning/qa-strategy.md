# Roadmap Management: QA Strategy

*Date: 2026-03-18*

## Overview

The roadmap management feature requires comprehensive test coverage across file I/O operations, CLI commands, interactive UI, and AI integration. Testing must verify backward compatibility with legacy formats, validate error handling for malformed data, and ensure consistent behavior across both table and JSON output modes.

**Test philosophy**: Follow the project's established patterns using standard Go testing with race detector (`go test -race`), table-driven test design, testify assertions, and tests colocated with source code. Focus on unit tests for pure logic, integration tests for file system operations, and command-level tests for CLI behavior.

## Test Layers

### Unit Tests

**What gets unit tested**: Pure logic without external dependencies.

#### internal/roadmap/roadmap.go

**Format detection and parsing**:
```go
func TestLoadRoadmapFile_WrapperFormat(t *testing.T)
func TestLoadRoadmapFile_ArrayFormat(t *testing.T)
func TestLoadRoadmapFile_MalformedJSON_ReturnsError(t *testing.T)
func TestLoadRoadmapFile_EmptyFile_ReturnsError(t *testing.T)
```

**Validation logic**:
```go
func TestRoadmap_Validate_RequiredFields(t *testing.T)
func TestRoadmap_Validate_DuplicateSlugs(t *testing.T)
func TestRoadmap_Validate_StatusEnum(t *testing.T)
func TestRoadmap_Validate_PriorityEnum(t *testing.T)
func TestRoadmap_Validate_EmptyRoadmap_NoError(t *testing.T)
```

**Item operations**:
```go
func TestRoadmap_FindBySlug(t *testing.T)
func TestRoadmap_AddItem_ValidatesUniqueness(t *testing.T)
func TestRoadmap_UpdateItem_PreservesOtherFields(t *testing.T)
func TestRoadmap_RemoveItem(t *testing.T)
```

#### internal/ui/output.go

**Formatter functions** (extend existing FormatVersion pattern):
```go
func TestFormatRoadmapTable_TableMode(t *testing.T)
func TestFormatRoadmapTable_JSONMode(t *testing.T)
func TestFormatRoadmapDetail_TableMode(t *testing.T)
func TestFormatRoadmapDetail_JSONMode(t *testing.T)
func TestFormatRoadmapItem_TableMode(t *testing.T)
func TestFormatRoadmapItem_JSONMode(t *testing.T)
func TestFormatRoadmapTable_EmptyRoadmap(t *testing.T)
```

#### internal/ui/picker.go

**Picker logic** (mocked user input):
```go
func TestShowPicker_ReturnsSelectedValue(t *testing.T)
func TestShowPicker_EmptyOptions_ReturnsError(t *testing.T)
func TestShowPicker_Cancellation_ReturnsError(t *testing.T)
func TestShowPicker_FilteringEnabled(t *testing.T)
```

### Integration Tests

**What gets integration tested**: File system operations, configuration resolution, template rendering.

#### internal/roadmap/loader.go

**File loading with fallback** (using t.TempDir()):
```go
func TestLoadRoadmap_NewLocationExists(t *testing.T) {
    // Create .muster/roadmap.json only
    // Verify loads from new location
}

func TestLoadRoadmap_FallbackToLegacy(t *testing.T) {
    // Create only .roadmap.json
    // Verify loads from legacy location
}

func TestLoadRoadmap_BothExist_PrioritizesNew(t *testing.T) {
    // Create both files with different content
    // Verify new location takes precedence
}

func TestLoadRoadmap_NeitherExists_ReturnsEmpty(t *testing.T) {
    // Don't create any files
    // Verify returns empty roadmap, no error
}

func TestLoadRoadmap_MalformedNew_DoesNotFallback(t *testing.T) {
    // Create malformed .muster/roadmap.json
    // Create valid .roadmap.json
    // Verify returns error, doesn't use fallback
}

func TestLoadRoadmap_ValidLegacyArray_Parses(t *testing.T) {
    // Create legacy file in array format
    // Verify parses correctly
}
```

**File saving** (using t.TempDir()):
```go
func TestSaveRoadmap_CreatesDirectory(t *testing.T) {
    // Save to directory without .muster/ dir
    // Verify directory created with 0755 permissions
}

func TestSaveRoadmap_WritesWrapperFormat(t *testing.T) {
    // Save roadmap
    // Read raw JSON and verify wrapper format
}

func TestSaveRoadmap_FilePermissions(t *testing.T) {
    // Save roadmap
    // Verify file has 0644 permissions
}

func TestSaveRoadmap_TrailingNewline(t *testing.T) {
    // Save roadmap
    // Verify file ends with newline
}

func TestSaveRoadmap_Overwrites(t *testing.T) {
    // Save, modify, save again
    // Verify second save overwrites first
}
```

**Round-trip testing**:
```go
func TestRoadmap_RoundTrip_PreservesData(t *testing.T) {
    // Load, save, load again
    // Verify data unchanged
}

func TestRoadmap_RoundTrip_MigratesArrayToWrapper(t *testing.T) {
    // Create legacy array format
    // Load and save
    // Verify saved as wrapper format
}
```

### E2E Tests

**What gets E2E tested**: Full command execution with real file system and output capture.

#### cmd/status_test.go

**Command existence and flags**:
```go
func TestStatusCommand_Exists(t *testing.T)
func TestStatusCommand_HasNoAdditionalFlags(t *testing.T)
func TestStatusCommand_AcceptsMaxOneArg(t *testing.T)
```

**Table output** (following version_test.go pattern):
```go
func TestStatusCommand_NoArgs_ShowsTable(t *testing.T) {
    // Create temp dir with roadmap file
    // Set output mode to TableMode
    // Execute command, capture output
    // Verify table contains all items
}

func TestStatusCommand_WithSlug_ShowsDetail(t *testing.T) {
    // Execute with slug argument
    // Verify detail view for that item
}

func TestStatusCommand_EmptyRoadmap_TableMode(t *testing.T) {
    // Create empty roadmap
    // Verify friendly message displayed
}
```

**JSON output**:
```go
func TestStatusCommand_NoArgs_JSONMode(t *testing.T) {
    // Set output mode to JSONMode
    // Execute command, capture output
    // Unmarshal and verify JSON structure
}

func TestStatusCommand_WithSlug_JSONMode(t *testing.T) {
    // Execute with slug in JSON mode
    // Verify single object output
}

func TestStatusCommand_EmptyRoadmap_JSONMode(t *testing.T) {
    // Verify outputs empty array []
}
```

**Error cases**:
```go
func TestStatusCommand_InvalidSlug_ReturnsError(t *testing.T)
func TestStatusCommand_MalformedRoadmap_ReturnsError(t *testing.T)
```

#### cmd/add_test.go

**Command structure**:
```go
func TestAddCommand_Exists(t *testing.T)
func TestAddCommand_HasExpectedFlags(t *testing.T) {
    // Verify --title, --priority, --context flags exist
    // Verify correct types (all string)
}
```

**Non-interactive mode** (with flags):
```go
func TestAddCommand_WithFlags_AddsItem(t *testing.T) {
    // Create temp dir
    // Execute with --title, --priority, --context
    // Load roadmap and verify item added
}

func TestAddCommand_WithFlags_GeneratesSlug(t *testing.T) {
    // Execute with title only
    // Verify slug generated from title
}

func TestAddCommand_DuplicateSlug_ReturnsError(t *testing.T) {
    // Add item
    // Try to add with same slug
    // Verify error
}
```

**Interactive mode** (skipped or mocked):
```go
func TestAddCommand_Interactive_RequiresTerminal(t *testing.T) {
    // Execute without flags in non-terminal
    // Verify error about missing required flags
}
```

**AI integration** (will need mocking in actual implementation):
```go
// Note: These tests skip actual AI calls or use fixtures
func TestAddCommand_AIResponse_ValidJSON(t *testing.T) {
    t.Skip("Requires AI mocking infrastructure")
}
```

#### cmd/sync_test.go

**Command structure**:
```go
func TestSyncCommand_Exists(t *testing.T)
func TestSyncCommand_HasExpectedFlags(t *testing.T) {
    // Verify --source, --target, --yes, --dry-run, --delete
}
```

**Dry-run mode**:
```go
func TestSyncCommand_DryRun_NoChanges(t *testing.T) {
    // Create source and target with differences
    // Execute with --dry-run
    // Verify target unchanged
    // Verify output shows planned changes
}
```

**Sync operations**:
```go
func TestSyncCommand_AddsNewItems(t *testing.T)
func TestSyncCommand_UpdatesExistingItems(t *testing.T)
func TestSyncCommand_WithDelete_RemovesItems(t *testing.T)
func TestSyncCommand_WithoutDelete_PreservesExtra(t *testing.T)
```

**Confirmation prompts**:
```go
func TestSyncCommand_WithoutYes_RequiresConfirmation(t *testing.T) {
    // Note: May need to mock picker or skip
    t.Skip("Requires picker mocking")
}

func TestSyncCommand_WithYes_SkipsConfirmation(t *testing.T)
```

**Error cases**:
```go
func TestSyncCommand_SourceNotFound_ReturnsError(t *testing.T)
func TestSyncCommand_TargetMalformed_ReturnsError(t *testing.T)
```

## Key Test Scenarios

### File Operations (internal/roadmap/)

**Happy path**:
- Load from new location (.muster/roadmap.json)
- Load from legacy location (.roadmap.json)
- Load and parse both wrapper and array formats
- Save always writes wrapper format to new location
- Empty roadmap loads without error
- Round-trip preserves all fields including optional ones

**Edge cases**:
- Both locations exist: new takes precedence
- Neither location exists: returns empty, no error
- Legacy array format: parses correctly, saves as wrapper
- Empty items array in wrapper format
- Optional fields (pr_url, branch) null vs omitted
- Unicode in title/context fields
- Very long context strings (stress test)

**Error scenarios**:
- Malformed JSON in new location: error, no fallback
- Malformed JSON in legacy location (when new doesn't exist): error
- Invalid JSON syntax
- Wrong JSON structure (e.g., object instead of array)
- Missing required fields post-parse
- Duplicate slugs in loaded data
- Invalid enum values (status, priority)
- File read permission denied
- Directory write permission denied

**Backward compatibility**:
- Legacy .roadmap.json loads successfully
- Array format `[{...}]` parses to same result as wrapper `{"items": [...]}`
- After loading array format, save writes wrapper format
- Both locations work on all platforms (Windows, macOS, Linux)

### CLI Commands (cmd/status.go, cmd/add.go, cmd/sync.go)

**Flag handling**:
- All flags exist with correct types
- Flag defaults match specification
- GetString/GetBool calls handle errors
- Invalid flag values show helpful errors

**Output modes**:
- TableMode produces human-readable output
- JSONMode produces valid, parseable JSON
- Empty results: friendly message in table, empty array in JSON
- Output goes to cmd.OutOrStdout() for testability
- Format respects root --format flag
- Mode cleanup in tests (defer ui.SetOutputMode(originalMode))

**Error messages**:
- Missing roadmap file: clear message with path
- Malformed roadmap: includes file path and parse error
- Invalid slug: suggests valid slugs or use status to see list
- Duplicate slug on add: shows existing item
- AI tool not found: installation instructions
- Config parse error: file path and line number if available

### Interactive Picker (internal/ui/picker.go)

**Selection**:
- User selects item from list
- Returns selected value (not label)
- Fuzzy filtering works (charmbracelet/huh built-in)
- Arrow keys navigate, Enter selects

**Cancellation**:
- ESC or Ctrl+C returns error
- Error distinguishable from other errors (errors.Is check)
- Caller handles cancellation gracefully

**Empty list handling**:
- Empty options slice returns error immediately
- Error message indicates no items available
- Doesn't crash or hang

### AI Integration

**Template rendering**:
- add-item-prompt.md.tmpl renders with PromptContext
- sync-match-prompt.md.tmpl renders with source/target items
- All template variables populated (no missing key errors)
- Fast model tier used ({{.Models.Fast}})
- Template parse errors caught and wrapped

**Response parsing**:
- Valid JSON response unmarshals to RoadmapItem
- Invalid JSON falls back to regex/markdown parsing
- Malformed response returns clear error
- Empty response handled gracefully

**Error handling**:
- Tool not found (exec.ErrNotFound): installation guidance
- Tool exits non-zero: stderr captured and included
- Rate limit errors: suggest wait and retry
- Authentication failures: suggest config check
- Template render errors: show template name and variable

## Quality Gates

**Before feature is considered complete**, all tests must pass:

1. **Unit test coverage**: All public functions in internal/roadmap, internal/ui formatters, internal/ui/picker
2. **Integration test coverage**: All file loading/saving scenarios, format detection, validation
3. **Command test coverage**: Each command has dedicated _test.go with flag, output mode, and error tests
4. **Race detector clean**: `make test` (go test -race ./...) passes with zero race conditions
5. **Backward compatibility verified**: Tests explicitly cover legacy .roadmap.json and array format
6. **Error paths tested**: All ErrRoadmapParse, file not found, validation failure scenarios covered
7. **Output mode coverage**: Every command tested in both TableMode and JSONMode
8. **Cross-platform paths**: filepath.Join used consistently, no hardcoded separators

**Acceptance criteria**:
- All requirements from synthesis.md marked MUST have corresponding tests
- Tests follow existing patterns from cmd/version_test.go, cmd/code_test.go, internal/config/project_test.go
- No skipped tests in main test run (AI integration tests may be skipped with clear TODOs)
- Test names follow convention: Test<Component>_<Scenario>_<ExpectedOutcome>
- Table-driven tests used where multiple similar scenarios exist

## Test Conventions

**Following project patterns**:

### Test file structure
```go
package cmd  // or package roadmap, package ui

import (
    "bytes"
    "encoding/json"
    "os"
    "path/filepath"
    "testing"

    "github.com/abenz1267/muster/internal/ui"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)
```

### Table-driven test pattern
```go
func TestRoadmap_Validate(t *testing.T) {
    tests := []struct {
        name      string
        roadmap   *Roadmap
        wantErr   bool
        errContains string
    }{
        {
            name: "valid roadmap",
            roadmap: &Roadmap{Items: []RoadmapItem{
                {Slug: "test", Title: "Test", Priority: "high", Status: "planned", Context: "context"},
            }},
            wantErr: false,
        },
        {
            name: "missing slug",
            roadmap: &Roadmap{Items: []RoadmapItem{
                {Title: "Test", Priority: "high", Status: "planned", Context: "context"},
            }},
            wantErr: true,
            errContains: "slug is required",
        },
        // ... more cases
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            err := tt.roadmap.Validate()
            if tt.wantErr {
                require.Error(t, err)
                assert.Contains(t, err.Error(), tt.errContains)
            } else {
                assert.NoError(t, err)
            }
        })
    }
}
```

### Temporary directory pattern
```go
func TestLoadRoadmap_NewLocation(t *testing.T) {
    tmpDir := t.TempDir()  // Auto-cleaned up after test
    musterDir := filepath.Join(tmpDir, ".muster")
    require.NoError(t, os.MkdirAll(musterDir, 0755))

    // Write test file
    content := `{"items": [{"slug": "test", "title": "Test", ...}]}`
    require.NoError(t, os.WriteFile(
        filepath.Join(musterDir, "roadmap.json"),
        []byte(content),
        0644,
    ))

    // Test load
    roadmap, err := LoadRoadmap(tmpDir)
    require.NoError(t, err)
    assert.Equal(t, 1, len(roadmap.Items))
}
```

### Output mode testing pattern
```go
func TestFormatRoadmapTable_JSONMode(t *testing.T) {
    originalMode := ui.GetOutputMode()
    defer ui.SetOutputMode(originalMode)  // Restore after test

    ui.SetOutputMode(ui.JSONMode)

    output, err := ui.FormatRoadmapTable(roadmap)
    assert.NoError(t, err)

    // Verify valid JSON
    var items []RoadmapItem
    err = json.Unmarshal([]byte(output), &items)
    assert.NoError(t, err, "output should be valid JSON")
}
```

### Command testing pattern
```go
func TestStatusCommand_RunE(t *testing.T) {
    // Create temp directory with test roadmap
    tmpDir := t.TempDir()
    // ... setup files ...

    // Change to temp directory
    oldDir, err := os.Getwd()
    require.NoError(t, err)
    defer os.Chdir(oldDir)
    require.NoError(t, os.Chdir(tmpDir))

    // Capture output
    buf := new(bytes.Buffer)
    statusCmd.SetOut(buf)

    // Execute
    err = statusCmd.RunE(statusCmd, []string{})
    assert.NoError(t, err)

    // Verify output
    output := buf.String()
    assert.Contains(t, output, "test-slug")
}
```

### Assertions
- Use `require.*` for fatal assertions (if fails, stop test)
- Use `assert.*` for non-fatal assertions (if fails, continue test)
- Prefer specific assertions: `assert.Equal`, `assert.Contains`, `assert.True`
- Include descriptive messages: `assert.NoError(t, err, "loading roadmap should not error")`

### Test naming
- Function: `Test<ComponentName>_<Scenario>_<ExpectedOutcome>`
- Examples:
  - `TestLoadRoadmap_NewLocationExists_LoadsSuccessfully`
  - `TestStatusCommand_WithSlug_ShowsDetail`
  - `TestRoadmap_Validate_DuplicateSlugs_ReturnsError`

### Race detector
- All tests must pass with `-race` flag
- Avoid shared state between tests
- Use proper synchronization for concurrent operations
- Run `make test` which includes `-race` flag

### Skipped tests
- Use `t.Skip()` with clear reason for tests requiring mocking infrastructure
- Add TODO comment with what's needed to unskip
- Don't skip core functionality tests

### Test organization
- Tests colocated with source: cmd/status_test.go tests cmd/status.go
- Group related tests with comments: `// File loading tests`
- One test file per source file in same package
- Helper functions at bottom of test file or in internal/testutil/
