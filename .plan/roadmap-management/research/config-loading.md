# Config and File Loading Patterns

*Researched: 2026-03-18*
*Scope: Audit of internal/config/ for file loading, error handling, format migration, and validation patterns to inform .muster/roadmap.json loader implementation with backward compatibility*

---

## Key Findings

The muster config system uses a well-established pattern for loading configuration files with backward compatibility, deep merging, and graceful error handling. Key patterns identified:

1. **Backward Compatibility via Fallback Paths**: LoadProjectConfig checks `.muster/config.yml` first, then falls back to `.dev-agent/config.yml` (lines 16-36 in internal/config/project.go)

2. **Missing Files Are Not Errors**: Both user and project config loaders return default empty configs when files don't exist, only erroring on malformed YAML (project.go:29-35, user.go:38-41)

3. **Local Override Pattern**: Base config + optional .local.yml variant, with deep merge giving local precedence (project.go:38-51)

4. **Error Context Wrapping**: All errors wrap with fmt.Errorf and %w for context and stack traces (project.go:22, 33, 46)

5. **YAML Parsing with gopkg.in/yaml.v3**: Standard unmarshal into Go structs with pointer fields for optional values

6. **File Permissions**: All files written with 0644 permissions (readable by all, writable by owner)

7. **Validation is Post-Load**: No upfront schema validation; structs use pointer fields so missing values are nil, not zero values

## Detailed Analysis

### File Loading Pattern

**Standard Load Sequence** (internal/config/project.go:15-52):

1. Attempt to load base config file
2. If file doesn't exist (`os.IsNotExist(err)`), try fallback path or return empty default
3. If file exists but is malformed (parse error), return error immediately
4. Attempt to load `.local.yml` variant
5. If local file exists, deep merge it over base config
6. Return merged result

**Example from LoadProjectConfig**:
```go
basePath := filepath.Join(dir, ".muster", "config.yml")
baseConfig, err := loadProjectConfigFile(basePath)
if err != nil {
    // If the file exists but has a parse error, don't try fallback
    if !os.IsNotExist(err) {
        return nil, fmt.Errorf("failed to load project config from %s: %w", basePath, err)
    }

    // Fall back to .dev-agent/config.yml for backward compatibility
    fallbackPath := filepath.Join(dir, ".dev-agent", "config.yml")
    baseConfig, err = loadProjectConfigFile(fallbackPath)
    if err != nil {
        // If neither file exists, return default empty config
        if os.IsNotExist(err) {
            baseConfig = &ProjectConfig{}
        } else {
            return nil, fmt.Errorf("failed to load project config from %s: %w", fallbackPath, err)
        }
    }
}
```

**File Reading** (internal/config/project.go:54-67):
```go
func loadProjectConfigFile(path string) (*ProjectConfig, error) {
    data, err := os.ReadFile(path)
    if err != nil {
        return nil, err  // Returns os.PathError with path and cause
    }

    var config ProjectConfig
    if err := yaml.Unmarshal(data, &config); err != nil {
        return nil, fmt.Errorf("failed to parse %s: %w: %w", path, ErrConfigParse, err)
    }

    return &config, nil
}
```

### Error Handling Strategies

**Three-Tier Error Classification**:

1. **Not an error** - File doesn't exist → return default config
   - Check: `os.IsNotExist(err)`
   - Action: Return empty/default struct, no error

2. **Parse error** - File exists but is malformed → return error with context
   - Check: YAML unmarshal fails
   - Action: Wrap error with file path and ErrConfigParse sentinel
   - Example: `fmt.Errorf("failed to parse %s: %w: %w", path, ErrConfigParse, err)`

3. **Read error** - File exists but can't be read (permissions, etc.) → return error
   - Check: `os.ReadFile` fails with non-IsNotExist error
   - Action: Return error as-is (os.PathError includes context)

**Sentinel Errors** (internal/config/config.go:25-29):
```go
var (
    // ErrConfigParse indicates a YAML parsing error
    ErrConfigParse = errors.New("config parse error")
)
```

Used for error categorization and testing. Double-wrapped with %w to preserve both sentinel and underlying error.

**Test Coverage** (internal/config/project_test.go:217-276):
- Missing files don't error (line 225-231)
- Malformed base config returns error with "failed to parse" (line 233-243)
- Malformed local override returns error with "failed to load local config" (line 245-256)

### Format Migration and Backward Compatibility

**Location Fallback** (internal/config/project.go:16-36):
- Primary path: `.muster/config.yml`
- Fallback path: `.dev-agent/config.yml`
- Only tries fallback if primary doesn't exist
- If primary exists but is malformed, **does not** try fallback

**Important Rule**: Parse errors block fallback attempts. This prevents masking configuration errors by silently falling back to old configs.

**Deep Merge Strategy** (internal/config/project.go:69-139):
- Field-level merge for struct fields (Defaults)
- Entire value replacement for maps (Pipeline, LocalOverrides)
- Override can add new sections not in base
- Empty override causes no changes
- Tests verify 6 merge scenarios (project_test.go:60-215)

**Example Merge Behavior**:
```yaml
# base: .muster/config.yml
defaults:
  tool: cursor
  provider: openai
  model: gpt-4o

# local: .muster/config.local.yml
defaults:
  model: claude-opus-4

# result: field-level merge
defaults:
  tool: cursor        # from base
  provider: openai    # from base
  model: claude-opus-4  # from local
```

### Validation Approaches

**No Explicit Validation Functions**: The codebase does not define separate validation functions. Validation is implicit:

1. **YAML Parsing** - Structure validation via yaml.Unmarshal
2. **Nil Checks** - Pointer fields distinguish "not set" from "set to zero value"
3. **Default Filling** - LoadUserConfig fills missing defaults after parse (user.go:60-75)
4. **Test-Based Verification** - Extensive test coverage validates behavior

**Pointer Fields Pattern** (internal/config/config.go:38-117):
```go
type DefaultsConfig struct {
    Tool     *string `yaml:"tool"`
    Provider *string `yaml:"provider"`
    Model    *string `yaml:"model"`
}
```

Allows distinguishing:
- Field not present in YAML → nil pointer
- Field present but empty → pointer to empty string
- Field present with value → pointer to value

**Default Filling Example** (internal/config/user.go:60-75):
```go
// Fill in defaults if not provided
if config.Defaults == nil {
    config.Defaults = &DefaultsConfig{}
}
if config.Defaults.Tool == nil {
    tool := DefaultTool
    config.Defaults.Tool = &tool
}
```

**Test Validation Pattern** (internal/config/user_test.go:17-154):
```go
tests := []struct {
    name      string
    fixture   string
    wantErr   bool
    validate  func(t *testing.T, cfg *UserConfig)
}
```

Each test case includes a validation function that checks expected values. This is the primary validation strategy.

### Save/Write Patterns

**No Save Functions in Config Package**: The config package only loads; it does not save. However, other packages show consistent patterns:

**JSON Writing** (internal/ui/output.go:62-66):
```go
data, err := json.MarshalIndent(info, "", "  ")
if err != nil {
    return "", err
}
return string(data), nil
```

**File Writing** (internal/prompt/stage.go:200):
```go
if err := os.WriteFile(outPath, []byte(rendered), 0644); err != nil {
    return fmt.Errorf("failed to write file %s: %w", outPath, err)
}
```

**Consistent Pattern**:
1. Marshal data to bytes (JSON or YAML)
2. Write with `os.WriteFile(path, data, 0644)`
3. Wrap errors with path context
4. Always use 0644 permissions (user rw, group/other read)

### Platform and Path Handling

**Cross-Platform Paths** (internal/config/user.go:16-35):
- Use `os.UserConfigDir()` for platform-specific config directory
- Use `filepath.Join()` for path construction (handles separators)
- Documented platform differences in comments

**Path Separator Handling**:
- Never use string concatenation for paths
- Always use `filepath.Join()` which uses correct separator for platform
- Tests verify cross-platform behavior (user_test.go:229-258)

**Platform-Specific Paths**:
- Linux: `$XDG_CONFIG_HOME/muster/config.yml` (typically `~/.config/muster/config.yml`)
- macOS: `~/Library/Application Support/muster/config.yml`
- Windows: `%AppData%/muster/config.yml`

## Recommendations

Based on the established patterns, the roadmap loader should:

### 1. File Location and Backward Compatibility

**Recommended Load Order**:
1. Try `.muster/roadmap.json` (new location)
2. If not exists, try `.roadmap.json` (legacy location)
3. If neither exists, return empty roadmap (not an error)

**Implementation Pattern**:
```go
func LoadRoadmap(dir string) (*Roadmap, error) {
    // Try new location first
    newPath := filepath.Join(dir, ".muster", "roadmap.json")
    roadmap, err := loadRoadmapFile(newPath)
    if err != nil {
        if !os.IsNotExist(err) {
            // Parse error - don't try fallback
            return nil, fmt.Errorf("failed to load roadmap from %s: %w", newPath, err)
        }

        // Try legacy location
        legacyPath := filepath.Join(dir, ".roadmap.json")
        roadmap, err = loadRoadmapFile(legacyPath)
        if err != nil {
            if os.IsNotExist(err) {
                // Neither exists - return empty roadmap
                return &Roadmap{Items: []RoadmapItem{}}, nil
            }
            return nil, fmt.Errorf("failed to load roadmap from %s: %w", legacyPath, err)
        }
    }

    return roadmap, nil
}
```

### 2. Format Migration (Array vs Wrapper)

**Detect Format at Parse Time**:
```go
func loadRoadmapFile(path string) (*Roadmap, error) {
    data, err := os.ReadFile(path)
    if err != nil {
        return nil, err
    }

    // Try wrapper format first (new format)
    var wrapper struct {
        Items []RoadmapItem `json:"items"`
    }
    if err := json.Unmarshal(data, &wrapper); err == nil && wrapper.Items != nil {
        return &Roadmap{Items: wrapper.Items}, nil
    }

    // Try array format (legacy)
    var items []RoadmapItem
    if err := json.Unmarshal(data, &items); err != nil {
        return nil, fmt.Errorf("failed to parse %s: %w", path, err)
    }

    return &Roadmap{Items: items}, nil
}
```

**Always Save in New Format**:
```go
func SaveRoadmap(dir string, roadmap *Roadmap) error {
    path := filepath.Join(dir, ".muster", "roadmap.json")

    // Ensure .muster directory exists
    musterDir := filepath.Join(dir, ".muster")
    if err := os.MkdirAll(musterDir, 0755); err != nil {
        return fmt.Errorf("failed to create .muster directory: %w", err)
    }

    // Marshal with wrapper format
    wrapper := struct {
        Items []RoadmapItem `json:"items"`
    }{
        Items: roadmap.Items,
    }

    data, err := json.MarshalIndent(wrapper, "", "  ")
    if err != nil {
        return fmt.Errorf("failed to marshal roadmap: %w", err)
    }

    // Write with trailing newline
    data = append(data, '\n')

    if err := os.WriteFile(path, data, 0644); err != nil {
        return fmt.Errorf("failed to write roadmap to %s: %w", path, err)
    }

    return nil
}
```

### 3. Error Handling

**Follow Three-Tier Pattern**:
1. File not exists → return empty roadmap (no error)
2. File exists but malformed → return error with context
3. File exists but unreadable → return error

**Use Error Wrapping**:
- Always wrap with `fmt.Errorf` and `%w` for context
- Include file path in error messages
- Consider adding sentinel error: `var ErrRoadmapParse = errors.New("roadmap parse error")`

### 4. Validation

**Post-Load Validation**:
```go
func (r *Roadmap) Validate() error {
    seen := make(map[string]bool)
    for i, item := range r.Items {
        // Check required fields
        if item.Slug == "" {
            return fmt.Errorf("item %d: slug is required", i)
        }
        if item.Title == "" {
            return fmt.Errorf("item %d: title is required", i)
        }

        // Check for duplicate slugs
        if seen[item.Slug] {
            return fmt.Errorf("duplicate slug: %s", item.Slug)
        }
        seen[item.Slug] = true

        // Validate status enum
        validStatuses := map[string]bool{
            "planned": true, "in_progress": true,
            "completed": true, "blocked": true,
        }
        if !validStatuses[item.Status] {
            return fmt.Errorf("item %s: invalid status: %s", item.Slug, item.Status)
        }
    }
    return nil
}
```

Call validation after load: `roadmap, err := LoadRoadmap(dir); err == nil { err = roadmap.Validate() }`

### 5. Type Definitions

**Use Pointer Fields for Optional Values**:
```go
type RoadmapItem struct {
    Slug     string  `json:"slug"`      // required
    Title    string  `json:"title"`     // required
    Priority string  `json:"priority"`  // required
    Status   string  `json:"status"`    // required
    Context  string  `json:"context"`   // required
    PRUrl    *string `json:"pr_url,omitempty"`    // optional
    Branch   *string `json:"branch,omitempty"`    // optional
}
```

The `omitempty` tag ensures nil pointers don't appear in JSON output.

### 6. What NOT to Do

**Don't**:
- Use `.local.json` variants (roadmap is version controlled, no local overrides needed)
- Create multiple save locations (always save to `.muster/roadmap.json`)
- Validate JSON schema before unmarshal (let unmarshal handle structure, validate semantics after)
- Error on missing file (return empty roadmap like config loaders do)
- Use relative paths (always use `filepath.Join()` with absolute base dir)
- Write files without error context wrapping

## Open Questions

### 1. Should we migrate files automatically?

**Question**: When loading from `.roadmap.json` (legacy location), should we automatically save to `.muster/roadmap.json` (new location)?

**Why it matters**: Affects user migration experience. Auto-migration is convenient but could surprise users with unexpected file changes.

**What I tried**: Examined config loading - it does NOT auto-migrate from `.dev-agent/` to `.muster/`. It reads from legacy location transparently but doesn't move files.

**Recommendation**: Follow config pattern - load from either location, but only save to new location on explicit save operations (add, update, etc.). Don't auto-migrate on read.

### 2. Should we delete legacy file after migration?

**Question**: If we load from `.roadmap.json` and later save to `.muster/roadmap.json`, should we delete the old file?

**Why it matters**: Prevents confusion from having two files. But deleting user files can be dangerous.

**What I tried**: Config system doesn't address this (no automatic cleanup).

**Recommendation**: Don't auto-delete. Optionally add a `muster roadmap migrate` command that explicitly moves the file and deletes the old one, with user confirmation.

### 3. How should we handle corrupted legacy files?

**Question**: If `.roadmap.json` exists but is corrupted, and `.muster/roadmap.json` doesn't exist, should we return error or return empty?

**Why it matters**: Affects error recovery. Returning empty could lose data; returning error blocks all operations.

**What I tried**: Config pattern blocks fallback on parse errors (project.go:20-23). If primary exists but is malformed, don't try fallback.

**Recommendation**: Follow config pattern - if `.muster/roadmap.json` exists but is malformed, return error immediately. Only try `.roadmap.json` if `.muster/roadmap.json` doesn't exist. Parse errors should block, not fall through.

### 4. Should we support array format indefinitely?

**Question**: Should `SaveRoadmap` support saving in array format for backward compatibility, or always save wrapper format?

**Why it matters**: Affects compatibility with external tools that might expect array format.

**What I tried**: Config files don't have format versioning. They use struct tags and deep merge but don't maintain multiple save formats.

**Recommendation**: Always save wrapper format. Load transparently handles both. This provides forward migration path. Document format in README if needed.

## References

### File Loading
- `internal/config/project.go:15-52` - LoadProjectConfig with fallback logic
- `internal/config/project.go:54-67` - loadProjectConfigFile helper
- `internal/config/user.go:24-78` - LoadUserConfig with default handling
- `internal/config/project.go:69-139` - mergeProjectConfigs deep merge

### Error Handling
- `internal/config/config.go:25-29` - ErrConfigParse sentinel
- `internal/config/project.go:20-23` - Parse error blocks fallback
- `internal/config/project.go:29-35` - IsNotExist check for default return

### Testing
- `internal/config/project_test.go:217-276` - Error handling tests
- `internal/config/project_test.go:60-215` - Deep merge tests
- `internal/config/user_test.go:12-154` - Validation test pattern

### Type Definitions
- `internal/config/config.go:38-117` - Pointer field pattern
- `internal/config/config.go:32-36` - Default constants

### File Writing
- `internal/ui/output.go:62-66` - JSON MarshalIndent usage
- `internal/prompt/stage.go:200` - os.WriteFile with error wrapping
- All uses of `os.WriteFile` use 0644 permissions

### Path Handling
- `internal/config/user.go:16-35` - Cross-platform path comments
- `internal/config/user_test.go:229-258` - Platform path testing
