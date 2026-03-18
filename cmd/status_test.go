package cmd

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

func TestStatusCommand_Exists(t *testing.T) {
	// Verify the command exists and can be retrieved
	assert.NotNil(t, statusCmd, "status command should exist")
	assert.Equal(t, "status [slug]", statusCmd.Use, "command use should be 'status [slug]'")
}

func TestStatusCommand_AcceptsMaxOneArg(t *testing.T) {
	// Verify the command accepts at most one argument
	assert.NotNil(t, statusCmd.Args, "Args validator should be set")

	// Test with no args - should pass
	err := statusCmd.Args(statusCmd, []string{})
	assert.NoError(t, err, "should accept zero arguments")

	// Test with one arg - should pass
	err = statusCmd.Args(statusCmd, []string{"slug"})
	assert.NoError(t, err, "should accept one argument")

	// Test with two args - should fail
	err = statusCmd.Args(statusCmd, []string{"slug1", "slug2"})
	assert.Error(t, err, "should reject two arguments")
}

func TestStatusCommand_TableOutputWithItems(t *testing.T) {
	// Save and restore output mode
	originalMode := ui.GetOutputMode()
	defer ui.SetOutputMode(originalMode)
	ui.SetOutputMode(ui.TableMode)

	// Create a temporary directory with a roadmap file
	tmpDir := t.TempDir()
	musterDir := filepath.Join(tmpDir, ".muster")
	require.NoError(t, os.MkdirAll(musterDir, 0755)) //nolint:gosec // G301: Test directory permissions //nolint:gosec // G301: Test directory permissions

	roadmapContent := `{
  "items": [
    {
      "slug": "feature-a",
      "title": "Feature A",
      "priority": "high",
      "status": "in_progress",
      "context": "Implement feature A"
    },
    {
      "slug": "feature-b",
      "title": "Feature B",
      "priority": "medium",
      "status": "planned",
      "context": "Plan feature B"
    }
  ]
}`
	require.NoError(t, os.WriteFile(filepath.Join(musterDir, "roadmap.json"), []byte(roadmapContent), 0644)) //nolint:gosec // G306: Test file permissions //nolint:gosec // G306: Test file permissions

	// Change to temp directory for the test
	origDir, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(tmpDir))
	defer func() { _ = os.Chdir(origDir) }()

	// Create a buffer to capture output
	buf := new(bytes.Buffer)
	statusCmd.SetOut(buf)

	// Execute command without args
	err = statusCmd.RunE(statusCmd, []string{})
	assert.NoError(t, err, "command should execute without error")

	// Verify output contains table header and items
	output := buf.String()
	assert.Contains(t, output, "SLUG", "output should contain table header")
	assert.Contains(t, output, "feature-a", "output should contain first item")
	assert.Contains(t, output, "feature-b", "output should contain second item")
}

func TestStatusCommand_DetailOutputWithSlug(t *testing.T) {
	// Save and restore output mode
	originalMode := ui.GetOutputMode()
	defer ui.SetOutputMode(originalMode)
	ui.SetOutputMode(ui.TableMode)

	// Create a temporary directory with a roadmap file
	tmpDir := t.TempDir()
	musterDir := filepath.Join(tmpDir, ".muster")
	require.NoError(t, os.MkdirAll(musterDir, 0755)) //nolint:gosec // G301: Test directory permissions

	roadmapContent := `{
  "items": [
    {
      "slug": "feature-a",
      "title": "Feature A",
      "priority": "high",
      "status": "in_progress",
      "context": "Implement feature A"
    }
  ]
}`
	require.NoError(t, os.WriteFile(filepath.Join(musterDir, "roadmap.json"), []byte(roadmapContent), 0644)) //nolint:gosec // G306: Test file permissions

	// Change to temp directory for the test
	origDir, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(tmpDir))
	defer func() { _ = os.Chdir(origDir) }()

	// Create a buffer to capture output
	buf := new(bytes.Buffer)
	statusCmd.SetOut(buf)

	// Execute command with slug argument
	err = statusCmd.RunE(statusCmd, []string{"feature-a"})
	assert.NoError(t, err, "command should execute without error")

	// Verify output contains detail fields
	output := buf.String()
	assert.Contains(t, output, "Slug:", "output should contain Slug field")
	assert.Contains(t, output, "feature-a", "output should contain slug value")
	assert.Contains(t, output, "Title:", "output should contain Title field")
	assert.Contains(t, output, "Feature A", "output should contain title value")
	assert.Contains(t, output, "Context:", "output should contain Context field")
	assert.Contains(t, output, "Implement feature A", "output should contain context value")
}

func TestStatusCommand_EmptyRoadmapFriendlyMessage(t *testing.T) {
	// Save and restore output mode
	originalMode := ui.GetOutputMode()
	defer ui.SetOutputMode(originalMode)
	ui.SetOutputMode(ui.TableMode)

	// Create a temporary directory with an empty roadmap file
	tmpDir := t.TempDir()
	musterDir := filepath.Join(tmpDir, ".muster")
	require.NoError(t, os.MkdirAll(musterDir, 0755)) //nolint:gosec // G301: Test directory permissions

	roadmapContent := `{
  "items": []
}`
	require.NoError(t, os.WriteFile(filepath.Join(musterDir, "roadmap.json"), []byte(roadmapContent), 0644)) //nolint:gosec // G306: Test file permissions

	// Change to temp directory for the test
	origDir, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(tmpDir))
	defer func() { _ = os.Chdir(origDir) }()

	// Create a buffer to capture output
	buf := new(bytes.Buffer)
	statusCmd.SetOut(buf)

	// Execute command without args
	err = statusCmd.RunE(statusCmd, []string{})
	assert.NoError(t, err, "command should execute without error")

	// Verify output contains friendly message
	output := buf.String()
	assert.Contains(t, output, "No roadmap items found", "output should contain friendly message")
}

func TestStatusCommand_JSONModeTableOutput(t *testing.T) {
	// Save and restore output mode
	originalMode := ui.GetOutputMode()
	defer ui.SetOutputMode(originalMode)
	ui.SetOutputMode(ui.JSONMode)

	// Create a temporary directory with a roadmap file
	tmpDir := t.TempDir()
	musterDir := filepath.Join(tmpDir, ".muster")
	require.NoError(t, os.MkdirAll(musterDir, 0755)) //nolint:gosec // G301: Test directory permissions

	roadmapContent := `{
  "items": [
    {
      "slug": "feature-a",
      "title": "Feature A",
      "priority": "high",
      "status": "in_progress",
      "context": "Implement feature A"
    }
  ]
}`
	require.NoError(t, os.WriteFile(filepath.Join(musterDir, "roadmap.json"), []byte(roadmapContent), 0644)) //nolint:gosec // G306: Test file permissions

	// Change to temp directory for the test
	origDir, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(tmpDir))
	defer func() { _ = os.Chdir(origDir) }()

	// Create a buffer to capture output
	buf := new(bytes.Buffer)
	statusCmd.SetOut(buf)

	// Execute command without args
	err = statusCmd.RunE(statusCmd, []string{})
	assert.NoError(t, err, "command should execute without error")

	// Verify output is valid JSON array
	output := buf.String()
	var items []map[string]interface{}
	err = json.Unmarshal([]byte(output), &items)
	assert.NoError(t, err, "output should be valid JSON")
	assert.Len(t, items, 1, "should have one item")
	assert.Equal(t, "feature-a", items[0]["slug"])
}

func TestStatusCommand_JSONModeDetailOutput(t *testing.T) {
	// Save and restore output mode
	originalMode := ui.GetOutputMode()
	defer ui.SetOutputMode(originalMode)
	ui.SetOutputMode(ui.JSONMode)

	// Create a temporary directory with a roadmap file
	tmpDir := t.TempDir()
	musterDir := filepath.Join(tmpDir, ".muster")
	require.NoError(t, os.MkdirAll(musterDir, 0755)) //nolint:gosec // G301: Test directory permissions

	roadmapContent := `{
  "items": [
    {
      "slug": "feature-a",
      "title": "Feature A",
      "priority": "high",
      "status": "in_progress",
      "context": "Implement feature A"
    }
  ]
}`
	require.NoError(t, os.WriteFile(filepath.Join(musterDir, "roadmap.json"), []byte(roadmapContent), 0644)) //nolint:gosec // G306: Test file permissions

	// Change to temp directory for the test
	origDir, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(tmpDir))
	defer func() { _ = os.Chdir(origDir) }()

	// Create a buffer to capture output
	buf := new(bytes.Buffer)
	statusCmd.SetOut(buf)

	// Execute command with slug argument
	err = statusCmd.RunE(statusCmd, []string{"feature-a"})
	assert.NoError(t, err, "command should execute without error")

	// Verify output is valid JSON object
	output := buf.String()
	var item map[string]interface{}
	err = json.Unmarshal([]byte(output), &item)
	assert.NoError(t, err, "output should be valid JSON")
	assert.Equal(t, "feature-a", item["slug"])
	assert.Equal(t, "Feature A", item["title"])
	assert.Equal(t, "Implement feature A", item["context"])
}

func TestStatusCommand_EmptyRoadmapJSON(t *testing.T) {
	// Save and restore output mode
	originalMode := ui.GetOutputMode()
	defer ui.SetOutputMode(originalMode)
	ui.SetOutputMode(ui.JSONMode)

	// Create a temporary directory with an empty roadmap file
	tmpDir := t.TempDir()
	musterDir := filepath.Join(tmpDir, ".muster")
	require.NoError(t, os.MkdirAll(musterDir, 0755)) //nolint:gosec // G301: Test directory permissions

	roadmapContent := `{
  "items": []
}`
	require.NoError(t, os.WriteFile(filepath.Join(musterDir, "roadmap.json"), []byte(roadmapContent), 0644)) //nolint:gosec // G306: Test file permissions

	// Change to temp directory for the test
	origDir, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(tmpDir))
	defer func() { _ = os.Chdir(origDir) }()

	// Create a buffer to capture output
	buf := new(bytes.Buffer)
	statusCmd.SetOut(buf)

	// Execute command without args
	err = statusCmd.RunE(statusCmd, []string{})
	assert.NoError(t, err, "command should execute without error")

	// Verify output is empty JSON array
	output := buf.String()
	assert.Equal(t, "[]\n", output, "output should be empty JSON array")
}

func TestStatusCommand_InvalidSlugError(t *testing.T) {
	// Create a temporary directory with a roadmap file
	tmpDir := t.TempDir()
	musterDir := filepath.Join(tmpDir, ".muster")
	require.NoError(t, os.MkdirAll(musterDir, 0755)) //nolint:gosec // G301: Test directory permissions

	roadmapContent := `{
  "items": [
    {
      "slug": "feature-a",
      "title": "Feature A",
      "priority": "high",
      "status": "in_progress",
      "context": "Implement feature A"
    }
  ]
}`
	require.NoError(t, os.WriteFile(filepath.Join(musterDir, "roadmap.json"), []byte(roadmapContent), 0644)) //nolint:gosec // G306: Test file permissions

	// Change to temp directory for the test
	origDir, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(tmpDir))
	defer func() { _ = os.Chdir(origDir) }()

	// Create a buffer to capture output
	buf := new(bytes.Buffer)
	statusCmd.SetOut(buf)

	// Execute command with invalid slug
	err = statusCmd.RunE(statusCmd, []string{"invalid-slug"})
	assert.Error(t, err, "command should return error for invalid slug")
	assert.Contains(t, err.Error(), "not found", "error should mention slug not found")
	assert.Contains(t, err.Error(), "invalid-slug", "error should contain the slug")
}

func TestStatusCommand_MalformedRoadmapError(t *testing.T) {
	// Create a temporary directory with a malformed roadmap file
	tmpDir := t.TempDir()
	musterDir := filepath.Join(tmpDir, ".muster")
	require.NoError(t, os.MkdirAll(musterDir, 0755)) //nolint:gosec // G301: Test directory permissions

	// Invalid JSON
	roadmapContent := `{
  "items": [
    {
      "slug": "feature-a",
      "title": "Feature A",
      "priority": "high"
      // Missing comma and closing braces
`
	require.NoError(t, os.WriteFile(filepath.Join(musterDir, "roadmap.json"), []byte(roadmapContent), 0644)) //nolint:gosec // G306: Test file permissions

	// Change to temp directory for the test
	origDir, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(tmpDir))
	defer func() { _ = os.Chdir(origDir) }()

	// Create a buffer to capture output
	buf := new(bytes.Buffer)
	statusCmd.SetOut(buf)

	// Execute command
	err = statusCmd.RunE(statusCmd, []string{})
	assert.Error(t, err, "command should return error for malformed roadmap")
	assert.Contains(t, err.Error(), "malformed", "error should mention malformed file")
}
