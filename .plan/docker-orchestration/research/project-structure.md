# Project Structure and Patterns

*Researched: 2026-03-18*
*Scope: Analyzed internal/ package organization, cmd/ patterns, error handling, testing approaches, and build configuration to establish patterns for Docker package implementation*

---

## Key Findings

### Package Organization
The project follows a clean, idiomatic Go structure with clear separation between CLI commands (`cmd/`), internal packages (`internal/`), and embedded assets. The `internal/` directory contains 4 packages: `ui`, `prompt`, `docker`, and `testutil`. Each package follows a consistent pattern:
- Single-purpose packages with focused responsibility
- Exported types and functions use clear naming
- Package-level variables use `go:embed` for asset management
- Tests colocated with source files using `_test.go` suffix

### Command Pattern
Commands use Cobra framework with a consistent structure:
- `cmd/root.go`: Base command with global flags and persistent pre-run hooks
- Individual command files (e.g., `cmd/version.go`) that register with root via `init()`
- Commands implement `RunE` (not `Run`) to return errors properly
- Output written to `cmd.OutOrStdout()` for testability

### Error Handling
- **Standard library `fmt.Errorf` with `%w` for wrapping**: `return fmt.Errorf("failed to format version: %w", err)`
- **Validation before execution**: Check format flags in `PersistentPreRunE` before any command runs
- **Clear error messages with context**: `return fmt.Errorf("invalid format: %s (must be 'json' or 'table')", format)`
- **No custom error types yet**: Simple error wrapping is sufficient for Phase 0 complexity

### Testing Approach
- **testify for all assertions**: Both `assert` (soft checks) and `require` (hard stops) used consistently
- **Table-driven tests**: Default pattern for multiple test cases (see `internal/ui/output_test.go`)
- **Test cleanup with `t.Cleanup()`**: Used to restore state (e.g., output mode)
- **Integration tests use `os/exec`**: Build actual binary and test execution (see `main_test.go`)
- **Golden files supported**: Helper in `testutil.AssertGoldenFile()` with update flag

---

## Detailed Analysis

### 1. Directory Structure

```
muster/
├── main.go                          # Entry point, calls cmd.Execute()
├── cmd/                             # Cobra commands
│   ├── root.go                      # Root command + global flags
│   ├── root_test.go                 # Tests for root command setup
│   ├── version.go                   # version subcommand
│   └── version_test.go              # Table-driven tests
├── internal/
│   ├── ui/                          # Output formatting (JSON/table)
│   │   ├── output.go                # TTY detection, mode management
│   │   └── output_test.go           # Table-driven tests with edge cases
│   ├── prompt/                      # Embedded prompt templates
│   │   ├── embed.go                 # go:embed prompts
│   │   ├── embed_test.go            # Embedded FS tests
│   │   └── prompts/test/example.md.tmpl  # Placeholder template
│   ├── docker/                      # Docker asset embedding
│   │   ├── embed.go                 # go:embed docker assets
│   │   ├── embed_test.go            # Embedded FS tests
│   │   └── docker/README.md         # Placeholder file
│   └── testutil/                    # Test helpers
│       ├── helpers.go               # Golden file helpers, command checks
│       └── helpers_test.go          # Tests for test utilities
├── go.mod                           # Go 1.23, minimal dependencies
├── Makefile                         # Cross-platform build targets
└── .github/workflows/
    └── ci.yml                       # Test on Linux, macOS, Windows
```

### 2. Package Patterns

#### internal/ui/output.go
- **Type definitions**: `type OutputMode string` with const values
- **Package-level state**: `var currentMode OutputMode` protected by `sync.Mutex`
- **Exported functions**: `SetOutputMode()`, `GetOutputMode()`, `IsInteractive()`, `FormatVersion()`
- **TTY detection**: Uses `golang.org/x/term.IsTerminal(int(os.Stdout.Fd()))`
- **Struct with JSON tags**: `type VersionInfo struct` includes `json:"fieldName"` tags
- **Format switching**: Single function handles both JSON and table output based on mode

#### internal/prompt/embed.go & internal/docker/embed.go
- **Pattern**: Simple package with single exported `embed.FS` variable
- **Comment documentation**: Brief docstring explaining the embed and noting phase status
- **Embed directive**: `//go:embed all:prompts` or `//go:embed all:docker`
- **Variable naming**: Capitalized for export (`Prompts`, `Assets`)

#### internal/testutil/helpers.go
- **Golden file testing**: `AssertGoldenFile(t, path, actual, update)` with directory creation
- **Command checks**: `RequireCommand(t, "name")` uses `exec.LookPath()` and `require.NoError()`
- **Uses `t.Helper()`**: All test helpers call `t.Helper()` to improve error reporting

### 3. Command Implementation Pattern

**File**: `cmd/version.go`

```go
// Package-level vars for build-time injection
var (
    version = "dev"
    commit  = "none"
    date    = "unknown"
)

// Command definition
var versionCmd = &cobra.Command{
    Use:   "version",
    Short: "Print version information",
    Args:  cobra.NoArgs,             // Explicit arg validation
    RunE: func(cmd *cobra.Command, args []string) error {
        // Build data structure
        info := ui.VersionInfo{...}

        // Delegate to formatter
        output, err := ui.FormatVersion(info)
        if err != nil {
            return fmt.Errorf("failed to format version: %w", err)
        }

        // Write to configurable output
        fmt.Fprintln(cmd.OutOrStdout(), output)
        return nil
    },
}

// Registration in init
func init() {
    rootCmd.AddCommand(versionCmd)
}
```

**Key patterns**:
- Use `RunE` (not `Run`) to return errors
- Use `cobra.NoArgs` for validation
- Write to `cmd.OutOrStdout()` for testability
- Delegate formatting to internal packages
- Return wrapped errors with context
- Register in `init()` function

### 4. Testing Patterns

#### Table-Driven Tests (internal/ui/output_test.go:11-60)
```go
func TestFormatVersion_TableMode(t *testing.T) {
    tests := []struct {
        name string
        info VersionInfo
    }{
        {
            name: "complete version info",
            info: VersionInfo{...},
        },
        {
            name: "minimal version info",
            info: VersionInfo{...},
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            prevMode := GetOutputMode()
            t.Cleanup(func() { SetOutputMode(prevMode) })

            SetOutputMode(TableMode)

            result, err := FormatVersion(tt.info)
            require.NoError(t, err)

            assert.Contains(t, result, tt.info.Version)
            // ...more assertions
        })
    }
}
```

**Patterns observed**:
- Test table structure: `name string` + input fields + expected outputs
- `t.Run(tt.name, ...)` for subtests
- `t.Cleanup()` to restore state instead of defer
- `require.NoError()` for fatal checks
- `assert.Contains()` / `assert.Equal()` for validation

#### Integration Tests (main_test.go:12-27)
```go
func TestMain_SuccessExitCode(t *testing.T) {
    // Build the binary
    buildCmd := exec.Command("go", "build", "-o", "/tmp/muster-test", ".")
    err := buildCmd.Run()
    require.NoError(t, err, "building binary should succeed")

    // Run the binary with help flag
    cmd := exec.Command("/tmp/muster-test", "--help")
    output, err := cmd.CombinedOutput()
    assert.NoError(t, err, "command should exit with code 0")

    // Verify output
    outputStr := string(output)
    assert.Contains(t, outputStr, "muster")
}
```

**Patterns**:
- Build real binary with `go build`
- Execute with `exec.Command()`
- Use `CombinedOutput()` to capture stdout/stderr
- Assert on exit code via error type
- Parse output as string for validation

#### Golden File Tests (testutil/helpers.go:15-41)
```go
func AssertGoldenFile(t *testing.T, goldenPath string, actual string, update bool) {
    t.Helper()

    if update {
        os.MkdirAll(filepath.Dir(goldenPath), 0755)
        os.WriteFile(goldenPath, []byte(actual), 0644)
        t.Logf("Updated golden file: %s", goldenPath)
        return
    }

    expected, err := os.ReadFile(goldenPath)
    if err != nil {
        t.Fatalf("failed to read golden file %s: %v", goldenPath, err)
    }

    assert.Equal(t, string(expected), actual)
}
```

**Usage pattern**:
- Pass `-update` flag to regenerate golden files
- Review diffs in version control
- Store golden files in `testdata/` directories

#### Test Naming Conventions
- Functions: `Test<PackageName>_<Behavior>` (e.g., `TestFormatVersion_JSONMode`)
- Subtests: Descriptive phrases (e.g., "complete version info", "missing field")
- Test tables: `tests` variable with `name` field for each case

### 5. Error Handling Patterns

**Wrapping errors** (`cmd/version.go:32`):
```go
if err != nil {
    return fmt.Errorf("failed to format version: %w", err)
}
```

**Validation with context** (`cmd/root.go:26`):
```go
if format != "json" && format != "table" {
    return fmt.Errorf("invalid format: %s (must be 'json' or 'table')", format)
}
```

**No custom error types**: All errors use `fmt.Errorf` with `%w` for wrapping. No sentinel errors or custom error types defined yet.

### 6. Build Configuration

#### Makefile Patterns
- **Cross-platform support**: Detects Windows vs Unix and sets variables accordingly
- **Version injection**: Uses `git describe` and `git rev-parse` for version info
- **LDFLAGS**: `-s -w` for smaller binaries + `-X` for build-time variable injection
- **Targets**: `build`, `test`, `lint`, `install`, `clean`
- **CGO disabled**: `CGO_ENABLED=0` for static binaries

#### GitHub Actions CI (`.github/workflows/ci.yml`)
- **Matrix testing**: Tests on `ubuntu-latest`, `macos-latest`, `windows-latest`
- **Go version**: `1.23` specified
- **Test flags**: `-v -race` for verbose output and race detection
- **Smoke tests**: Runs `./dist/muster version` after build
- **Separate lint job**: Uses `golangci-lint-action@v4`

### 7. Dependencies (go.mod)

**Direct dependencies**:
- `github.com/spf13/cobra v1.8.0` - CLI framework
- `github.com/stretchr/testify v1.9.0` - Testing assertions
- `golang.org/x/term v0.18.0` - TTY detection

**Notable indirect dependencies**:
- `gopkg.in/yaml.v3 v3.0.1` - YAML parsing (via testify)

**What's NOT included yet** (per design doc, to be added in later phases):
- `github.com/charmbracelet/lipgloss` - Terminal UI
- `github.com/charmbracelet/huh` - Interactive picker
- `github.com/Masterminds/semver/v3` - Semver parsing
- Docker SDK (will shell out to `docker compose` instead)

---

## Recommendations

### For Docker Package Implementation

1. **Follow the embed pattern**:
   - Create `internal/docker/compose.go` for compose file generation
   - Create `internal/docker/auth.go` for provider authentication detection
   - Create `internal/docker/container.go` for lifecycle management (start/stop/exec/list)
   - Keep `embed.go` focused on asset embedding only

2. **Error handling**:
   - Use `fmt.Errorf("context: %w", err)` for all error wrapping
   - Validate inputs early and return descriptive errors
   - Don't create custom error types unless multiple packages need to handle specific errors

3. **Testing approach**:
   - Unit tests for compose generation (assert YAML structure, no Docker required)
   - Integration tests for container lifecycle (guard with `if testing.Short() { t.Skip() }`)
   - Table-driven tests for auth detection (mock filesystem/env vars)
   - Golden files for generated compose.yml output

4. **Package structure**:
   ```
   internal/docker/
   ├── embed.go           # Asset embedding (existing)
   ├── embed_test.go      # Asset tests (existing)
   ├── compose.go         # Compose file generation
   ├── compose_test.go    # Unit tests for compose generation
   ├── auth.go            # Provider auth detection
   ├── auth_test.go       # Table-driven auth tests
   ├── container.go       # Lifecycle (start/stop/exec/list)
   ├── container_test.go  # Integration tests (short-guarded)
   └── testdata/          # Fixtures for tests
       ├── compose/       # Golden files for compose output
       └── auth/          # Mock credential files
   ```

5. **Function signatures**:
   - Return `(T, error)` not just `error` for functions producing values
   - Accept `context.Context` as first param for long-running operations
   - Use `io.Writer` for output instead of returning strings (streaming)

6. **External command execution**:
   - Use `exec.Command()` with argument arrays, not shell strings
   - Example: `exec.Command("docker", "compose", "up", "-d")` not `sh -c "docker compose up -d"`
   - Capture stderr for error messages
   - Set working directory with `cmd.Dir = ...`

7. **Container labeling pattern**:
   ```go
   type ContainerLabels struct {
       Managed bool   // muster.managed=true
       Project string // muster.project=<name>
       Slug    string // muster.slug=<slug> (optional)
   }
   ```

   Query with: `docker ps --filter label=muster.managed=true --format json`

8. **Compose generation**:
   - Start with base embedded `docker-compose.yml`
   - Layer overrides programmatically using map merging
   - Write to `~/.cache/muster/{project}/docker-compose.yml`
   - Use `os.MkdirAll()` for directory creation
   - Use `filepath.Join()` for all path operations (cross-platform)

9. **Auth detection**:
   - Check file existence: `os.Stat()` and handle `os.IsNotExist()`
   - Read env vars: `os.Getenv()` and check for empty string
   - For Bedrock: detect `~/.aws/credentials` or `AWS_PROFILE` env var
   - For Max: detect `~/.claude/.credentials.json`
   - Return structured config, not just bool

10. **Concurrency considerations**:
    - If container operations might be called concurrently, protect with `sync.Mutex`
    - For now, single-threaded operation is fine (only `muster in --all` needs locking, handled at pipeline level)

---

## Open Questions

1. **Container networking**: Should the Docker package handle network creation or assume networks exist?
   - *Note*: Design doc mentions "networks:" in dev-agent config, so package should support network references but not create them

2. **Volume mount validation**: Should we validate that source paths exist before mounting?
   - *Recommendation*: Yes, fail early with clear error message rather than Docker error

3. **Docker version compatibility**: What's the minimum Docker Compose version?
   - *Design doc uses "docker compose" (v2) not "docker-compose" (v1)*
   - Should detect version and error if < v2

4. **Container logs**: Where should logs be written? Stdout, file, or both?
   - *Design doc mentions `.muster/work/{slug}/pipeline.log` for pipeline, but container logs?*
   - Recommendation: Capture to pipeline log for `muster in`, stdout for `muster code --yolo`

5. **Cleanup on failure**: If container start fails, should we clean up partial state?
   - *Recommendation*: Leave container for debugging, document that `muster down` is cleanup command

6. **Provider auth priority**: If multiple auth methods available (e.g., env var + credentials file), which wins?
   - *Recommendation*: Env var > credentials file > auto-detect, documented in order

---

## References

### Files Examined

**Commands**:
- `/Users/andrew.benz/work/muster/muster-main/cmd/root.go` (lines 1-58)
- `/Users/andrew.benz/work/muster/muster-main/cmd/version.go` (lines 1-43)
- `/Users/andrew.benz/work/muster/muster-main/main.go` (lines 1-13)

**Internal packages**:
- `/Users/andrew.benz/work/muster/muster-main/internal/ui/output.go` (lines 1-83)
- `/Users/andrew.benz/work/muster/muster-main/internal/prompt/embed.go` (lines 1-10)
- `/Users/andrew.benz/work/muster/muster-main/internal/docker/embed.go` (lines 1-10)
- `/Users/andrew.benz/work/muster/muster-main/internal/testutil/helpers.go` (lines 1-50)

**Tests**:
- `/Users/andrew.benz/work/muster/muster-main/cmd/version_test.go` (lines 1-135)
- `/Users/andrew.benz/work/muster/muster-main/cmd/root_test.go` (lines 1-51)
- `/Users/andrew.benz/work/muster/muster-main/internal/ui/output_test.go` (lines 1-366)
- `/Users/andrew.benz/work/muster/muster-main/internal/prompt/embed_test.go` (lines 1-55)
- `/Users/andrew.benz/work/muster/muster-main/internal/docker/embed_test.go` (lines 1-39)
- `/Users/andrew.benz/work/muster/muster-main/internal/testutil/helpers_test.go` (lines 1-76)
- `/Users/andrew.benz/work/muster/muster-main/main_test.go` (lines 1-68)

**Configuration**:
- `/Users/andrew.benz/work/muster/muster-main/go.mod` (lines 1-18)
- `/Users/andrew.benz/work/muster/muster-main/Makefile` (lines 1-55)
- `/Users/andrew.benz/work/muster/muster-main/.github/workflows/ci.yml` (lines 1-54)
- `/Users/andrew.benz/work/muster/muster-main/.gitignore` (lines 1-14)

**Design documentation**:
- `/Users/andrew.benz/work/muster/muster-main/docs/design.md` (complete file, 576 lines)
