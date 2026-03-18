package roadmap

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadRoadmap_NewLocation(t *testing.T) {
	dir := t.TempDir()

	// Create .muster directory and roadmap.json
	musterDir := filepath.Join(dir, ".muster")
	require.NoError(t, os.MkdirAll(musterDir, 0755)) //nolint:gosec // G301: Test directory permissions

	roadmapJSON := `{
  "items": [
    {
      "slug": "test-item",
      "title": "Test Item",
      "priority": "high",
      "status": "planned",
      "context": "Test context"
    }
  ]
}`
	err := os.WriteFile(filepath.Join(musterDir, "roadmap.json"), []byte(roadmapJSON), 0644) //nolint:gosec // G306: Test file permissions
	require.NoError(t, err)

	// Load roadmap
	roadmap, err := LoadRoadmap(dir)
	require.NoError(t, err)
	require.NotNil(t, roadmap)
	require.Len(t, roadmap.Items, 1)
	assert.Equal(t, "test-item", roadmap.Items[0].Slug)
	assert.Equal(t, "Test Item", roadmap.Items[0].Title)
	assert.Equal(t, PriorityHigh, roadmap.Items[0].Priority)
	assert.Equal(t, StatusPlanned, roadmap.Items[0].Status)
	assert.Equal(t, "Test context", roadmap.Items[0].Context)
}

func TestLoadRoadmap_FallbackToLegacy(t *testing.T) {
	dir := t.TempDir()

	// Create only legacy .roadmap.json (no .muster directory)
	roadmapJSON := `{
  "items": [
    {
      "slug": "legacy-item",
      "title": "Legacy Item",
      "priority": "medium",
      "status": "in_progress",
      "context": "Legacy context"
    }
  ]
}`
	err := os.WriteFile(filepath.Join(dir, ".roadmap.json"), []byte(roadmapJSON), 0644) //nolint:gosec // G306: Test file permissions
	require.NoError(t, err)

	// Load roadmap
	roadmap, err := LoadRoadmap(dir)
	require.NoError(t, err)
	require.NotNil(t, roadmap)
	require.Len(t, roadmap.Items, 1)
	assert.Equal(t, "legacy-item", roadmap.Items[0].Slug)
	assert.Equal(t, "Legacy Item", roadmap.Items[0].Title)
	assert.Equal(t, PriorityMedium, roadmap.Items[0].Priority)
	assert.Equal(t, StatusInProgress, roadmap.Items[0].Status)
}

func TestLoadRoadmap_BothExist_NewWins(t *testing.T) {
	dir := t.TempDir()

	// Create .muster/roadmap.json
	musterDir := filepath.Join(dir, ".muster")
	require.NoError(t, os.MkdirAll(musterDir, 0755)) //nolint:gosec // G301: Test directory permissions

	newRoadmapJSON := `{
  "items": [
    {
      "slug": "new-item",
      "title": "New Item",
      "priority": "high",
      "status": "planned",
      "context": "New context"
    }
  ]
}`
	err := os.WriteFile(filepath.Join(musterDir, "roadmap.json"), []byte(newRoadmapJSON), 0644) //nolint:gosec // G306: Test file permissions
	require.NoError(t, err)

	// Create legacy .roadmap.json
	legacyRoadmapJSON := `{
  "items": [
    {
      "slug": "legacy-item",
      "title": "Legacy Item",
      "priority": "low",
      "status": "completed",
      "context": "Legacy context"
    }
  ]
}`
	err = os.WriteFile(filepath.Join(dir, ".roadmap.json"), []byte(legacyRoadmapJSON), 0644) //nolint:gosec // G306: Test file permissions
	require.NoError(t, err)

	// Load roadmap - should load from new location
	roadmap, err := LoadRoadmap(dir)
	require.NoError(t, err)
	require.NotNil(t, roadmap)
	require.Len(t, roadmap.Items, 1)
	assert.Equal(t, "new-item", roadmap.Items[0].Slug)
	assert.Equal(t, "New Item", roadmap.Items[0].Title)
}

func TestLoadRoadmap_NeitherExists_ReturnsEmpty(t *testing.T) {
	dir := t.TempDir()

	// Load roadmap from empty directory
	roadmap, err := LoadRoadmap(dir)
	require.NoError(t, err)
	require.NotNil(t, roadmap)
	require.NotNil(t, roadmap.Items)
	assert.Len(t, roadmap.Items, 0)
}

func TestLoadRoadmap_MalformedNew_DoesNotFallback(t *testing.T) {
	dir := t.TempDir()

	// Create .muster/roadmap.json with malformed JSON
	musterDir := filepath.Join(dir, ".muster")
	require.NoError(t, os.MkdirAll(musterDir, 0755)) //nolint:gosec // G301: Test directory permissions

	malformedJSON := `{
  "items": [
    {
      "slug": "test-item"
      "missing-comma": true
    }
  ]
}`
	err := os.WriteFile(filepath.Join(musterDir, "roadmap.json"), []byte(malformedJSON), 0644) //nolint:gosec // G306: Test file permissions
	require.NoError(t, err)

	// Create valid legacy .roadmap.json
	legacyRoadmapJSON := `{
  "items": [
    {
      "slug": "legacy-item",
      "title": "Legacy Item",
      "priority": "low",
      "status": "completed",
      "context": "Legacy context"
    }
  ]
}`
	err = os.WriteFile(filepath.Join(dir, ".roadmap.json"), []byte(legacyRoadmapJSON), 0644) //nolint:gosec // G306: Test file permissions
	require.NoError(t, err)

	// Load roadmap - should fail without falling back
	roadmap, err := LoadRoadmap(dir)
	require.Error(t, err)
	assert.Nil(t, roadmap)
	assert.Contains(t, err.Error(), "failed to load roadmap from")
	assert.Contains(t, err.Error(), ".muster/roadmap.json")
	// Verify error includes underlying JSON parse error details
	assert.Contains(t, err.Error(), "invalid character")
}

func TestLoadRoadmap_BothMalformed_ReturnsNewFileError(t *testing.T) {
	dir := t.TempDir()

	// Create .muster/roadmap.json with malformed JSON
	musterDir := filepath.Join(dir, ".muster")
	require.NoError(t, os.MkdirAll(musterDir, 0755)) //nolint:gosec // G301: Test directory permissions

	malformedNewJSON := `{
  "items": [
    {
      "slug": "new-item"
      "missing-comma": true
    }
  ]
}`
	err := os.WriteFile(filepath.Join(musterDir, "roadmap.json"), []byte(malformedNewJSON), 0644) //nolint:gosec // G306: Test file permissions
	require.NoError(t, err)

	// Create malformed legacy .roadmap.json
	malformedLegacyJSON := `{
  "items": [
    {
      "slug": "legacy-item",
      "invalid": json
    }
  ]
}`
	err = os.WriteFile(filepath.Join(dir, ".roadmap.json"), []byte(malformedLegacyJSON), 0644) //nolint:gosec // G306: Test file permissions
	require.NoError(t, err)

	// Load roadmap - should return error from new file, not fallback
	roadmap, err := LoadRoadmap(dir)
	require.Error(t, err)
	assert.Nil(t, roadmap)
	// Should report error from new file location
	assert.Contains(t, err.Error(), "failed to load roadmap from")
	assert.Contains(t, err.Error(), ".muster/roadmap.json")
	// Should include JSON parse error
	assert.Contains(t, err.Error(), "invalid character")
}

func TestLoadRoadmap_ArrayFormat(t *testing.T) {
	dir := t.TempDir()

	// Create roadmap.json in array format (legacy)
	musterDir := filepath.Join(dir, ".muster")
	require.NoError(t, os.MkdirAll(musterDir, 0755)) //nolint:gosec // G301: Test directory permissions

	arrayFormatJSON := `[
  {
    "slug": "array-item",
    "title": "Array Item",
    "priority": "low",
    "status": "blocked",
    "context": "Array format context"
  }
]`
	err := os.WriteFile(filepath.Join(musterDir, "roadmap.json"), []byte(arrayFormatJSON), 0644) //nolint:gosec // G306: Test file permissions
	require.NoError(t, err)

	// Load roadmap
	roadmap, err := LoadRoadmap(dir)
	require.NoError(t, err)
	require.NotNil(t, roadmap)
	require.Len(t, roadmap.Items, 1)
	assert.Equal(t, "array-item", roadmap.Items[0].Slug)
	assert.Equal(t, "Array Item", roadmap.Items[0].Title)
	assert.Equal(t, PriorityLow, roadmap.Items[0].Priority)
	assert.Equal(t, StatusBlocked, roadmap.Items[0].Status)
}

func TestSaveRoadmap_CreatesDirectory(t *testing.T) {
	dir := t.TempDir()

	// Create a simple roadmap
	roadmap := &Roadmap{
		Items: []RoadmapItem{
			{
				Slug:     "test-item",
				Title:    "Test Item",
				Priority: PriorityHigh,
				Status:   StatusPlanned,
				Context:  "Test context",
			},
		},
	}

	// Save roadmap
	err := SaveRoadmap(dir, roadmap)
	require.NoError(t, err)

	// Verify .muster directory exists
	musterDir := filepath.Join(dir, ".muster")
	info, err := os.Stat(musterDir)
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}

func TestSaveRoadmap_WritesWrapperFormat(t *testing.T) {
	dir := t.TempDir()

	// Create a roadmap
	roadmap := &Roadmap{
		Items: []RoadmapItem{
			{
				Slug:     "wrapper-test",
				Title:    "Wrapper Test",
				Priority: PriorityMedium,
				Status:   StatusInProgress,
				Context:  "Wrapper format test",
			},
		},
	}

	// Save roadmap
	err := SaveRoadmap(dir, roadmap)
	require.NoError(t, err)

	// Read the file and verify it's in wrapper format
	filePath := filepath.Join(dir, ".muster", "roadmap.json")
	data, err := os.ReadFile(filePath) //nolint:gosec // G304: Test file path is controlled
	require.NoError(t, err)

	// Verify wrapper format by checking for "items" key
	assert.Contains(t, string(data), `"items"`)

	// Verify it can be parsed as wrapper format
	var loaded Roadmap
	err = json.Unmarshal(data, &loaded)
	require.NoError(t, err)
	require.Len(t, loaded.Items, 1)
	assert.Equal(t, "wrapper-test", loaded.Items[0].Slug)
}

func TestSaveRoadmap_FilePermissions(t *testing.T) {
	dir := t.TempDir()

	// Create a roadmap
	roadmap := &Roadmap{
		Items: []RoadmapItem{
			{
				Slug:     "perm-test",
				Title:    "Permission Test",
				Priority: PriorityLow,
				Status:   StatusCompleted,
				Context:  "Permission test",
			},
		},
	}

	// Save roadmap
	err := SaveRoadmap(dir, roadmap)
	require.NoError(t, err)

	// Check file permissions
	filePath := filepath.Join(dir, ".muster", "roadmap.json")
	info, err := os.Stat(filePath)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0644), info.Mode().Perm())
}

func TestSaveRoadmap_TrailingNewline(t *testing.T) {
	dir := t.TempDir()

	// Create a roadmap
	roadmap := &Roadmap{
		Items: []RoadmapItem{
			{
				Slug:     "newline-test",
				Title:    "Newline Test",
				Priority: PriorityHigh,
				Status:   StatusPlanned,
				Context:  "Newline test",
			},
		},
	}

	// Save roadmap
	err := SaveRoadmap(dir, roadmap)
	require.NoError(t, err)

	// Read the file and check for trailing newline
	filePath := filepath.Join(dir, ".muster", "roadmap.json")
	data, err := os.ReadFile(filePath) //nolint:gosec // G304: Test file path is controlled
	require.NoError(t, err)
	assert.True(t, len(data) > 0, "File should not be empty")
	assert.Equal(t, byte('\n'), data[len(data)-1], "File should end with newline")
}

func TestRoundTrip_PreservesData(t *testing.T) {
	dir := t.TempDir()

	// Create a roadmap with optional fields
	prURL := "https://github.com/test/pr/1"
	branch := "feature/test"
	originalRoadmap := &Roadmap{
		Items: []RoadmapItem{
			{
				Slug:     "roundtrip-test",
				Title:    "Round Trip Test",
				Priority: PriorityMedium,
				Status:   StatusInProgress,
				Context:  "Round trip test context",
				PRUrl:    &prURL,
				Branch:   &branch,
			},
		},
	}

	// Save roadmap
	err := SaveRoadmap(dir, originalRoadmap)
	require.NoError(t, err)

	// Load roadmap
	loadedRoadmap, err := LoadRoadmap(dir)
	require.NoError(t, err)

	// Verify all data is preserved
	require.Len(t, loadedRoadmap.Items, 1)
	item := loadedRoadmap.Items[0]
	assert.Equal(t, "roundtrip-test", item.Slug)
	assert.Equal(t, "Round Trip Test", item.Title)
	assert.Equal(t, PriorityMedium, item.Priority)
	assert.Equal(t, StatusInProgress, item.Status)
	assert.Equal(t, "Round trip test context", item.Context)
	require.NotNil(t, item.PRUrl)
	assert.Equal(t, "https://github.com/test/pr/1", *item.PRUrl)
	require.NotNil(t, item.Branch)
	assert.Equal(t, "feature/test", *item.Branch)
}

func TestRoundTrip_MigratesArrayToWrapper(t *testing.T) {
	dir := t.TempDir()

	// Create roadmap.json in array format (legacy)
	musterDir := filepath.Join(dir, ".muster")
	require.NoError(t, os.MkdirAll(musterDir, 0755)) //nolint:gosec // G301: Test directory permissions

	arrayFormatJSON := `[
  {
    "slug": "migrate-test",
    "title": "Migration Test",
    "priority": "lower",
    "status": "blocked",
    "context": "Migration test"
  }
]`
	err := os.WriteFile(filepath.Join(musterDir, "roadmap.json"), []byte(arrayFormatJSON), 0644) //nolint:gosec // G306: Test file permissions
	require.NoError(t, err)

	// Load roadmap (reads array format)
	roadmap, err := LoadRoadmap(dir)
	require.NoError(t, err)
	require.Len(t, roadmap.Items, 1)

	// Save roadmap (should write wrapper format)
	err = SaveRoadmap(dir, roadmap)
	require.NoError(t, err)

	// Read the file and verify it's now in wrapper format
	filePath := filepath.Join(musterDir, "roadmap.json")
	data, err := os.ReadFile(filePath) //nolint:gosec // G304: Test file path is controlled
	require.NoError(t, err)
	assert.Contains(t, string(data), `"items"`)

	// Load again to verify it still works
	reloadedRoadmap, err := LoadRoadmap(dir)
	require.NoError(t, err)
	require.Len(t, reloadedRoadmap.Items, 1)
	assert.Equal(t, "migrate-test", reloadedRoadmap.Items[0].Slug)
}
