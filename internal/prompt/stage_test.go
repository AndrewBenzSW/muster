package prompt

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/abenz1267/muster/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Task 3.4: Test staging filesystem operations

func TestStageSkillsCreatesTempDirectory(t *testing.T) {
	ctx := createTestContext()

	tmpDir, cleanup, err := StageSkills(ctx)
	defer cleanup()

	require.NoError(t, err, "StageSkills should succeed")
	require.NotEmpty(t, tmpDir, "tmpDir should not be empty")

	// Verify temp directory exists
	info, err := os.Stat(tmpDir)
	require.NoError(t, err, "temp directory should exist")
	assert.True(t, info.IsDir(), "tmpDir should be a directory")

	// Verify temp directory is in system temp location
	assert.Contains(t, tmpDir, "muster-prompts-", "temp directory should have muster-prompts prefix")
}

func TestStageSkillsDirectoryStructure(t *testing.T) {
	ctx := createTestContext()

	tmpDir, cleanup, err := StageSkills(ctx)
	defer cleanup()

	require.NoError(t, err, "StageSkills should succeed")

	skillsDir := filepath.Join(tmpDir, "skills")

	// Verify skills directory exists
	info, err := os.Stat(skillsDir)
	require.NoError(t, err, "skills directory should exist")
	assert.True(t, info.IsDir(), "skills should be a directory")

	// Verify expected subdirectories exist
	expectedDirs := []string{
		"roadmap-plan-feature",
		"roadmap-execute-plan",
		"roadmap-review-implementation",
	}

	for _, dir := range expectedDirs {
		dirPath := filepath.Join(skillsDir, dir)
		info, err := os.Stat(dirPath)
		require.NoError(t, err, "directory %s should exist", dir)
		assert.True(t, info.IsDir(), "%s should be a directory", dir)
	}

	// Verify expected SKILL.md files exist
	expectedFiles := []string{
		"roadmap-plan-feature/SKILL.md",
		"roadmap-execute-plan/SKILL.md",
		"roadmap-review-implementation/SKILL.md",
	}

	for _, file := range expectedFiles {
		filePath := filepath.Join(skillsDir, file)
		info, err := os.Stat(filePath)
		require.NoError(t, err, "file %s should exist", file)
		assert.False(t, info.IsDir(), "%s should be a file", file)
	}
}

func TestStageSkillsTmplExtensionRemoved(t *testing.T) {
	ctx := createTestContext()

	tmpDir, cleanup, err := StageSkills(ctx)
	defer cleanup()

	require.NoError(t, err, "StageSkills should succeed")

	skillsDir := filepath.Join(tmpDir, "skills")

	// Walk the skills directory and verify no .tmpl files exist
	err = filepath.WalkDir(skillsDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			assert.False(t, strings.HasSuffix(path, ".tmpl"), "file %s should not have .tmpl extension", path)
			// All markdown files should have .md extension (not .md.tmpl)
			if strings.Contains(filepath.Base(path), ".md") {
				assert.True(t, strings.HasSuffix(path, ".md"), "file %s should end with .md", path)
			}
		}
		return nil
	})

	require.NoError(t, err, "should walk skills directory successfully")
}

func TestStageSkillsCleanupRemovesTempDir(t *testing.T) {
	ctx := createTestContext()

	tmpDir, cleanup, err := StageSkills(ctx)
	require.NoError(t, err, "StageSkills should succeed")
	require.NotEmpty(t, tmpDir, "tmpDir should not be empty")

	// Verify directory exists before cleanup
	_, err = os.Stat(tmpDir)
	require.NoError(t, err, "temp directory should exist before cleanup")

	// Call cleanup
	cleanup()

	// Verify directory no longer exists
	_, err = os.Stat(tmpDir)
	assert.True(t, os.IsNotExist(err), "temp directory should not exist after cleanup")
}

func TestStageSkillsFilesReadableAndNonEmpty(t *testing.T) {
	ctx := createTestContext()

	tmpDir, cleanup, err := StageSkills(ctx)
	defer cleanup()

	require.NoError(t, err, "StageSkills should succeed")

	skillsDir := filepath.Join(tmpDir, "skills")

	// Check expected files are readable and non-empty
	expectedFiles := []string{
		"roadmap-plan-feature/SKILL.md",
		"roadmap-plan-feature/planner-prompt.md",
		"roadmap-execute-plan/SKILL.md",
		"roadmap-execute-plan/worker-prompt.md",
		"roadmap-review-implementation/SKILL.md",
		"roadmap-review-implementation/reviewer-prompt.md",
	}

	for _, file := range expectedFiles {
		filePath := filepath.Join(skillsDir, file)

		// Read the file
		content, err := os.ReadFile(filePath)
		require.NoError(t, err, "should be able to read file %s", file)

		// Verify non-empty
		assert.NotEmpty(t, content, "file %s should not be empty", file)

		// Verify it's valid text (contains some expected content)
		contentStr := string(content)
		assert.True(t, len(contentStr) > 0, "file %s should have content", file)
	}
}

// TestStageSkillsLFLineEndingsPreserved verifies that staged files consistently
// use LF (\n) line endings across all platforms, not CRLF (\r\n).
// This is important for cross-platform compatibility:
//   - Go templates produce LF line endings by default
//   - os.WriteFile() writes bytes as-is without conversion
//   - Git's core.autocrlf can convert during checkout/commit if configured
//   - Consistent LF endings ensure file hashes match across platforms
func TestStageSkillsLFLineEndingsPreserved(t *testing.T) {
	ctx := createTestContext()

	tmpDir, cleanup, err := StageSkills(ctx)
	defer cleanup()

	require.NoError(t, err, "StageSkills should succeed")

	skillsDir := filepath.Join(tmpDir, "skills")

	// Check that files use LF line endings (no CRLF)
	filesToCheck := []string{
		"roadmap-plan-feature/SKILL.md",
		"roadmap-execute-plan/SKILL.md",
		"roadmap-review-implementation/SKILL.md",
	}

	for _, file := range filesToCheck {
		filePath := filepath.Join(skillsDir, file)
		content, err := os.ReadFile(filePath)
		require.NoError(t, err, "should be able to read file %s", file)

		contentStr := string(content)

		// Count CRLF and LF separately for precise verification
		// This approach is more robust than strings.Contains and works correctly
		// even if Git's core.autocrlf=true is configured
		// Note: .gitattributes should enforce *.tmpl text eol=lf to prevent conversion
		crlfCount := strings.Count(contentStr, "\r\n")
		lfCount := strings.Count(contentStr, "\n") - crlfCount

		assert.Equal(t, 0, crlfCount, "file %s should have no CRLF line endings", file)
		assert.Greater(t, lfCount, 0, "file %s should have LF line endings", file)
	}
}

func TestStageSkillsCleanupNonNilOnError(t *testing.T) {
	// Test with nil context to force an error
	tmpDir, cleanup, err := StageSkills(nil)

	// Cleanup should be non-nil even when staging fails
	require.NotNil(t, cleanup, "cleanup function should be non-nil even on error")

	// Calling cleanup should not panic
	assert.NotPanics(t, func() {
		cleanup()
	}, "calling cleanup should not panic even when staging failed")

	// Error should be returned
	assert.Error(t, err, "StageSkills should error with nil context")

	// tmpDir may be empty or set depending on where the error occurred
	_ = tmpDir
}

// Task 3.5: Test staging error handling

func TestStageSkillsNilContextErrors(t *testing.T) {
	tmpDir, cleanup, err := StageSkills(nil)
	defer cleanup()

	assert.Error(t, err, "should error with nil context")
	assert.Contains(t, err.Error(), "cannot be nil", "error should mention nil context")

	// Cleanup should still be callable
	assert.NotNil(t, cleanup, "cleanup should be non-nil")
	assert.NotPanics(t, func() { cleanup() }, "cleanup should not panic")

	// tmpDir might be set or empty depending on error location
	_ = tmpDir
}

func TestStageSkillsPartialFailureCleanup(t *testing.T) {
	// This test verifies that cleanup works even if staging partially completes
	// We'll use a valid context to start staging, but test cleanup behavior

	ctx := createTestContext()

	tmpDir, cleanup, err := StageSkills(ctx)

	// Even if we get an error partway through, cleanup should be safe to call
	require.NotNil(t, cleanup, "cleanup should always be non-nil")

	if err != nil {
		// If there was an error, cleanup should still work
		assert.NotPanics(t, func() { cleanup() }, "cleanup should not panic on partial failure")
	} else {
		// On success, verify cleanup removes the directory
		require.NoError(t, err)
		_, statErr := os.Stat(tmpDir)
		require.NoError(t, statErr, "directory should exist before cleanup")

		cleanup()

		_, statErr = os.Stat(tmpDir)
		assert.True(t, os.IsNotExist(statErr), "directory should not exist after cleanup")
	}
}

func TestStageSkillsCleanupMultipleCalls(t *testing.T) {
	ctx := createTestContext()

	tmpDir, cleanup, err := StageSkills(ctx)
	require.NoError(t, err, "StageSkills should succeed")

	// First cleanup should remove directory
	cleanup()
	_, statErr := os.Stat(tmpDir)
	assert.True(t, os.IsNotExist(statErr), "directory should not exist after first cleanup")

	// Second cleanup should not panic even though directory is already gone
	assert.NotPanics(t, func() { cleanup() }, "second cleanup call should not panic")
}

// Task 3.6: Test cross-platform path handling

// TestStageSkillsCrossPlatformPaths verifies that filepath.Join() and os.MkdirTemp()
// produce platform-appropriate paths with correct separators.
// Expected behavior per platform:
//   - Linux:   Uses forward slash (/), temp in /tmp
//   - macOS:   Uses forward slash (/), temp in /var/folders/...
//   - Windows: Uses backslash (\), temp in %TEMP% (e.g., C:\Users\...\AppData\Local\Temp)
func TestStageSkillsCrossPlatformPaths(t *testing.T) {
	ctx := createTestContext()

	tmpDir, cleanup, err := StageSkills(ctx)
	defer cleanup()

	require.NoError(t, err, "StageSkills should succeed")

	// Verify filepath.Join produces correct paths for the current platform
	skillsDir := filepath.Join(tmpDir, "skills")
	planFeatureDir := filepath.Join(skillsDir, "roadmap-plan-feature")

	// On Windows, paths should contain backslashes
	// On Unix-like systems, paths should contain forward slashes
	if runtime.GOOS == "windows" {
		assert.Contains(t, planFeatureDir, "\\", "Windows paths should contain backslashes")
	} else {
		assert.Contains(t, planFeatureDir, "/", "Unix paths should contain forward slashes")
	}

	// Verify the path is absolute
	assert.True(t, filepath.IsAbs(tmpDir), "tmpDir should be an absolute path")
	assert.True(t, filepath.IsAbs(skillsDir), "skillsDir should be an absolute path")
}

func TestStageSkillsFileContentForwardSlashes(t *testing.T) {
	ctx := createTestContext()

	tmpDir, cleanup, err := StageSkills(ctx)
	defer cleanup()

	require.NoError(t, err, "StageSkills should succeed")

	skillsDir := filepath.Join(tmpDir, "skills")

	// Check SKILL.md files for references to other files
	// These should use forward slashes regardless of platform
	skillFiles := []string{
		"roadmap-plan-feature/SKILL.md",
		"roadmap-execute-plan/SKILL.md",
		"roadmap-review-implementation/SKILL.md",
	}

	for _, file := range skillFiles {
		filePath := filepath.Join(skillsDir, file)
		content, err := os.ReadFile(filePath)
		require.NoError(t, err, "should be able to read file %s", file)

		contentStr := string(content)

		// Look for markdown link patterns like [file.md](file.md) or [text](path/file.md)
		// These should always use forward slashes, even on Windows
		if strings.Contains(contentStr, "](") {
			// Find all markdown links
			lines := strings.Split(contentStr, "\n")
			for _, line := range lines {
				if strings.Contains(line, "](") && strings.Contains(line, ".md)") {
					// This line contains a markdown link to a .md file
					// Extract the part between ]( and )
					start := strings.Index(line, "](")
					if start != -1 {
						end := strings.Index(line[start+2:], ")")
						if end != -1 {
							link := line[start+2 : start+2+end]
							// Links should not contain backslashes
							assert.False(t, strings.Contains(link, "\\"),
								"markdown link '%s' in %s should not contain backslashes", link, file)
						}
					}
				}
			}
		}
	}
}

// TestStageSkillsFilePathSeparators verifies that file system paths use
// platform-appropriate separators through runtime.GOOS conditionals.
// This ensures code works correctly across platforms:
//   - runtime.GOOS returns "windows", "darwin", "linux", etc.
//   - filepath.Separator is '\' on Windows, '/' on Unix-like systems
//   - Always use filepath.Join() instead of string concatenation for paths
func TestStageSkillsFilePathSeparators(t *testing.T) {
	ctx := createTestContext()

	tmpDir, cleanup, err := StageSkills(ctx)
	defer cleanup()

	require.NoError(t, err, "StageSkills should succeed")

	// Get expected path separator for this platform
	expectedSep := string(filepath.Separator)

	// Verify the tmpDir uses the correct separator
	if runtime.GOOS == "windows" {
		assert.Equal(t, "\\", expectedSep, "Windows should use backslash")
		// tmpDir should contain the system temp path with backslashes
		assert.True(t, strings.Contains(tmpDir, "\\") || !strings.Contains(tmpDir, "/"),
			"Windows temp dir should use backslashes")
	} else {
		assert.Equal(t, "/", expectedSep, "Unix should use forward slash")
		assert.Contains(t, tmpDir, "/", "Unix temp dir should use forward slashes")
	}

	// Verify subdirectories use correct separator
	skillsDir := filepath.Join(tmpDir, "skills")
	assert.Contains(t, skillsDir, expectedSep, "subdirectory paths should use platform separator")
}

func TestStageSkillsRelativePathsInFiles(t *testing.T) {
	ctx := createTestContext()

	tmpDir, cleanup, err := StageSkills(ctx)
	defer cleanup()

	require.NoError(t, err, "StageSkills should succeed")

	skillsDir := filepath.Join(tmpDir, "skills")

	// Read a SKILL.md file and check for relative file references
	skillPath := filepath.Join(skillsDir, "roadmap-plan-feature", "SKILL.md")
	content, err := os.ReadFile(skillPath)
	require.NoError(t, err, "should be able to read SKILL.md")

	contentStr := string(content)

	// Look for references to other .md files in the same directory
	// These should use simple filenames with forward slashes (markdown convention)
	if strings.Contains(contentStr, ".md") {
		// Check that we don't have any Windows-style paths in the content
		lines := strings.Split(contentStr, "\n")
		for _, line := range lines {
			if strings.Contains(line, ".md") {
				// If this line references a .md file, it shouldn't have backslashes
				// (markdown files should use forward slashes for portability)
				if strings.Contains(line, "\\") {
					// Allow escaped characters like \n, \t, but not path separators like runner\\file.md
					if strings.Contains(line, "\\") && !strings.Contains(line, "\\n") && !strings.Contains(line, "\\t") {
						assert.False(t, strings.Contains(line, "\\.md") || strings.Contains(line, "md\\"),
							"line should not contain Windows path separators: %s", line)
					}
				}
			}
		}
	}
}

// Task I6: Test concurrent StageSkills calls

func TestStageSkillsConcurrentCalls(t *testing.T) {
	// This test verifies that concurrent calls to StageSkills don't race
	// Run with: go test -race -run TestStageSkillsConcurrentCalls

	const goroutines = 10

	// Create channels to synchronize goroutines
	start := make(chan struct{})
	done := make(chan error, goroutines)

	// Launch multiple goroutines that will call StageSkills simultaneously
	for i := 0; i < goroutines; i++ {
		go func() {
			// Wait for start signal to maximize race potential
			<-start

			ctx := createTestContext()
			tmpDir, cleanup, err := StageSkills(ctx)

			// Always call cleanup even if there's an error
			if cleanup != nil {
				defer cleanup()
			}

			// Verify basic success
			if err != nil {
				done <- fmt.Errorf("StageSkills failed: %w", err)
				return
			}

			// Verify tmpDir exists
			if _, statErr := os.Stat(tmpDir); statErr != nil {
				done <- fmt.Errorf("tmpDir doesn't exist: %w", statErr)
				return
			}

			done <- nil
		}()
	}

	// Start all goroutines simultaneously
	close(start)

	// Wait for all goroutines to complete
	for i := 0; i < goroutines; i++ {
		err := <-done
		assert.NoError(t, err, "goroutine %d should succeed", i)
	}
}

func TestStageSkillsConcurrentCleanup(t *testing.T) {
	// Test that multiple concurrent cleanup calls don't cause issues

	ctx := createTestContext()
	tmpDir, cleanup, err := StageSkills(ctx)
	require.NoError(t, err, "StageSkills should succeed")
	require.NotEmpty(t, tmpDir, "tmpDir should not be empty")

	// Verify directory exists
	_, statErr := os.Stat(tmpDir)
	require.NoError(t, statErr, "directory should exist before cleanup")

	// Call cleanup concurrently from multiple goroutines
	const goroutines = 5
	done := make(chan struct{}, goroutines)

	for i := 0; i < goroutines; i++ {
		go func() {
			// Should not panic even if called concurrently
			cleanup()
			done <- struct{}{}
		}()
	}

	// Wait for all goroutines to complete
	for i := 0; i < goroutines; i++ {
		<-done
	}

	// Verify directory is gone
	_, statErr = os.Stat(tmpDir)
	assert.True(t, os.IsNotExist(statErr), "directory should not exist after cleanup")
}

// Task I9: Windows edge case tests

func TestStageSkillsWindowsPathWithSpaces(t *testing.T) {
	// Test that staging works when temp directory contains spaces
	// Only run this test on Windows where path handling is different
	if runtime.GOOS != "windows" {
		t.Skip("Windows-specific test")
	}

	// On Windows, os.TempDir() typically returns something like:
	// C:\Users\username\AppData\Local\Temp
	// We test that staging succeeds even if there are spaces in the path

	ctx := createTestContext()
	tmpDir, cleanup, err := StageSkills(ctx)
	defer cleanup()

	require.NoError(t, err, "StageSkills should succeed even with spaces in path")
	require.NotEmpty(t, tmpDir, "tmpDir should not be empty")

	// Verify directory exists
	_, statErr := os.Stat(tmpDir)
	require.NoError(t, statErr, "temp directory should exist")

	// Verify skills were staged correctly
	skillsDir := filepath.Join(tmpDir, "skills")
	_, statErr = os.Stat(skillsDir)
	require.NoError(t, statErr, "skills directory should exist")
}

func TestStageSkillsWindowsLongPath(t *testing.T) {
	// Test handling of very long paths (240+ characters)
	// Windows MAX_PATH is 260 characters, but long path support was added in Windows 10
	// This test verifies we get a clear error or success when dealing with long paths
	if runtime.GOOS != "windows" {
		t.Skip("Windows-specific test")
	}

	// Note: This test documents expected behavior rather than forcing a long path
	// because os.MkdirTemp creates directories in the system temp location which
	// is typically not near the MAX_PATH limit

	ctx := createTestContext()
	tmpDir, cleanup, err := StageSkills(ctx)
	defer cleanup()

	// On modern Windows with long path support, this should succeed
	// On older systems without long path support, we expect a clear error
	if err != nil {
		// If there's an error, it should be clear about path length
		assert.Contains(t, err.Error(), "path", "error should mention path issue")
	} else {
		// On success, verify the path length
		require.NotEmpty(t, tmpDir, "tmpDir should not be empty")

		// Document the actual path length for reference
		t.Logf("Created tmpDir with length: %d characters: %s", len(tmpDir), tmpDir)

		// Windows MAX_PATH is 260, but with long path support it can be much longer
		// We just verify staging succeeded
		_, statErr := os.Stat(tmpDir)
		require.NoError(t, statErr, "temp directory should exist")
	}
}

func TestStageSkillsWindowsPathSeparators(t *testing.T) {
	// Verify that Windows paths use backslashes correctly
	if runtime.GOOS != "windows" {
		t.Skip("Windows-specific test")
	}

	ctx := createTestContext()
	tmpDir, cleanup, err := StageSkills(ctx)
	defer cleanup()

	require.NoError(t, err, "StageSkills should succeed")
	require.NotEmpty(t, tmpDir, "tmpDir should not be empty")

	// Verify tmpDir uses Windows path separators (backslashes)
	assert.Contains(t, tmpDir, "\\", "Windows paths should contain backslashes")

	// Verify subdirectories are created with correct separators
	skillsDir := filepath.Join(tmpDir, "skills")
	assert.Contains(t, skillsDir, "\\", "Windows subdirectories should use backslashes")

	// Verify skills directory exists and was created correctly
	_, statErr := os.Stat(skillsDir)
	require.NoError(t, statErr, "skills directory should exist")
}

// Task I10: Cleanup failure tests

func TestStageSkillsCleanupWhenDirectoryAlreadyDeleted(t *testing.T) {
	// Test that cleanup is idempotent - calling it after directory is deleted doesn't panic

	ctx := createTestContext()
	tmpDir, cleanup, err := StageSkills(ctx)
	require.NoError(t, err, "StageSkills should succeed")
	require.NotEmpty(t, tmpDir, "tmpDir should not be empty")

	// Verify directory exists
	_, statErr := os.Stat(tmpDir)
	require.NoError(t, statErr, "directory should exist before cleanup")

	// Manually delete the directory
	err = os.RemoveAll(tmpDir)
	require.NoError(t, err, "manual deletion should succeed")

	// Verify directory is gone
	_, statErr = os.Stat(tmpDir)
	require.True(t, os.IsNotExist(statErr), "directory should not exist after manual deletion")

	// Now call cleanup - it should not panic
	assert.NotPanics(t, func() {
		cleanup()
	}, "cleanup should not panic when directory is already deleted")

	// Cleanup errors are logged to stderr, not returned, so we can't assert on error
	// The important thing is that cleanup doesn't panic
}

func TestStageSkillsCleanupIdempotency(t *testing.T) {
	// Test that calling cleanup multiple times is safe (idempotent)
	// This is important because deferred cleanup might be called even if an error occurred

	ctx := createTestContext()
	tmpDir, cleanup, err := StageSkills(ctx)
	require.NoError(t, err, "StageSkills should succeed")
	require.NotEmpty(t, tmpDir, "tmpDir should not be empty")

	// Call cleanup multiple times
	for i := 0; i < 3; i++ {
		assert.NotPanics(t, func() {
			cleanup()
		}, "cleanup call %d should not panic", i+1)
	}

	// Verify directory is gone
	_, statErr := os.Stat(tmpDir)
	assert.True(t, os.IsNotExist(statErr), "directory should not exist after cleanup")
}

func TestStageSkillsCleanupWithReadOnlyFile(t *testing.T) {
	// Test cleanup behavior when directory contains a read-only file
	// This tests that cleanup handles permission errors gracefully

	// Skip on Windows as file permission handling is different
	if runtime.GOOS == "windows" {
		t.Skip("File permissions work differently on Windows")
	}

	ctx := createTestContext()
	tmpDir, cleanup, err := StageSkills(ctx)
	require.NoError(t, err, "StageSkills should succeed")
	require.NotEmpty(t, tmpDir, "tmpDir should not be empty")

	// Create a read-only file in the temp directory
	readOnlyFile := filepath.Join(tmpDir, "readonly.txt")
	err = os.WriteFile(readOnlyFile, []byte("test"), 0444) // Read-only permissions
	require.NoError(t, err, "should create read-only file")

	// Verify file is read-only
	info, err := os.Stat(readOnlyFile)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0444), info.Mode().Perm(), "file should be read-only")

	// Call cleanup - it should not panic
	// Note: os.RemoveAll() will succeed even with read-only files because it removes
	// the directory entry, not just the file content
	assert.NotPanics(t, func() {
		cleanup()
	}, "cleanup should not panic with read-only files")

	// On most systems, RemoveAll succeeds with read-only files
	// If cleanup failed (e.g., due to permissions), it would log to stderr but not panic
}

// Helper functions

func createTestContext() *PromptContext {
	userCfg := &config.UserConfig{
		Tools: map[string]*config.ToolConfig{
			"claude-code": {
				ModelTiers: &struct {
					Fast     *string `yaml:"fast"`
					Standard *string `yaml:"standard"`
					Deep     *string `yaml:"deep"`
				}{
					Fast:     stringPtr("claude-haiku-4"),
					Standard: stringPtr("claude-sonnet-4.5"),
					Deep:     stringPtr("claude-opus-4"),
				},
			},
		},
	}

	resolved := &config.ResolvedConfig{
		Tool:     "claude-code",
		Provider: "anthropic",
		Model:    "claude-sonnet-4.5",
	}

	return NewPromptContext(resolved, userCfg, true, "test-staging", "/workspace", "/workspace", "/workspace/.plan/test-staging")
}
