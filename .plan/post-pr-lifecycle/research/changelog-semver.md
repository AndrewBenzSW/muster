# CHANGELOG & Semver

*Researched: 2026-03-20*
*Scope: CHANGELOG format and version management patterns*

---

## Key Findings

1. **CHANGELOG.md exists and follows Keep a Changelog 1.0.0 format** with an explicit `[Unreleased]` section at the top. The project declares adherence to Semantic Versioning 2.0.0.
2. **No `internal/version/` package exists yet** -- this is a greenfield implementation. The design doc (`docs/design.md:217,504`) calls for `internal/version/` with CHANGELOG parsing, semver bump, and release section promotion.
3. **No semver library is currently in `go.mod`** -- the design doc recommends `github.com/Masterminds/semver/v3` (`docs/design.md:367`) but it hasn't been added yet.
4. **Version is injected at build time via ldflags** -- `cmd/version.go:12-14` defines `version`, `commit`, `date` as package-level vars set by the Makefile (`Makefile:33`) using `git describe --tags --always --dirty`.
5. **Tags follow `v{major}.{minor}.{patch}` format** -- existing tags: `v0.2.0`, `v0.3.0`, `v0.4.0`, `v0.4.1`. GoReleaser triggers on `v*` tag push (`release.yml:6`).

## Detailed Analysis

### CHANGELOG Format

The CHANGELOG (`CHANGELOG.md`) follows Keep a Changelog strictly:

- **Header**: `# Changelog` (line 1)
- **Preamble**: Standard Keep a Changelog attribution text (lines 3-6)
- **Sections**: `## [Unreleased]`, then `## [X.Y.Z] - YYYY-MM-DD` in descending order
- **Categories used**: `### Added`, `### Changed` (no `### Fixed`, `### Removed`, `### Deprecated`, `### Security` yet, but these are valid per the spec)
- **Entry format**: `- **Feature Name**: Description` -- bold feature name prefix with colon-separated description
- **4 releases documented**: 0.1.0, 0.2.0, 0.3.0, 0.4.0
- **`[Unreleased]` section is currently empty** (line 8-9, just the header with no entries)

Example entry format:
```markdown
## [0.4.0] - 2026-03-19

### Added

- **Mock AI Tool Infrastructure**: Centralized testing mock in `internal/testutil/`...

### Changed

- **AI Invocation Function**: Exported `ai.InvokeAI` as reassignable package variable...
```

Note: The CHANGELOG does not include link references at the bottom (e.g., `[0.4.0]: https://github.com/...`). This is optional per Keep a Changelog but common in mature projects.

### Version Injection and Build Pipeline

- `Makefile:24`: `VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")`
- `Makefile:33`: LDFLAGS inject version into `cmd.version`, `cmd.commit`, `cmd.date`
- `cmd/version.go:11-15`: Three package-level string vars with defaults `"dev"`, `"none"`, `"unknown"`
- `.goreleaser.yml:25-27`: GoReleaser also injects `{{.Version}}`, `{{.Commit}}`, `{{.Date}}` via ldflags
- GoReleaser handles the actual binary release on tag push (`.github/workflows/release.yml`)

### Tag History

| Tag | Date |
|-----|------|
| v0.2.0 | 2026-03-18 |
| v0.3.0 | 2026-03-18 |
| v0.4.0 | 2026-03-19 |
| v0.4.1 | 2026-03-20 |

All tags are lightweight or annotated `v{MAJOR}.{MINOR}.{PATCH}` with no pre-release suffixes used yet.

### Design Doc Expectations for `internal/version/`

From `docs/design.md:504`:
> `internal/version/`: CHANGELOG parsing, semver bump, release section promotion

From `docs/design.md:397`:
> Version/changelog: Table-driven unit tests -- Semver bump, CHANGELOG section promotion, edge cases (pre-release, no changelog)

From `docs/design.md:527` (the `prepare` step of `muster in`):
> prepare: merge main, version bump, changelog, roadmap cleanup, commit

The `muster out` command (`docs/design.md:505`) is described as the post-PR lifecycle handler. However, version tagging appears to be more closely associated with the `prepare` step of `muster in`. The `out` command focuses on: monitor CI checks, push fixes, wait for merge, pull latest main, clean up worktree + roadmap entry. Version tagging may happen in `out` as part of cleanup (creating the tag after merge).

### Recommended Library

The design doc specifies `github.com/Masterminds/semver/v3` (`docs/design.md:367`). This is the standard Go semver library supporting:
- Parsing: `semver.NewVersion("1.2.3")`
- Comparison: `v1.LessThan(v2)`
- Increment: `v.IncMajor()`, `v.IncMinor()`, `v.IncPatch()`
- Constraints: `semver.NewConstraint(">= 1.0")`

Not yet in `go.mod` -- needs to be added.

### GoReleaser Integration

`.goreleaser.yml` generates its own changelog from git commits (lines 45-57) with conventional commit filtering (`feat:`, `fix:`, `docs:`, `test:`). This is separate from `CHANGELOG.md` and used only for GitHub Release notes. The two changelog mechanisms are independent.

## Recommendations

### CHANGELOG Parsing

The parser needs to handle:
1. **Extract `[Unreleased]` section content** -- everything between `## [Unreleased]` and the next `## [X.Y.Z]` header
2. **Promote `[Unreleased]` to a versioned section** -- replace `## [Unreleased]` content with `## [X.Y.Z] - YYYY-MM-DD` and insert a fresh empty `## [Unreleased]`
3. **Extract the latest version number** -- parse the first `## [X.Y.Z]` after `[Unreleased]` to determine the current version
4. **Handle missing CHANGELOG gracefully** -- the design doc calls for testing "no changelog" edge case

Regex for section headers: `^## \[(\d+\.\d+\.\d+|Unreleased)\]`

### Semver Bump Logic

Based on the CHANGELOG categories used:
- `### Added` or `### Changed` or `### Deprecated` or `### Removed` → **minor** bump
- `### Fixed` or `### Security` → **patch** bump
- An explicit breaking change marker (TBD) → **major** bump

Alternatively, the bump level could be:
- Explicitly specified via flag (`--patch`, `--minor`, `--major`)
- Inferred from CHANGELOG categories (as above)
- Determined by AI analysis of the changes

Given that `muster out` focuses on post-PR lifecycle and the `prepare` step in `muster in` handles the version bump, the simplest approach is to support explicit flags with optional category-based inference as a default.

### Version Tagging

The tag format must match what GoReleaser expects: `v{MAJOR}.{MINOR}.{PATCH}`. After the CHANGELOG is promoted and committed, create an annotated git tag:
```
git tag -a v0.5.0 -m "Release v0.5.0"
```

### Suggested `internal/version/` Package API

```go
// Parse CHANGELOG.md, return structured sections
func ParseChangelog(path string) (*Changelog, error)

// Get current version from latest tagged release section
func (c *Changelog) LatestVersion() (*semver.Version, error)

// Promote [Unreleased] to a versioned section with today's date
func (c *Changelog) Promote(version semver.Version) error

// Write the modified changelog back to disk
func (c *Changelog) Write(path string) error

// Determine bump type from changelog categories
func (c *Changelog) SuggestBump() (BumpType, error)

// Apply bump to a version
func Bump(v *semver.Version, bump BumpType) semver.Version
```

## Open Questions

1. **Where does version tagging actually happen -- `muster in` (prepare step) or `muster out`?**
   - The design doc mentions version bump in the `prepare` step of `muster in` (line 527), but `muster out` handles post-merge cleanup. Tagging logically happens after merge to main, which is `muster out` territory.
   - Likely answer: `muster in`'s `prepare` step does the CHANGELOG promotion and version bump commit. `muster out` creates the git tag after the PR merges to main. This separates concerns cleanly.

2. **Should bump level be explicit (flag) or inferred from CHANGELOG categories?**
   - Both is safest. Default to inference with `--patch`/`--minor`/`--major` override.
   - The design doc doesn't specify, so this needs a design decision.

3. **Should `muster out` handle projects without a CHANGELOG?**
   - Design doc says to test the "no changelog" edge case (line 397). Versioning should work without a CHANGELOG (just tag), but CHANGELOG promotion should be skippable.

4. **Pre-release version support (e.g., `v1.0.0-rc.1`)?**
   - No pre-release versions exist in the tag history. The design doc mentions pre-release as a test edge case (line 397) but doesn't specify behavior. Masterminds/semver handles this natively.

5. **Should link references be added to the bottom of CHANGELOG?**
   - The current CHANGELOG lacks them. Keep a Changelog recommends them. This is a nice-to-have but adds parsing complexity.

## References

- `CHANGELOG.md` -- Full changelog, Keep a Changelog format, 89 lines
- `cmd/version.go:11-15` -- Version variables set via ldflags
- `Makefile:23-33` -- Version extraction and ldflags injection
- `.goreleaser.yml` -- Release config, ldflags, snapshot naming
- `.github/workflows/release.yml` -- Tag-triggered release workflow
- `docs/design.md:217` -- Planned `internal/version/` directory
- `docs/design.md:367` -- Recommended `Masterminds/semver/v3` library
- `docs/design.md:397` -- Version/changelog test expectations
- `docs/design.md:501-508` -- Phase 4 deliverables including version package
- `docs/design.md:527` -- `prepare` step: version bump + changelog
- `go.mod` -- Current dependencies (no semver library yet)
- `.muster/roadmap.json:10-16` -- `post-pr-lifecycle` roadmap item
