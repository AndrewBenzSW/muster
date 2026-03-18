# Plan Review: Docker Container Orchestration

*Adversarial Review — 2026-03-18*

---

## Overview

This document presents findings from an adversarial review of the Phase 2 Docker orchestration planning documents (product scope, architecture, QA strategy). The review identifies gaps, challenges assumptions, checks consistency between requirements and design, and assesses implementation feasibility.

All findings are severity-leveled (BLOCKER, SHOULD FIX, NICE TO HAVE) with specific references to source documents and suggested fixes.

---

## BLOCKER Findings

These issues must be resolved before implementation can succeed.

### B1: Missing Asset Extraction Implementation Details

**Finding**: Architecture describes `ExtractAssets()` function that extracts embedded Docker assets to `~/.cache/muster/docker-assets/{hash}/` but doesn't specify:
- What happens if extraction fails mid-process (partial state)?
- Whether extraction is atomic or can leave corrupted cache
- How hash is computed (content hash of what exactly? Individual files? The entire FS?)
- Whether multiple muster processes can safely extract concurrently

**Evidence**:
- architecture.md:462-467 describes extraction but omits error recovery
- No mention in QA strategy for testing partial extraction scenarios
- Concurrent extraction could cause race conditions if not handled

**Impact**: First-time users or users upgrading muster versions could hit extraction failures that leave cache in broken state, preventing container startup.

**Suggested fix**:
- Extract to temporary directory first (`{hash}.tmp`), then atomic rename to `{hash}/`
- Compute hash over sorted concatenation of embedded file paths + contents
- Use filesystem lock (`{hash}.lock`) during extraction
- Add integration test for concurrent extraction (two processes extracting same version)

---

### B2: Windows Path Handling Unverified

**Finding**: Architecture claims cross-platform support but Windows-specific concerns are deferred to "verification testing in Phase 2" without concrete plan.

**Evidence**:
- product-scope.md:263 calls out Windows compatibility as open question
- architecture.md:713-725 describes path handling with `filepath.Join` but doesn't address Docker Desktop VM path translation
- QA strategy §Risk Mitigation identifies Windows testing gap but only suggests "manual testing"

**Windows-specific risks**:
- Volume mount syntax differs (`C:\Users\...` vs Linux paths)
- Docker Desktop on Windows runs in WSL2 VM, host paths need translation
- Cache directory on Windows is `C:\Users\{user}\.cache` which may not be in Docker Desktop's shared folders by default

**Impact**: `muster code --yolo` may fail on Windows with cryptic Docker volume mount errors. No CI coverage means bugs slip to production.

**Suggested fix**:
- Add Windows runner to CI with integration tests
- Document Windows-specific setup (Docker Desktop file sharing configuration)
- Test volume mount path conversion explicitly: create test case with Windows paths, verify compose file generation
- Consider using forward slashes in volume mount source paths (Docker accepts them on Windows)

---

### B3: Config Resolution Missing Pipeline Step Enumeration

**Finding**: Config resolution requires scanning "all pipeline steps" but there's no mechanism to enumerate which steps exist.

**Evidence**:
- synthesis.md:186-189 requires scanning all steps upfront for auth
- architecture.md:318-324 `CollectAuthRequirements(cfg, steps []string)` takes step names as parameter
- product-scope.md user stories reference steps: `plan`, `execute`, `review`, `add`, but these aren't defined anywhere
- No documentation of how `steps []string` parameter is populated

**Chain of failures**:
1. `CollectAuthRequirements` needs list of all step names
2. Step names come from project config's pipeline section
3. But project config uses `map[string]StepOverride` which could have any keys
4. How does caller know which keys are actual steps vs typos?
5. If a typo exists (e.g., `exeucte` instead of `execute`), auth for that provider silently won't be collected

**Impact**: Missing auth detection if project config has typos or references steps that don't actually get executed. Could cause mid-pipeline failures.

**Suggested fix**:
- Define canonical step names in `docs/design.md` Phase 4+ planning
- For Phase 2 (interactive only), hardcode `steps := []string{"interactive"}` in `cmd/code.go`
- Add validation: if project config references step names not in canonical list, warn but don't fail
- Document in architecture: "Phase 2 only supports interactive mode; pipeline step scanning deferred to Phase 6"

---

### B4: Compose Validation Performance Unresolved

**Finding**: Architecture proposes running `docker compose config` to validate generated files, but product scope lists this as "open question" about 1-second latency.

**Evidence**:
- product-scope.md:266 explicitly asks if validation is acceptable or should be behind flag
- architecture.md:407 states "Validate: Run `docker compose -f {path} config`" as part of generation flow
- synthesis.md:226 marks this as resolved ("Validate at runtime before starting") but doesn't address latency concern
- QA strategy requires validation in integration tests but doesn't test performance impact

**Conflict**: Design includes validation unconditionally, but product scope questions it. Synthesis claims it's resolved but doesn't explain the decision reasoning.

**Impact**: Every `muster code --yolo` invocation adds 1+ second latency. For rapid iteration (restarting container multiple times), this compounds to noticeable UX degradation.

**Suggested fix**:
- Document decision explicitly: validation is enabled by default for safety, can be disabled with `MUSTER_SKIP_COMPOSE_VALIDATION=1` env var
- Add benchmark test measuring validation overhead on typical compose file
- Consider caching: hash generated compose file, skip validation if hash matches previous validated generation
- Update product-scope.md to close open question with documented decision

---

### B5: Local Provider Reachability Check Blocks Startup

**Finding**: Synthesis requires local provider validation at startup with 2s timeout, but this breaks legitimate use cases.

**Evidence**:
- synthesis.md:210-212 resolves that startup validates local servers are running
- product-scope.md:110 requires validation with "Clear error message if local server is not running"
- architecture.md:341 `DetectLocalProvider()` includes optional reachability validation

**Legitimate blocked scenarios**:
1. User configures local provider in user config but current project doesn't use it (auth check fails despite not needed)
2. Local server takes >2s to respond during startup (Ollama pulling model)
3. User wants to start container before local server is ready, plans to start server later

**Impact**: False positive failures prevent container startup when local provider is configured but not actively used by current command.

**Suggested fix**:
- Only validate reachability if step resolution determines local provider is actually used
- Increase timeout to 5s with clear progress message ("Checking local provider at http://localhost:1234...")
- Make validation optional via flag: `--skip-provider-checks` for troubleshooting
- Document workaround: remove local provider from config if not using it for current project

---

## SHOULD FIX Findings

These issues should be fixed to improve quality but aren't blockers.

### S1: Auth Detection Error Messages Missing Examples

**Finding**: Architecture emphasizes "actionable error messages" but doesn't show examples of what these look like.

**Evidence**:
- architecture.md:760-763 lists example error messages
- synthesis.md:191-193 requires validation with actionable messages
- No examples in QA strategy test cases (should be in error message assertions)

**Risk**: Developers implement generic errors like "Bedrock auth failed" instead of helpful messages with fix instructions.

**Suggested fix**: Add error message examples to architecture.md §Error Handling:
```
provider anthropic requires ANTHROPIC_API_KEY; set it in your shell:
  export ANTHROPIC_API_KEY=sk-ant-...
or configure api_key_env in ~/.config/muster/config.yml

bedrock auth requires ~/.aws directory; run AWS CLI setup:
  aws configure sso
  aws sso login --profile <profile-name>
then ensure CLAUDE_CODE_USE_BEDROCK=1 in ~/.claude/settings.json
```

---

### S2: Deep Merge Semantics Underspecified

**Finding**: Config loading requires "deep merge" for `.local.yml` but implementation details are vague.

**Evidence**:
- architecture.md:296-303 describes `deepMerge(base, local []byte)` with high-level semantics
- synthesis.md:46 states "lists are replaced not appended" but doesn't explain nested lists
- QA strategy tests include "nested map merge" but not "map containing list" case

**Edge cases not addressed**:
- What if base has `env: {A: 1}` and local has `env: null`? (delete entire map? error?)
- What if base has `volumes: ["a"]` and local has `volumes: null`? (empty list? error?)
- Nested maps within lists: `volumes: [{type: bind, source: /a}]` — is this a map that merges or list that replaces?

**Impact**: Surprising behavior when users try to "remove" config via `.local.yml`. Merge may not do what they expect.

**Suggested fix**:
- Document explicit rules in architecture.md:
  - `null` in local removes the field from base
  - Empty maps `{}` and empty lists `[]` are valid values that replace base
  - Lists always replace (never merge), including lists of maps
- Add test cases for null values, empty values, and nested structures
- Consider YAML anchors/aliases for complex overrides (document as alternative pattern)

---

### S3: Container Labeling Missing Timestamp

**Finding**: Container labels include `muster.managed`, `muster.project`, `muster.slug` but no creation timestamp for orphan detection.

**Evidence**:
- architecture.md:238-254 defines label constants
- product-scope.md:76 requires `--orphans` flag to find containers with no active work item
- No mechanism to detect "stale" containers vs recently created ones

**Scenario**: User runs `muster code --yolo`, works for 10 minutes, then `muster down --orphans` removes their active container because it doesn't match a roadmap item yet.

**Impact**: `--orphans` flag is unsafe without age filtering. Could remove containers user is actively working in.

**Suggested fix**:
- Add `muster.created` label with RFC3339 timestamp
- `--orphans` flag should ignore containers created within last 1 hour
- Document in product-scope.md that `--orphans` has age threshold to prevent false positives
- Alternative: add `muster.status=active|stopped` label that gets set based on whether container is running

---

### S4: No Rollback Strategy for Compose Generation Failures

**Finding**: Compose generation writes to `~/.cache/muster/{project}/docker-compose.yml` but what if validation fails after writing?

**Evidence**:
- architecture.md:396-408 generates and writes file, then validates
- If validation fails, cached file is in broken state
- Next invocation might skip generation if file exists, attempt to use broken cached file

**Impact**: One bad generation can poison cache, requiring manual deletion of cache directory.

**Suggested fix**:
- Write to temporary file: `docker-compose.yml.tmp`
- Validate temporary file
- Atomic rename to `docker-compose.yml` only if validation passes
- On validation failure, delete temporary file, return error
- Add test case: trigger validation failure (malformed YAML), verify cache not corrupted

---

### S5: Proxy Domain Allowlist Merge Has No Conflict Detection

**Finding**: Domain allowlist merging combines base + provider-specific + user-defined domains but doesn't check for conflicts or invalid patterns.

**Evidence**:
- architecture.md:589-614 describes `GenerateAllowlist()` merging domains
- synthesis.md:64-67 requires merged allowlist from multiple sources
- No validation that domains are valid (could be empty string, invalid regex, etc.)

**Risks**:
- User adds empty string to `allowed_domains`, proxy accepts it, proxy config breaks
- User adds domain with typo (`*.anthropc.com`), API calls silently fail
- Duplicate domains waste proxy config space, no deduplication mentioned

**Suggested fix**:
- Validate each domain: non-empty, valid hostname or wildcard pattern
- Deduplicate domains before writing allowlist
- Warn (don't fail) on suspicious patterns: single-label domains, bare IPs without CIDR
- Add QA test case: malformed domain in config, verify clear error message

---

### S6: Missing Error Recovery for Docker Daemon Disconnection

**Finding**: Integration tests check Docker daemon reachability via `Ping()` but no handling for daemon stopping mid-execution.

**Evidence**:
- architecture.md:549 implements `Ping()` for startup check
- QA strategy includes "Ping Docker unreachable" test
- No handling if daemon stops while container is running (e.g., Docker Desktop restart)

**Scenario**: User runs `muster code --yolo`, Docker Desktop crashes or gets updated mid-session, muster hangs or panics on next Docker SDK call.

**Impact**: Poor UX when Docker daemon becomes unavailable during long-running operations.

**Suggested fix**:
- Wrap all Docker SDK calls in retry logic with exponential backoff
- Detect connection errors (check error type), distinguish from other errors
- Provide clear message: "Docker daemon connection lost, waiting for Docker to restart..."
- Add `context.WithTimeout` to all SDK operations to prevent infinite hangs
- Document in troubleshooting: if Docker Desktop updates, run `muster down --all` first

---

### S7: Bedrock AWS Credential Pre-Resolution Not Implemented

**Finding**: Synthesis requires pre-resolving Bedrock credentials via `aws configure export-credentials` but architecture doesn't implement it.

**Evidence**:
- synthesis.md:39 explicitly requires "pre-resolve credentials via `aws configure export-credentials`"
- architecture.md:350-358 describes Bedrock detection but only mounts `~/.aws` directory
- No call to AWS CLI in `DetectBedrockAuth()`

**Why pre-resolution matters**: AWS credentials from SSO expire quickly (hours). Mounting `~/.aws` isn't enough if creds are stale. Reference implementation `run.sh:155-174` explicitly runs `aws configure export-credentials` to ensure valid credentials.

**Impact**: Bedrock auth works initially but fails after SSO token expires, requiring user to exit container and re-auth.

**Suggested fix**:
- Implement pre-resolution in `DetectBedrockAuth()`:
  ```go
  func DetectBedrockAuth() (*BedrockAuth, error) {
      // Check settings.json for BEDROCK flag
      // If AWS_PROFILE set, run: aws configure export-credentials --profile $PROFILE
      // Parse output JSON, verify AccessKeyId/SecretAccessKey present
      // Return BedrockAuth with validated credentials
  }
  ```
- Add test case: mock `aws` command execution, verify credentials extracted
- Document in architecture that pre-resolution is required for SSO

---

### S8: Git Worktree Detection Algorithm Not Specified

**Finding**: Architecture mentions detecting "worktree dir and main repo dir from git" but doesn't specify how.

**Evidence**:
- architecture.md:647 references detecting workspace from git
- product-scope.md:22 requires worktree mounting but doesn't explain detection
- No reference to which git commands to run

**Git worktree scenarios**:
- Main repo working directory: `.git` is directory
- Worktree directory: `.git` is file pointing to main repo's `.git/worktrees/{name}`
- Bare repo (no working directory): `.git` is entire repo

**Impact**: Incorrect detection mounts wrong directories, AI agent can't see code or git history.

**Suggested fix**:
- Document algorithm in architecture.md:
  1. Run `git rev-parse --show-toplevel` to get working directory
  2. Run `git rev-parse --git-common-dir` to get main repo's .git location
  3. If paths differ, this is a worktree (mount both)
  4. If paths same, this is main repo (mount once read-write)
- Add integration test with actual git worktree setup
- Reference Phase 1 implementation if it already exists

---

### S9: Compose Merge Helpers Don't Handle Type Conflicts

**Finding**: Architecture describes merge helpers but doesn't address type mismatches between base and override.

**Evidence**:
- architecture.md:444-456 describes `mergeService()` and field merging
- Doesn't explain what happens if base has `ports: ["80:80"]` (list) and override has `ports: 80` (int)

**YAML flexibility**: gopkg.in/yaml.v3 allows same field to have different types in different files. Docker Compose spec has some fields that accept multiple types (e.g., `ports` can be string or list).

**Impact**: Merge panics with type assertion failure or silently produces invalid compose file.

**Suggested fix**:
- Document type precedence: override's type always wins (replace base value entirely)
- Add validation: after merge, check field types match Compose spec expectations
- Test case: base has field as string, override has field as list, verify merge succeeds
- Consider using `json.Marshal` roundtrip to normalize types

---

### S10: No Handling for Docker Compose v1 vs v2

**Finding**: Architecture assumes Docker Compose v2 (`docker compose`) but doesn't check version or handle v1 legacy command.

**Evidence**:
- architecture.md:744 explicitly states "Docker Compose v2 is a CLI plugin invoked as `docker compose`"
- QA strategy requires Docker Compose v2+ but no version check in code
- product-scope.md:241 lists "Docker Compose CLI (v2+)" as hard requirement

**Backward compatibility**: Some users still have v1 installed as `docker-compose` (hyphenated). Command will fail with "unknown command: compose".

**Impact**: Users with only v1 installed get cryptic error "docker: 'compose' is not a docker command".

**Suggested fix**:
- Add version check in `NewClient()`: run `docker compose version`, parse output
- If v2 not found, check for v1: `docker-compose version`
- If neither found, error: "Docker Compose v2+ required; install from https://docs.docker.com/compose/install/"
- If only v1 found, error: "Docker Compose v1 detected but v2+ required; upgrade to Docker Compose v2"
- Add QA test for version detection (mock command output)

---

## NICE TO HAVE Findings

These would improve the plan but aren't critical.

### N1: Asset Extraction Could Use Embedded Hash

**Finding**: Architecture proposes computing content hash at runtime but hash could be embedded at build time.

**Current approach (architecture.md:462-467)**: Compute SHA-256 of embedded FS at runtime, use as cache key.

**Alternative**: Use `go generate` to compute hash and embed as constant:
```go
//go:generate sh -c "find docker -type f -exec shasum -a 256 {} + | shasum -a 256 | awk '{print \"package docker\\nconst assetsHash = \\\"\" $1 \"\\\"\"}' > assets_hash.go"
```

**Benefit**: Faster startup (no hash computation), simpler code, hash visible in version command.

---

### N2: Compose Generation Could Use Builder Pattern

**Finding**: `ComposeOptions` struct has 7 fields, error-prone to construct correctly.

**Evidence**: architecture.md:384-394 shows `ComposeOptions` with many required fields.

**Alternative**: Builder pattern with validation:
```go
opts := docker.NewComposeOptions().
    WithProject(project).
    WithAuth(auth).
    WithDevAgent(devAgent).
    WithWorkspace(worktreeDir, mainRepoDir).
    Build()  // validates all required fields set
```

**Benefit**: Compile-time safety (can't forget required fields), fluent API, easier to extend.

---

### N3: Label Constants Could Include Version

**Finding**: Container labels don't include muster version that created them.

**Benefit**: Enables version-aware cleanup (`muster down --older-than v0.2.0`), troubleshooting (identify containers from old versions), migration helpers.

**Suggested**: Add `muster.version` label with semver string.

---

### N4: Integration Tests Could Use Testcontainers

**Finding**: QA strategy integration tests start real containers but don't isolate them.

**Alternative**: Use testcontainers-go library to spin up isolated Docker-in-Docker for tests.

**Benefit**: True isolation (tests can't interfere with host Docker state), parallel test execution safe, no cleanup burden on CI.

**Tradeoff**: Adds dependency, increases test complexity, slower startup (nested Docker).

---

### N5: Config Validation Could Provide Fix Commands

**Finding**: Architecture requires "actionable error messages" but doesn't suggest auto-fix commands.

**Enhancement**: Include executable fix commands in error output:
```
provider bedrock requires AWS credentials

Run these commands to fix:
  aws configure sso
  aws sso login --profile default
  echo 'CLAUDE_CODE_USE_BEDROCK=1' >> ~/.claude/settings.json

Or run: muster setup bedrock
```

**Benefit**: Reduces friction for first-time users, improves discoverability of features.

---

### N6: Compose File Could Include Comments

**Finding**: Generated compose files are pure YAML without comments explaining overrides.

**Enhancement**: Add comments documenting source of each override:
```yaml
services:
  dev-agent:
    # Base image from embedded docker/docker-compose.yml
    image: muster-dev-agent:latest

    # Auth overrides from ~/.aws
    volumes:
      - /home/user/.aws:/home/node/.aws:ro  # Bedrock auth (read-only)
```

**Benefit**: Easier debugging, users understand what muster generated and why.

**Implementation**: Use gopkg.in/yaml.v3's `HeadComment` field on nodes.

---

### N7: Docker SDK Client Could Pool Connections

**Finding**: `NewClient()` creates new SDK client per command invocation.

**Enhancement**: Implement client pooling or singleton pattern to reuse connections across invocations in same process.

**Benefit**: Faster repeated operations (no handshake overhead), fewer file descriptors.

**Tradeoff**: More complex lifecycle management, need to handle stale connections.

---

### N8: Parallel Step Execution Could Use Semaphore

**Finding**: Architecture mentions filesystem locks for parallel execution but doesn't limit concurrency.

**Risk**: User runs `muster in --all` with 50 work items, spawns 50 containers simultaneously, exhausts Docker/system resources.

**Enhancement**: Implement semaphore limiting concurrent containers (default: CPU count or 4, configurable via `MUSTER_MAX_PARALLEL`).

**Benefit**: Prevents resource exhaustion, predictable performance.

---

### N9: Auth Detection Could Cache Results

**Finding**: `CollectAuthRequirements()` runs expensive checks (filesystem stats, AWS CLI invocation) on every invocation.

**Enhancement**: Cache auth detection results for 5 minutes (or until config changes).

**Benefit**: Faster repeated commands, less AWS CLI overhead.

**Tradeoff**: Stale cache if credentials change mid-session (acceptable given cache timeout).

---

### N10: Error Messages Could Link to Documentation

**Finding**: Error messages provide fix instructions but no links to detailed docs.

**Enhancement**: Include documentation URLs in errors:
```
provider bedrock auth failed: ~/.aws directory not found

Fix: https://muster.dev/docs/providers/bedrock-setup
```

**Benefit**: Users get complete context and troubleshooting steps.

**Requirement**: Needs hosted documentation site (Phase 3+).

---

## Consistency Check

### Product Scope ↔ Architecture Alignment

| Requirement | Product Scope | Architecture | Status |
|------------|---------------|--------------|--------|
| Compose CLI usage | §122 | §500-531 | ✓ Consistent |
| Docker SDK for queries | §122 | §533-587 | ✓ Consistent |
| Label-based discovery | §70-78 | §232-254 | ✓ Consistent |
| Auth upfront scanning | §128-131 | §315-349 | ✓ Consistent |
| Bedrock detection | §37-39 | §350-358 | ⚠ Missing pre-resolution (S7) |
| Max detection | §37-39 | §350-358 | ✓ Consistent |
| Local provider URL rewrite | §106-112 | §359-373 | ✓ Consistent |
| Config resolution fallback | §135-137 | §263-280 | ✓ Consistent |
| Deep merge semantics | §135-137 | §296-303 | ⚠ Underspecified (S2) |
| Single compose file | §92-94 | §379-408 | ✓ Consistent |
| Read-only credential mounts | §131 | §410-441 | ✓ Consistent |
| Validation before startup | §38-39 | §315-324, §407 | ✓ Consistent |
| Container persistence | §52-57 | §898 | ✓ Consistent |

### Architecture ↔ QA Strategy Alignment

| Component | Architecture LOC | QA Test Count | Coverage Gap |
|----------|------------------|---------------|--------------|
| Config resolution | ~80 lines | 15+ tests | ✓ Good coverage |
| Config loading/merging | ~160 lines | 12+ tests | ⚠ Missing null handling (S2) |
| Compose generation | ~250 lines | 20+ tests | ✓ Good coverage |
| Auth detection | ~200 lines | 18+ tests | ⚠ Missing pre-resolution test (S7) |
| Container lifecycle | ~150 lines | 8+ tests | ⚠ Missing disconnect handling (S6) |
| Merge helpers | (in compose.go) | 8+ tests | ⚠ Missing type conflict tests (S9) |

### Synthesis ↔ Product Scope Alignment

| Synthesis Requirement | Product Scope Section | Traceability |
|----------------------|---------------------|--------------|
| Scan all steps upfront | §128-131 | ✓ Explicit |
| Provider auth patterns | §37-39 (Story 2) | ✓ Explicit |
| Config resolution flow | §135-137 (Story 2) | ✓ Explicit |
| Compose merge semantics | §92-94 (Story 5) | ✓ Explicit |
| Container labels | §70-78 (Story 4) | ✓ Explicit |
| Localhost URL rewriting | §106-112 (Story 6) | ✓ Explicit |
| Read-only mounts | §131 | ✓ Explicit |
| Proxy allowlist merge | §147-148 | ✓ Explicit |

**Overall consistency**: Strong traceability between requirements and design. Key gaps are in implementation details (error recovery, edge cases) rather than missing requirements.

---

## Feasibility Assessment

### Implementation Complexity by Component

| Component | Architecture LOC | Complexity | Risk | Confidence |
|----------|------------------|------------|------|-----------|
| Config loading | ~80 | Low | Low | High - standard YAML parsing |
| Config merging | ~60 | Medium | Medium | Medium - deep merge edge cases (S2) |
| Config resolution | ~80 | Medium | Low | High - straightforward fallback chain |
| Auth detection (Bedrock) | ~80 | High | High | Low - requires AWS CLI integration (S7) |
| Auth detection (Max) | ~30 | Low | Low | High - simple file check |
| Auth detection (API key) | ~20 | Low | Low | High - env var check |
| Auth detection (Local) | ~40 | Medium | Medium | Medium - URL rewriting + validation |
| Compose generation | ~150 | Medium | Medium | Medium - YAML marshaling is well-understood |
| Compose merging | ~100 | High | High | Low - type conflicts, edge cases (S9) |
| Asset extraction | ~40 | Medium | Medium | Medium - atomic operations needed (B1) |
| Container lifecycle | ~150 | Low | Low | High - wrapper around CLI commands |
| Label-based queries | ~80 | Low | Low | High - Docker SDK has good support |
| Domain allowlist merge | ~50 | Low | Medium | High - string operations, needs validation (S5) |

**Highest risk components**:
1. Bedrock auth detection (requires AWS CLI shelling, credential parsing)
2. Compose merging (type conflicts, nested structures)
3. Asset extraction (atomicity, concurrency)
4. Deep merge semantics (null handling, list edge cases)

**Overall feasibility**: Achievable within Phase 2 scope if BLOCKER issues addressed. Estimated 2,140 lines is reasonable given component breakdown. Biggest unknowns are Windows testing (B2) and Bedrock pre-resolution (S7).

---

## Summary

**BLOCKER count**: 5
**SHOULD FIX count**: 10
**NICE TO HAVE count**: 10

**Critical path to unblock**:
1. B3: Define step enumeration strategy for Phase 2 (hardcode "interactive" mode)
2. B1: Specify atomic asset extraction with concurrency safety
3. B2: Add Windows CI runner, test path handling
4. B4: Document compose validation latency decision, add opt-out
5. B5: Make local provider validation conditional on usage

**Highest value SHOULD FIX items**:
- S7: Implement Bedrock credential pre-resolution (aligns with reference implementation)
- S2: Document deep merge semantics for null and empty values
- S8: Specify git worktree detection algorithm
- S10: Add Docker Compose version check

**Overall assessment**: Plan is well-structured with strong traceability between requirements and architecture. Main gaps are in error recovery, edge case handling, and Windows compatibility. The 5 BLOCKER issues must be resolved before implementation; the 10 SHOULD FIX issues would significantly improve quality and robustness. Implementation is feasible if blockers addressed and highest-risk components (Bedrock auth, compose merging) given extra testing attention.
