package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/abenz1267/muster/internal/coding"
	// "github.com/abenz1267/muster/internal/config"
	"github.com/abenz1267/muster/internal/roadmap"
	"github.com/abenz1267/muster/internal/ui"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockInteractiveToolFunc is a test helper that wraps a function to implement InteractiveCodingTool
type mockInteractiveToolFunc struct {
	fn func(context.Context, coding.InteractiveConfig) error
}

func (m mockInteractiveToolFunc) RunInteractive(ctx context.Context, cfg coding.InteractiveConfig) error {
	return m.fn(ctx, cfg)
}

// Test 1-5: Command structure

func TestPlanCommand_Exists(t *testing.T) {
	assert.NotNil(t, planCmd, "plan command should exist")
	assert.Equal(t, "plan [slug]", planCmd.Use, "command use should be 'plan [slug]'")
}

func TestPlanCommand_HasRunE(t *testing.T) {
	assert.NotNil(t, planCmd.RunE, "plan command should have RunE function")
}

func TestPlanCommand_AcceptsMaxOneArg(t *testing.T) {
	assert.NotNil(t, planCmd.Args, "plan command should have Args validator")

	// Test with no args
	err := planCmd.Args(planCmd, []string{})
	assert.NoError(t, err, "should accept 0 arguments")

	// Test with one arg
	err = planCmd.Args(planCmd, []string{"test-slug"})
	assert.NoError(t, err, "should accept 1 argument")

	// Test with too many args
	err = planCmd.Args(planCmd, []string{"slug1", "slug2"})
	assert.Error(t, err, "should reject 2 arguments")
}

func TestPlanCommand_ForceFlag(t *testing.T) {
	forceFlag := planCmd.Flags().Lookup("force")
	require.NotNil(t, forceFlag, "force flag should exist")
	assert.Equal(t, "bool", forceFlag.Value.Type(), "force should be bool")
	assert.Equal(t, "false", forceFlag.DefValue, "force default should be false")
	assert.NotEmpty(t, forceFlag.Usage, "force should have usage text")
}

func TestPlanCommand_VerboseFlag(t *testing.T) {
	// Verbose flag should be inherited from root command (persistent flag)
	verboseFlag := rootCmd.PersistentFlags().Lookup("verbose")
	require.NotNil(t, verboseFlag, "verbose flag should exist on root command")
	assert.Equal(t, "bool", verboseFlag.Value.Type(), "verbose should be bool")
	assert.Equal(t, "false", verboseFlag.DefValue, "verbose default should be false")
	assert.NotEmpty(t, verboseFlag.Usage, "verbose should have usage text")
}

// Test 6-9: Config/roadmap errors

func TestRunPlan_ProjectConfigParseError(t *testing.T) {
	// Create temp dir with malformed project config
	tmpDir := t.TempDir()

	// Create .muster directory
	musterDir := filepath.Join(tmpDir, ".muster")
	err := os.MkdirAll(musterDir, 0750) //nolint:gosec // G301: Test directory permissions
	require.NoError(t, err)

	// Write malformed config
	configPath := filepath.Join(musterDir, "config.yml")
	err = os.WriteFile(configPath, []byte("invalid: yaml: content: ["), 0600) //nolint:gosec // G306: Test file permissions
	require.NoError(t, err)

	// Create valid roadmap
	roadmapPath := filepath.Join(musterDir, "roadmap.json")
	err = os.WriteFile(roadmapPath, []byte(`{"items":[]}`), 0600) //nolint:gosec // G306: Test file permissions
	require.NoError(t, err)

	// Change to temp dir
	origDir, _ := os.Getwd()
	defer func() { _ = os.Chdir(origDir) }()
	_ = os.Chdir(tmpDir)

	// Create command
	cmd := &cobra.Command{}
	cmd.SetErr(new(bytes.Buffer))

	// Add flags
	cmd.Flags().Bool("force", false, "")
	cmd.Flags().Bool("verbose", false, "")

	// Run
	err = runPlan(cmd, []string{"test-slug"})

	// Should return config parse error
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "config file malformed")
}

func TestRunPlan_RoadmapParseError(t *testing.T) {
	// Create temp dir with malformed roadmap
	tmpDir := t.TempDir()

	// Create .muster directory
	musterDir := filepath.Join(tmpDir, ".muster")
	err := os.MkdirAll(musterDir, 0750) //nolint:gosec // G301: Test directory permissions
	require.NoError(t, err)

	// Write malformed roadmap
	roadmapPath := filepath.Join(musterDir, "roadmap.json")
	err = os.WriteFile(roadmapPath, []byte(`{"items": [invalid json]}`), 0600) //nolint:gosec // G306: Test file permissions
	require.NoError(t, err)

	// Change to temp dir
	origDir, _ := os.Getwd()
	defer func() { _ = os.Chdir(origDir) }()
	_ = os.Chdir(tmpDir)

	// Create command
	cmd := &cobra.Command{}
	cmd.SetErr(new(bytes.Buffer))

	// Add flags
	cmd.Flags().Bool("force", false, "")
	cmd.Flags().Bool("verbose", false, "")

	// Run
	err = runPlan(cmd, []string{"test-slug"})

	// Should return roadmap parse error
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "roadmap file is malformed")
}

func TestRunPlan_SlugNotFound(t *testing.T) {
	// Create temp dir with valid roadmap
	tmpDir := t.TempDir()

	// Create .muster directory
	musterDir := filepath.Join(tmpDir, ".muster")
	err := os.MkdirAll(musterDir, 0750) //nolint:gosec // G301: Test directory permissions
	require.NoError(t, err)

	// Write valid roadmap with one item
	roadmapPath := filepath.Join(musterDir, "roadmap.json")
	roadmapData := `{
		"items": [
			{
				"slug": "existing-slug",
				"title": "Existing Item",
				"priority": "medium",
				"status": "planned",
				"context": "Test context"
			}
		]
	}`
	err = os.WriteFile(roadmapPath, []byte(roadmapData), 0600) //nolint:gosec // G306: Test file permissions
	require.NoError(t, err)

	// Change to temp dir
	origDir, _ := os.Getwd()
	defer func() { _ = os.Chdir(origDir) }()
	_ = os.Chdir(tmpDir)

	// Create command
	cmd := &cobra.Command{}
	cmd.SetErr(new(bytes.Buffer))

	// Add flags
	cmd.Flags().Bool("force", false, "")
	cmd.Flags().Bool("verbose", false, "")

	// Run with non-existent slug
	err = runPlan(cmd, []string{"non-existent-slug"})

	// Should return not found error
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestRunPlan_EmptyRoadmap(t *testing.T) {
	// Create temp dir with empty roadmap
	tmpDir := t.TempDir()

	// Create .muster directory
	musterDir := filepath.Join(tmpDir, ".muster")
	err := os.MkdirAll(musterDir, 0750) //nolint:gosec // G301: Test directory permissions
	require.NoError(t, err)

	// Write empty roadmap
	roadmapPath := filepath.Join(musterDir, "roadmap.json")
	err = os.WriteFile(roadmapPath, []byte(`{"items":[]}`), 0600) //nolint:gosec // G306: Test file permissions
	require.NoError(t, err)

	// Change to temp dir
	origDir, _ := os.Getwd()
	defer func() { _ = os.Chdir(origDir) }()
	_ = os.Chdir(tmpDir)

	// Create command
	cmd := &cobra.Command{}
	cmd.SetErr(new(bytes.Buffer))

	// Add flags
	cmd.Flags().Bool("force", false, "")
	cmd.Flags().Bool("verbose", false, "")

	// Run without slug (non-interactive mode)
	err = runPlan(cmd, []string{})

	// Should return error about empty roadmap or non-interactive mode
	assert.Error(t, err)
}

// Test 14-20: Slug resolution

func TestResolveSlug_ArgumentMode_Found(t *testing.T) {
	rm := &roadmap.Roadmap{
		Items: []roadmap.RoadmapItem{
			{
				Slug:     "test-slug",
				Title:    "Test Item",
				Priority: roadmap.PriorityMedium,
				Status:   roadmap.StatusPlanned,
				Context:  "Test context",
			},
		},
	}

	var buf bytes.Buffer
	slug, item, err := resolveSlug([]string{"test-slug"}, rm, false, &buf, nil)

	assert.NoError(t, err)
	assert.Equal(t, "test-slug", slug)
	assert.NotNil(t, item)
	assert.Equal(t, "Test Item", item.Title)
}

func TestResolveSlug_ArgumentMode_NotFound(t *testing.T) {
	rm := &roadmap.Roadmap{
		Items: []roadmap.RoadmapItem{
			{
				Slug:     "test-slug",
				Title:    "Test Item",
				Priority: roadmap.PriorityMedium,
				Status:   roadmap.StatusPlanned,
				Context:  "Test context",
			},
		},
	}

	var buf bytes.Buffer
	_, _, err := resolveSlug([]string{"non-existent"}, rm, false, &buf, nil)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestResolveSlug_ArgumentMode_CompletedWarning(t *testing.T) {
	rm := &roadmap.Roadmap{
		Items: []roadmap.RoadmapItem{
			{
				Slug:     "completed-slug",
				Title:    "Completed Item",
				Priority: roadmap.PriorityMedium,
				Status:   roadmap.StatusCompleted,
				Context:  "Test context",
			},
		},
	}

	var buf bytes.Buffer
	slug, item, err := resolveSlug([]string{"completed-slug"}, rm, true, &buf, nil)

	assert.NoError(t, err)
	assert.Equal(t, "completed-slug", slug)
	assert.NotNil(t, item)
	assert.Contains(t, buf.String(), "Warning")
	assert.Contains(t, buf.String(), "completed")
}

func TestResolveSlug_PickerMode_NonInteractive(t *testing.T) {
	rm := &roadmap.Roadmap{
		Items: []roadmap.RoadmapItem{
			{
				Slug:     "test-slug",
				Title:    "Test Item",
				Priority: roadmap.PriorityMedium,
				Status:   roadmap.StatusPlanned,
				Context:  "Test context",
			},
		},
	}

	var buf bytes.Buffer
	_, _, err := resolveSlug([]string{}, rm, false, &buf, nil)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "non-interactive")
}

func TestResolveSlug_PickerMode_EmptyRoadmap(t *testing.T) {
	rm := &roadmap.Roadmap{
		Items: []roadmap.RoadmapItem{},
	}

	var buf bytes.Buffer
	_, _, err := resolveSlug([]string{}, rm, true, &buf, nil)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "empty")
}

func TestResolveSlug_PickerMode_AllCompleted(t *testing.T) {
	rm := &roadmap.Roadmap{
		Items: []roadmap.RoadmapItem{
			{
				Slug:     "completed-1",
				Title:    "Completed Item 1",
				Priority: roadmap.PriorityMedium,
				Status:   roadmap.StatusCompleted,
				Context:  "Test context",
			},
			{
				Slug:     "completed-2",
				Title:    "Completed Item 2",
				Priority: roadmap.PriorityMedium,
				Status:   roadmap.StatusCompleted,
				Context:  "Test context",
			},
		},
	}

	var buf bytes.Buffer
	_, _, err := resolveSlug([]string{}, rm, true, &buf, nil)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no non-completed items")
}

// MockPicker for testing picker interactions
type MockPicker struct {
	ReturnValue     string
	ReturnError     error
	CapturedTitle   string
	CapturedOptions []ui.PickerOption
}

func (m *MockPicker) Show(title string, options []ui.PickerOption, cfg ui.PickerConfig) (string, error) {
	m.CapturedTitle = title
	m.CapturedOptions = options
	if m.ReturnError != nil {
		return "", m.ReturnError
	}
	return m.ReturnValue, nil
}

func TestResolveSlug_PickerMode_Success(t *testing.T) {
	rm := &roadmap.Roadmap{
		Items: []roadmap.RoadmapItem{
			{
				Slug:     "high-priority",
				Title:    "High Priority Item",
				Priority: roadmap.PriorityHigh,
				Status:   roadmap.StatusPlanned,
				Context:  "Test context",
			},
			{
				Slug:     "medium-priority",
				Title:    "Medium Priority Item",
				Priority: roadmap.PriorityMedium,
				Status:   roadmap.StatusPlanned,
				Context:  "Test context",
			},
			{
				Slug:     "completed-item",
				Title:    "Completed Item",
				Priority: roadmap.PriorityHigh,
				Status:   roadmap.StatusCompleted,
				Context:  "Test context",
			},
		},
	}

	mockPicker := &MockPicker{
		ReturnValue: "medium-priority",
	}

	var buf bytes.Buffer
	slug, item, err := resolveSlug([]string{}, rm, true, &buf, mockPicker)

	assert.NoError(t, err)
	assert.Equal(t, "medium-priority", slug)
	assert.NotNil(t, item)
	assert.Equal(t, "Medium Priority Item", item.Title)

	// Verify picker was called correctly
	assert.Equal(t, "Select a roadmap item to plan:", mockPicker.CapturedTitle)
	assert.Len(t, mockPicker.CapturedOptions, 2, "should filter out completed items")

	// Verify sorting: high priority first
	assert.Equal(t, "high-priority", mockPicker.CapturedOptions[0].Value)
	assert.Equal(t, "medium-priority", mockPicker.CapturedOptions[1].Value)
}

func TestResolveSlug_PickerMode_PrioritySorting(t *testing.T) {
	rm := &roadmap.Roadmap{
		Items: []roadmap.RoadmapItem{
			{
				Slug:     "lower-priority",
				Title:    "Lower Priority",
				Priority: roadmap.PriorityLower,
				Status:   roadmap.StatusPlanned,
				Context:  "Test context",
			},
			{
				Slug:     "high-priority",
				Title:    "High Priority",
				Priority: roadmap.PriorityHigh,
				Status:   roadmap.StatusPlanned,
				Context:  "Test context",
			},
			{
				Slug:     "low-priority",
				Title:    "Low Priority",
				Priority: roadmap.PriorityLow,
				Status:   roadmap.StatusPlanned,
				Context:  "Test context",
			},
			{
				Slug:     "medium-priority",
				Title:    "Medium Priority",
				Priority: roadmap.PriorityMedium,
				Status:   roadmap.StatusPlanned,
				Context:  "Test context",
			},
		},
	}

	mockPicker := &MockPicker{
		ReturnValue: "high-priority",
	}

	var buf bytes.Buffer
	_, _, err := resolveSlug([]string{}, rm, true, &buf, mockPicker)

	assert.NoError(t, err)

	// Verify priority order: high, medium, low, lower
	require.Len(t, mockPicker.CapturedOptions, 4)
	assert.Equal(t, "high-priority", mockPicker.CapturedOptions[0].Value)
	assert.Equal(t, "medium-priority", mockPicker.CapturedOptions[1].Value)
	assert.Equal(t, "low-priority", mockPicker.CapturedOptions[2].Value)
	assert.Equal(t, "lower-priority", mockPicker.CapturedOptions[3].Value)
}

func TestResolveSlug_PickerMode_AlphabeticalSorting(t *testing.T) {
	rm := &roadmap.Roadmap{
		Items: []roadmap.RoadmapItem{
			{
				Slug:     "zebra",
				Title:    "Zebra Item",
				Priority: roadmap.PriorityMedium,
				Status:   roadmap.StatusPlanned,
				Context:  "Test context",
			},
			{
				Slug:     "apple",
				Title:    "Apple Item",
				Priority: roadmap.PriorityMedium,
				Status:   roadmap.StatusPlanned,
				Context:  "Test context",
			},
			{
				Slug:     "banana",
				Title:    "Banana Item",
				Priority: roadmap.PriorityMedium,
				Status:   roadmap.StatusPlanned,
				Context:  "Test context",
			},
		},
	}

	mockPicker := &MockPicker{
		ReturnValue: "apple",
	}

	var buf bytes.Buffer
	_, _, err := resolveSlug([]string{}, rm, true, &buf, mockPicker)

	assert.NoError(t, err)

	// Verify alphabetical order within same priority
	require.Len(t, mockPicker.CapturedOptions, 3)
	assert.Equal(t, "apple", mockPicker.CapturedOptions[0].Value)
	assert.Equal(t, "banana", mockPicker.CapturedOptions[1].Value)
	assert.Equal(t, "zebra", mockPicker.CapturedOptions[2].Value)
}

func TestResolveSlug_PickerMode_LabelFormat(t *testing.T) {
	rm := &roadmap.Roadmap{
		Items: []roadmap.RoadmapItem{
			{
				Slug:     "test-slug",
				Title:    "Test Item",
				Priority: roadmap.PriorityHigh,
				Status:   roadmap.StatusInProgress,
				Context:  "Test context",
			},
		},
	}

	mockPicker := &MockPicker{
		ReturnValue: "test-slug",
	}

	var buf bytes.Buffer
	_, _, err := resolveSlug([]string{}, rm, true, &buf, mockPicker)

	assert.NoError(t, err)

	// Verify label format: "{slug} - {title} [{priority}, {status}]"
	require.Len(t, mockPicker.CapturedOptions, 1)
	expectedLabel := "test-slug - Test Item [high, in_progress]"
	assert.Equal(t, expectedLabel, mockPicker.CapturedOptions[0].Label)
}

func TestResolveSlug_PickerMode_Error(t *testing.T) {
	rm := &roadmap.Roadmap{
		Items: []roadmap.RoadmapItem{
			{
				Slug:     "test-slug",
				Title:    "Test Item",
				Priority: roadmap.PriorityMedium,
				Status:   roadmap.StatusPlanned,
				Context:  "Test context",
			},
		},
	}

	mockPicker := &MockPicker{
		ReturnError: errors.New("picker cancelled"),
	}

	var buf bytes.Buffer
	_, _, err := resolveSlug([]string{}, rm, true, &buf, mockPicker)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to select item")
}

// Test ensurePlanDir

func TestEnsurePlanDir_CreatesStructure(t *testing.T) {
	tmpDir := t.TempDir()

	planDir, err := ensurePlanDir(tmpDir, "test-slug")

	assert.NoError(t, err)
	assert.NotEmpty(t, planDir)

	// Verify absolute path
	assert.True(t, filepath.IsAbs(planDir), "should return absolute path")

	// Verify directory structure exists
	assert.DirExists(t, planDir)
	assert.DirExists(t, filepath.Join(planDir, "research"))
	assert.DirExists(t, filepath.Join(planDir, "synthesis"))

	// Verify path contains expected components
	assert.Contains(t, planDir, ".muster")
	assert.Contains(t, planDir, "work")
	assert.Contains(t, planDir, "test-slug")
	assert.Contains(t, planDir, "plan")
}

func TestEnsurePlanDir_Idempotent(t *testing.T) {
	tmpDir := t.TempDir()

	// Create once
	planDir1, err1 := ensurePlanDir(tmpDir, "test-slug")
	assert.NoError(t, err1)

	// Create again
	planDir2, err2 := ensurePlanDir(tmpDir, "test-slug")
	assert.NoError(t, err2)

	// Should return same path
	assert.Equal(t, planDir1, planDir2)

	// Verify structure still exists
	assert.DirExists(t, planDir2)
	assert.DirExists(t, filepath.Join(planDir2, "research"))
	assert.DirExists(t, filepath.Join(planDir2, "synthesis"))
}

func TestEnsurePlanDir_DifferentSlugs(t *testing.T) {
	tmpDir := t.TempDir()

	planDir1, err1 := ensurePlanDir(tmpDir, "slug-1")
	assert.NoError(t, err1)

	planDir2, err2 := ensurePlanDir(tmpDir, "slug-2")
	assert.NoError(t, err2)

	// Should create different directories
	assert.NotEqual(t, planDir1, planDir2)
	assert.Contains(t, planDir1, "slug-1")
	assert.Contains(t, planDir2, "slug-2")

	// Both should exist
	assert.DirExists(t, planDir1)
	assert.DirExists(t, planDir2)
}

func TestEnsurePlanDir_NestedSlug(t *testing.T) {
	tmpDir := t.TempDir()

	// Test with slug containing hyphens (common case)
	planDir, err := ensurePlanDir(tmpDir, "feature-nested-item-123")
	assert.NoError(t, err)
	assert.DirExists(t, planDir)
	assert.Contains(t, planDir, "feature-nested-item-123")
}

// Test 21-30: Skill staging, invocation, and verification

func TestRunPlan_SkillStagingSuccess(t *testing.T) {
	// Create temp dir with valid roadmap
	tmpDir := t.TempDir()

	// Create .muster directory
	musterDir := filepath.Join(tmpDir, ".muster")
	err := os.MkdirAll(musterDir, 0750) //nolint:gosec // G301: Test directory permissions
	require.NoError(t, err)

	// Write valid roadmap
	roadmapPath := filepath.Join(musterDir, "roadmap.json")
	roadmapData := `{
		"items": [
			{
				"slug": "test-feature",
				"title": "Test Feature",
				"priority": "medium",
				"status": "planned",
				"context": "Test context"
			}
		]
	}`
	err = os.WriteFile(roadmapPath, []byte(roadmapData), 0600) //nolint:gosec // G306: Test file permissions
	require.NoError(t, err)

	// Change to temp dir
	origDir, _ := os.Getwd()
	defer func() { _ = os.Chdir(origDir) }()
	_ = os.Chdir(tmpDir)

	// Mock interactiveToolFactory to write stub plan file
	origFactory := interactiveToolFactory
	defer func() { interactiveToolFactory = origFactory }()

	var capturedTmpDir string
	interactiveToolFactory = func() (coding.InteractiveCodingTool, error) {
		return mockInteractiveToolFunc{fn: func(ctx context.Context, cfg coding.InteractiveConfig) error {
			capturedTmpDir = cfg.PluginDir
			// Write stub plan file
			cwd, _ := os.Getwd()
			planDir := filepath.Join(cwd, ".muster", "work", "test-feature", "plan")
			planPath := filepath.Join(planDir, "implementation-plan.md")
			return os.WriteFile(planPath, []byte("# Test Plan\n"), 0600) //nolint:gosec // G306: Test file permissions
		}}, nil
	}

	// Create command
	cmd := &cobra.Command{}
	cmd.SetErr(new(bytes.Buffer))

	// Add flags
	cmd.Flags().Bool("force", false, "")
	cmd.Flags().Bool("verbose", false, "")

	// Run
	err = runPlan(cmd, []string{"test-feature"})

	// Should succeed
	assert.NoError(t, err)

	// Verify tmpDir was passed to invoker
	assert.NotEmpty(t, capturedTmpDir)
	assert.Contains(t, capturedTmpDir, "muster-prompts-")
}

func TestRunPlan_CleanupRunsOnSuccess(t *testing.T) {
	// Create temp dir with valid roadmap
	tmpDir := t.TempDir()

	// Create .muster directory
	musterDir := filepath.Join(tmpDir, ".muster")
	err := os.MkdirAll(musterDir, 0750) //nolint:gosec // G301: Test directory permissions
	require.NoError(t, err)

	// Write valid roadmap
	roadmapPath := filepath.Join(musterDir, "roadmap.json")
	roadmapData := `{
		"items": [
			{
				"slug": "test-cleanup",
				"title": "Test Cleanup",
				"priority": "medium",
				"status": "planned",
				"context": "Test context"
			}
		]
	}`
	err = os.WriteFile(roadmapPath, []byte(roadmapData), 0600) //nolint:gosec // G306: Test file permissions
	require.NoError(t, err)

	// Change to temp dir
	origDir, _ := os.Getwd()
	defer func() { _ = os.Chdir(origDir) }()
	_ = os.Chdir(tmpDir)

	// Mock interactiveToolFactory
	origFactory := interactiveToolFactory
	defer func() { interactiveToolFactory = origFactory }()

	var capturedTmpDir string
	interactiveToolFactory = func() (coding.InteractiveCodingTool, error) {
		return mockInteractiveToolFunc{fn: func(ctx context.Context, cfg coding.InteractiveConfig) error {
			capturedTmpDir = cfg.PluginDir
			// Write stub plan file
			cwd, _ := os.Getwd()
			planDir := filepath.Join(cwd, ".muster", "work", "test-cleanup", "plan")
			planPath := filepath.Join(planDir, "implementation-plan.md")
			return os.WriteFile(planPath, []byte("# Test Plan\n"), 0600) //nolint:gosec // G306: Test file permissions
		}}, nil
	}

	// Create command
	cmd := &cobra.Command{}
	cmd.SetErr(new(bytes.Buffer))

	// Add flags
	cmd.Flags().Bool("force", false, "")
	cmd.Flags().Bool("verbose", false, "")

	// Run
	err = runPlan(cmd, []string{"test-cleanup"})
	assert.NoError(t, err)

	// Verify tmpDir no longer exists (cleanup ran)
	_, err = os.Stat(capturedTmpDir)
	assert.True(t, os.IsNotExist(err), "tmpDir should be cleaned up after success")
}

func TestRunPlan_CleanupRunsOnError(t *testing.T) {
	// Create temp dir with valid roadmap
	tmpDir := t.TempDir()

	// Create .muster directory
	musterDir := filepath.Join(tmpDir, ".muster")
	err := os.MkdirAll(musterDir, 0750) //nolint:gosec // G301: Test directory permissions
	require.NoError(t, err)

	// Write valid roadmap
	roadmapPath := filepath.Join(musterDir, "roadmap.json")
	roadmapData := `{
		"items": [
			{
				"slug": "test-error",
				"title": "Test Error",
				"priority": "medium",
				"status": "planned",
				"context": "Test context"
			}
		]
	}`
	err = os.WriteFile(roadmapPath, []byte(roadmapData), 0600) //nolint:gosec // G306: Test file permissions
	require.NoError(t, err)

	// Change to temp dir
	origDir, _ := os.Getwd()
	defer func() { _ = os.Chdir(origDir) }()
	_ = os.Chdir(tmpDir)

	// Mock interactiveToolFactory to fail
	origFactory := interactiveToolFactory
	defer func() { interactiveToolFactory = origFactory }()

	var capturedTmpDir string
	interactiveToolFactory = func() (coding.InteractiveCodingTool, error) {
		return mockInteractiveToolFunc{fn: func(ctx context.Context, cfg coding.InteractiveConfig) error {
			capturedTmpDir = cfg.PluginDir
			return fmt.Errorf("invocation failed")
		}}, nil
	}

	// Create command
	cmd := &cobra.Command{}
	cmd.SetErr(new(bytes.Buffer))

	// Add flags
	cmd.Flags().Bool("force", false, "")
	cmd.Flags().Bool("verbose", false, "")

	// Run
	err = runPlan(cmd, []string{"test-error"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invocation failed")

	// Verify tmpDir no longer exists (cleanup ran even on error)
	_, err = os.Stat(capturedTmpDir)
	assert.True(t, os.IsNotExist(err), "tmpDir should be cleaned up even on error")
}

func TestRunPlan_PromptContextParameters(t *testing.T) {
	// Create temp dir with valid roadmap
	tmpDir := t.TempDir()

	// Create .muster directory
	musterDir := filepath.Join(tmpDir, ".muster")
	err := os.MkdirAll(musterDir, 0750) //nolint:gosec // G301: Test directory permissions
	require.NoError(t, err)

	// Write valid roadmap
	roadmapPath := filepath.Join(musterDir, "roadmap.json")
	roadmapData := `{
		"items": [
			{
				"slug": "test-context",
				"title": "Test Context",
				"priority": "medium",
				"status": "planned",
				"context": "Test context"
			}
		]
	}`
	err = os.WriteFile(roadmapPath, []byte(roadmapData), 0600) //nolint:gosec // G306: Test file permissions
	require.NoError(t, err)

	// Change to temp dir
	origDir, _ := os.Getwd()
	defer func() { _ = os.Chdir(origDir) }()
	_ = os.Chdir(tmpDir)

	// Mock interactiveToolFactory
	origFactory := interactiveToolFactory
	defer func() { interactiveToolFactory = origFactory }()

	interactiveToolFactory = func() (coding.InteractiveCodingTool, error) {
		return mockInteractiveToolFunc{fn: func(ctx context.Context, cfg coding.InteractiveConfig) error {
			// Write stub plan file
			cwd, _ := os.Getwd()
			planDir := filepath.Join(cwd, ".muster", "work", "test-context", "plan")
			planPath := filepath.Join(planDir, "implementation-plan.md")
			return os.WriteFile(planPath, []byte("# Test Plan\n"), 0600) //nolint:gosec // G306: Test file permissions
		}}, nil
	}

	// Create command
	cmd := &cobra.Command{}
	cmd.SetErr(new(bytes.Buffer))

	// Add flags
	cmd.Flags().Bool("force", false, "")
	cmd.Flags().Bool("verbose", false, "")

	// Run - this test mainly verifies the code compiles and runs with correct context parameters
	err = runPlan(cmd, []string{"test-context"})
	assert.NoError(t, err)

	// Verify plan directory was created with correct slug
	cwd, _ := os.Getwd()
	planDir := filepath.Join(cwd, ".muster", "work", "test-context", "plan")
	assert.DirExists(t, planDir)
}

func TestRunPlan_CommandConstruction(t *testing.T) {
	// This test verifies the command construction with interactiveToolFactory
	// We'll call the default factory and verify it returns a tool

	// Call interactiveToolFactory
	tool, err := interactiveToolFactory()

	// Should succeed in creating the tool
	assert.NoError(t, err)
	assert.NotNil(t, tool)
}

func TestRunPlan_EnvOverridesApplied(t *testing.T) {
	// Create temp dir with valid roadmap and project config with provider override
	tmpDir := t.TempDir()

	// Create .muster directory
	musterDir := filepath.Join(tmpDir, ".muster")
	err := os.MkdirAll(musterDir, 0750) //nolint:gosec // G301: Test directory permissions
	require.NoError(t, err)

	// Write project config with provider base_url
	configPath := filepath.Join(musterDir, "config.yml")
	configData := `
providers:
  anthropic:
    base_url: "http://localhost:8000"
`
	err = os.WriteFile(configPath, []byte(configData), 0600) //nolint:gosec // G306: Test file permissions
	require.NoError(t, err)

	// Write valid roadmap
	roadmapPath := filepath.Join(musterDir, "roadmap.json")
	roadmapData := `{
		"items": [
			{
				"slug": "test-env",
				"title": "Test Env",
				"priority": "medium",
				"status": "planned",
				"context": "Test context"
			}
		]
	}`
	err = os.WriteFile(roadmapPath, []byte(roadmapData), 0600) //nolint:gosec // G306: Test file permissions
	require.NoError(t, err)

	// Change to temp dir
	origDir, _ := os.Getwd()
	defer func() { _ = os.Chdir(origDir) }()
	_ = os.Chdir(tmpDir)

	// Mock interactiveToolFactory to capture environment
	origFactory := interactiveToolFactory
	defer func() { interactiveToolFactory = origFactory }()

	var capturedEnv bool
	interactiveToolFactory = func() (coding.InteractiveCodingTool, error) {
		return mockInteractiveToolFunc{fn: func(ctx context.Context, cfg coding.InteractiveConfig) error {
			// Verify env overrides are passed correctly
			if baseURL, ok := cfg.Env["ANTHROPIC_BASE_URL"]; ok && baseURL == "http://localhost:8000" {
				capturedEnv = true
			}
			// Write stub plan file
			cwd, _ := os.Getwd()
			planDir := filepath.Join(cwd, ".muster", "work", "test-env", "plan")
			planPath := filepath.Join(planDir, "implementation-plan.md")
			return os.WriteFile(planPath, []byte("# Test Plan\n"), 0600) //nolint:gosec // G306: Test file permissions
		}}, nil
	}

	// Create command
	cmd := &cobra.Command{}
	cmd.SetErr(new(bytes.Buffer))

	// Add flags
	cmd.Flags().Bool("force", false, "")
	cmd.Flags().Bool("verbose", false, "")

	// Run
	err = runPlan(cmd, []string{"test-env"})
	assert.NoError(t, err)

	// Verify env override was captured
	assert.True(t, capturedEnv, "env overrides should include ANTHROPIC_BASE_URL")
}

func TestRunPlan_VerificationMissingFile(t *testing.T) {
	// Create temp dir with valid roadmap
	tmpDir := t.TempDir()

	// Create .muster directory
	musterDir := filepath.Join(tmpDir, ".muster")
	err := os.MkdirAll(musterDir, 0750) //nolint:gosec // G301: Test directory permissions
	require.NoError(t, err)

	// Write valid roadmap
	roadmapPath := filepath.Join(musterDir, "roadmap.json")
	roadmapData := `{
		"items": [
			{
				"slug": "test-verify",
				"title": "Test Verify",
				"priority": "medium",
				"status": "planned",
				"context": "Test context"
			}
		]
	}`
	err = os.WriteFile(roadmapPath, []byte(roadmapData), 0600) //nolint:gosec // G306: Test file permissions
	require.NoError(t, err)

	// Change to temp dir
	origDir, _ := os.Getwd()
	defer func() { _ = os.Chdir(origDir) }()
	_ = os.Chdir(tmpDir)

	// Mock interactiveToolFactory to NOT write plan file
	origFactory := interactiveToolFactory
	defer func() { interactiveToolFactory = origFactory }()

	interactiveToolFactory = func() (coding.InteractiveCodingTool, error) {
		return mockInteractiveToolFunc{fn: func(ctx context.Context, cfg coding.InteractiveConfig) error {
			// Don't write plan file - verification should fail
			return nil
		}}, nil
	}

	// Create command
	cmd := &cobra.Command{}
	cmd.SetErr(new(bytes.Buffer))

	// Add flags
	cmd.Flags().Bool("force", false, "")
	cmd.Flags().Bool("verbose", false, "")

	// Run
	err = runPlan(cmd, []string{"test-verify"})

	// Should fail with verification error
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "implementation-plan.md was not created")
}

func TestRunPlan_VerificationEmptyFile(t *testing.T) {
	// Create temp dir with valid roadmap
	tmpDir := t.TempDir()

	// Create .muster directory
	musterDir := filepath.Join(tmpDir, ".muster")
	err := os.MkdirAll(musterDir, 0750) //nolint:gosec // G301: Test directory permissions
	require.NoError(t, err)

	// Write valid roadmap
	roadmapPath := filepath.Join(musterDir, "roadmap.json")
	roadmapData := `{
		"items": [
			{
				"slug": "test-empty",
				"title": "Test Empty",
				"priority": "medium",
				"status": "planned",
				"context": "Test context"
			}
		]
	}`
	err = os.WriteFile(roadmapPath, []byte(roadmapData), 0600) //nolint:gosec // G306: Test file permissions
	require.NoError(t, err)

	// Change to temp dir
	origDir, _ := os.Getwd()
	defer func() { _ = os.Chdir(origDir) }()
	_ = os.Chdir(tmpDir)

	// Mock interactiveToolFactory to write empty plan file
	origFactory := interactiveToolFactory
	defer func() { interactiveToolFactory = origFactory }()

	interactiveToolFactory = func() (coding.InteractiveCodingTool, error) {
		return mockInteractiveToolFunc{fn: func(ctx context.Context, cfg coding.InteractiveConfig) error {
			// Write empty plan file
			cwd, _ := os.Getwd()
			planDir := filepath.Join(cwd, ".muster", "work", "test-empty", "plan")
			planPath := filepath.Join(planDir, "implementation-plan.md")
			return os.WriteFile(planPath, []byte(""), 0600) //nolint:gosec // G306: Test file permissions
		}}, nil
	}

	// Create command
	cmd := &cobra.Command{}
	cmd.SetErr(new(bytes.Buffer))

	// Add flags
	cmd.Flags().Bool("force", false, "")
	cmd.Flags().Bool("verbose", false, "")

	// Run
	err = runPlan(cmd, []string{"test-empty"})

	// Should fail with empty file error
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "plan file exists but is empty")
}

func TestRunPlan_VerificationSuccess(t *testing.T) {
	// Create temp dir with valid roadmap
	tmpDir := t.TempDir()

	// Create .muster directory
	musterDir := filepath.Join(tmpDir, ".muster")
	err := os.MkdirAll(musterDir, 0750) //nolint:gosec // G301: Test directory permissions
	require.NoError(t, err)

	// Write valid roadmap
	roadmapPath := filepath.Join(musterDir, "roadmap.json")
	roadmapData := `{
		"items": [
			{
				"slug": "test-success",
				"title": "Test Success",
				"priority": "medium",
				"status": "planned",
				"context": "Test context"
			}
		]
	}`
	err = os.WriteFile(roadmapPath, []byte(roadmapData), 0600) //nolint:gosec // G306: Test file permissions
	require.NoError(t, err)

	// Change to temp dir
	origDir, _ := os.Getwd()
	defer func() { _ = os.Chdir(origDir) }()
	_ = os.Chdir(tmpDir)

	// Mock interactiveToolFactory to write valid plan file
	origFactory := interactiveToolFactory
	defer func() { interactiveToolFactory = origFactory }()

	interactiveToolFactory = func() (coding.InteractiveCodingTool, error) {
		return mockInteractiveToolFunc{fn: func(ctx context.Context, cfg coding.InteractiveConfig) error {
			// Write valid plan file
			cwd, _ := os.Getwd()
			planDir := filepath.Join(cwd, ".muster", "work", "test-success", "plan")
			planPath := filepath.Join(planDir, "implementation-plan.md")
			return os.WriteFile(planPath, []byte("# Implementation Plan\n\nThis is a valid plan.\n"), 0600) //nolint:gosec // G306: Test file permissions
		}}, nil
	}

	// Create command
	cmd := &cobra.Command{}
	cmd.SetErr(new(bytes.Buffer))

	// Add flags
	cmd.Flags().Bool("force", false, "")
	cmd.Flags().Bool("verbose", false, "")

	// Run
	err = runPlan(cmd, []string{"test-success"})

	// Should succeed
	assert.NoError(t, err)

	// Verify plan file exists and has content
	cwd, _ := os.Getwd()
	planPath := filepath.Join(cwd, ".muster", "work", "test-success", "plan", "implementation-plan.md")
	assert.FileExists(t, planPath)
	content, err := os.ReadFile(planPath) //nolint:gosec // G304: Test file in controlled test directory
	assert.NoError(t, err)
	assert.Contains(t, string(content), "Implementation Plan")
}

// Test 31-35: Directory creation, permissions, overwrite behavior

func TestEnsurePlanDir_CreatesAllSubdirectories(t *testing.T) {
	tmpDir := t.TempDir()

	planDir, err := ensurePlanDir(tmpDir, "test-slug")
	assert.NoError(t, err)

	// Verify all subdirectories exist
	assert.DirExists(t, planDir)
	assert.DirExists(t, filepath.Join(planDir, "research"))
	assert.DirExists(t, filepath.Join(planDir, "synthesis"))

	// Verify path structure
	assert.Contains(t, planDir, ".muster")
	assert.Contains(t, planDir, "work")
	assert.Contains(t, planDir, "test-slug")
	assert.Contains(t, planDir, "plan")
}

func TestEnsurePlanDir_Permissions(t *testing.T) {
	tmpDir := t.TempDir()

	planDir, err := ensurePlanDir(tmpDir, "test-perms")
	assert.NoError(t, err)

	// Verify directory permissions are 0755
	info, err := os.Stat(planDir)
	assert.NoError(t, err)
	assert.Equal(t, os.FileMode(0755)|os.ModeDir, info.Mode())

	// Verify subdirectory permissions
	researchInfo, err := os.Stat(filepath.Join(planDir, "research"))
	assert.NoError(t, err)
	assert.Equal(t, os.FileMode(0755)|os.ModeDir, researchInfo.Mode())

	synthesisInfo, err := os.Stat(filepath.Join(planDir, "synthesis"))
	assert.NoError(t, err)
	assert.Equal(t, os.FileMode(0755)|os.ModeDir, synthesisInfo.Mode())
}

func TestRunPlan_ExistingPlanInteractiveYes(t *testing.T) {
	tmpDir := t.TempDir()

	// Create .muster directory
	musterDir := filepath.Join(tmpDir, ".muster")
	err := os.MkdirAll(musterDir, 0750) //nolint:gosec // G301: Test directory permissions
	require.NoError(t, err)

	// Write valid roadmap
	roadmapPath := filepath.Join(musterDir, "roadmap.json")
	roadmapData := `{
		"items": [
			{
				"slug": "test-overwrite-yes",
				"title": "Test Overwrite Yes",
				"priority": "medium",
				"status": "planned",
				"context": "Test context"
			}
		]
	}`
	err = os.WriteFile(roadmapPath, []byte(roadmapData), 0600) //nolint:gosec // G306: Test file permissions
	require.NoError(t, err)

	// Create existing plan file
	planDir := filepath.Join(musterDir, "work", "test-overwrite-yes", "plan")
	err = os.MkdirAll(planDir, 0750) //nolint:gosec // G301: Test directory permissions
	require.NoError(t, err)
	existingPlanPath := filepath.Join(planDir, "implementation-plan.md")
	err = os.WriteFile(existingPlanPath, []byte("# Old Plan\n"), 0600) //nolint:gosec // G306: Test file permissions
	require.NoError(t, err)

	// Change to temp dir
	origDir, _ := os.Getwd()
	defer func() { _ = os.Chdir(origDir) }()
	_ = os.Chdir(tmpDir)

	// Mock interactiveToolFactory to write new plan file
	origFactory := interactiveToolFactory
	defer func() { interactiveToolFactory = origFactory }()

	interactiveToolFactory = func() (coding.InteractiveCodingTool, error) {
		return mockInteractiveToolFunc{fn: func(ctx context.Context, cfg coding.InteractiveConfig) error {
			cwd, _ := os.Getwd()
			planDir := filepath.Join(cwd, ".muster", "work", "test-overwrite-yes", "plan")
			planPath := filepath.Join(planDir, "implementation-plan.md")
			return os.WriteFile(planPath, []byte("# New Plan\n"), 0600) //nolint:gosec // G306: Test file permissions
		}}, nil
	}

	// Create command with simulated stdin
	cmd := &cobra.Command{}
	cmd.SetErr(new(bytes.Buffer))

	// Simulate user input "y\n"
	stdin := strings.NewReader("y\n")

	// Add flags
	cmd.Flags().Bool("force", false, "")
	cmd.Flags().Bool("verbose", false, "")

	// We need to test the confirmOverwrite function directly since we can't easily mock stdin in runPlan
	cwd, _ := os.Getwd()
	planPath := filepath.Join(cwd, ".muster", "work", "test-overwrite-yes", "plan", "implementation-plan.md")
	overwrite, err := confirmOverwrite(planPath, true, stdin, cmd.ErrOrStderr(), false, false)
	assert.NoError(t, err)
	assert.True(t, overwrite)
}

func TestRunPlan_ExistingPlanInteractiveNo(t *testing.T) {
	tmpDir := t.TempDir()

	// Create .muster directory
	musterDir := filepath.Join(tmpDir, ".muster")
	err := os.MkdirAll(musterDir, 0750) //nolint:gosec // G301: Test directory permissions
	require.NoError(t, err)

	// Create existing plan file
	planDir := filepath.Join(musterDir, "work", "test-overwrite-no", "plan")
	err = os.MkdirAll(planDir, 0750) //nolint:gosec // G301: Test directory permissions
	require.NoError(t, err)
	existingPlanPath := filepath.Join(planDir, "implementation-plan.md")
	err = os.WriteFile(existingPlanPath, []byte("# Old Plan\n"), 0600) //nolint:gosec // G306: Test file permissions
	require.NoError(t, err)

	// Simulate user input "n\n"
	stdin := strings.NewReader("n\n")

	// Test confirmOverwrite function
	var buf bytes.Buffer
	overwrite, err := confirmOverwrite(existingPlanPath, true, stdin, &buf, false, false)
	assert.NoError(t, err)
	assert.False(t, overwrite)
	assert.Contains(t, buf.String(), "Plan already exists")
}

func TestRunPlan_ExistingPlanNonInteractiveNoForce_Errors(t *testing.T) {
	// Test confirmOverwrite function with non-interactive and no force
	stdin := strings.NewReader("")
	var buf bytes.Buffer
	planPath := "/test/plan.md"

	overwrite, err := confirmOverwrite(planPath, false, stdin, &buf, false, false)
	assert.Error(t, err)
	assert.False(t, overwrite)
	assert.Contains(t, err.Error(), "non-interactive mode")
	assert.Contains(t, err.Error(), planPath)
}

func TestRunPlan_ExistingPlanWithForce(t *testing.T) {
	tmpDir := t.TempDir()

	// Create .muster directory
	musterDir := filepath.Join(tmpDir, ".muster")
	err := os.MkdirAll(musterDir, 0750) //nolint:gosec // G301: Test directory permissions
	require.NoError(t, err)

	// Write valid roadmap
	roadmapPath := filepath.Join(musterDir, "roadmap.json")
	roadmapData := `{
		"items": [
			{
				"slug": "test-force",
				"title": "Test Force",
				"priority": "medium",
				"status": "planned",
				"context": "Test context"
			}
		]
	}`
	err = os.WriteFile(roadmapPath, []byte(roadmapData), 0600) //nolint:gosec // G306: Test file permissions
	require.NoError(t, err)

	// Create existing plan file
	planDir := filepath.Join(musterDir, "work", "test-force", "plan")
	err = os.MkdirAll(planDir, 0750) //nolint:gosec // G301: Test directory permissions
	require.NoError(t, err)
	existingPlanPath := filepath.Join(planDir, "implementation-plan.md")
	err = os.WriteFile(existingPlanPath, []byte("# Old Plan\n"), 0600) //nolint:gosec // G306: Test file permissions
	require.NoError(t, err)

	// Change to temp dir
	origDir, _ := os.Getwd()
	defer func() { _ = os.Chdir(origDir) }()
	_ = os.Chdir(tmpDir)

	// Mock interactiveToolFactory to write new plan file
	origFactory := interactiveToolFactory
	defer func() { interactiveToolFactory = origFactory }()

	interactiveToolFactory = func() (coding.InteractiveCodingTool, error) {
		return mockInteractiveToolFunc{fn: func(ctx context.Context, cfg coding.InteractiveConfig) error {
			cwd, _ := os.Getwd()
			planDir := filepath.Join(cwd, ".muster", "work", "test-force", "plan")
			planPath := filepath.Join(planDir, "implementation-plan.md")
			return os.WriteFile(planPath, []byte("# New Plan\n"), 0600) //nolint:gosec // G306: Test file permissions
		}}, nil
	}

	// Create command
	cmd := &cobra.Command{}
	cmd.SetErr(new(bytes.Buffer))

	// Add flags with force=true
	cmd.Flags().Bool("force", true, "")
	cmd.Flags().Bool("verbose", false, "")

	// Run
	err = runPlan(cmd, []string{"test-force"})
	assert.NoError(t, err)

	// Verify plan file was overwritten
	cwd, _ := os.Getwd()
	planPath := filepath.Join(cwd, ".muster", "work", "test-force", "plan", "implementation-plan.md")
	content, err := os.ReadFile(planPath) //nolint:gosec // G304: Test file in controlled test directory
	assert.NoError(t, err)
	assert.Contains(t, string(content), "New Plan")
}

// Test 36-39: Output, JSON, verbose, read-only

func TestRunPlan_TableOutput(t *testing.T) {
	tmpDir := t.TempDir()

	// Create .muster directory
	musterDir := filepath.Join(tmpDir, ".muster")
	err := os.MkdirAll(musterDir, 0750) //nolint:gosec // G301: Test directory permissions
	require.NoError(t, err)

	// Write valid roadmap
	roadmapPath := filepath.Join(musterDir, "roadmap.json")
	roadmapData := `{
		"items": [
			{
				"slug": "test-table-output",
				"title": "Test Table Output",
				"priority": "medium",
				"status": "planned",
				"context": "Test context"
			}
		]
	}`
	err = os.WriteFile(roadmapPath, []byte(roadmapData), 0600) //nolint:gosec // G306: Test file permissions
	require.NoError(t, err)

	// Change to temp dir
	origDir, _ := os.Getwd()
	defer func() { _ = os.Chdir(origDir) }()
	_ = os.Chdir(tmpDir)

	// Ensure table mode
	ui.SetOutputMode(ui.TableMode)

	// Mock interactiveToolFactory to write plan file
	origFactory := interactiveToolFactory
	defer func() { interactiveToolFactory = origFactory }()

	interactiveToolFactory = func() (coding.InteractiveCodingTool, error) {
		return mockInteractiveToolFunc{fn: func(ctx context.Context, cfg coding.InteractiveConfig) error {
			cwd, _ := os.Getwd()
			planDir := filepath.Join(cwd, ".muster", "work", "test-table-output", "plan")
			planPath := filepath.Join(planDir, "implementation-plan.md")
			return os.WriteFile(planPath, []byte("# Test Plan\n"), 0600) //nolint:gosec // G306: Test file permissions
		}}, nil
	}

	// Create command
	cmd := &cobra.Command{}
	cmd.SetErr(new(bytes.Buffer))

	// Capture stdout
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)

	// Add flags
	cmd.Flags().Bool("force", false, "")
	cmd.Flags().Bool("verbose", false, "")

	// Run
	err = runPlan(cmd, []string{"test-table-output"})
	assert.NoError(t, err)

	// Note: The actual output goes to fmt.Printf which writes to os.Stdout, not cmd.Out
	// We can't easily capture os.Stdout in this test, but we verify the function succeeds
	// and the plan file is created
	cwd, _ := os.Getwd()
	planPath := filepath.Join(cwd, ".muster", "work", "test-table-output", "plan", "implementation-plan.md")
	assert.FileExists(t, planPath)
}

func TestRunPlan_JSONOutput(t *testing.T) {
	tmpDir := t.TempDir()

	// Create .muster directory
	musterDir := filepath.Join(tmpDir, ".muster")
	err := os.MkdirAll(musterDir, 0750) //nolint:gosec // G301: Test directory permissions
	require.NoError(t, err)

	// Write valid roadmap
	roadmapPath := filepath.Join(musterDir, "roadmap.json")
	roadmapData := `{
		"items": [
			{
				"slug": "test-json-output",
				"title": "Test JSON Output",
				"priority": "medium",
				"status": "planned",
				"context": "Test context"
			}
		]
	}`
	err = os.WriteFile(roadmapPath, []byte(roadmapData), 0600) //nolint:gosec // G306: Test file permissions
	require.NoError(t, err)

	// Change to temp dir
	origDir, _ := os.Getwd()
	defer func() { _ = os.Chdir(origDir) }()
	_ = os.Chdir(tmpDir)

	// Set JSON mode
	ui.SetOutputMode(ui.JSONMode)
	defer ui.SetOutputMode(ui.TableMode) // Reset for other tests

	// Mock interactiveToolFactory to write plan file
	origFactory := interactiveToolFactory
	defer func() { interactiveToolFactory = origFactory }()

	interactiveToolFactory = func() (coding.InteractiveCodingTool, error) {
		return mockInteractiveToolFunc{fn: func(ctx context.Context, cfg coding.InteractiveConfig) error {
			cwd, _ := os.Getwd()
			planDir := filepath.Join(cwd, ".muster", "work", "test-json-output", "plan")
			planPath := filepath.Join(planDir, "implementation-plan.md")
			return os.WriteFile(planPath, []byte("# Test Plan\n"), 0600) //nolint:gosec // G306: Test file permissions
		}}, nil
	}

	// Create command
	cmd := &cobra.Command{}
	cmd.SetErr(new(bytes.Buffer))

	// Add flags
	cmd.Flags().Bool("force", false, "")
	cmd.Flags().Bool("verbose", false, "")

	// Run
	err = runPlan(cmd, []string{"test-json-output"})
	assert.NoError(t, err)

	// Verify plan file is created
	cwd, _ := os.Getwd()
	planPath := filepath.Join(cwd, ".muster", "work", "test-json-output", "plan", "implementation-plan.md")
	assert.FileExists(t, planPath)
}

func TestRunPlan_VerboseMode(t *testing.T) {
	tmpDir := t.TempDir()

	// Create .muster directory
	musterDir := filepath.Join(tmpDir, ".muster")
	err := os.MkdirAll(musterDir, 0750) //nolint:gosec // G301: Test directory permissions
	require.NoError(t, err)

	// Write valid roadmap
	roadmapPath := filepath.Join(musterDir, "roadmap.json")
	roadmapData := `{
		"items": [
			{
				"slug": "test-verbose",
				"title": "Test Verbose",
				"priority": "medium",
				"status": "planned",
				"context": "Test context"
			}
		]
	}`
	err = os.WriteFile(roadmapPath, []byte(roadmapData), 0600) //nolint:gosec // G306: Test file permissions
	require.NoError(t, err)

	// Change to temp dir
	origDir, _ := os.Getwd()
	defer func() { _ = os.Chdir(origDir) }()
	_ = os.Chdir(tmpDir)

	// Mock interactiveToolFactory to write plan file
	origFactory := interactiveToolFactory
	defer func() { interactiveToolFactory = origFactory }()

	interactiveToolFactory = func() (coding.InteractiveCodingTool, error) {
		return mockInteractiveToolFunc{fn: func(ctx context.Context, cfg coding.InteractiveConfig) error {
			cwd, _ := os.Getwd()
			planDir := filepath.Join(cwd, ".muster", "work", "test-verbose", "plan")
			planPath := filepath.Join(planDir, "implementation-plan.md")
			return os.WriteFile(planPath, []byte("# Test Plan\n"), 0600) //nolint:gosec // G306: Test file permissions
		}}, nil
	}

	// Create command
	cmd := &cobra.Command{}
	cmd.SetErr(new(bytes.Buffer))

	// Add flags with verbose=true
	cmd.Flags().Bool("force", false, "")
	cmd.Flags().Bool("verbose", true, "")

	// Run - Note: verbose output goes to os.Stderr, which cannot be captured by cmd.SetErr
	// This test verifies the command succeeds with verbose flag
	err = runPlan(cmd, []string{"test-verbose"})
	assert.NoError(t, err)

	// Verify plan file was created
	cwd, _ := os.Getwd()
	planPath := filepath.Join(cwd, ".muster", "work", "test-verbose", "plan", "implementation-plan.md")
	assert.FileExists(t, planPath)
}

func TestRunPlan_VerificationMinimalContent_Passes(t *testing.T) {
	tmpDir := t.TempDir()

	// Create .muster directory
	musterDir := filepath.Join(tmpDir, ".muster")
	err := os.MkdirAll(musterDir, 0750) //nolint:gosec // G301: Test directory permissions
	require.NoError(t, err)

	// Write valid roadmap
	roadmapPath := filepath.Join(musterDir, "roadmap.json")
	roadmapData := `{
		"items": [
			{
				"slug": "test-minimal-content",
				"title": "Test Minimal Content",
				"priority": "medium",
				"status": "planned",
				"context": "Test context"
			}
		]
	}`
	err = os.WriteFile(roadmapPath, []byte(roadmapData), 0600) //nolint:gosec // G306: Test file permissions
	require.NoError(t, err)

	// Change to temp dir
	origDir, _ := os.Getwd()
	defer func() { _ = os.Chdir(origDir) }()
	_ = os.Chdir(tmpDir)

	// Mock interactiveToolFactory to write minimal plan file (2 bytes: "a\n")
	// This tests the boundary between empty (0 bytes) and non-empty files.
	// Size checking is intentional - we verify the file has non-zero size
	// rather than parsing Markdown structure, which would be overkill for
	// detecting completely empty files.
	origFactory := interactiveToolFactory
	defer func() { interactiveToolFactory = origFactory }()

	interactiveToolFactory = func() (coding.InteractiveCodingTool, error) {
		return mockInteractiveToolFunc{fn: func(ctx context.Context, cfg coding.InteractiveConfig) error {
			cwd, _ := os.Getwd()
			planDir := filepath.Join(cwd, ".muster", "work", "test-minimal-content", "plan")
			planPath := filepath.Join(planDir, "implementation-plan.md")
			return os.WriteFile(planPath, []byte("a\n"), 0600) //nolint:gosec // G306: Test file permissions
		}}, nil
	}

	// Create command
	cmd := &cobra.Command{}
	cmd.SetErr(new(bytes.Buffer))

	// Add flags
	cmd.Flags().Bool("force", false, "")
	cmd.Flags().Bool("verbose", false, "")

	// Run - should succeed with minimal content
	err = runPlan(cmd, []string{"test-minimal-content"})
	assert.NoError(t, err)

	// Verify plan file was created with minimal content
	cwd, _ := os.Getwd()
	planPath := filepath.Join(cwd, ".muster", "work", "test-minimal-content", "plan", "implementation-plan.md")
	assert.FileExists(t, planPath)

	// Verify it has the minimal content
	content, err := os.ReadFile(planPath) //nolint:gosec // G304: Test file path is controlled
	require.NoError(t, err)
	assert.Equal(t, "a\n", string(content))
}

func TestRunPlan_VerboseWithForce_LogsOverwrite(t *testing.T) {
	tmpDir := t.TempDir()

	// Create .muster directory
	musterDir := filepath.Join(tmpDir, ".muster")
	err := os.MkdirAll(musterDir, 0750) //nolint:gosec // G301: Test directory permissions
	require.NoError(t, err)

	// Write valid roadmap
	roadmapPath := filepath.Join(musterDir, "roadmap.json")
	roadmapData := `{
		"items": [
			{
				"slug": "test-verbose-force",
				"title": "Test Verbose Force",
				"priority": "medium",
				"status": "planned",
				"context": "Test context"
			}
		]
	}`
	err = os.WriteFile(roadmapPath, []byte(roadmapData), 0600) //nolint:gosec // G306: Test file permissions
	require.NoError(t, err)

	// Change to temp dir
	origDir, _ := os.Getwd()
	defer func() { _ = os.Chdir(origDir) }()
	_ = os.Chdir(tmpDir)

	// Create existing plan file
	cwd, _ := os.Getwd()
	planDir := filepath.Join(cwd, ".muster", "work", "test-verbose-force", "plan")
	err = os.MkdirAll(planDir, 0750) //nolint:gosec // G301: Test directory permissions
	require.NoError(t, err)
	planPath := filepath.Join(planDir, "implementation-plan.md")
	err = os.WriteFile(planPath, []byte("# Old Plan\n"), 0600) //nolint:gosec // G306: Test file permissions
	require.NoError(t, err)

	// Mock interactiveToolFactory to write new plan file
	origFactory := interactiveToolFactory
	defer func() { interactiveToolFactory = origFactory }()

	interactiveToolFactory = func() (coding.InteractiveCodingTool, error) {
		return mockInteractiveToolFunc{fn: func(ctx context.Context, cfg coding.InteractiveConfig) error {
			return os.WriteFile(planPath, []byte("# New Plan\n"), 0600) //nolint:gosec // G306: Test file permissions
		}}, nil
	}

	// Create command with stderr buffer
	cmd := &cobra.Command{}
	stderrBuf := new(bytes.Buffer)
	cmd.SetErr(stderrBuf)

	// Add flags with verbose=true and force=true
	cmd.Flags().Bool("force", true, "")
	cmd.Flags().Bool("verbose", true, "")

	// Run
	err = runPlan(cmd, []string{"test-verbose-force"})
	assert.NoError(t, err)

	// Verify stderr contains overwrite message
	stderr := stderrBuf.String()
	assert.Contains(t, stderr, "Overwriting existing plan", "stderr should contain overwrite message")

	// Verify plan file was updated
	assert.FileExists(t, planPath)
	content, err := os.ReadFile(planPath) //nolint:gosec // G304: Test file path is controlled
	require.NoError(t, err)
	assert.Equal(t, "# New Plan\n", string(content))
}

func TestRunPlan_ReadOnly(t *testing.T) {
	tmpDir := t.TempDir()

	// Create .muster directory
	musterDir := filepath.Join(tmpDir, ".muster")
	err := os.MkdirAll(musterDir, 0750) //nolint:gosec // G301: Test directory permissions
	require.NoError(t, err)

	// Write valid roadmap
	roadmapPath := filepath.Join(musterDir, "roadmap.json")
	roadmapData := `{
		"items": [
			{
				"slug": "test-readonly",
				"title": "Test ReadOnly",
				"priority": "medium",
				"status": "planned",
				"context": "Test context"
			}
		]
	}`
	err = os.WriteFile(roadmapPath, []byte(roadmapData), 0600) //nolint:gosec // G306: Test file permissions
	require.NoError(t, err)

	// Change to temp dir
	origDir, _ := os.Getwd()
	defer func() { _ = os.Chdir(origDir) }()
	_ = os.Chdir(tmpDir)

	// Capture roadmap file content before plan (content-based verification)
	roadmapContentBefore, err := os.ReadFile(roadmapPath) //nolint:gosec // G304: Test file path is controlled
	require.NoError(t, err)

	// Read roadmap before plan
	rmBefore, err := roadmap.LoadRoadmap(".")
	require.NoError(t, err)
	itemBefore := rmBefore.FindBySlug("test-readonly")
	require.NotNil(t, itemBefore)

	// Mock interactiveToolFactory to write plan file
	origFactory := interactiveToolFactory
	defer func() { interactiveToolFactory = origFactory }()

	interactiveToolFactory = func() (coding.InteractiveCodingTool, error) {
		return mockInteractiveToolFunc{fn: func(ctx context.Context, cfg coding.InteractiveConfig) error {
			cwd, _ := os.Getwd()
			planDir := filepath.Join(cwd, ".muster", "work", "test-readonly", "plan")
			planPath := filepath.Join(planDir, "implementation-plan.md")
			return os.WriteFile(planPath, []byte("# Test Plan\n"), 0600) //nolint:gosec // G306: Test file permissions
		}}, nil
	}

	// Create command
	cmd := &cobra.Command{}
	cmd.SetErr(new(bytes.Buffer))

	// Add flags
	cmd.Flags().Bool("force", false, "")
	cmd.Flags().Bool("verbose", false, "")

	// Run
	err = runPlan(cmd, []string{"test-readonly"})
	assert.NoError(t, err)

	// Capture roadmap file content after plan (content-based verification)
	roadmapContentAfter, err := os.ReadFile(roadmapPath) //nolint:gosec // G304: Test file path is controlled
	require.NoError(t, err)

	// Verify roadmap file content is unchanged (primary check)
	assert.Equal(t, roadmapContentBefore, roadmapContentAfter, "roadmap file content should be unchanged")

	// Read roadmap after plan
	rmAfter, err := roadmap.LoadRoadmap(".")
	require.NoError(t, err)
	itemAfter := rmAfter.FindBySlug("test-readonly")
	require.NotNil(t, itemAfter)

	// Verify roadmap item is unchanged (secondary check)
	assert.Equal(t, itemBefore.Status, itemAfter.Status)
	assert.Equal(t, itemBefore.Title, itemAfter.Title)
	assert.Equal(t, itemBefore.Priority, itemAfter.Priority)
}

// Test 40-42: Cross-platform paths

func TestRunPlan_CrossPlatformPaths(t *testing.T) {
	tmpDir := t.TempDir()

	// Verify filepath.Join is used (this is a code inspection test)
	// We verify that the paths work correctly across platforms

	// Create .muster directory
	musterDir := filepath.Join(tmpDir, ".muster")
	err := os.MkdirAll(musterDir, 0750) //nolint:gosec // G301: Test directory permissions
	require.NoError(t, err)

	// Write valid roadmap
	roadmapPath := filepath.Join(musterDir, "roadmap.json")
	roadmapData := `{
		"items": [
			{
				"slug": "test-crossplatform",
				"title": "Test CrossPlatform",
				"priority": "medium",
				"status": "planned",
				"context": "Test context"
			}
		]
	}`
	err = os.WriteFile(roadmapPath, []byte(roadmapData), 0600) //nolint:gosec // G306: Test file permissions
	require.NoError(t, err)

	// Change to temp dir
	origDir, _ := os.Getwd()
	defer func() { _ = os.Chdir(origDir) }()
	_ = os.Chdir(tmpDir)

	// Mock interactiveToolFactory to write plan file
	origFactory := interactiveToolFactory
	defer func() { interactiveToolFactory = origFactory }()

	interactiveToolFactory = func() (coding.InteractiveCodingTool, error) {
		return mockInteractiveToolFunc{fn: func(ctx context.Context, cfg coding.InteractiveConfig) error {
			cwd, _ := os.Getwd()
			planDir := filepath.Join(cwd, ".muster", "work", "test-crossplatform", "plan")
			planPath := filepath.Join(planDir, "implementation-plan.md")
			return os.WriteFile(planPath, []byte("# Test Plan\n"), 0600) //nolint:gosec // G306: Test file permissions
		}}, nil
	}

	// Create command
	cmd := &cobra.Command{}
	cmd.SetErr(new(bytes.Buffer))

	// Add flags
	cmd.Flags().Bool("force", false, "")
	cmd.Flags().Bool("verbose", false, "")

	// Run
	err = runPlan(cmd, []string{"test-crossplatform"})
	assert.NoError(t, err)

	// Verify paths work correctly with filepath.Join
	cwd, _ := os.Getwd()
	planDir := filepath.Join(cwd, ".muster", "work", "test-crossplatform", "plan")
	assert.DirExists(t, planDir)

	// Verify subdirectories use filepath.Join
	researchDir := filepath.Join(planDir, "research")
	assert.DirExists(t, researchDir)

	synthesisDir := filepath.Join(planDir, "synthesis")
	assert.DirExists(t, synthesisDir)

	// Verify plan file path uses filepath.Join
	planPath := filepath.Join(planDir, "implementation-plan.md")
	assert.FileExists(t, planPath)
}

// Mock Tests for interactiveToolFactory

func TestRunPlan_WithMock_VerifiesInteractiveCall(t *testing.T) {
	// Save original factory
	original := interactiveToolFactory
	defer func() { interactiveToolFactory = original }()

	// Create temp directories
	tmpDir := t.TempDir()
	callsDir := filepath.Join(tmpDir, "calls")
	require.NoError(t, os.MkdirAll(callsDir, 0755)) //nolint:gosec // G301: Test directory permissions

	// Create test project structure
	projectDir := filepath.Join(tmpDir, "project")
	musterDir := filepath.Join(projectDir, ".muster")
	require.NoError(t, os.MkdirAll(musterDir, 0755)) //nolint:gosec // G301: Test directory permissions

	// Create roadmap
	roadmapContent := `{"items": [{"slug": "test-feature", "title": "Test Feature", "status": "planned", "priority": "high"}]}`
	require.NoError(t, os.WriteFile(filepath.Join(musterDir, "roadmap.json"), []byte(roadmapContent), 0644)) //nolint:gosec // G306: Test file permissions

	// Change to project directory
	oldDir, err := os.Getwd()
	require.NoError(t, err)
	defer func() { _ = os.Chdir(oldDir) }()
	require.NoError(t, os.Chdir(projectDir))

	// Replace factory with mock
	mockTool := coding.NewMockInteractiveCodingTool(callsDir)
	interactiveToolFactory = func() (coding.InteractiveCodingTool, error) {
		return mockTool, nil
	}

	// Create command
	cmd := &cobra.Command{}
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetOut(new(bytes.Buffer))
	cmd.Flags().Bool("force", false, "")
	cmd.Flags().Bool("verbose", false, "")

	// Run
	_ = runPlan(cmd, []string{"test-feature"})

	// Verify RunInteractive was called
	callFile := filepath.Join(callsDir, "001-interactive.json")
	require.FileExists(t, callFile, "RunInteractive should have been called")

	// Verify call details
	data, err := os.ReadFile(callFile) //nolint:gosec // G304: Reading test fixture
	require.NoError(t, err)

	var call struct {
		Tool      string            `json:"tool"`
		Model     string            `json:"model"`
		PluginDir string            `json:"plugin_dir"`
		Env       map[string]string `json:"env,omitempty"`
	}
	require.NoError(t, json.Unmarshal(data, &call))

	// Verify correct tool, model, and pluginDir
	assert.NotEmpty(t, call.Tool, "tool should be set")
	assert.NotEmpty(t, call.Model, "model should be set")
	assert.NotEmpty(t, call.PluginDir, "plugin_dir should be set")
}

func TestRunPlan_WithMock_ToolError_PropagatesError(t *testing.T) {
	// Save original factory
	original := interactiveToolFactory
	defer func() { interactiveToolFactory = original }()

	// Create temp directories
	tmpDir := t.TempDir()
	callsDir := filepath.Join(tmpDir, "calls")
	require.NoError(t, os.MkdirAll(callsDir, 0755)) //nolint:gosec // G301: Test directory permissions

	// Create test project structure
	projectDir := filepath.Join(tmpDir, "project")
	musterDir := filepath.Join(projectDir, ".muster")
	require.NoError(t, os.MkdirAll(musterDir, 0755)) //nolint:gosec // G301: Test directory permissions

	// Create roadmap
	roadmapContent := `{"items": [{"slug": "test-feature", "title": "Test Feature", "status": "planned", "priority": "high"}]}`
	require.NoError(t, os.WriteFile(filepath.Join(musterDir, "roadmap.json"), []byte(roadmapContent), 0644)) //nolint:gosec // G306: Test file permissions

	// Change to project directory
	oldDir, err := os.Getwd()
	require.NoError(t, err)
	defer func() { _ = os.Chdir(oldDir) }()
	require.NoError(t, os.Chdir(projectDir))

	// Create response file to simulate error
	responseContent := `{"error": "simulated tool error"}`
	require.NoError(t, os.WriteFile(filepath.Join(callsDir, "001-interactive-response.json"), []byte(responseContent), 0644)) //nolint:gosec // G306: Test file permissions

	// Replace factory with mock
	mockTool := coding.NewMockInteractiveCodingTool(callsDir)
	interactiveToolFactory = func() (coding.InteractiveCodingTool, error) {
		return mockTool, nil
	}

	// Create command
	cmd := &cobra.Command{}
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetOut(new(bytes.Buffer))
	cmd.Flags().Bool("force", false, "")
	cmd.Flags().Bool("verbose", false, "")

	// Run should return error
	err = runPlan(cmd, []string{"test-feature"})
	require.Error(t, err, "should return error when tool fails")
	assert.Contains(t, err.Error(), "simulated tool error", "error should be propagated from tool")
}
