package prompt

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Sentinel errors for prompt package
var (
	// ErrTemplateRender indicates a template rendering error
	ErrTemplateRender = errors.New("template render error")
)

// cleanupStalePrompts scans the temp directory for stale prompt staging directories
// and removes them. A directory is considered stale if it matches the pattern
// "muster-prompts-*" and has not been modified in more than 24 hours.
// Errors during individual directory removal are logged but do not cause the
// function to fail. Only complete scan failures return an error.
//
// Race condition risk: If multiple goroutines call StageSkills concurrently,
// they may race on cleanup of stale directories. This is generally safe as
// os.RemoveAll is idempotent, but cleanup errors may be logged multiple times.
//
// Platform-specific behavior:
// os.TempDir() returns platform-specific temporary directories:
//   - Linux:   $TMPDIR (typically /tmp)
//   - macOS:   $TMPDIR (typically /var/folders/...)
//   - Windows: %TEMP% or %TMP% (typically C:\Users\username\AppData\Local\Temp)
//
// filepath.Join() automatically uses the correct path separator for the platform:
//   - Unix-like systems: forward slash (/)
//   - Windows: backslash (\)
func cleanupStalePrompts() error {
	// os.TempDir() returns platform-specific temp directory
	tempDir := os.TempDir()
	entries, err := os.ReadDir(tempDir)
	if err != nil {
		return fmt.Errorf("failed to scan temp directory: %w", err)
	}

	cutoff := time.Now().Add(-24 * time.Hour)

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		name := entry.Name()
		if !strings.HasPrefix(name, "muster-prompts-") {
			continue
		}

		// filepath.Join() uses platform-appropriate path separators
		fullPath := filepath.Join(tempDir, name)
		info, err := entry.Info()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to get info for %s: %v\n", fullPath, err)
			continue
		}

		if info.ModTime().Before(cutoff) {
			if err := os.RemoveAll(fullPath); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to remove stale directory %s: %v\n", fullPath, err)
			} else {
				fmt.Fprintf(os.Stderr, "Removed stale prompt directory: %s\n", fullPath)
			}
		}
	}

	return nil
}

// StageSkills stages all skill templates to a temporary directory for use by
// the roadmap execution system. It renders each .md.tmpl file found in the
// embedded prompts filesystem and writes the results to a temporary directory
// structure that mirrors the source layout.
//
// The function returns:
//   - tmpDir: the absolute path to the temporary directory containing staged skills
//   - cleanup: a function that removes the temporary directory (always non-nil)
//   - err: any error encountered during staging
//
// The cleanup function is always returned, even if staging fails. This allows
// callers to safely defer cleanup() immediately after the call. If an error
// occurs before tmpDir is created, cleanup is a no-op.
//
// Directory structure:
//   - tmpDir/skills/roadmap-plan-feature/SKILL.md
//   - tmpDir/skills/roadmap-plan-feature/planner-prompt.md
//   - tmpDir/skills/roadmap-execute-plan/SKILL.md
//   - tmpDir/skills/roadmap-execute-plan/worker-prompt.md
//   - tmpDir/skills/roadmap-review-implementation/SKILL.md
//   - tmpDir/skills/roadmap-review-implementation/reviewer-prompt.md
//   - etc.
func StageSkills(ctx *PromptContext) (tmpDir string, cleanup func(), err error) {
	// Default cleanup is a no-op
	cleanup = func() {}

	// Early validation: context cannot be nil
	if ctx == nil {
		return "", cleanup, fmt.Errorf("StageSkills: context cannot be nil")
	}

	// Clean up stale prompt directories
	if err := cleanupStalePrompts(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to cleanup stale prompts: %v\n", err)
	}

	// Create temp directory
	tmpDir, err = os.MkdirTemp("", "muster-prompts-")
	if err != nil {
		return "", cleanup, fmt.Errorf("failed to create temp directory: %w", err)
	}

	// Update cleanup to remove the temp directory
	cleanup = func() {
		if err := os.RemoveAll(tmpDir); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to cleanup temp directory %s: %v\n", tmpDir, err)
		}
	}

	// Create skills directory
	skillsDir := filepath.Join(tmpDir, "skills")
	if err := os.MkdirAll(skillsDir, 0755); err != nil { //nolint:gosec // G301: Standard directory permissions for skill staging
		return tmpDir, cleanup, fmt.Errorf("failed to create skills directory: %w", err)
	}

	// Walk the embedded prompts filesystem
	err = fs.WalkDir(Prompts, "prompts", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Skip directories
		if d.IsDir() {
			return nil
		}

		// Only process .md.tmpl files
		if !strings.HasSuffix(path, ".md.tmpl") {
			return nil
		}

		// Render the template
		rendered, err := RenderTemplate(path, ctx)
		if err != nil {
			return fmt.Errorf("failed to render template %s: %w", path, err)
		}

		// Compute output path
		// Input: prompts/plan-feature/SKILL.md.tmpl
		// Output: tmpDir/skills/roadmap-plan-feature/SKILL.md
		relPath, err := filepath.Rel("prompts", path)
		if err != nil {
			return fmt.Errorf("failed to compute relative path for %s: %w", path, err)
		}

		// Split into directory and filename
		dir, file := filepath.Split(relPath)

		// Transform directory name: plan-feature -> roadmap-plan-feature
		dir = strings.TrimSuffix(dir, string(filepath.Separator))
		if dir != "" && dir != "test" {
			dir = "roadmap-" + dir
		}

		// Strip .tmpl extension from filename
		file = strings.TrimSuffix(file, ".tmpl")

		// Construct full output path
		var outPath string
		if dir != "" && dir != "test" {
			outPath = filepath.Join(skillsDir, dir, file)
		} else {
			// Skip test files
			if dir == "test" {
				return nil
			}
			outPath = filepath.Join(skillsDir, file)
		}

		// Create output directory
		outDir := filepath.Dir(outPath)
		if err := os.MkdirAll(outDir, 0755); err != nil { //nolint:gosec // G301: Standard directory permissions for skill staging
			return fmt.Errorf("failed to create output directory %s: %w", outDir, err)
		}

		// Write rendered template
		// Note: Line ending handling is cross-platform compatible:
		// - os.WriteFile() writes bytes as-is without line ending conversion
		// - Template rendering produces LF line endings (\n) consistently
		// - Git's core.autocrlf setting handles conversions during checkout/commit if needed
		// - Tests verify LF endings are preserved (see TestStageSkillsLFLineEndingsPreserved)
		// This ensures consistent behavior across Linux, macOS, and Windows
		if err := os.WriteFile(outPath, []byte(rendered), 0644); err != nil { //nolint:gosec // G306: Standard file permissions for skill files
			return fmt.Errorf("failed to write file %s: %w", outPath, err)
		}

		return nil
	})

	if err != nil {
		return tmpDir, cleanup, fmt.Errorf("failed to stage skills: %w", err)
	}

	// Verify directory structure
	expectedSkills := []string{
		"roadmap-plan-feature/SKILL.md",
		"roadmap-execute-plan/SKILL.md",
		"roadmap-review-implementation/SKILL.md",
	}

	for _, skill := range expectedSkills {
		skillPath := filepath.Join(skillsDir, skill)
		if _, err := os.Stat(skillPath); err != nil {
			return tmpDir, cleanup, fmt.Errorf("verification failed: expected file not found: %s", skillPath)
		}
	}

	return tmpDir, cleanup, nil
}
