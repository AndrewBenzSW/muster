package ui

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/abenz1267/muster/internal/roadmap"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFormatRoadmapTable_TableMode(t *testing.T) {
	// Save and restore output mode
	originalMode := GetOutputMode()
	t.Cleanup(func() { SetOutputMode(originalMode) })

	SetOutputMode(TableMode)

	items := []roadmap.RoadmapItem{
		{
			Slug:     "item-1",
			Title:    "First Item",
			Priority: roadmap.PriorityHigh,
			Status:   roadmap.StatusPlanned,
			Context:  "Context for first item",
		},
		{
			Slug:     "item-2",
			Title:    "Second Item",
			Priority: roadmap.PriorityMedium,
			Status:   roadmap.StatusInProgress,
			Context:  "Context for second item",
		},
	}

	output, err := FormatRoadmapTable(items)
	require.NoError(t, err)

	// Verify header is present
	assert.Contains(t, output, "SLUG")
	assert.Contains(t, output, "TITLE")
	assert.Contains(t, output, "PRIORITY")
	assert.Contains(t, output, "STATUS")

	// Verify data rows
	assert.Contains(t, output, "item-1")
	assert.Contains(t, output, "First Item")
	assert.Contains(t, output, "high")
	assert.Contains(t, output, "planned")

	assert.Contains(t, output, "item-2")
	assert.Contains(t, output, "Second Item")
	assert.Contains(t, output, "medium")
	assert.Contains(t, output, "in_progress")
}

func TestFormatRoadmapTable_TableMode_Empty(t *testing.T) {
	// Save and restore output mode
	originalMode := GetOutputMode()
	t.Cleanup(func() { SetOutputMode(originalMode) })

	SetOutputMode(TableMode)

	items := []roadmap.RoadmapItem{}

	output, err := FormatRoadmapTable(items)
	require.NoError(t, err)

	// Verify friendly message
	assert.Equal(t, "No roadmap items found. Run 'muster add' to create one.", output)
}

func TestFormatRoadmapTable_JSONMode(t *testing.T) {
	// Save and restore output mode
	originalMode := GetOutputMode()
	t.Cleanup(func() { SetOutputMode(originalMode) })

	SetOutputMode(JSONMode)

	items := []roadmap.RoadmapItem{
		{
			Slug:     "json-item-1",
			Title:    "JSON Item One",
			Priority: roadmap.PriorityLow,
			Status:   roadmap.StatusCompleted,
			Context:  "JSON context",
		},
		{
			Slug:     "json-item-2",
			Title:    "JSON Item Two",
			Priority: roadmap.PriorityHigh,
			Status:   roadmap.StatusBlocked,
			Context:  "Another JSON context",
		},
	}

	output, err := FormatRoadmapTable(items)
	require.NoError(t, err)

	// Verify it's valid JSON
	var parsed []RoadmapTableItem
	err = json.Unmarshal([]byte(output), &parsed)
	require.NoError(t, err, "output should be valid JSON")

	// Verify structure - unwrapped array
	require.Len(t, parsed, 2)
	assert.Equal(t, "json-item-1", parsed[0].Slug)
	assert.Equal(t, "JSON Item One", parsed[0].Title)
	assert.Equal(t, "low", parsed[0].Priority)
	assert.Equal(t, "completed", parsed[0].Status)

	assert.Equal(t, "json-item-2", parsed[1].Slug)
	assert.Equal(t, "JSON Item Two", parsed[1].Title)
	assert.Equal(t, "high", parsed[1].Priority)
	assert.Equal(t, "blocked", parsed[1].Status)

	// Verify no wrapper format (should not contain "items" key)
	assert.NotContains(t, output, `"items"`)
}

func TestFormatRoadmapTable_JSONMode_Empty(t *testing.T) {
	// Save and restore output mode
	originalMode := GetOutputMode()
	t.Cleanup(func() { SetOutputMode(originalMode) })

	SetOutputMode(JSONMode)

	items := []roadmap.RoadmapItem{}

	output, err := FormatRoadmapTable(items)
	require.NoError(t, err)

	// Verify empty array output
	assert.Equal(t, "[]", output)

	// Verify it can be parsed
	var parsed []RoadmapTableItem
	err = json.Unmarshal([]byte(output), &parsed)
	require.NoError(t, err, "empty array should be valid JSON")
	assert.Len(t, parsed, 0)
}

func TestFormatRoadmapDetail_TableMode(t *testing.T) {
	// Save and restore output mode
	originalMode := GetOutputMode()
	t.Cleanup(func() { SetOutputMode(originalMode) })

	SetOutputMode(TableMode)

	prURL := "https://github.com/test/pr/123"
	branch := "feature/test-branch"
	item := roadmap.RoadmapItem{
		Slug:     "detail-item",
		Title:    "Detail Item",
		Priority: roadmap.PriorityMedium,
		Status:   roadmap.StatusInProgress,
		Context:  "This is a detailed context",
		PRUrl:    &prURL,
		Branch:   &branch,
	}

	output, err := FormatRoadmapDetail(item)
	require.NoError(t, err)

	// Verify labeled key-value format
	assert.Contains(t, output, "Slug:     detail-item")
	assert.Contains(t, output, "Title:    Detail Item")
	assert.Contains(t, output, "Priority: medium")
	assert.Contains(t, output, "Status:   in_progress")
	assert.Contains(t, output, "Context:  This is a detailed context")
	assert.Contains(t, output, "PR URL:   https://github.com/test/pr/123")
	assert.Contains(t, output, "Branch:   feature/test-branch")
}

func TestFormatRoadmapDetail_TableMode_NoOptionalFields(t *testing.T) {
	// Save and restore output mode
	originalMode := GetOutputMode()
	t.Cleanup(func() { SetOutputMode(originalMode) })

	SetOutputMode(TableMode)

	item := roadmap.RoadmapItem{
		Slug:     "simple-item",
		Title:    "Simple Item",
		Priority: roadmap.PriorityLow,
		Status:   roadmap.StatusPlanned,
		Context:  "Simple context",
	}

	output, err := FormatRoadmapDetail(item)
	require.NoError(t, err)

	// Verify required fields present
	assert.Contains(t, output, "Slug:     simple-item")
	assert.Contains(t, output, "Title:    Simple Item")
	assert.Contains(t, output, "Priority: low")
	assert.Contains(t, output, "Status:   planned")
	assert.Contains(t, output, "Context:  Simple context")

	// Verify optional fields not present
	assert.NotContains(t, output, "PR URL:")
	assert.NotContains(t, output, "Branch:")
}

func TestFormatRoadmapDetail_JSONMode(t *testing.T) {
	// Save and restore output mode
	originalMode := GetOutputMode()
	t.Cleanup(func() { SetOutputMode(originalMode) })

	SetOutputMode(JSONMode)

	prURL := "https://github.com/test/pr/456"
	branch := "feature/json-test"
	item := roadmap.RoadmapItem{
		Slug:     "json-detail",
		Title:    "JSON Detail",
		Priority: roadmap.PriorityHigh,
		Status:   roadmap.StatusBlocked,
		Context:  "JSON detail context",
		PRUrl:    &prURL,
		Branch:   &branch,
	}

	output, err := FormatRoadmapDetail(item)
	require.NoError(t, err)

	// Verify it's valid JSON
	var parsed RoadmapDetailItem
	err = json.Unmarshal([]byte(output), &parsed)
	require.NoError(t, err, "output should be valid JSON")

	// Verify single object (not array)
	assert.NotContains(t, strings.TrimSpace(output), "[")

	// Verify all fields
	assert.Equal(t, "json-detail", parsed.Slug)
	assert.Equal(t, "JSON Detail", parsed.Title)
	assert.Equal(t, "high", parsed.Priority)
	assert.Equal(t, "blocked", parsed.Status)
	assert.Equal(t, "JSON detail context", parsed.Context)
	require.NotNil(t, parsed.PRUrl)
	assert.Equal(t, "https://github.com/test/pr/456", *parsed.PRUrl)
	require.NotNil(t, parsed.Branch)
	assert.Equal(t, "feature/json-test", *parsed.Branch)
}

func TestFormatRoadmapDetail_JSONMode_NoOptionalFields(t *testing.T) {
	// Save and restore output mode
	originalMode := GetOutputMode()
	t.Cleanup(func() { SetOutputMode(originalMode) })

	SetOutputMode(JSONMode)

	item := roadmap.RoadmapItem{
		Slug:     "minimal-json",
		Title:    "Minimal JSON",
		Priority: roadmap.PriorityLower,
		Status:   roadmap.StatusCompleted,
		Context:  "Minimal context",
	}

	output, err := FormatRoadmapDetail(item)
	require.NoError(t, err)

	// Verify it's valid JSON
	var parsed RoadmapDetailItem
	err = json.Unmarshal([]byte(output), &parsed)
	require.NoError(t, err, "output should be valid JSON")

	// Verify required fields
	assert.Equal(t, "minimal-json", parsed.Slug)
	assert.Equal(t, "Minimal JSON", parsed.Title)
	assert.Equal(t, "lower", parsed.Priority)
	assert.Equal(t, "completed", parsed.Status)
	assert.Equal(t, "Minimal context", parsed.Context)

	// Verify optional fields are nil
	assert.Nil(t, parsed.PRUrl)
	assert.Nil(t, parsed.Branch)

	// Verify omitempty works (optional fields not in output)
	assert.NotContains(t, output, "pr_url")
	assert.NotContains(t, output, "branch")
}

func TestFormatRoadmapItem_TableMode(t *testing.T) {
	// Save and restore output mode
	originalMode := GetOutputMode()
	t.Cleanup(func() { SetOutputMode(originalMode) })

	SetOutputMode(TableMode)

	item := roadmap.RoadmapItem{
		Slug:     "confirm-item",
		Title:    "Confirmation Item",
		Priority: roadmap.PriorityHigh,
		Status:   roadmap.StatusPlanned,
		Context:  "Context for confirmation",
	}

	output, err := FormatRoadmapItem(item)
	require.NoError(t, err)

	// Verify brief confirmation format
	expected := "Added: confirm-item - Confirmation Item [high, planned]"
	assert.Equal(t, expected, output)
}

func TestFormatRoadmapItem_JSONMode(t *testing.T) {
	// Save and restore output mode
	originalMode := GetOutputMode()
	t.Cleanup(func() { SetOutputMode(originalMode) })

	SetOutputMode(JSONMode)

	item := roadmap.RoadmapItem{
		Slug:     "json-confirm",
		Title:    "JSON Confirmation",
		Priority: roadmap.PriorityMedium,
		Status:   roadmap.StatusInProgress,
		Context:  "JSON context",
	}

	output, err := FormatRoadmapItem(item)
	require.NoError(t, err)

	// Verify it's valid JSON
	var parsed RoadmapTableItem
	err = json.Unmarshal([]byte(output), &parsed)
	require.NoError(t, err, "output should be valid JSON")

	// Verify brief fields only
	assert.Equal(t, "json-confirm", parsed.Slug)
	assert.Equal(t, "JSON Confirmation", parsed.Title)
	assert.Equal(t, "medium", parsed.Priority)
	assert.Equal(t, "in_progress", parsed.Status)

	// Verify it doesn't contain context (brief output)
	assert.NotContains(t, output, "context")
}

func TestOutputModeIsolation(t *testing.T) {
	// Verify that changing mode in one test doesn't affect another
	// by testing save/restore pattern
	originalMode := GetOutputMode()
	t.Cleanup(func() { SetOutputMode(originalMode) })

	// Change mode
	SetOutputMode(JSONMode)
	assert.Equal(t, JSONMode, GetOutputMode())

	// Cleanup should restore
	SetOutputMode(originalMode)
	assert.Equal(t, originalMode, GetOutputMode())
}
