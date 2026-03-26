package cmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/abenz1267/muster/internal/ai"
	"github.com/abenz1267/muster/internal/coding"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSyncCommand_MockAIFuzzyMatching tests sync with mock AI that returns fuzzy match results.
// Provides source and target roadmaps with unmatched items, mock returns match results JSON,
// verifies items are matched and updated correctly.
func TestSyncCommand_MockAIFuzzyMatching(t *testing.T) {
	// Create temp directory
	tmpDir := t.TempDir()

	// Create source roadmap with renamed item
	sourceContent := `{
  "items": [
    {
      "slug": "feature-new-name",
      "title": "Feature with New Name",
      "priority": "high",
      "status": "completed",
      "context": "This feature was renamed"
    }
  ]
}`
	sourcePath := filepath.Join(tmpDir, "source.json")
	require.NoError(t, os.WriteFile(sourcePath, []byte(sourceContent), 0644)) //nolint:gosec // G306: Test file permissions

	// Create target roadmap with old name
	targetContent := `{
  "items": [
    {
      "slug": "feature-old-name",
      "title": "Feature with Old Name",
      "priority": "medium",
      "status": "in_progress",
      "context": "This feature has an old name"
    }
  ]
}`
	musterDir := filepath.Join(tmpDir, ".muster")
	require.NoError(t, os.MkdirAll(musterDir, 0755)) //nolint:gosec // G301: Test directory permissions
	targetPath := filepath.Join(musterDir, "roadmap.json")
	require.NoError(t, os.WriteFile(targetPath, []byte(targetContent), 0644)) //nolint:gosec // G306: Test file permissions

	// Change to temp directory
	origDir, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(tmpDir))
	defer func() { _ = os.Chdir(origDir) }()

	// Save original factory and replace with mock
	originalFactory := codingToolFactory
	defer func() { codingToolFactory = originalFactory }()

	// Mock AI response: high-confidence match between the two items
	mockResponse := `[{
  "source_slug": "feature-new-name",
  "target_slug": "feature-old-name",
  "confidence": 0.85,
  "reason": "Strong title similarity and context overlap indicate these are the same feature"
}]`

	codingToolFactory = func() (coding.CodingTool, error) {
		return &mockSyncTool{response: mockResponse}, nil
	}

	// Create command instance with fresh flags
	cmd := &cobra.Command{
		Use:  "sync",
		RunE: syncCmd.RunE,
	}
	cmd.Flags().String("source", ".roadmap.json", "Source roadmap file path")
	cmd.Flags().String("target", ".muster/roadmap.json", "Target roadmap file path")
	cmd.Flags().Bool("yes", false, "Accept all AI matches without confirmation")
	cmd.Flags().Bool("dry-run", false, "Preview changes without saving")
	cmd.Flags().Bool("delete", false, "Delete target items not matched by any source item")
	cmd.Flags().Bool("verbose", false, "Enable verbose output")

	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)

	// Execute sync
	cmd.SetArgs([]string{"--source", sourcePath, "--target", targetPath})
	err = cmd.Execute()
	assert.NoError(t, err, "command should execute without error")

	// Verify target file was updated with source values
	afterContent, err := os.ReadFile(targetPath) //nolint:gosec // G304: Test file reading
	require.NoError(t, err)
	assert.Contains(t, string(afterContent), "Feature with New Name", "target should have updated title from source")
	assert.Contains(t, string(afterContent), "completed", "target should have updated status from source")
	assert.Contains(t, string(afterContent), "This feature was renamed", "target should have updated context from source")
	assert.Contains(t, string(afterContent), "feature-old-name", "target should keep original slug")
	assert.NotContains(t, string(afterContent), "feature-new-name", "target should not have source slug (fuzzy match preserves target slug)")

	// Verify output shows 1 updated item (fuzzy matched)
	output := buf.String()
	assert.Contains(t, output, "Updated: 1", "output should show 1 updated item")
	assert.Contains(t, output, "Added: 0", "output should show 0 added items")
}

// TestSyncCommand_MockAIFailureFallback tests that sync continues with exact matches when AI fails.
// Mock returns error, verify sync continues with exact matches only and adds unmatched as new.
func TestSyncCommand_MockAIFailureFallback(t *testing.T) {
	// Create temp directory
	tmpDir := t.TempDir()

	// Create source roadmap with one matching and one non-matching item
	sourceContent := `{
  "items": [
    {
      "slug": "feature-a",
      "title": "Feature A Updated",
      "priority": "high",
      "status": "completed",
      "context": "Updated feature A"
    },
    {
      "slug": "feature-new",
      "title": "New Feature",
      "priority": "medium",
      "status": "planned",
      "context": "Brand new feature"
    }
  ]
}`
	sourcePath := filepath.Join(tmpDir, "source.json")
	require.NoError(t, os.WriteFile(sourcePath, []byte(sourceContent), 0644)) //nolint:gosec // G306: Test file permissions

	// Create target roadmap with one matching and one non-matching item
	targetContent := `{
  "items": [
    {
      "slug": "feature-a",
      "title": "Feature A",
      "priority": "low",
      "status": "in_progress",
      "context": "Original feature A"
    },
    {
      "slug": "feature-old",
      "title": "Old Feature",
      "priority": "low",
      "status": "planned",
      "context": "Old feature"
    }
  ]
}`
	musterDir := filepath.Join(tmpDir, ".muster")
	require.NoError(t, os.MkdirAll(musterDir, 0755)) //nolint:gosec // G301: Test directory permissions
	targetPath := filepath.Join(musterDir, "roadmap.json")
	require.NoError(t, os.WriteFile(targetPath, []byte(targetContent), 0644)) //nolint:gosec // G306: Test file permissions

	// Change to temp directory
	origDir, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(tmpDir))
	defer func() { _ = os.Chdir(origDir) }()

	// Save original factory and replace with mock that returns error
	originalFactory := codingToolFactory
	defer func() { codingToolFactory = originalFactory }()

	codingToolFactory = func() (coding.CodingTool, error) {
		return &mockSyncTool{shouldFail: true}, nil
	}

	// Create command instance with fresh flags
	cmd := &cobra.Command{
		Use:  "sync",
		RunE: syncCmd.RunE,
	}
	cmd.Flags().String("source", ".roadmap.json", "Source roadmap file path")
	cmd.Flags().String("target", ".muster/roadmap.json", "Target roadmap file path")
	cmd.Flags().Bool("yes", false, "Accept all AI matches without confirmation")
	cmd.Flags().Bool("dry-run", false, "Preview changes without saving")
	cmd.Flags().Bool("delete", false, "Delete target items not matched by any source item")
	cmd.Flags().Bool("verbose", false, "Enable verbose output")

	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)

	// Execute sync with verbose to see AI failure message
	cmd.SetArgs([]string{"--source", sourcePath, "--target", targetPath, "--verbose"})
	err = cmd.Execute()
	assert.NoError(t, err, "command should execute without error despite AI failure")

	// Verify target file has exact match updated and new item added
	afterContent, err := os.ReadFile(targetPath) //nolint:gosec // G304: Test file reading
	require.NoError(t, err)

	// Exact match: feature-a should be updated
	assert.Contains(t, string(afterContent), "Feature A Updated", "target should have updated title for exact match")
	assert.Contains(t, string(afterContent), "completed", "target should have updated status for exact match")

	// No fuzzy match: feature-new added as new, feature-old preserved
	assert.Contains(t, string(afterContent), "feature-new", "new item should be added")
	assert.Contains(t, string(afterContent), "feature-old", "old item should be preserved")

	// Verify output shows 1 updated (exact match), 1 added (new)
	output := buf.String()
	assert.Contains(t, output, "Updated: 1", "output should show 1 updated item from exact match")
	assert.Contains(t, output, "Added: 1", "output should show 1 added item (no fuzzy match)")
}

// mockSyncTool is a simple mock implementation of coding.CodingTool for sync tests
type mockSyncTool struct {
	response   string
	shouldFail bool
}

func (m *mockSyncTool) Invoke(ctx context.Context, cfg ai.InvokeConfig) (*ai.InvokeResult, error) {
	if m.shouldFail {
		return nil, &coding.SimulatedToolError{Message: "mock AI tool failure"}
	}
	return &ai.InvokeResult{
		RawOutput: m.response,
	}, nil
}
