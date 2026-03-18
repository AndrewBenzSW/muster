# Implementation Plan: Docker Container Orchestration

## Summary

Implement Docker container orchestration for sandboxed AI coding sessions in the muster CLI (Phase 2). This replaces the 540-line `run.sh` bash script with typed Go code, enabling `muster code --yolo` for sandboxed interactive sessions and `muster down` for container teardown. The architecture uses a hybrid Docker SDK + Compose CLI pattern with YAML marshaling for compose file generation.

**Scope**: 6 implementation phases, ~2,300 lines across 24 files (source + tests + assets).

---

## Phase Dependency Graph

```
Phase 1: Config Types & Loading
    |
    v
Phase 2: Config Resolution & Merge
    |
    v
Phase 3: Auth Detection -----> Phase 4: Compose Generation & Assets
    |                               |
    v                               v
Phase 5: Container Lifecycle & Docker Client
    |
    v
Phase 6: Command Integration (code --yolo, down)
```

Phases 1-2 are sequential (types must exist before resolution). Phases 3-4 can be developed in parallel once Phase 2 is done. Phase 5 depends on both 3 and 4. Phase 6 wires everything together.

---

## Phase 1: Config Types and Loading

Build the config type system and file loaders for all three config layers.

| ID | Task | Key Details |
|----|------|-------------|
| 1.1 | Define config types | Create `internal/config/types.go` with `UserConfig`, `ProjectConfig`, `DevAgentConfig`, `StepConfig`, `StepOverride`, `ProviderConfig` structs. Use `yaml` struct tags. `PipelineConfig` needs custom `UnmarshalYAML` to separate `defaults` key from step names in the YAML mapping. |
| 1.2 | Implement user config loader | Create `internal/config/user.go` with `LoadUserConfig(path string) (*UserConfig, error)`. Read file, unmarshal YAML. If file missing, return error: `"user config not found at %s; run 'muster init' or create it manually"`. |
| 1.3 | Implement project config loader | Create `internal/config/project.go` with `LoadProjectConfig(dir string) (*ProjectConfig, error)`. Load `.muster/config.yml`, if `.muster/config.local.yml` exists apply deep merge. Missing base file returns nil (project config is optional for `muster code`). |
| 1.4 | Implement dev-agent config loader | Create `internal/config/devagent.go` with `LoadDevAgentConfig(dir string) (*DevAgentConfig, error)`. Load `.muster/dev-agent/config.yml` with `.muster/dev-agent/config.local.yml` overlay. Missing file returns empty defaults (no domains, no env, no volumes). |
| 1.5 | Implement top-level loader | Create `internal/config/config.go` with `LoadAll(userConfigPath, projectDir string) (*Config, error)`. Calls all three loaders, returns assembled `Config`. |
| 1.6 | Write user config tests | Create `internal/config/user_test.go`. Table-driven tests: valid config, minimal config, missing file error, malformed YAML error. Use `testdata/` fixtures. |
| 1.7 | Write project config tests | Create `internal/config/project_test.go`. Table-driven tests: valid config, missing file (returns nil), malformed YAML. |
| 1.8 | Write dev-agent config tests | Create `internal/config/devagent_test.go`. Table-driven tests: full config, empty defaults, domain list parsing. |
| 1.9 | Create test fixtures | Create `internal/config/testdata/` with: `user.yml`, `user.minimal.yml`, `project.yml`, `devagent.yml`, `devagent.invalid.yml`. |

**Acceptance criteria**:
- `LoadUserConfig` parses the full user config schema from design.md (tools, providers, models, defaults)
- `LoadProjectConfig` parses pipeline config with step overrides
- `LoadDevAgentConfig` parses allowed_domains, env, volumes, networks
- Missing user config returns actionable error message
- Missing project/dev-agent config returns nil/empty defaults (not error)
- All unit tests pass: `go test ./internal/config/ -run TestLoad -v`

---

## Phase 2: Config Resolution and Deep Merge

Implement the step resolution algorithm and `.local.yml` deep merge logic.

| ID | Task | Key Details |
|----|------|-------------|
| 2.1 | Implement deep merge | Create `internal/config/merge.go` with `deepMerge(base, local []byte) ([]byte, error)`. Unmarshal both to `map[string]any`, walk recursively. Semantics per review B/S2: `null` in local removes field from base, empty maps/lists are valid values that replace base, lists always replace (never merge). |
| 2.2 | Implement step resolution | Create `internal/config/resolve.go` with `ResolveStep(stepName string, project *ProjectConfig, user *UserConfig) (StepConfig, error)`. Fallback chain: step-specific > pipeline.defaults > user.default. Tier resolution for `muster-{tier}` prefixed models. |
| 2.3 | Implement config validation | Add `func (c *Config) Validate() []error` to `config.go`. Collect all errors: referenced tools exist, providers exist for tool, tier names resolve, `api_key_env` env vars are set. Return all errors at once. Include actionable error messages per review S1: `"provider anthropic requires ANTHROPIC_API_KEY; set it in your shell:\n  export ANTHROPIC_API_KEY=sk-ant-...\nor configure api_key_env in ~/.config/muster/config.yml"` |
| 2.4 | Wire .local.yml into project loader | Update `LoadProjectConfig` and `LoadDevAgentConfig` to call `deepMerge` when `.local.yml` exists. |
| 2.5 | Write deep merge tests | Create `internal/config/merge_test.go`. Table-driven tests: scalar override, list replacement, nested map merge, null removes field, empty map replaces, empty list replaces, missing local file (base unchanged). |
| 2.6 | Write resolution tests | Create `internal/config/resolve_test.go`. Table-driven tests: step-specific override, fallback to pipeline defaults, fallback to user defaults, tier resolution (`muster-deep` -> `opus`), literal model passthrough, missing tool error, missing provider error, missing tier error. |
| 2.7 | Write validation tests | Add validation test cases: multiple missing providers collected, missing env var, unknown tool. Assert error messages contain actionable fix instructions. |

**Acceptance criteria**:
- `deepMerge` correctly handles: scalar override, list replacement, nested map merge, null removal, empty value replacement
- `ResolveStep` follows the full fallback chain and tier resolution
- `Validate` collects all errors (not just first) and returns actionable messages
- All unit tests pass: `go test ./internal/config/ -v`

---

## Phase 3: Auth Detection

Implement per-provider auth detection with validation and error collection.

| ID | Task | Key Details |
|----|------|-------------|
| 3.1 | Create auth types and labels | Create `internal/docker/labels.go` with label constants (`LabelManaged`, `LabelProject`, `LabelSlug`, `LabelCreated`) and `Labels(project, slug string)` builder. Include `muster.created` with RFC3339 timestamp per review S3. |
| 3.2 | Implement Bedrock auth detection | In `internal/docker/auth.go`: `DetectBedrockAuth() (*BedrockAuth, error)`. Read `~/.claude/settings.json`, check for `CLAUDE_CODE_USE_BEDROCK`. Extract AWS profile/region from settings or env. Per review S7: run `aws configure export-credentials --profile $PROFILE` to pre-resolve credentials, verify AccessKeyId present. Merge host settings into embedded `settings.json` template. Error message: `"bedrock auth requires ~/.aws directory; run AWS CLI setup:\n  aws configure sso\n  aws sso login --profile <profile-name>"` |
| 3.3 | Implement Max auth detection | `DetectMaxAuth() (*MaxAuth, error)`. Check `~/.claude/.credentials.json` exists. Error message: `"max auth requires ~/.claude/.credentials.json; log in with Claude Desktop to generate credentials"` |
| 3.4 | Implement API key auth detection | `DetectAPIKeyAuth(envVar string) (*APIKeyAuth, error)`. Check env var is set and non-empty. Error message: `"provider requires %s; set it in your shell:\n  export %s=..."` |
| 3.5 | Implement local provider detection | `DetectLocalProvider(baseURL string) (*LocalAuth, error)`. Rewrite localhost/127.0.0.1/::1 URLs to `host.docker.internal`. Per review B5: reachability validation only runs when the local provider is actually used by the current command (not globally). Use 5-second timeout with progress message. |
| 3.6 | Implement auth collection | `CollectAuthRequirements(cfg *config.Config, steps []string) (*AuthRequirements, []error)`. Per review B3: Phase 2 only supports interactive mode, so `cmd/code.go` passes `steps := []string{"interactive"}` which resolves using `pipeline.defaults` and `user.default`. Pipeline step scanning deferred to Phase 6. Deduplicate providers, call detection functions, collect all errors. |
| 3.7 | Write auth detection tests | Create `internal/docker/auth_test.go`. Table-driven tests using `t.TempDir()` and `t.Setenv()`: Bedrock with/without settings.json, Max with/without credentials, API key set/missing, localhost rewrite (localhost, 127.0.0.1, ::1, non-localhost passthrough), env var precedence over file. Per review S7: test that Bedrock detection calls aws CLI (mock the exec). |
| 3.8 | Write labels tests | Test label builder: with slug, without slug, timestamp format. |

**Acceptance criteria**:
- Bedrock detection reads settings.json, pre-resolves AWS credentials via CLI, returns merged settings
- Max detection validates credentials file exists
- API key detection validates env var is set with actionable error on failure
- Local provider rewrites all localhost variants to `host.docker.internal`
- Local provider reachability check only runs when local provider is actually used (not globally) [B5]
- `CollectAuthRequirements` with `steps=["interactive"]` resolves auth using defaults [B3]
- All errors collected, not fail-on-first, with actionable messages [S1]
- `muster.created` timestamp label included [S3]
- All unit tests pass: `go test ./internal/docker/ -run TestDetect -v`

---

## Phase 4: Compose Generation and Asset Extraction

Generate merged compose files from embedded base + runtime overrides.

| ID | Task | Key Details |
|----|------|-------------|
| 4.1 | Create compose types | Define `ComposeFile`, `Service`, `BuildConfig` structs in `internal/docker/compose.go` with YAML tags. Use `map[string]any` for flexible fields (Volumes, Networks at top level). |
| 4.2 | Implement compose merge helpers | `mergeComposeFiles(base, override *ComposeFile)`, `mergeService(base, override *Service) *Service`, `appendUnique(base, add []string) []string`. Per review S9: when types conflict between base and override, override type always wins (replace entirely). |
| 4.3 | Implement compose generation | `GenerateComposeFile(opts ComposeOptions) (string, error)`. Load embedded base, apply auth overrides (`applyAuth`), apply workspace mounts (worktree + .git read-only), apply dev-agent config (env/volumes/networks), apply labels (including `muster.created` timestamp), apply user override if exists. Per review S4: write to temp file first (`docker-compose.yml.tmp`), validate with `docker compose config`, then atomic rename. Per review B4: validation enabled by default, skip if `MUSTER_SKIP_COMPOSE_VALIDATION=1` is set. |
| 4.4 | Implement domain allowlist generation | `GenerateAllowlist(assetDir string, auth *AuthRequirements, devAgent *config.DevAgentConfig) (string, error)`. Merge base `allowed-domains.txt` + provider domains + project domains. Per review S5: validate each domain (non-empty, valid hostname/wildcard), deduplicate before writing. |
| 4.5 | Implement asset extraction | `ExtractAssets() (string, error)`. Compute SHA-256 hash of embedded FS. Per review B1: extract to temp directory (`{hash}.tmp`), then atomic rename to `{hash}/`. Use filesystem lock (`{hash}.lock`) via `os.OpenFile` with `O_CREATE|O_EXCL` during extraction for concurrency safety. If final directory already exists, skip extraction. |
| 4.6 | Implement git worktree detection | Per review S8: `DetectWorkspace() (worktreeDir, mainRepoDir string, err error)`. Run `git rev-parse --show-toplevel` for working directory and `git rev-parse --git-common-dir` for main .git location. If paths differ, this is a worktree (mount both). If same, this is main repo (mount once read-write). |
| 4.7 | Populate embedded Docker assets | Add actual files to `internal/docker/docker/`: `docker-compose.yml` (base compose with dev-agent + proxy services), `agent.Dockerfile`, `proxy.Dockerfile`, `entrypoint.sh`, `squid.conf`, `allowed-domains.txt`, `settings.json`, `settings.max.json`. Port from reference implementation. |
| 4.8 | Write compose generation tests | Create `internal/docker/compose_test.go`. Table-driven tests: base loading, Bedrock/Max/API key/local auth injection, workspace mount, dev-agent env/volumes, label injection, user override merge, service merge (scalar replace, map merge, list append, dedup). Per review S9: test type conflict (base has string, override has list). Golden file tests with `-update` flag. |
| 4.9 | Write asset extraction tests | Test: first extraction creates directory, second extraction skips, atomic rename on success. Integration test (short-guarded): verify extracted files match embedded. |
| 4.10 | Write allowlist tests | Test: domain merge, deduplication, provider-specific domains added, invalid domain rejected with warning. |
| 4.11 | Write worktree detection tests | Integration tests (short-guarded): create temp git repo, create worktree, verify detection. |

**Acceptance criteria**:
- Generated compose file contains correct services, volumes, env vars, labels for all auth types
- Compose file written atomically: temp file -> validate -> rename [S4]
- Validation enabled by default, opt-out via `MUSTER_SKIP_COMPOSE_VALIDATION=1` [B4]
- Asset extraction is atomic with temp dir + rename + filesystem lock [B1]
- Domain allowlist deduplicated and validated [S5]
- Git worktree detection uses `git rev-parse --show-toplevel` and `--git-common-dir` [S8]
- Compose merge handles type conflicts (override type wins) [S9]
- Golden files match expected output
- All unit tests pass: `go test ./internal/docker/ -run TestGenerate -v`
- All unit tests pass: `go test ./internal/docker/ -run TestExtract -v`

---

## Phase 5: Container Lifecycle and Docker Client

Implement the Docker SDK client wrapper and compose CLI operations.

| ID | Task | Key Details |
|----|------|-------------|
| 5.1 | Implement Docker client | Create `internal/docker/container.go` with `Client` struct wrapping Docker SDK. `NewClient() (*Client, error)` initializes with `client.FromEnv` and `client.WithAPIVersionNegotiation()`. Per review S10: on creation, run `docker compose version` to verify v2+. If `docker compose` not found, check for `docker-compose` (v1) and return clear error: `"Docker Compose v2+ required; install from https://docs.docker.com/compose/install/"`. If only v1: `"Docker Compose v1 detected but v2+ required; upgrade at https://docs.docker.com/compose/install/"`. |
| 5.2 | Implement compose CLI operations | `ComposeUp(ctx, composeFile, projectName)`, `ComposeDown(ctx, composeFile, projectName)`, `ComposeExec(ctx, composeFile, projectName, service, cmd)`. All via `exec.CommandContext("docker", "compose", ...)` with stdout/stderr/stdin passthrough. Per review S6: use `context.WithTimeout` (default 5 minutes for up/down, no timeout for exec). |
| 5.3 | Implement container queries | `ListContainers(ctx, project, slug) ([]ContainerInfo, error)` using Docker SDK label filtering. `Ping(ctx) error` to check daemon reachability. Per review S6: wrap SDK calls with timeout context to prevent infinite hangs on daemon disconnection. |
| 5.4 | Implement `Ping` with version check | `Ping` checks daemon reachability and compose version in a single call. On connection failure, return: `"Docker daemon is not running; start Docker Desktop or run 'sudo systemctl start docker'"`. |
| 5.5 | Write container lifecycle integration tests | Create `internal/docker/container_test.go`. All guarded with `if testing.Short() { t.Skip() }`. Tests: ComposeUp with labels, ListContainers by project/slug, ComposeExec runs command, ComposeDown removes containers, Ping succeeds when Docker running. Use minimal alpine compose file. |
| 5.6 | Write version detection tests | Unit test: mock `docker compose version` output, verify parsing. Test v2 detected, v1 detected (error), neither found (error). |

**Acceptance criteria**:
- `NewClient` verifies Docker Compose v2+ is installed [S10]
- Compose CLI operations stream stdout/stderr and use timeout contexts [S6]
- `ListContainers` filters by project and slug labels
- `Ping` returns actionable error when Docker daemon not running
- Integration tests (with Docker) pass: start container, verify labels, exec command, tear down
- All tests pass: `go test ./internal/docker/ -run TestCompose -v -short=false` (with Docker)

---

## Phase 6: Command Integration

Wire everything together in `cmd/code.go` and `cmd/down.go`.

| ID | Task | Key Details |
|----|------|-------------|
| 6.1 | Implement `muster code` command | Create `cmd/code.go`. Register with rootCmd. Flags: `--yolo`, `--tool`, `--no-plugin`, `--main-branch`, `--current-path`. Without `--yolo`: load config, stage prompts, exec tool locally (Phase 1 wire-up). With `--yolo`: full Docker flow (load config, validate, collect auth with `steps=["interactive"]` [B3], extract assets, detect workspace, generate compose, start containers, exec tool in container). |
| 6.2 | Implement `muster down` command | Create `cmd/down.go`. Register with rootCmd. Flags: `--all`, `--orphans`, `--project`. Determine project from flag or current directory basename. Without args: `down --all` for current project. With slug arg: down containers matching slug. `--orphans`: list containers, cross-reference slugs against `.muster/roadmap.json` status, down those not `in_progress`. Per review S3: `--orphans` ignores containers created within last hour (using `muster.created` label). |
| 6.3 | Add Docker dependency to go.mod | Run `go get github.com/docker/docker/client`. Promote `gopkg.in/yaml.v3` from indirect to direct. |
| 6.4 | Write command integration tests | Integration tests (short-guarded): build binary, run `muster code --yolo` with fixture config and Docker running, verify container starts with labels. Run `muster down --all`, verify containers removed. Test `muster code --yolo` without config returns actionable error. |
| 6.5 | Add Windows CI runner | Per review B2: add `windows-latest` to CI matrix for integration tests. Test volume mount path generation with Windows paths. Document Docker Desktop file sharing configuration requirement in README. |
| 6.6 | Run full test suite | Run `go test ./... -v` on all platforms (Linux, macOS, Windows). Run `go test ./... -v -short=false` in CI with Docker. Verify all golden files are up to date. Verify code coverage >= 75% for `internal/config` and `internal/docker`. |

**Acceptance criteria**:
- `muster code --yolo` successfully boots container, execs tool, leaves container running
- `muster code --yolo` without config returns actionable error
- `muster code --yolo` with missing auth returns all validation errors at once
- `muster down <slug>` tears down containers matching slug
- `muster down --all` tears down all project containers
- `muster down --orphans` finds containers with stale slugs, ignores containers < 1 hour old [S3]
- Phase 2 uses `steps=["interactive"]` for auth collection, not pipeline step scanning [B3]
- Windows CI runner passes integration tests [B2]
- All unit tests pass on Linux, macOS, Windows
- Integration tests pass in CI with Docker installed
- Code coverage >= 75% for new packages

---

## Test Strategy

### Unit Tests (76+ cases, no Docker)

| Package | File | Cases | Focus |
|---------|------|-------|-------|
| `internal/config` | `user_test.go` | 5+ | Valid/invalid/missing user config |
| `internal/config` | `project_test.go` | 5+ | Valid/missing project config |
| `internal/config` | `devagent_test.go` | 5+ | Full/empty/invalid dev-agent config |
| `internal/config` | `merge_test.go` | 8+ | Scalar override, list replace, null removal, empty value, nested merge |
| `internal/config` | `resolve_test.go` | 10+ | Fallback chain, tier resolution, validation errors |
| `internal/docker` | `auth_test.go` | 18+ | Per-provider detection with mock FS/env |
| `internal/docker` | `compose_test.go` | 20+ | Generation, merge, golden files |
| `internal/docker` | `labels_test.go` | 3+ | Label builder, timestamp |
| `internal/docker` | `allowlist_test.go` | 5+ | Domain merge, dedup, validation |

### Integration Tests (17+ cases, Docker required, short-guarded)

| Package | File | Cases | Focus |
|---------|------|-------|-------|
| `internal/docker` | `container_test.go` | 8+ | Start/stop/exec/list with labels |
| `internal/docker` | `compose_test.go` | 4+ | `docker compose config` validation |
| `internal/docker` | `assets_test.go` | 2+ | Extract + cache verification |
| `internal/docker` | `worktree_test.go` | 3+ | Git worktree detection |

### E2E Tests (13+ cases, built binary + Docker)

| Package | File | Cases | Focus |
|---------|------|-------|-------|
| `cmd` | `code_test.go` | 4+ | `muster code --yolo` success/failure |
| `cmd` | `down_test.go` | 6+ | Slug/all/orphans teardown |
| `cmd` | `doctor_test.go` | 3+ | Config validation, missing auth |

### Quality Gates (pre-merge)

1. All unit tests pass on Linux, macOS, Windows
2. Integration tests pass in CI with Docker
3. Golden files up to date (no diffs after `-update`)
4. Generated compose files pass `docker compose config`
5. Code coverage >= 75% for `internal/config` and `internal/docker`
6. `golangci-lint run` reports 0 issues
7. Cross-platform smoke test: `muster version` exits 0 on all platforms
8. Windows CI runner passes [B2]

---

## Reviewer Findings: Resolution Summary

### BLOCKERs (all addressed)

| ID | Finding | Resolution | Phase |
|----|---------|------------|-------|
| B1 | Asset extraction needs atomic temp dir + rename + lock | Extract to `{hash}.tmp`, atomic rename, filesystem lock via `O_CREATE\|O_EXCL` | 4.5 |
| B2 | Windows path handling needs CI runner + Docker Desktop tests | Add `windows-latest` to CI matrix, test path conversion | 6.5 |
| B3 | Step enumeration -- hardcode "interactive" for Phase 2 | `cmd/code.go` passes `steps=["interactive"]`, resolves via defaults; pipeline scanning deferred to Phase 6 | 3.6, 6.1 |
| B4 | Compose validation enabled by default, opt-out via env var | Validate by default, skip if `MUSTER_SKIP_COMPOSE_VALIDATION=1` | 4.3 |
| B5 | Local provider validation only when step actually uses it | Reachability check conditional on local provider appearing in resolved steps | 3.5, 3.6 |

### SHOULD FIX (all incorporated)

| ID | Finding | Resolution | Phase |
|----|---------|------------|-------|
| S1 | Error message examples in relevant tasks | Actionable error templates with fix commands in auth detection and validation | 2.3, 3.2-3.5 |
| S2 | Deep merge semantics: null removes, empty replaces, lists replace | Documented and tested in merge implementation | 2.1, 2.5 |
| S3 | Add `muster.created` timestamp label | `LabelCreated` with RFC3339 timestamp; `--orphans` ignores containers < 1 hour old | 3.1, 6.2 |
| S4 | Atomic compose file writing (temp, validate, rename) | Write to `.tmp`, validate, rename on success, delete temp on failure | 4.3 |
| S5 | Proxy domain allowlist validation and deduplication | Validate non-empty hostname, deduplicate before writing | 4.4 |
| S6 | Docker daemon disconnection handling with timeouts | `context.WithTimeout` on all SDK and compose CLI calls | 5.2, 5.3 |
| S7 | Bedrock AWS credential pre-resolution | Run `aws configure export-credentials` in `DetectBedrockAuth` | 3.2 |
| S8 | Git worktree detection via `git rev-parse` | `--show-toplevel` + `--git-common-dir` to detect worktree vs main repo | 4.6 |
| S9 | Compose merge type conflict handling | Override type always wins (replace base value entirely) | 4.2 |
| S10 | Docker Compose v1 vs v2 version detection | Version check in `NewClient`, clear error messages for v1 or missing | 5.1 |

---

## Known Limitations

These items are explicitly deferred and documented as future improvements.

| ID | Limitation | Workaround | Future Phase |
|----|-----------|------------|-------------|
| N1 | Asset hash computed at runtime, not build time | Negligible startup cost; could use `go generate` later | Post-Phase 2 |
| N2 | `ComposeOptions` uses flat struct, not builder pattern | Struct is sufficient for current field count | Post-Phase 2 |
| N3 | Container labels don't include muster CLI version | Can be added later for version-aware cleanup | Post-Phase 2 |
| N4 | Integration tests don't use testcontainers for isolation | Tests use `t.Cleanup` for teardown; testcontainers adds complexity | Post-Phase 2 |
| N5 | Config validation doesn't provide auto-fix commands | Error messages include manual fix instructions | Phase 7 (`muster doctor --fix`) |
| N6 | Generated compose files don't include source comments | Users can `cat` the compose file for debugging | Post-Phase 2 |
| N7 | Docker SDK client created per command, not pooled | Single command = single client; pooling adds complexity | Post-Phase 2 |
| N8 | No semaphore limiting parallel container count | `muster in --all` not implemented until Phase 6 | Phase 6 |
| N9 | Auth detection results not cached across invocations | Detection is fast enough per-invocation; caching adds staleness risk | Post-Phase 2 |
| N10 | Error messages don't link to hosted documentation | No hosted docs exist yet | Phase 8 |
| L1 | Auth changes require `muster down` and restart | No hot-reload; could add `muster reload-auth` later | Post-Phase 2 |
| L2 | No iptables/nftables firewall layer | Proxy is sole isolation mechanism across all platforms | Post-Phase 2 |
| L3 | No config environment variable expansion (`${VAR}`) | Users can use `.local.yml` overrides | Post-Phase 2 |
| L4 | Pipeline step scanning not implemented | Phase 2 uses `steps=["interactive"]`; full scanning in Phase 6 | Phase 6 |

---

## References

- **Synthesis**: `.plan/docker-orchestration/synthesis/synthesis.md`
- **Architecture**: `.plan/docker-orchestration/planning/architecture.md`
- **Product scope**: `.plan/docker-orchestration/planning/product-scope.md`
- **QA strategy**: `.plan/docker-orchestration/planning/qa-strategy.md`
- **Plan review**: `.plan/docker-orchestration/planning/plan-review.md`
- **Design doc**: `docs/design.md` (Configuration: lines 84-192, Architecture: lines 194-237, Docker: lines 339-359)
- **Existing code**: `cmd/root.go`, `cmd/version.go`, `internal/ui/output.go`, `internal/docker/embed.go`, `internal/testutil/helpers.go`
