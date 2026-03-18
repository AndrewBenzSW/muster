# QA Strategy: Docker Container Orchestration

*QA Strategist — 2026-03-18*

---

## Overview

This document defines the test strategy for Phase 2 Docker container orchestration. The testing approach follows the project's established patterns (table-driven tests, testify assertions, golden files, integration tests guarded by `testing.Short()`) while ensuring comprehensive coverage of the hybrid Docker SDK + Compose CLI architecture.

The strategy is organized by test layers (unit, integration, E2E) with specific scenarios traced back to synthesis requirements and architecture design. Quality gates ensure the feature works across all platforms (Linux, macOS, Windows) before merging.

---

## Test Layers

### Layer 1: Unit Tests (No Docker Required)

**Scope**: Pure Go logic that doesn't require Docker daemon, filesystem mocking, or environment variable injection.

**Run frequency**: Every commit, all platforms in CI.

**Dependencies**: testify, t.TempDir(), t.Setenv(), golden files with -update flag.

| Component | Test Count (est.) | Key Scenarios |
|-----------|-------------------|---------------|
| Config resolution | 15+ | Step resolution, tier lookup, fallback chain, validation |
| Config loading/merging | 12+ | YAML parsing, .local.yml deep merge, malformed files |
| Compose generation | 20+ | Base loading, auth overrides, merge semantics, label injection |
| Auth detection | 18+ | Provider detection with mocked filesystem/env vars |
| Merge helpers | 8+ | Map merge, list append/dedupe, service override |
| Label builder | 3+ | Label map construction, optional slug |

**Total unit test estimate**: 76+ test cases across 6 packages.

---

### Layer 2: Integration Tests (Docker Required)

**Scope**: Tests that require Docker daemon, real container operations, or actual compose validation.

**Run frequency**: CI only (guarded by `if testing.Short() { t.Skip() }`).

**Dependencies**: Docker daemon running, `docker compose` CLI v2+.

| Component | Test Count (est.) | Key Scenarios |
|-----------|-------------------|---------------|
| Container lifecycle | 8+ | Start/stop/exec/list containers, label filtering |
| Compose validation | 4+ | Generated files pass `docker compose config` |
| Asset extraction | 2+ | Extract to cache, verify versioning |
| Local provider reachability | 3+ | HTTP GET with timeout, unreachable servers |

**Total integration test estimate**: 17+ test cases.

---

### Layer 3: End-to-End Tests (Full Command Flow)

**Scope**: Test complete command flows via built binary with fixtures.

**Run frequency**: CI only, smoke tests on all platforms.

**Dependencies**: Built muster binary, Docker daemon, fixture config files.

| Command | Test Count (est.) | Key Scenarios |
|---------|-------------------|---------------|
| `muster code --yolo` | 4+ | Successful launch, auth injection, missing config |
| `muster down` | 6+ | Teardown by slug, --all, --orphans, label filtering |
| `muster doctor` | 3+ | Config validation, missing auth, reachability checks |

**Total E2E test estimate**: 13+ test cases.

---

## Detailed Test Scenarios

### Config Resolution (`internal/config/resolve_test.go`)

**Requirements traceability**: synthesis.md MUST §Config Resolution, architecture.md §Config Resolution

**Test function**: `TestResolveStep`

| Scenario | Input | Expected Output | Requirement |
|----------|-------|-----------------|-------------|
| Step-specific override | Step: plan with tool=claude, provider=anthropic, model=opus | StepConfig{claude, anthropic, opus} | Config fallback chain |
| Fallback to pipeline defaults | Step: execute with no overrides, defaults={tool=opencode} | StepConfig{opencode, ...} | Fallback to pipeline.defaults |
| Fallback to user defaults | Step: review with no overrides, defaults empty, user={tool=claude} | StepConfig{claude, ...} | Fallback to user.default |
| Tier resolution | Step model=muster-deep, user.tools[claude].models[deep]=opus | StepConfig{model=opus} | Model tier resolution |
| Literal model passthrough | Step model=sonnet-4, no tier mapping | StepConfig{model=sonnet-4} | Non-tier models pass through |
| Missing tool error | Step tool=invalid-tool | Error: "tool 'invalid-tool' not found in user config" | Validation: tool exists |
| Missing provider error | Step provider=invalid, tool=claude | Error: "provider 'invalid' not found for tool 'claude'" | Validation: provider exists for tool |
| Missing tier error | Step model=muster-ultra, tier not in models map | Error: "tier 'ultra' not found in models for tool 'claude'" | Validation: tier resolves |
| Multiple step scan | Steps: [plan, execute, review], collect unique providers | Deduplicated provider list | Upfront auth scanning |

**Test implementation pattern**:
```go
func TestResolveStep(t *testing.T) {
    tests := []struct {
        name     string
        stepName string
        project  *ProjectConfig
        user     *UserConfig
        want     StepConfig
        wantErr  string
    }{
        {
            name:     "step-specific override",
            stepName: "plan",
            project:  &ProjectConfig{...},
            user:     &UserConfig{...},
            want:     StepConfig{Tool: "claude", Provider: "anthropic", Model: "opus"},
        },
        // ... more cases
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got, err := ResolveStep(tt.stepName, tt.project, tt.user)
            if tt.wantErr != "" {
                require.Error(t, err)
                assert.Contains(t, err.Error(), tt.wantErr)
                return
            }
            require.NoError(t, err)
            assert.Equal(t, tt.want, got)
        })
    }
}
```

---

### Config Loading and Merging (`internal/config/*_test.go`)

**Requirements traceability**: synthesis.md MUST §Config Resolution, architecture.md §Config Loading and Merging

**Test function**: `TestLoadProjectConfig`, `TestLoadDevAgentConfig`, `TestDeepMerge`

| Scenario | Input | Expected Output | Requirement |
|----------|-------|-----------------|-------------|
| Load valid user config | Valid ~/.config/muster/config.yml | Parsed UserConfig struct | User config loading |
| Missing user config | Non-existent path | Error: "user config required" | Fail early on missing config |
| Malformed YAML | Invalid YAML syntax | Error with file path + line number | Clear validation errors |
| Field-level override | base={x: 1, y: 2}, local={y: 3} | merged={x: 1, y: 3} | Deep merge: local scalar wins |
| List replacement | base={domains: [a, b]}, local={domains: [c]} | merged={domains: [c]} | Lists replaced, not appended |
| Nested map merge | base={env: {A: 1}}, local={env: {B: 2}} | merged={env: {A: 1, B: 2}} | Recursive map merge |
| Missing local file | base exists, local missing | base unchanged | .local.yml is optional |
| Collect all validation errors | Multiple missing providers/tools | []error with all issues | Don't fail on first error |
| Validation: API key env var set | Provider with api_key_env=ANTHROPIC_API_KEY, var not set | Error: "ANTHROPIC_API_KEY not set" | Validate env vars exist |

**Fixtures**: `testdata/config/user.yml`, `testdata/config/project.yml`, `testdata/config/project.local.yml`

---

### Compose Generation (`internal/docker/compose_test.go`)

**Requirements traceability**: synthesis.md MUST §Compose File Generation, architecture.md §Compose Generation

**Test function**: `TestGenerateComposeFile`

| Scenario | Input Options | Expected YAML Contains | Requirement |
|----------|---------------|------------------------|-------------|
| Base compose loading | Minimal ComposeOptions | Services: dev-agent, proxy | Load embedded base |
| Bedrock auth injection | Auth with BedrockAuth | Volume: ~/.aws:/home/node/.aws:ro, env: AWS_PROFILE | Auth override application |
| Max auth injection | Auth with MaxAuth | Volume: credentials.json mount | Auth override application |
| API key injection | Auth with APIKeyAuth | Environment: ANTHROPIC_API_KEY={value} | Auth override application |
| Local provider extra_hosts | Auth with LocalAuth | extra_hosts: host.docker.internal:host-gateway | Localhost URL rewriting |
| Workspace mount | WorktreeDir=/path/to/worktree | Volume: /path/to/worktree:/workspace | Workspace config |
| Git directory mount | MainRepoDir=/path/to/.git | Volume: /path/to/.git:/workspace/.git:ro | Read-only .git mount |
| Dev-agent env vars | DevAgent.Env={FOO: bar} | Environment: FOO=bar | Merge user env vars |
| Dev-agent volumes | DevAgent.Volumes=[/host:/container] | Volumes: /host:/container | Merge user volumes |
| Domain allowlist merge | DevAgent.AllowedDomains=[example.com], Bedrock auth | Merged allowlist with .amazonaws.com + example.com | Domain merging |
| Label injection | Project=my-api, Slug=add-feature | Labels: muster.managed=true, muster.project=my-api, muster.slug=add-feature | Container labeling |
| User override merge | Base + override compose file | Merged services from both | Multi-file layering |
| Service merge: replace scalar | base={image: v1}, override={image: v2} | Service with image: v2 | Compose merge semantics |
| Service merge: merge map | base={env: {A: 1}}, override={env: {B: 2}} | Environment: A=1, B=2 | Compose merge semantics |
| Service merge: append list | base={volumes: [a]}, override={volumes: [b]} | Volumes: [a, b] | Compose merge semantics |
| List deduplication | base={volumes: [a]}, override={volumes: [a, b]} | Volumes: [a, b] (no duplicate) | appendUnique helper |
| Write to cache directory | Project=my-api | File at ~/.cache/muster/my-api/docker-compose.yml | Cache directory creation |
| Golden file comparison | Full generation with all options | Matches testdata/compose/full.golden | Regression detection |

**Test implementation pattern**:
```go
func TestGenerateComposeFile(t *testing.T) {
    tests := []struct {
        name          string
        opts          ComposeOptions
        wantContains  []string
        goldenFile    string
    }{
        {
            name: "bedrock auth injection",
            opts: ComposeOptions{
                Auth: &AuthRequirements{
                    Bedrock: &BedrockAuth{AWSDir: "/home/user/.aws", AWSProfile: "default"},
                },
            },
            wantContains: []string{
                "~/.aws:/home/node/.aws:ro",
                "AWS_PROFILE: default",
            },
        },
        // ... more cases
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            path, err := GenerateComposeFile(tt.opts)
            require.NoError(t, err)

            content, err := os.ReadFile(path)
            require.NoError(t, err)

            for _, want := range tt.wantContains {
                assert.Contains(t, string(content), want)
            }

            if tt.goldenFile != "" {
                testutil.AssertGoldenFile(t, tt.goldenFile, string(content), *update)
            }
        })
    }
}
```

**Golden files**: `testdata/compose/full.golden`, `testdata/compose/bedrock.golden`, `testdata/compose/max.golden`, `testdata/compose/local.golden`

---

### Auth Detection (`internal/docker/auth_test.go`)

**Requirements traceability**: synthesis.md MUST §Provider Authentication, architecture.md §Auth Detection

**Test function**: `TestCollectAuthRequirements`, `TestDetectBedrockAuth`, `TestDetectMaxAuth`, `TestDetectAPIKeyAuth`, `TestDetectLocalProvider`

| Scenario | Mocked Environment | Expected Output | Requirement |
|----------|-------------------|-----------------|-------------|
| Bedrock: settings.json with flag | ~/.claude/settings.json with CLAUDE_CODE_USE_BEDROCK=1 | BedrockAuth with AWS profile/region | Bedrock detection pattern |
| Bedrock: missing settings | No ~/.claude/settings.json | Error: "bedrock requires ~/.claude/settings.json" | Fail fast on missing auth |
| Max: credentials file exists | ~/.claude/.credentials.json present | MaxAuth with path | Max detection pattern |
| Max: credentials missing | No credentials file | Error: "max requires ~/.claude/.credentials.json" | Fail fast on missing auth |
| API key: env var set | ANTHROPIC_API_KEY=sk-ant-123 | APIKeyAuth with value | API key detection |
| API key: env var missing | ANTHROPIC_API_KEY not set | Error: "ANTHROPIC_API_KEY required" | Fail fast on missing auth |
| Local: localhost rewrite | base_url: http://localhost:1234 | ContainerURL: http://host.docker.internal:1234 | Localhost URL rewriting |
| Local: 127.0.0.1 rewrite | base_url: http://127.0.0.1:8080 | ContainerURL: http://host.docker.internal:8080 | IPv4 localhost rewriting |
| Local: ::1 rewrite | base_url: http://[::1]:8080 | ContainerURL: http://host.docker.internal:8080 | IPv6 localhost rewriting |
| Local: non-localhost passthrough | base_url: http://remote.example.com | ContainerURL: http://remote.example.com | No rewrite for remote URLs |
| Collect auth: dedupe providers | Steps: [plan: bedrock, execute: bedrock] | Single BedrockAuth | Deduplicate providers |
| Collect auth: multiple providers | Steps: [plan: anthropic, execute: bedrock] | APIKeyAuth + BedrockAuth | Multi-provider support |
| Collect auth: all errors collected | Missing Bedrock + missing Max | []error with both issues | Collect all validation errors |
| Env var precedence | Bedrock via both settings + AWS_PROFILE env | Env var wins | Environment variable precedence |

**Test implementation pattern**:
```go
func TestDetectBedrockAuth(t *testing.T) {
    tests := []struct {
        name        string
        setupFS     func(dir string)
        setupEnv    map[string]string
        want        *BedrockAuth
        wantErr     string
    }{
        {
            name: "settings.json with bedrock flag",
            setupFS: func(dir string) {
                os.MkdirAll(filepath.Join(dir, ".claude"), 0755)
                os.WriteFile(
                    filepath.Join(dir, ".claude", "settings.json"),
                    []byte(`{"env": {"CLAUDE_CODE_USE_BEDROCK": "1"}}`),
                    0644,
                )
            },
            setupEnv: map[string]string{"AWS_PROFILE": "default"},
            want:     &BedrockAuth{AWSProfile: "default"},
        },
        // ... more cases
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            dir := t.TempDir()
            if tt.setupFS != nil {
                tt.setupFS(dir)
            }
            for k, v := range tt.setupEnv {
                t.Setenv(k, v)
            }
            t.Setenv("HOME", dir)

            got, err := DetectBedrockAuth()
            if tt.wantErr != "" {
                require.Error(t, err)
                assert.Contains(t, err.Error(), tt.wantErr)
                return
            }
            require.NoError(t, err)
            assert.Equal(t, tt.want.AWSProfile, got.AWSProfile)
        })
    }
}
```

**Fixtures**: `testdata/auth/settings.json`, `testdata/auth/settings.bedrock.json`, `testdata/auth/credentials.json`

---

### Container Lifecycle (`internal/docker/container_test.go`)

**Requirements traceability**: synthesis.md MUST §Container Lifecycle, architecture.md §Container Lifecycle

**Test function**: `TestComposeUp_Integration`, `TestComposeDown_Integration`, `TestComposeExec_Integration`, `TestListContainers_Integration`

**Guard**: All tests prefixed with `if testing.Short() { t.Skip("skipping integration test requiring Docker") }`

| Scenario | Setup | Action | Expected Outcome | Requirement |
|----------|-------|--------|------------------|-------------|
| Compose up | Minimal compose file with labels | ComposeUp() | Container running with labels | Container start |
| Label filtering | Multiple containers with different projects | ListContainers(project="my-api") | Only my-api containers returned | Label-based discovery |
| Slug filtering | Containers with/without slug label | ListContainers(project, slug="feat-1") | Only slug=feat-1 returned | Slug-specific queries |
| Compose exec | Running container | ComposeExec("dev-agent", ["echo", "test"]) | Command executes, output captured | Step invocation |
| Compose down | Running containers | ComposeDown() | Containers removed | Teardown |
| Ping Docker daemon | Docker running | Ping() | No error | Daemon reachability check |
| Ping Docker unreachable | Docker not running (skip in CI) | Ping() | Error: "Docker daemon not running" | Clear error messages |
| Asset extraction | First run | ExtractAssets() | Files in ~/.cache/muster/docker-assets/{hash}/ | Asset versioning |
| Asset extraction: cached | Second run with same hash | ExtractAssets() | Same directory reused, no extraction | Skip re-extraction |

**Test implementation pattern**:
```go
func TestComposeUp_Integration(t *testing.T) {
    if testing.Short() {
        t.Skip("skipping integration test requiring Docker")
    }

    // Create minimal compose file
    tmpDir := t.TempDir()
    composePath := filepath.Join(tmpDir, "docker-compose.yml")
    composeYAML := `
services:
  test-agent:
    image: alpine:latest
    command: sleep 3600
    labels:
      muster.managed: "true"
      muster.project: "test-project"
      muster.slug: "test-slug"
`
    os.WriteFile(composePath, []byte(composeYAML), 0644)

    client, err := NewClient()
    require.NoError(t, err)
    defer client.Close()

    ctx := context.Background()

    // Start container
    err = client.ComposeUp(ctx, composePath, "test-project")
    require.NoError(t, err)

    // Verify container is running with labels
    containers, err := client.ListContainers(ctx, "test-project", "test-slug")
    require.NoError(t, err)
    assert.Len(t, containers, 1)
    assert.Equal(t, "test-project", containers[0].Project)
    assert.Equal(t, "test-slug", containers[0].Slug)

    // Clean up
    err = client.ComposeDown(ctx, composePath, "test-project")
    require.NoError(t, err)
}
```

---

### Command Integration (`cmd/code_test.go`, `cmd/down_test.go`)

**Requirements traceability**: product-scope.md User Stories 1, 3, 4

**Test function**: `TestCodeCommand_Integration`, `TestDownCommand_Integration`

**Guard**: Integration tests use built binary and real Docker.

| Scenario | Command | Fixtures | Expected Output | Requirement |
|----------|---------|----------|-----------------|-------------|
| Code --yolo success | muster code --yolo | Valid config, Docker running | Container starts, labels present | User Story 1 |
| Code --yolo missing config | muster code --yolo | No user config | Exit 1, error: "user config required" | Fail fast on missing config |
| Code --yolo missing auth | muster code --yolo | Config with Bedrock, no AWS creds | Exit 1, error: "bedrock auth requires ~/.aws" | Fail fast on missing auth |
| Down by slug | muster down test-slug | Running container with slug=test-slug | Container removed | User Story 4 |
| Down --all | muster down --all | Multiple containers for project | All project containers removed | User Story 4 |
| Down --orphans | muster down --orphans | Containers with no active roadmap item | Only orphan containers removed | User Story 4 |

**Test implementation pattern**:
```go
func TestCodeCommand_Integration(t *testing.T) {
    if testing.Short() {
        t.Skip("skipping integration test")
    }

    // Build binary
    binary := filepath.Join(t.TempDir(), "muster")
    buildCmd := exec.Command("go", "build", "-o", binary, ".")
    require.NoError(t, buildCmd.Run())

    // Set up fixtures
    configDir := t.TempDir()
    // ... write user config fixture

    // Run command
    cmd := exec.Command(binary, "code", "--yolo")
    cmd.Env = append(os.Environ(), "HOME="+configDir)
    output, err := cmd.CombinedOutput()

    // Assert outcome
    assert.NoError(t, err)
    assert.Contains(t, string(output), "Container started")

    // Verify container exists with labels
    client, _ := docker.NewClient()
    containers, _ := client.ListContainers(context.Background(), "", "")
    // ... assert labels

    // Clean up
    exec.Command(binary, "down", "--all").Run()
}
```

---

## Quality Gates

### Pre-Merge Checklist

All items must pass before merging to main:

1. **Unit tests pass on all platforms**
   - Linux (ubuntu-latest)
   - macOS (macos-latest)
   - Windows (windows-latest)
   - Command: `go test ./... -v`
   - Expected: 76+ test cases pass, 0 failures

2. **Integration tests pass in CI**
   - CI environment with Docker installed
   - Command: `go test ./... -v -short=false`
   - Expected: 17+ additional test cases pass

3. **Golden files up to date**
   - No diffs in `testdata/**/*.golden` after running with `-update`
   - Command: `go test ./internal/docker -update`
   - Expected: No changes in git status

4. **Compose validation passes**
   - Generated compose files valid per `docker compose config`
   - Tested in integration tests
   - Expected: No syntax errors

5. **Code coverage threshold**
   - Minimum 75% coverage for new packages
   - Command: `go test ./internal/config ./internal/docker -coverprofile=coverage.out`
   - Expected: `go tool cover -func=coverage.out | grep total | awk '{print $3}' >= 75%`

6. **Linting passes**
   - Command: `golangci-lint run`
   - Expected: 0 issues

7. **Cross-platform smoke test**
   - Build binary on all platforms
   - Run `muster version` successfully
   - Expected: Version info printed, exit 0

8. **E2E smoke test** (manual until Phase 3)
   - `muster code --yolo` starts container on all platforms
   - `muster down` removes container successfully
   - Expected: No panics, containers have correct labels

---

## Test Data Management

### Fixtures

All test fixtures stored in `testdata/` directories per package:

```
internal/config/testdata/
  user.yml                  # Valid user config
  user.minimal.yml          # Minimal user config
  project.yml               # Valid project config
  project.local.yml         # Local override
  devagent.yml              # Dev-agent config
  devagent.invalid.yml      # Malformed YAML for error testing

internal/docker/testdata/
  compose/
    base.yml                # Embedded base compose
    full.golden             # Golden file: full generation
    bedrock.golden          # Golden file: Bedrock auth
    max.golden              # Golden file: Max auth
    local.golden            # Golden file: local provider
  auth/
    settings.json           # Claude settings without Bedrock
    settings.bedrock.json   # Claude settings with Bedrock flag
    credentials.json        # Mock Max credentials
```

### Golden File Update Workflow

1. Make code changes to compose generation or config resolution
2. Run tests with update flag: `go test ./internal/docker -update`
3. Review diffs in version control: `git diff testdata/`
4. If changes are expected, commit updated golden files
5. If changes are unexpected, fix the code

---

## Success Criteria

### Functional Correctness

- All unit tests pass on Linux, macOS, Windows
- All integration tests pass in CI with Docker installed
- `muster code --yolo` successfully launches container with skills (manual verification)
- `muster down` cleanly tears down containers by slug/project (manual verification)
- Auth injection works for all provider types: Bedrock, Max, Anthropic, OpenRouter, local (integration tests)

### Quality Metrics

- Code coverage >= 75% for `internal/config` and `internal/docker` packages
- 0 linting errors from golangci-lint
- All golden files up to date with no unexpected diffs
- Test suite completes in < 5 minutes on CI (unit tests < 30s, integration tests < 4m30s)

### User Experience

- Auth validation errors provide actionable fix instructions (tested via error message assertions)
- Generated compose files are valid per `docker compose config` (integration tests)
- Container startup completes in < 10 seconds after image pull (manual timing verification)
- `muster down --orphans` correctly identifies and removes stale containers (integration test)

### Security

- Credential files mounted read-only (assert `:ro` in generated compose YAML)
- Network allowlist enforced via proxy (assert domain list merged correctly)
- Local provider access restricted to host gateway (assert `extra_hosts` in compose)
- No plaintext secrets in logs or error messages (manual audit of test output)

---

## Risk Mitigation

### Known Test Gaps

1. **Windows Docker Desktop compatibility**: Integration tests may not catch Windows-specific path issues. Mitigation: Manual testing on Windows in Phase 2, add Windows CI runner if issues found.

2. **Network proxy enforcement**: Unit tests cannot verify Squid proxy actually blocks disallowed domains. Mitigation: Manual testing with curl inside running container.

3. **Concurrent execution**: No tests for `muster in --all` parallel execution with locks. Mitigation: Deferred to Phase 6 when pipeline implementation lands.

4. **Image pull failures**: Integration tests assume images are cached or pull succeeds. Mitigation: Use lightweight `alpine:latest` in tests, document network requirements for CI.

5. **Stale container cleanup**: No automated tests for orphan detection across multiple projects. Mitigation: Manual testing with multiple projects and roadmaps.

### Flaky Test Prevention

- **Docker daemon availability**: Integration tests skip gracefully if Docker not running (via `testing.Short()`)
- **Port conflicts**: Tests use ephemeral ports or no exposed ports to avoid conflicts
- **Filesystem races**: All tests use `t.TempDir()` for isolated state
- **Environment pollution**: Use `t.Setenv()` to restore env vars after tests
- **Cleanup failures**: Use `t.Cleanup()` to ensure teardown even if test fails

---

## References

### Requirements Traceability

- synthesis.md MUST §Container Orchestration → Container lifecycle integration tests
- synthesis.md MUST §Provider Authentication → Auth detection unit tests
- synthesis.md MUST §Config Resolution → Config resolution table-driven tests
- synthesis.md MUST §Compose File Generation → Compose generation unit tests + golden files
- synthesis.md MUST §Container Lifecycle → Container start/stop/exec integration tests
- product-scope.md User Story 1 → `muster code --yolo` E2E test
- product-scope.md User Story 4 → `muster down` E2E test with label queries

### Architecture References

- architecture.md §Config Resolution → Config resolution test scenarios
- architecture.md §Auth Detection → Auth detection test scenarios with filesystem mocking
- architecture.md §Compose Generation → Compose generation test scenarios with golden files
- architecture.md §Container Lifecycle → Integration tests guarded by testing.Short()

### Project Patterns

- project-structure.md §Testing Approach → Table-driven tests, testify, golden files, t.TempDir()
- docs/design.md §Testing Strategy → No mocks for git/docker, integration tests with real binaries
