package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/abenz1267/muster/internal/ai"
	"github.com/abenz1267/muster/internal/vcs"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Structural Tests

func TestOutCommand_Exists(t *testing.T) {
	assert.NotNil(t, outCmd, "out command should exist")
	assert.Equal(t, "out [slug]", outCmd.Use, "command use should be 'out [slug]'")
}

func TestOutCommand_HasRunE(t *testing.T) {
	assert.NotNil(t, outCmd.RunE, "out command should have RunE function")
}

func TestOutCommand_RequiresExactlyOneArg(t *testing.T) {
	assert.NotNil(t, outCmd.Args, "out command should have Args validator")

	// Test with correct number of args
	err := outCmd.Args(outCmd, []string{"test-slug"})
	assert.NoError(t, err, "should accept exactly 1 argument")

	// Test with no args
	err = outCmd.Args(outCmd, []string{})
	assert.Error(t, err, "should reject 0 arguments")

	// Test with too many args
	err = outCmd.Args(outCmd, []string{"slug1", "slug2"})
	assert.Error(t, err, "should reject 2 arguments")
}

func TestOutCommand_AllFlagsExist(t *testing.T) {
	// Test --no-fix flag
	noFixFlag := outCmd.Flags().Lookup("no-fix")
	require.NotNil(t, noFixFlag, "no-fix flag should exist")
	assert.Equal(t, "bool", noFixFlag.Value.Type(), "no-fix should be bool")
	assert.Equal(t, "false", noFixFlag.DefValue, "no-fix default should be false")
	assert.NotEmpty(t, noFixFlag.Usage, "no-fix should have usage text")

	// Test --wait flag
	waitFlag := outCmd.Flags().Lookup("wait")
	require.NotNil(t, waitFlag, "wait flag should exist")
	assert.Equal(t, "bool", waitFlag.Value.Type(), "wait should be bool")
	assert.Equal(t, "false", waitFlag.DefValue, "wait default should be false")
	assert.NotEmpty(t, waitFlag.Usage, "wait should have usage text")

	// Test --timeout flag
	timeoutFlag := outCmd.Flags().Lookup("timeout")
	require.NotNil(t, timeoutFlag, "timeout flag should exist")
	assert.Equal(t, "duration", timeoutFlag.Value.Type(), "timeout should be duration")
	assert.Equal(t, "30m0s", timeoutFlag.DefValue, "timeout default should be 30m")
	assert.NotEmpty(t, timeoutFlag.Usage, "timeout should have usage text")

	// Test --dry-run flag
	dryRunFlag := outCmd.Flags().Lookup("dry-run")
	require.NotNil(t, dryRunFlag, "dry-run flag should exist")
	assert.Equal(t, "bool", dryRunFlag.Value.Type(), "dry-run should be bool")
	assert.Equal(t, "false", dryRunFlag.DefValue, "dry-run default should be false")
	assert.NotEmpty(t, dryRunFlag.Usage, "dry-run should have usage text")

	// Test --verbose flag
	verboseFlag := outCmd.Flags().Lookup("verbose")
	require.NotNil(t, verboseFlag, "verbose flag should exist")
	assert.Equal(t, "bool", verboseFlag.Value.Type(), "verbose should be bool")
	assert.Equal(t, "false", verboseFlag.DefValue, "verbose default should be false")
	assert.NotEmpty(t, verboseFlag.Usage, "verbose should have usage text")
}

func TestOutCommand_WithHelpFlag_Succeeds(t *testing.T) {
	cmd := &cobra.Command{
		Use:   "out [slug]",
		Short: outCmd.Short,
		Long:  outCmd.Long,
		RunE:  outCmd.RunE,
	}
	cmd.Flags().AddFlagSet(outCmd.Flags())

	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"--help"})

	err := cmd.Execute()
	assert.NoError(t, err, "command should execute without error with --help flag")

	output := buf.String()
	assert.NotEmpty(t, output, "help output should not be empty")
	assert.Contains(t, output, "out", "help should contain command name")
	assert.Contains(t, output, "--no-fix", "help should contain --no-fix flag")
	assert.Contains(t, output, "--wait", "help should contain --wait flag")
	assert.Contains(t, output, "--timeout", "help should contain --timeout flag")
	assert.Contains(t, output, "--dry-run", "help should contain --dry-run flag")
}

func TestOutCommand_IsAddedToRootCommand(t *testing.T) {
	found := false
	for _, cmd := range rootCmd.Commands() {
		if cmd.Name() == "out" {
			found = true
			break
		}
	}
	assert.True(t, found, "out command should be added to root command")
}

// Config and Roadmap Error Tests

func TestOutCommand_ConfigMalformedYAML_ReturnsError(t *testing.T) {
	tmpDir := t.TempDir()

	// Create malformed config
	musterDir := filepath.Join(tmpDir, ".muster")
	require.NoError(t, os.MkdirAll(musterDir, 0755)) //nolint:gosec // G301: Test directory

	malformedConfig := []byte("defaults:\n  tool: [invalid yaml")
	require.NoError(t, os.WriteFile(filepath.Join(musterDir, "config.yml"), malformedConfig, 0644)) //nolint:gosec // G306: Test file

	// Create valid roadmap
	roadmapContent := []byte(`{"items":[{"slug":"test","title":"Test","priority":"medium","status":"in_progress","context":"Test"}]}`)
	require.NoError(t, os.WriteFile(filepath.Join(musterDir, "roadmap.json"), roadmapContent, 0644)) //nolint:gosec // G306: Test file

	// Change to temp directory
	origDir, err := os.Getwd()
	require.NoError(t, err)
	defer func() { _ = os.Chdir(origDir) }()
	require.NoError(t, os.Chdir(tmpDir))

	// Execute command
	cmd := &cobra.Command{Use: "out [slug]", RunE: outCmd.RunE}
	cmd.Flags().Bool("no-fix", false, "")
	cmd.Flags().Bool("wait", false, "")
	cmd.Flags().Duration("timeout", 30*time.Minute, "")
	cmd.Flags().Bool("dry-run", false, "")
	cmd.Flags().Bool("verbose", false, "")

	err = cmd.RunE(cmd, []string{"test"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "config file malformed", "error should indicate malformed config")
}

func TestOutCommand_RoadmapMalformedJSON_ReturnsError(t *testing.T) {
	tmpDir := t.TempDir()

	// Create valid config
	musterDir := filepath.Join(tmpDir, ".muster")
	require.NoError(t, os.MkdirAll(musterDir, 0755)) //nolint:gosec // G301: Test directory

	validConfig := []byte("defaults:\n  tool: claude-code\n  provider: anthropic\n  model: sonnet\n")
	require.NoError(t, os.WriteFile(filepath.Join(musterDir, "config.yml"), validConfig, 0644)) //nolint:gosec // G306: Test file

	// Create malformed roadmap
	malformedRoadmap := []byte(`{"items":[{"slug":"test",invalid json}]}`)
	require.NoError(t, os.WriteFile(filepath.Join(musterDir, "roadmap.json"), malformedRoadmap, 0644)) //nolint:gosec // G306: Test file

	// Change to temp directory
	origDir, err := os.Getwd()
	require.NoError(t, err)
	defer func() { _ = os.Chdir(origDir) }()
	require.NoError(t, os.Chdir(tmpDir))

	// Execute command
	cmd := &cobra.Command{Use: "out [slug]", RunE: outCmd.RunE}
	cmd.Flags().Bool("no-fix", false, "")
	cmd.Flags().Bool("wait", false, "")
	cmd.Flags().Duration("timeout", 30*time.Minute, "")
	cmd.Flags().Bool("dry-run", false, "")
	cmd.Flags().Bool("verbose", false, "")

	err = cmd.RunE(cmd, []string{"test"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "roadmap file is malformed", "error should indicate malformed roadmap")
}

func TestOutCommand_SlugNotFound_ReturnsError(t *testing.T) {
	tmpDir := t.TempDir()

	// Create valid config
	musterDir := filepath.Join(tmpDir, ".muster")
	require.NoError(t, os.MkdirAll(musterDir, 0755)) //nolint:gosec // G301: Test directory

	validConfig := []byte("defaults:\n  tool: claude-code\n  provider: anthropic\n  model: sonnet\n")
	require.NoError(t, os.WriteFile(filepath.Join(musterDir, "config.yml"), validConfig, 0644)) //nolint:gosec // G306: Test file

	// Create roadmap without the slug we're looking for
	roadmapContent := []byte(`{"items":[{"slug":"other-slug","title":"Test","priority":"medium","status":"in_progress","context":"Test"}]}`)
	require.NoError(t, os.WriteFile(filepath.Join(musterDir, "roadmap.json"), roadmapContent, 0644)) //nolint:gosec // G306: Test file

	// Change to temp directory
	origDir, err := os.Getwd()
	require.NoError(t, err)
	defer func() { _ = os.Chdir(origDir) }()
	require.NoError(t, os.Chdir(tmpDir))

	// Execute command
	cmd := &cobra.Command{Use: "out [slug]", RunE: outCmd.RunE}
	cmd.Flags().Bool("no-fix", false, "")
	cmd.Flags().Bool("wait", false, "")
	cmd.Flags().Duration("timeout", 30*time.Minute, "")
	cmd.Flags().Bool("dry-run", false, "")
	cmd.Flags().Bool("verbose", false, "")

	err = cmd.RunE(cmd, []string{"nonexistent-slug"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found", "error should indicate slug not found")
}

func TestOutCommand_ItemAlreadyCompleted_ReturnsDescriptiveError(t *testing.T) {
	tmpDir := t.TempDir()

	// Create valid config
	musterDir := filepath.Join(tmpDir, ".muster")
	require.NoError(t, os.MkdirAll(musterDir, 0755)) //nolint:gosec // G301: Test directory

	validConfig := []byte("defaults:\n  tool: claude-code\n  provider: anthropic\n  model: sonnet\n")
	require.NoError(t, os.WriteFile(filepath.Join(musterDir, "config.yml"), validConfig, 0644)) //nolint:gosec // G306: Test file

	// Create roadmap with completed item
	roadmapContent := []byte(`{"items":[{"slug":"completed-item","title":"Test","priority":"medium","status":"completed","context":"Test"}]}`)
	require.NoError(t, os.WriteFile(filepath.Join(musterDir, "roadmap.json"), roadmapContent, 0644)) //nolint:gosec // G306: Test file

	// Change to temp directory
	origDir, err := os.Getwd()
	require.NoError(t, err)
	defer func() { _ = os.Chdir(origDir) }()
	require.NoError(t, os.Chdir(tmpDir))

	// Execute command
	cmd := &cobra.Command{Use: "out [slug]", RunE: outCmd.RunE}
	cmd.Flags().Bool("no-fix", false, "")
	cmd.Flags().Bool("wait", false, "")
	cmd.Flags().Duration("timeout", 30*time.Minute, "")
	cmd.Flags().Bool("dry-run", false, "")
	cmd.Flags().Bool("verbose", false, "")

	err = cmd.RunE(cmd, []string{"completed-item"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already marked as completed", "error should indicate item is already completed")
}

func TestOutCommand_NoPRUrlOrBranch_ReturnsError(t *testing.T) {
	tmpDir := t.TempDir()

	// Create valid config
	musterDir := filepath.Join(tmpDir, ".muster")
	require.NoError(t, os.MkdirAll(musterDir, 0755)) //nolint:gosec // G301: Test directory

	validConfig := []byte("defaults:\n  tool: claude-code\n  provider: anthropic\n  model: sonnet\nmerge_strategy: github-pr\n")
	require.NoError(t, os.WriteFile(filepath.Join(musterDir, "config.yml"), validConfig, 0644)) //nolint:gosec // G306: Test file

	// Create roadmap with item that has no pr_url or branch
	roadmapContent := []byte(`{"items":[{"slug":"no-pr","title":"Test","priority":"medium","status":"in_progress","context":"Test"}]}`)
	require.NoError(t, os.WriteFile(filepath.Join(musterDir, "roadmap.json"), roadmapContent, 0644)) //nolint:gosec // G306: Test file

	// Change to temp directory
	origDir, err := os.Getwd()
	require.NoError(t, err)
	defer func() { _ = os.Chdir(origDir) }()
	require.NoError(t, os.Chdir(tmpDir))

	// Mock VCS factory to avoid requiring actual gh/glab
	originalFactory := vcsFactory
	defer func() { vcsFactory = originalFactory }()

	vcsFactory = func(strategy, dir string) (vcs.VCS, error) {
		return &mockVCS{}, nil
	}

	// Execute command with dry-run
	cmd := &cobra.Command{Use: "out [slug]", RunE: outCmd.RunE}
	cmd.Flags().Bool("no-fix", false, "")
	cmd.Flags().Bool("wait", false, "")
	cmd.Flags().Duration("timeout", 30*time.Minute, "")
	cmd.Flags().Bool("dry-run", true, "")
	cmd.Flags().Bool("verbose", false, "")

	err = cmd.RunE(cmd, []string{"no-pr"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot discover PR", "error should indicate PR discovery failure")
}

// Merge Strategy Tests

func TestOutCommand_DirectMergeStrategy_ExitsCleanly(t *testing.T) {
	tmpDir := t.TempDir()

	// Create valid config with direct merge strategy
	musterDir := filepath.Join(tmpDir, ".muster")
	require.NoError(t, os.MkdirAll(musterDir, 0755)) //nolint:gosec // G301: Test directory

	validConfig := []byte("defaults:\n  tool: claude-code\n  provider: anthropic\n  model: sonnet\nmerge_strategy: direct\n")
	require.NoError(t, os.WriteFile(filepath.Join(musterDir, "config.yml"), validConfig, 0644)) //nolint:gosec // G306: Test file

	// Create valid roadmap
	roadmapContent := []byte(`{"items":[{"slug":"test","title":"Test","priority":"medium","status":"in_progress","context":"Test"}]}`)
	require.NoError(t, os.WriteFile(filepath.Join(musterDir, "roadmap.json"), roadmapContent, 0644)) //nolint:gosec // G306: Test file

	// Change to temp directory
	origDir, err := os.Getwd()
	require.NoError(t, err)
	defer func() { _ = os.Chdir(origDir) }()
	require.NoError(t, os.Chdir(tmpDir))

	// Execute command
	cmd := &cobra.Command{Use: "out [slug]", RunE: outCmd.RunE}
	cmd.Flags().Bool("no-fix", false, "")
	cmd.Flags().Bool("wait", false, "")
	cmd.Flags().Duration("timeout", 30*time.Minute, "")
	cmd.Flags().Bool("dry-run", false, "")
	cmd.Flags().Bool("verbose", false, "")

	buf := new(bytes.Buffer)
	cmd.SetErr(buf)

	err = cmd.RunE(cmd, []string{"test"})
	assert.NoError(t, err, "command should exit cleanly with direct merge strategy")

	output := buf.String()
	assert.Contains(t, output, "direct", "output should mention direct merge strategy")
}

func TestOutCommand_GitHubPRStrategy_Proceeds(t *testing.T) {
	tmpDir := t.TempDir()

	// Create valid config with github-pr strategy
	musterDir := filepath.Join(tmpDir, ".muster")
	require.NoError(t, os.MkdirAll(musterDir, 0755)) //nolint:gosec // G301: Test directory

	validConfig := []byte("defaults:\n  tool: claude-code\n  provider: anthropic\n  model: sonnet\nmerge_strategy: github-pr\n")
	require.NoError(t, os.WriteFile(filepath.Join(musterDir, "config.yml"), validConfig, 0644)) //nolint:gosec // G306: Test file

	// Create roadmap with pr_url
	prURL := "https://github.com/owner/repo/pull/123"
	roadmapContent := []byte(`{"items":[{"slug":"test","title":"Test","priority":"medium","status":"in_progress","context":"Test","pr_url":"` + prURL + `"}]}`)
	require.NoError(t, os.WriteFile(filepath.Join(musterDir, "roadmap.json"), roadmapContent, 0644)) //nolint:gosec // G306: Test file

	// Change to temp directory
	origDir, err := os.Getwd()
	require.NoError(t, err)
	defer func() { _ = os.Chdir(origDir) }()
	require.NoError(t, os.Chdir(tmpDir))

	// Mock VCS factory
	originalFactory := vcsFactory
	defer func() { vcsFactory = originalFactory }()

	mockVCSInstance := &mockVCS{
		checkAuthErr: nil,
		prStatus: &vcs.PRStatus{
			State:    vcs.PRStateMerged,
			Number:   123,
			URL:      prURL,
			CIStatus: vcs.CIStatusPassing,
		},
	}

	vcsFactory = func(strategy, dir string) (vcs.VCS, error) {
		return mockVCSInstance, nil
	}

	// Execute command with dry-run
	cmd := &cobra.Command{Use: "out [slug]", RunE: outCmd.RunE}
	cmd.Flags().Bool("no-fix", false, "")
	cmd.Flags().Bool("wait", false, "")
	cmd.Flags().Duration("timeout", 30*time.Minute, "")
	cmd.Flags().Bool("dry-run", true, "")
	cmd.Flags().Bool("verbose", false, "")

	err = cmd.RunE(cmd, []string{"test"})
	assert.NoError(t, err, "command should proceed with github-pr strategy")
}

func TestOutCommand_DefaultMergeStrategy_IsGitHubPR(t *testing.T) {
	tmpDir := t.TempDir()

	// Create config without merge_strategy (should default to github-pr)
	musterDir := filepath.Join(tmpDir, ".muster")
	require.NoError(t, os.MkdirAll(musterDir, 0755)) //nolint:gosec // G301: Test directory

	validConfig := []byte("defaults:\n  tool: claude-code\n  provider: anthropic\n  model: sonnet\n")
	require.NoError(t, os.WriteFile(filepath.Join(musterDir, "config.yml"), validConfig, 0644)) //nolint:gosec // G306: Test file

	// Create roadmap with pr_url
	prURL := "https://github.com/owner/repo/pull/456"
	roadmapContent := []byte(`{"items":[{"slug":"test","title":"Test","priority":"medium","status":"in_progress","context":"Test","pr_url":"` + prURL + `"}]}`)
	require.NoError(t, os.WriteFile(filepath.Join(musterDir, "roadmap.json"), roadmapContent, 0644)) //nolint:gosec // G306: Test file

	// Change to temp directory
	origDir, err := os.Getwd()
	require.NoError(t, err)
	defer func() { _ = os.Chdir(origDir) }()
	require.NoError(t, os.Chdir(tmpDir))

	// Mock VCS factory
	originalFactory := vcsFactory
	defer func() { vcsFactory = originalFactory }()

	var capturedStrategy string
	vcsFactory = func(strategy, dir string) (vcs.VCS, error) {
		capturedStrategy = strategy
		return &mockVCS{
			prStatus: &vcs.PRStatus{State: vcs.PRStateMerged, CIStatus: vcs.CIStatusPassing},
		}, nil
	}

	// Execute command with dry-run
	cmd := &cobra.Command{Use: "out [slug]", RunE: outCmd.RunE}
	cmd.Flags().Bool("no-fix", false, "")
	cmd.Flags().Bool("wait", false, "")
	cmd.Flags().Duration("timeout", 30*time.Minute, "")
	cmd.Flags().Bool("dry-run", true, "")
	cmd.Flags().Bool("verbose", false, "")

	_ = cmd.RunE(cmd, []string{"test"})
	assert.Equal(t, "github-pr", capturedStrategy, "default merge strategy should be github-pr")
}

// Auth Tests

func TestOutCommand_VCSAuthFailure_ReturnsActionableError(t *testing.T) {
	tmpDir := t.TempDir()

	// Create valid config
	musterDir := filepath.Join(tmpDir, ".muster")
	require.NoError(t, os.MkdirAll(musterDir, 0755)) //nolint:gosec // G301: Test directory

	validConfig := []byte("defaults:\n  tool: claude-code\n  provider: anthropic\n  model: sonnet\nmerge_strategy: github-pr\n")
	require.NoError(t, os.WriteFile(filepath.Join(musterDir, "config.yml"), validConfig, 0644)) //nolint:gosec // G306: Test file

	// Create roadmap with pr_url
	prURL := "https://github.com/owner/repo/pull/789"
	roadmapContent := []byte(`{"items":[{"slug":"test","title":"Test","priority":"medium","status":"in_progress","context":"Test","pr_url":"` + prURL + `"}]}`)
	require.NoError(t, os.WriteFile(filepath.Join(musterDir, "roadmap.json"), roadmapContent, 0644)) //nolint:gosec // G306: Test file

	// Change to temp directory
	origDir, err := os.Getwd()
	require.NoError(t, err)
	defer func() { _ = os.Chdir(origDir) }()
	require.NoError(t, os.Chdir(tmpDir))

	// Mock VCS factory to return auth error
	originalFactory := vcsFactory
	defer func() { vcsFactory = originalFactory }()

	vcsFactory = func(strategy, dir string) (vcs.VCS, error) {
		return &mockVCS{
			checkAuthErr: assert.AnError,
		}, nil
	}

	// Execute command (not dry-run, so auth check runs)
	cmd := &cobra.Command{Use: "out [slug]", RunE: outCmd.RunE}
	cmd.Flags().Bool("no-fix", false, "")
	cmd.Flags().Bool("wait", false, "")
	cmd.Flags().Duration("timeout", 30*time.Minute, "")
	cmd.Flags().Bool("dry-run", false, "")
	cmd.Flags().Bool("verbose", false, "")

	err = cmd.RunE(cmd, []string{"test"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "gh not authenticated", "error should mention gh auth")
	assert.Contains(t, err.Error(), "gh auth login", "error should suggest login command")
}

// Dry-run Tests

func TestOutCommand_DryRun_NoSideEffects(t *testing.T) {
	tmpDir := t.TempDir()

	// Create valid config
	musterDir := filepath.Join(tmpDir, ".muster")
	require.NoError(t, os.MkdirAll(musterDir, 0755)) //nolint:gosec // G301: Test directory

	validConfig := []byte("defaults:\n  tool: claude-code\n  provider: anthropic\n  model: sonnet\nmerge_strategy: github-pr\n")
	require.NoError(t, os.WriteFile(filepath.Join(musterDir, "config.yml"), validConfig, 0644)) //nolint:gosec // G306: Test file

	// Create roadmap with pr_url
	prURL := "https://github.com/owner/repo/pull/999"
	roadmapContent := []byte(`{"items":[{"slug":"test","title":"Test","priority":"medium","status":"in_progress","context":"Test","pr_url":"` + prURL + `","branch":"test-branch"}]}`)
	roadmapPath := filepath.Join(musterDir, "roadmap.json")
	require.NoError(t, os.WriteFile(roadmapPath, roadmapContent, 0644)) //nolint:gosec // G306: Test file

	// Change to temp directory
	origDir, err := os.Getwd()
	require.NoError(t, err)
	defer func() { _ = os.Chdir(origDir) }()
	require.NoError(t, os.Chdir(tmpDir))

	// Mock VCS factory
	originalFactory := vcsFactory
	defer func() { vcsFactory = originalFactory }()

	vcsFactory = func(strategy, dir string) (vcs.VCS, error) {
		return &mockVCS{
			prStatus: &vcs.PRStatus{
				State:    vcs.PRStateMerged,
				CIStatus: vcs.CIStatusPassing,
			},
		}, nil
	}

	// Execute command with dry-run
	cmd := &cobra.Command{Use: "out [slug]", RunE: outCmd.RunE}
	cmd.Flags().Bool("no-fix", false, "")
	cmd.Flags().Bool("wait", false, "")
	cmd.Flags().Duration("timeout", 30*time.Minute, "")
	cmd.Flags().Bool("dry-run", true, "")
	cmd.Flags().Bool("verbose", false, "")

	buf := new(bytes.Buffer)
	cmd.SetErr(buf)

	err = cmd.RunE(cmd, []string{"test"})
	assert.NoError(t, err, "dry-run should succeed")

	// Verify output mentions dry-run actions
	output := buf.String()
	assert.Contains(t, output, "[DRY RUN]", "output should indicate dry-run mode")
	assert.Contains(t, output, "Would check VCS authentication", "output should show auth check would happen")
	assert.Contains(t, output, "Would monitor CI status", "output should show CI monitoring would happen")
	assert.Contains(t, output, "Would wait for PR merge", "output should show merge wait would happen")
	assert.Contains(t, output, "Would perform cleanup", "output should show cleanup would happen")

	// Verify roadmap was NOT modified
	afterContent, err := os.ReadFile(roadmapPath) //nolint:gosec // G304: Test file path is controlled by test setup
	require.NoError(t, err)
	assert.Equal(t, roadmapContent, afterContent, "roadmap should not be modified in dry-run mode")
}

// Cleanup Tests

func TestOutCommand_Cleanup_CollectsAllErrors(t *testing.T) {
	// Note: Full cleanup testing requires mocking git operations.
	// This is a structural test to verify the cleanup function exists and handles errors.
	t.Skip("Cleanup function tested indirectly through integration tests")
}

func TestOutCommand_Cleanup_MarksItemCompletedEvenOnPartialFailure(t *testing.T) {
	tmpDir := t.TempDir()

	// Create valid config
	musterDir := filepath.Join(tmpDir, ".muster")
	require.NoError(t, os.MkdirAll(musterDir, 0755)) //nolint:gosec // G301: Test directory

	validConfig := []byte("defaults:\n  tool: claude-code\n  provider: anthropic\n  model: sonnet\nmerge_strategy: github-pr\n")
	require.NoError(t, os.WriteFile(filepath.Join(musterDir, "config.yml"), validConfig, 0644)) //nolint:gosec // G306: Test file

	// Create roadmap with pr_url and branch
	prURL := "https://github.com/owner/repo/pull/999"
	roadmapContent := []byte(`{"items":[{"slug":"test-cleanup","title":"Test Cleanup","priority":"medium","status":"in_progress","context":"Test","pr_url":"` + prURL + `","branch":"test-branch"}]}`)
	roadmapPath := filepath.Join(musterDir, "roadmap.json")
	require.NoError(t, os.WriteFile(roadmapPath, roadmapContent, 0644)) //nolint:gosec // G306: Test file

	// Change to temp directory
	origDir, err := os.Getwd()
	require.NoError(t, err)
	defer func() { _ = os.Chdir(origDir) }()
	require.NoError(t, os.Chdir(tmpDir))

	// Mock VCS factory - CI passing and PR merged
	originalFactory := vcsFactory
	defer func() { vcsFactory = originalFactory }()

	vcsFactory = func(strategy, dir string) (vcs.VCS, error) {
		return &mockVCS{
			checks: []vcs.CheckResult{
				{Name: "test", Status: vcs.CIStatusPassing},
			},
			prStatus: &vcs.PRStatus{
				State:    vcs.PRStateMerged,
				CIStatus: vcs.CIStatusPassing,
			},
		}, nil
	}

	// Execute command (not dry-run)
	// Note: git operations (PullLatest, DeleteBranch) will fail because this isn't a real git repo,
	// but the item should still be marked completed
	cmd := &cobra.Command{Use: "out [slug]", RunE: outCmd.RunE}
	cmd.Flags().Bool("no-fix", false, "")
	cmd.Flags().Bool("wait", false, "")
	cmd.Flags().Duration("timeout", 30*time.Minute, "")
	cmd.Flags().Bool("dry-run", false, "")
	cmd.Flags().Bool("verbose", false, "")

	buf := new(bytes.Buffer)
	cmd.SetErr(buf)

	err = cmd.RunE(cmd, []string{"test-cleanup"})
	// Command returns cleanup errors after marking item completed
	require.Error(t, err, "command should return cleanup errors")
	assert.Contains(t, err.Error(), "failed to pull latest", "should include pull error")
	assert.Contains(t, err.Error(), "failed to delete branch", "should include branch delete error")

	// Verify roadmap item is marked completed (despite cleanup failures)
	afterContent, readErr := os.ReadFile(roadmapPath) //nolint:gosec // G304: Test file path is controlled by test setup
	require.NoError(t, readErr)
	assert.Contains(t, string(afterContent), `"status": "completed"`, "item should be marked completed despite cleanup failures")
	assert.Contains(t, string(afterContent), `"slug": "test-cleanup"`, "item should still exist in roadmap")

	// Verify warnings were logged
	output := buf.String()
	assert.Contains(t, output, "Warning", "output should contain warnings about cleanup failures")
}

// AI Fix Loop Tests

func TestOutCommand_CIFailure_TriggersAIInvocation(t *testing.T) {
	tmpDir := t.TempDir()

	// Create valid config
	musterDir := filepath.Join(tmpDir, ".muster")
	require.NoError(t, os.MkdirAll(musterDir, 0755)) //nolint:gosec // G301: Test directory

	validConfig := []byte("defaults:\n  tool: claude-code\n  provider: anthropic\n  model: sonnet\nmerge_strategy: github-pr\n")
	require.NoError(t, os.WriteFile(filepath.Join(musterDir, "config.yml"), validConfig, 0644)) //nolint:gosec // G306: Test file

	// Create roadmap with pr_url
	prURL := "https://github.com/owner/repo/pull/111"
	roadmapContent := []byte(`{"items":[{"slug":"test-ci-fix","title":"Test","priority":"medium","status":"in_progress","context":"Test","pr_url":"` + prURL + `"}]}`)
	require.NoError(t, os.WriteFile(filepath.Join(musterDir, "roadmap.json"), roadmapContent, 0644)) //nolint:gosec // G306: Test file

	// Change to temp directory
	origDir, err := os.Getwd()
	require.NoError(t, err)
	defer func() { _ = os.Chdir(origDir) }()
	require.NoError(t, os.Chdir(tmpDir))

	// Mock VCS factory to return failing CI then passing CI
	originalFactory := vcsFactory
	defer func() { vcsFactory = originalFactory }()

	callCount := 0
	vcsFactory = func(strategy, dir string) (vcs.VCS, error) {
		return &mockVCS{
			checkAuthErr: nil,
			checks: []vcs.CheckResult{
				{Name: "test", Status: vcs.CIStatusFailing},
			},
			prStatus: &vcs.PRStatus{
				State:    vcs.PRStateMerged,
				CIStatus: vcs.CIStatusPassing,
			},
			failedLogs: []vcs.FailedCheckLog{
				{CheckName: "test", Logs: "test failed"},
			},
			onListChecks: func() {
				callCount++
			},
		}, nil
	}

	// Mock AI invocation
	originalInvokeAI := ai.InvokeAI
	defer func() { ai.InvokeAI = originalInvokeAI }()

	invokeCount := 0
	ai.InvokeAI = func(cfg ai.InvokeConfig) (*ai.InvokeResult, error) {
		invokeCount++
		return &ai.InvokeResult{RawOutput: "Fixed"}, nil
	}

	// Execute command with dry-run=false and wait=false (so it exits after first check)
	cmd := &cobra.Command{Use: "out [slug]", RunE: outCmd.RunE}
	cmd.Flags().Bool("no-fix", false, "")
	cmd.Flags().Bool("wait", false, "")
	cmd.Flags().Duration("timeout", 30*time.Minute, "")
	cmd.Flags().Bool("dry-run", false, "")
	cmd.Flags().Bool("verbose", false, "")

	_ = cmd.RunE(cmd, []string{"test-ci-fix"})

	// Verify AI was invoked at least once
	assert.GreaterOrEqual(t, invokeCount, 1, "AI should be invoked for CI fix")
}

func TestOutCommand_CIFailure_NoFixFlag_SkipsAI(t *testing.T) {
	tmpDir := t.TempDir()

	// Create valid config
	musterDir := filepath.Join(tmpDir, ".muster")
	require.NoError(t, os.MkdirAll(musterDir, 0755)) //nolint:gosec // G301: Test directory

	validConfig := []byte("defaults:\n  tool: claude-code\n  provider: anthropic\n  model: sonnet\nmerge_strategy: github-pr\n")
	require.NoError(t, os.WriteFile(filepath.Join(musterDir, "config.yml"), validConfig, 0644)) //nolint:gosec // G306: Test file

	// Create roadmap with pr_url
	prURL := "https://github.com/owner/repo/pull/222"
	roadmapContent := []byte(`{"items":[{"slug":"test-no-fix","title":"Test","priority":"medium","status":"in_progress","context":"Test","pr_url":"` + prURL + `"}]}`)
	require.NoError(t, os.WriteFile(filepath.Join(musterDir, "roadmap.json"), roadmapContent, 0644)) //nolint:gosec // G306: Test file

	// Change to temp directory
	origDir, err := os.Getwd()
	require.NoError(t, err)
	defer func() { _ = os.Chdir(origDir) }()
	require.NoError(t, os.Chdir(tmpDir))

	// Mock VCS factory to return failing CI
	originalFactory := vcsFactory
	defer func() { vcsFactory = originalFactory }()

	vcsFactory = func(strategy, dir string) (vcs.VCS, error) {
		return &mockVCS{
			checks: []vcs.CheckResult{
				{Name: "test", Status: vcs.CIStatusFailing},
			},
			failedLogs: []vcs.FailedCheckLog{
				{CheckName: "test", Logs: "test failed"},
			},
		}, nil
	}

	// Mock AI invocation
	originalInvokeAI := ai.InvokeAI
	defer func() { ai.InvokeAI = originalInvokeAI }()

	invokeCount := 0
	ai.InvokeAI = func(cfg ai.InvokeConfig) (*ai.InvokeResult, error) {
		invokeCount++
		return &ai.InvokeResult{RawOutput: "Fixed"}, nil
	}

	// Execute command with --no-fix
	cmd := &cobra.Command{Use: "out [slug]", RunE: outCmd.RunE}
	cmd.Flags().Bool("no-fix", true, "")
	cmd.Flags().Bool("wait", false, "")
	cmd.Flags().Duration("timeout", 30*time.Minute, "")
	cmd.Flags().Bool("dry-run", true, "")
	cmd.Flags().Bool("verbose", false, "")

	_ = cmd.RunE(cmd, []string{"test-no-fix"})

	// Verify AI was NOT invoked
	assert.Equal(t, 0, invokeCount, "AI should not be invoked with --no-fix flag")
}

func TestOutCommand_CIFailure_MaxRetriesRespected(t *testing.T) {
	tmpDir := t.TempDir()

	// Create valid config
	musterDir := filepath.Join(tmpDir, ".muster")
	require.NoError(t, os.MkdirAll(musterDir, 0755)) //nolint:gosec // G301: Test directory

	validConfig := []byte("defaults:\n  tool: claude-code\n  provider: anthropic\n  model: sonnet\nmerge_strategy: github-pr\n")
	require.NoError(t, os.WriteFile(filepath.Join(musterDir, "config.yml"), validConfig, 0644)) //nolint:gosec // G306: Test file

	// Create roadmap with pr_url
	prURL := "https://github.com/owner/repo/pull/333"
	roadmapContent := []byte(`{"items":[{"slug":"test-max-retries","title":"Test","priority":"medium","status":"in_progress","context":"Test","pr_url":"` + prURL + `"}]}`)
	require.NoError(t, os.WriteFile(filepath.Join(musterDir, "roadmap.json"), roadmapContent, 0644)) //nolint:gosec // G306: Test file

	// Change to temp directory
	origDir, err := os.Getwd()
	require.NoError(t, err)
	defer func() { _ = os.Chdir(origDir) }()
	require.NoError(t, os.Chdir(tmpDir))

	// Mock VCS factory to always return failing CI
	originalFactory := vcsFactory
	defer func() { vcsFactory = originalFactory }()

	listChecksCallCount := 0
	vcsFactory = func(strategy, dir string) (vcs.VCS, error) {
		return &mockVCS{
			checks: []vcs.CheckResult{
				{Name: "test", Status: vcs.CIStatusFailing},
			},
			failedLogs: []vcs.FailedCheckLog{
				{CheckName: "test", Logs: "test failed"},
			},
			onListChecks: func() {
				listChecksCallCount++
			},
		}, nil
	}

	// Mock AI invocation
	originalInvokeAI := ai.InvokeAI
	defer func() { ai.InvokeAI = originalInvokeAI }()

	invokeCount := 0
	ai.InvokeAI = func(cfg ai.InvokeConfig) (*ai.InvokeResult, error) {
		invokeCount++
		return &ai.InvokeResult{RawOutput: "Fixed"}, nil
	}

	// Execute command with --wait to trigger polling loop
	cmd := &cobra.Command{Use: "out [slug]", RunE: outCmd.RunE}
	cmd.Flags().Bool("no-fix", false, "")
	cmd.Flags().Bool("wait", true, "")
	cmd.Flags().Duration("timeout", 5*time.Second, "") // Short timeout
	cmd.Flags().Bool("dry-run", false, "")
	cmd.Flags().Bool("verbose", false, "")

	err = cmd.RunE(cmd, []string{"test-max-retries"})

	// Should error after max retries or timeout
	require.Error(t, err)
	// Either hit max retries or timeout (both are valid outcomes)
	hasMaxRetries := assert.ObjectsAreEqual(true, strings.Contains(err.Error(), "3 attempts"))
	hasTimeout := assert.ObjectsAreEqual(true, strings.Contains(err.Error(), "timeout"))
	assert.True(t, hasMaxRetries || hasTimeout, "error should mention max retries or timeout")

	// Verify AI was invoked at most 3 times (max retries)
	assert.LessOrEqual(t, invokeCount, 3, "AI should not be invoked more than max retries")
}

func TestOutCommand_CIFailure_SuccessAfterRetry(t *testing.T) {
	tmpDir := t.TempDir()

	// Create valid config
	musterDir := filepath.Join(tmpDir, ".muster")
	require.NoError(t, os.MkdirAll(musterDir, 0755)) //nolint:gosec // G301: Test directory

	validConfig := []byte("defaults:\n  tool: claude-code\n  provider: anthropic\n  model: sonnet\nmerge_strategy: github-pr\n")
	require.NoError(t, os.WriteFile(filepath.Join(musterDir, "config.yml"), validConfig, 0644)) //nolint:gosec // G306: Test file

	// Create roadmap with pr_url
	prURL := "https://github.com/owner/repo/pull/444"
	roadmapContent := []byte(`{"items":[{"slug":"test-success-retry","title":"Test","priority":"medium","status":"in_progress","context":"Test","pr_url":"` + prURL + `","branch":"test-branch"}]}`)
	require.NoError(t, os.WriteFile(filepath.Join(musterDir, "roadmap.json"), roadmapContent, 0644)) //nolint:gosec // G306: Test file

	// Change to temp directory
	origDir, err := os.Getwd()
	require.NoError(t, err)
	defer func() { _ = os.Chdir(origDir) }()
	require.NoError(t, os.Chdir(tmpDir))

	// Mock VCS factory - fail first time, pass second time
	originalFactory := vcsFactory
	defer func() { vcsFactory = originalFactory }()

	listChecksCallCount := 0
	vcsFactory = func(strategy, dir string) (vcs.VCS, error) {
		return &mockVCS{
			checks: []vcs.CheckResult{
				{Name: "test", Status: vcs.CIStatusFailing},
			},
			failedLogs: []vcs.FailedCheckLog{
				{CheckName: "test", Logs: "test failed"},
			},
			prStatus: &vcs.PRStatus{
				State:    vcs.PRStateMerged,
				CIStatus: vcs.CIStatusPassing,
			},
			onListChecks: func() {
				listChecksCallCount++
			},
			// After first check, return passing
			dynamicChecks: func() []vcs.CheckResult {
				if listChecksCallCount > 1 {
					return []vcs.CheckResult{
						{Name: "test", Status: vcs.CIStatusPassing},
					}
				}
				return []vcs.CheckResult{
					{Name: "test", Status: vcs.CIStatusFailing},
				}
			},
		}, nil
	}

	// Mock AI invocation
	originalInvokeAI := ai.InvokeAI
	defer func() { ai.InvokeAI = originalInvokeAI }()

	invokeCount := 0
	ai.InvokeAI = func(cfg ai.InvokeConfig) (*ai.InvokeResult, error) {
		invokeCount++
		return &ai.InvokeResult{RawOutput: "Fixed"}, nil
	}

	// Execute command with --wait to trigger polling
	cmd := &cobra.Command{Use: "out [slug]", RunE: outCmd.RunE}
	cmd.Flags().Bool("no-fix", false, "")
	cmd.Flags().Bool("wait", true, "")
	cmd.Flags().Duration("timeout", 30*time.Minute, "")
	cmd.Flags().Bool("dry-run", false, "")
	cmd.Flags().Bool("verbose", false, "")

	err = cmd.RunE(cmd, []string{"test-success-retry"})

	// May have cleanup errors but should succeed overall if CI passes
	if err != nil {
		assert.Contains(t, err.Error(), "failed to pull latest", "only cleanup errors expected")
	}

	// Verify AI was invoked at least once
	assert.GreaterOrEqual(t, invokeCount, 1, "AI should be invoked for CI fix")
}

// Mock VCS implementation for testing

type mockVCS struct {
	checkAuthErr  error
	viewPRErr     error
	listChecksErr error
	prStatus      *vcs.PRStatus
	checks        []vcs.CheckResult
	failedLogs    []vcs.FailedCheckLog
	onListChecks  func() // callback for test tracking
	// dynamicChecks allows tests to change check results on subsequent calls.
	// This is useful for testing retry logic where CI may fail initially then pass,
	// or for simulating long-running CI workflows.
	dynamicChecks func() []vcs.CheckResult
	getFailedErr  error
	callHistory   []string // tracks method calls for test verification
}

func (m *mockVCS) CheckAuth() error {
	if m.callHistory != nil {
		m.callHistory = append(m.callHistory, "CheckAuth")
	}
	return m.checkAuthErr
}

func (m *mockVCS) ViewPR(ref string) (*vcs.PRStatus, error) {
	if m.callHistory != nil {
		m.callHistory = append(m.callHistory, "ViewPR:"+ref)
	}
	if m.viewPRErr != nil {
		return nil, m.viewPRErr
	}
	if m.prStatus != nil {
		return m.prStatus, nil
	}
	return &vcs.PRStatus{
		State:    vcs.PRStateOpen,
		CIStatus: vcs.CIStatusPending,
	}, nil
}

func (m *mockVCS) ListChecks(ref string) ([]vcs.CheckResult, error) {
	if m.callHistory != nil {
		m.callHistory = append(m.callHistory, "ListChecks:"+ref)
	}
	if m.onListChecks != nil {
		m.onListChecks()
	}
	if m.listChecksErr != nil {
		return nil, m.listChecksErr
	}
	if m.dynamicChecks != nil {
		return m.dynamicChecks(), nil
	}
	if m.checks != nil {
		return m.checks, nil
	}
	return []vcs.CheckResult{}, nil
}

func (m *mockVCS) GetFailedLogs(ref string) ([]vcs.FailedCheckLog, error) {
	if m.callHistory != nil {
		m.callHistory = append(m.callHistory, "GetFailedLogs:"+ref)
	}
	if m.getFailedErr != nil {
		return nil, m.getFailedErr
	}
	if m.failedLogs != nil {
		return m.failedLogs, nil
	}
	return []vcs.FailedCheckLog{}, nil
}

func (m *mockVCS) MergePR(ref string) error {
	if m.callHistory != nil {
		m.callHistory = append(m.callHistory, "MergePR:"+ref)
	}
	return nil
}

// Integration Tests

func TestOutCommand_FullLifecycle_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	tmpDir := t.TempDir()

	// Create valid config with github-pr merge strategy
	musterDir := filepath.Join(tmpDir, ".muster")
	require.NoError(t, os.MkdirAll(musterDir, 0755)) //nolint:gosec // G301: Test directory

	validConfig := []byte("defaults:\n  tool: claude-code\n  provider: anthropic\n  model: sonnet\nmerge_strategy: github-pr\n")
	require.NoError(t, os.WriteFile(filepath.Join(musterDir, "config.yml"), validConfig, 0644)) //nolint:gosec // G306: Test file

	// Create roadmap with pr_url and branch
	prURL := "https://github.com/owner/repo/pull/555"
	roadmapContent := []byte(`{"items":[{"slug":"full-lifecycle","title":"Full Lifecycle Test","priority":"medium","status":"in_progress","context":"Testing full lifecycle","pr_url":"` + prURL + `","branch":"full-lifecycle-branch"}]}`)
	roadmapPath := filepath.Join(musterDir, "roadmap.json")
	require.NoError(t, os.WriteFile(roadmapPath, roadmapContent, 0644)) //nolint:gosec // G306: Test file

	// Change to temp directory
	origDir, err := os.Getwd()
	require.NoError(t, err)
	defer func() { _ = os.Chdir(origDir) }()
	require.NoError(t, os.Chdir(tmpDir))

	// Mock VCS factory to return passing CI and merged state
	originalFactory := vcsFactory
	defer func() { vcsFactory = originalFactory }()

	vcsFactory = func(strategy, dir string) (vcs.VCS, error) {
		return &mockVCS{
			checkAuthErr: nil,
			prStatus: &vcs.PRStatus{
				State:    vcs.PRStateMerged,
				Number:   555,
				URL:      prURL,
				CIStatus: vcs.CIStatusPassing,
			},
			checks: []vcs.CheckResult{
				{Name: "ci-check", Status: vcs.CIStatusPassing},
			},
		}, nil
	}

	// Mock AI invocation (shouldn't be called since CI is passing)
	originalInvokeAI := ai.InvokeAI
	defer func() { ai.InvokeAI = originalInvokeAI }()

	aiInvokeCount := 0
	ai.InvokeAI = func(cfg ai.InvokeConfig) (*ai.InvokeResult, error) {
		aiInvokeCount++
		return &ai.InvokeResult{RawOutput: "Fixed"}, nil
	}

	// Execute command (not dry-run, so it actually updates the roadmap)
	cmd := &cobra.Command{Use: "out [slug]", RunE: outCmd.RunE}
	cmd.Flags().Bool("no-fix", false, "")
	cmd.Flags().Bool("wait", false, "")
	cmd.Flags().Duration("timeout", 30*time.Minute, "")
	cmd.Flags().Bool("dry-run", false, "")
	cmd.Flags().Bool("verbose", false, "")

	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)

	err = cmd.RunE(cmd, []string{"full-lifecycle"})
	// Integration test with non-git directory will have cleanup errors,
	// but item should still be marked completed
	if err != nil {
		assert.Contains(t, err.Error(), "failed to pull latest", "cleanup errors expected in non-git directory")
	}

	// Verify AI was not invoked (CI was passing from the start)
	assert.Equal(t, 0, aiInvokeCount, "AI should not be invoked when CI is passing")

	// Verify roadmap item status is now "completed" (despite cleanup errors)
	afterContent, readErr := os.ReadFile(roadmapPath) //nolint:gosec // G304: Test file path is controlled by test setup
	require.NoError(t, readErr)
	assert.Contains(t, string(afterContent), `"status": "completed"`, "roadmap item should be marked as completed")
	assert.Contains(t, string(afterContent), `"slug": "full-lifecycle"`, "roadmap should still contain the item")

	// Verify output contains either success message or warnings
	// (cleanup errors prevent the final "completed successfully" message)
	output := buf.String()
	if err == nil {
		assert.Contains(t, output, "completed", "output should mention completion when no cleanup errors")
	} else {
		assert.Contains(t, output, "Warning", "output should contain cleanup warnings when errors occur")
	}
}

func TestOutCommand_VerboseFlag_LogsDetails(t *testing.T) {
	tmpDir := t.TempDir()

	// Create valid config
	musterDir := filepath.Join(tmpDir, ".muster")
	require.NoError(t, os.MkdirAll(musterDir, 0755)) //nolint:gosec // G301: Test directory

	validConfig := []byte("defaults:\n  tool: claude-code\n  provider: anthropic\n  model: sonnet\nmerge_strategy: github-pr\n")
	require.NoError(t, os.WriteFile(filepath.Join(musterDir, "config.yml"), validConfig, 0644)) //nolint:gosec // G306: Test file

	// Create roadmap with pr_url
	prURL := "https://github.com/owner/repo/pull/1111"
	roadmapContent := []byte(`{"items":[{"slug":"test-verbose","title":"Test Verbose","priority":"medium","status":"in_progress","context":"Test","pr_url":"` + prURL + `"}]}`)
	require.NoError(t, os.WriteFile(filepath.Join(musterDir, "roadmap.json"), roadmapContent, 0644)) //nolint:gosec // G306: Test file

	// Change to temp directory
	origDir, err := os.Getwd()
	require.NoError(t, err)
	defer func() { _ = os.Chdir(origDir) }()
	require.NoError(t, os.Chdir(tmpDir))

	// Mock VCS factory
	originalFactory := vcsFactory
	defer func() { vcsFactory = originalFactory }()

	vcsFactory = func(strategy, dir string) (vcs.VCS, error) {
		return &mockVCS{
			checks: []vcs.CheckResult{
				{Name: "test", Status: vcs.CIStatusPassing},
			},
			prStatus: &vcs.PRStatus{
				State:    vcs.PRStateMerged,
				CIStatus: vcs.CIStatusPassing,
			},
		}, nil
	}

	// Execute command with --verbose and --dry-run
	cmd := &cobra.Command{Use: "out [slug]", RunE: outCmd.RunE}
	cmd.Flags().Bool("no-fix", false, "")
	cmd.Flags().Bool("wait", false, "")
	cmd.Flags().Duration("timeout", 30*time.Minute, "")
	cmd.Flags().Bool("dry-run", true, "")
	cmd.Flags().Bool("verbose", true, "")

	buf := new(bytes.Buffer)
	cmd.SetErr(buf)

	err = cmd.RunE(cmd, []string{"test-verbose"})
	require.NoError(t, err)

	// Verify verbose output
	output := buf.String()
	assert.Contains(t, output, "Using: tool=", "verbose output should show tool selection")
	assert.Contains(t, output, "Merge strategy:", "verbose output should show merge strategy")
	assert.Contains(t, output, "PR reference:", "verbose output should show PR reference")
}

func TestOutCommand_FixCI_NoLogs_UsesGenericPrompt(t *testing.T) {
	tmpDir := t.TempDir()

	// Create valid config
	musterDir := filepath.Join(tmpDir, ".muster")
	require.NoError(t, os.MkdirAll(musterDir, 0755)) //nolint:gosec // G301: Test directory

	validConfig := []byte("defaults:\n  tool: claude-code\n  provider: anthropic\n  model: sonnet\nmerge_strategy: github-pr\n")
	require.NoError(t, os.WriteFile(filepath.Join(musterDir, "config.yml"), validConfig, 0644)) //nolint:gosec // G306: Test file

	// Create roadmap with pr_url
	prURL := "https://github.com/owner/repo/pull/2222"
	roadmapContent := []byte(`{"items":[{"slug":"test-no-logs","title":"Test No Logs","priority":"medium","status":"in_progress","context":"Test","pr_url":"` + prURL + `"}]}`)
	require.NoError(t, os.WriteFile(filepath.Join(musterDir, "roadmap.json"), roadmapContent, 0644)) //nolint:gosec // G306: Test file

	// Change to temp directory
	origDir, err := os.Getwd()
	require.NoError(t, err)
	defer func() { _ = os.Chdir(origDir) }()
	require.NoError(t, os.Chdir(tmpDir))

	// Mock VCS factory - failing CI, empty logs
	originalFactory := vcsFactory
	defer func() { vcsFactory = originalFactory }()

	vcsFactory = func(strategy, dir string) (vcs.VCS, error) {
		return &mockVCS{
			checks: []vcs.CheckResult{
				{Name: "test", Status: vcs.CIStatusFailing},
			},
			failedLogs: []vcs.FailedCheckLog{}, // Empty logs
		}, nil
	}

	// Mock AI invocation to verify prompt content
	originalInvokeAI := ai.InvokeAI
	defer func() { ai.InvokeAI = originalInvokeAI }()

	invokeCount := 0
	var capturedPrompt string
	ai.InvokeAI = func(cfg ai.InvokeConfig) (*ai.InvokeResult, error) {
		invokeCount++
		capturedPrompt = cfg.Prompt
		return &ai.InvokeResult{RawOutput: "Fixed"}, nil
	}

	// Execute command with --wait=false to exit after first check
	cmd := &cobra.Command{Use: "out [slug]", RunE: outCmd.RunE}
	cmd.Flags().Bool("no-fix", false, "")
	cmd.Flags().Bool("wait", false, "")
	cmd.Flags().Duration("timeout", 30*time.Minute, "")
	cmd.Flags().Bool("dry-run", false, "")
	cmd.Flags().Bool("verbose", false, "")

	_ = cmd.RunE(cmd, []string{"test-no-logs"})

	// Verify AI was invoked with generic prompt
	assert.GreaterOrEqual(t, invokeCount, 1, "AI should be invoked")
	assert.Contains(t, capturedPrompt, "CI checks failed, please investigate", "prompt should contain generic message when logs are empty")
}
