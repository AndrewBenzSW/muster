package cmd

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDownCommand_Exists(t *testing.T) {
	// Verify the command exists and can be retrieved
	assert.NotNil(t, downCmd, "down command should exist")
	assert.Equal(t, "down [slug]", downCmd.Use, "command use should be 'down [slug]'")
}

func TestDownCommand_HasExpectedFlags(t *testing.T) {
	// Test all flag
	allFlag := downCmd.Flags().Lookup("all")
	assert.NotNil(t, allFlag, "all flag should exist")
	if allFlag != nil {
		assert.Equal(t, "bool", allFlag.Value.Type(), "all flag should be a bool")
		assert.Equal(t, "false", allFlag.DefValue, "all flag default should be false")
	}

	// Test orphans flag
	orphansFlag := downCmd.Flags().Lookup("orphans")
	assert.NotNil(t, orphansFlag, "orphans flag should exist")
	if orphansFlag != nil {
		assert.Equal(t, "bool", orphansFlag.Value.Type(), "orphans flag should be a bool")
		assert.Equal(t, "false", orphansFlag.DefValue, "orphans flag default should be false")
	}

	// Test project flag
	projectFlag := downCmd.Flags().Lookup("project")
	assert.NotNil(t, projectFlag, "project flag should exist")
	if projectFlag != nil {
		assert.Equal(t, "string", projectFlag.Value.Type(), "project flag should be a string")
		assert.Equal(t, "", projectFlag.DefValue, "project flag default should be empty string")
	}
}

func TestDownCommand_WithHelpFlag_Succeeds(t *testing.T) {
	// Create a fresh command instance to avoid state pollution
	cmd := &cobra.Command{
		Use:   "down [slug]",
		Short: "Stop and remove Docker containers",
		RunE:  downCmd.RunE,
	}

	// Copy flags from downCmd
	cmd.Flags().AddFlagSet(downCmd.Flags())

	// Create a buffer to capture output
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)

	// Set args to --help
	cmd.SetArgs([]string{"--help"})

	// Execute should not return an error with --help
	err := cmd.Execute()
	assert.NoError(t, err, "command should execute without error with --help flag")

	// Verify output contains usage information
	output := buf.String()
	assert.NotEmpty(t, output, "help output should not be empty")
	assert.Contains(t, output, "down", "help should contain command name")
	assert.Contains(t, output, "Stop and remove Docker containers", "help should contain description")
	assert.Contains(t, output, "--all", "help should contain --all flag")
	assert.Contains(t, output, "--orphans", "help should contain --orphans flag")
	assert.Contains(t, output, "--project", "help should contain --project flag")
}

func TestDownCommand_Description_Contains_KeyFeatures(t *testing.T) {
	// Verify the command description contains key features
	longDesc := downCmd.Long

	assert.Contains(t, longDesc, "Without arguments", "description should mention default behavior")
	assert.Contains(t, longDesc, "With slug argument", "description should mention slug argument")
	assert.Contains(t, longDesc, "--all", "description should mention --all flag")
	assert.Contains(t, longDesc, "--orphans", "description should mention --orphans flag")
	assert.Contains(t, longDesc, "Examples", "description should have examples")
}

func TestDownCommand_ShortDescription_IsConcise(t *testing.T) {
	// Verify the short description is actually short and descriptive
	shortDesc := downCmd.Short

	assert.NotEmpty(t, shortDesc, "short description should not be empty")
	assert.Less(t, len(shortDesc), 100, "short description should be less than 100 characters")
	assert.Contains(t, strings.ToLower(shortDesc), "stop", "short description should mention stop")
	assert.Contains(t, strings.ToLower(shortDesc), "docker", "short description should mention Docker")
}

func TestDownCommand_HasRunEFunction(t *testing.T) {
	// Verify the command has a RunE function
	assert.NotNil(t, downCmd.RunE, "down command should have a RunE function")
}

func TestDownCommand_IsAddedToRootCommand(t *testing.T) {
	// Verify the down command is added as a subcommand of root
	found := false
	for _, cmd := range rootCmd.Commands() {
		if cmd.Name() == "down" {
			found = true
			break
		}
	}
	assert.True(t, found, "down command should be added to root command")
}

func TestDownCommand_FlagParsing_All(t *testing.T) {
	// Test that --all flag can be set without error
	err := downCmd.Flags().Set("all", "true")
	assert.NoError(t, err, "setting all flag should not error")

	// Verify the flag value
	allValue, err := downCmd.Flags().GetBool("all")
	assert.NoError(t, err, "getting all flag should not error")
	assert.True(t, allValue, "all flag should be true")

	// Reset to default
	err = downCmd.Flags().Set("all", "false")
	assert.NoError(t, err, "resetting all flag should not error")
}

func TestDownCommand_FlagParsing_Orphans(t *testing.T) {
	// Test that --orphans flag can be set without error
	err := downCmd.Flags().Set("orphans", "true")
	assert.NoError(t, err, "setting orphans flag should not error")

	// Verify the flag value
	orphansValue, err := downCmd.Flags().GetBool("orphans")
	assert.NoError(t, err, "getting orphans flag should not error")
	assert.True(t, orphansValue, "orphans flag should be true")

	// Reset to default
	err = downCmd.Flags().Set("orphans", "false")
	assert.NoError(t, err, "resetting orphans flag should not error")
}

func TestDownCommand_FlagParsing_Project(t *testing.T) {
	// Test that --project flag can be set without error
	err := downCmd.Flags().Set("project", "myproject")
	assert.NoError(t, err, "setting project flag should not error")

	// Verify the flag value
	projectValue, err := downCmd.Flags().GetString("project")
	assert.NoError(t, err, "getting project flag should not error")
	assert.Equal(t, "myproject", projectValue, "project flag should have the set value")

	// Reset to default
	err = downCmd.Flags().Set("project", "")
	assert.NoError(t, err, "resetting project flag should not error")
}

func TestDownCommand_AllFlags_HaveUsageText(t *testing.T) {
	// Verify all flags have non-empty usage text
	flags := []string{"all", "orphans", "project"}

	for _, flagName := range flags {
		flag := downCmd.Flags().Lookup(flagName)
		require.NotNil(t, flag, "flag %s should exist", flagName)
		assert.NotEmpty(t, flag.Usage, "flag %s should have usage text", flagName)
	}
}

// Integration Tests

func TestDownCommand_WithoutDocker_ReturnsActionableError(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// This test verifies that when Docker is not running, we get an actionable error
	// We can't easily mock Docker being down, so this test may pass if Docker is running
	// The key is that IF Docker is not running, the error should be actionable

	cmd := &cobra.Command{
		Use:  "down",
		RunE: downCmd.RunE,
	}
	cmd.Flags().Bool("all", false, "")
	cmd.Flags().Bool("orphans", false, "")
	cmd.Flags().String("project", "", "")
	cmd.Flags().Bool("verbose", false, "")

	err := cmd.RunE(cmd, []string{})

	// If Docker is not running, should get error mentioning Docker
	if err != nil {
		errMsg := strings.ToLower(err.Error())
		hasDockerError := strings.Contains(errMsg, "docker") ||
			strings.Contains(errMsg, "daemon") ||
			strings.Contains(errMsg, "compose")

		assert.True(t, hasDockerError,
			"error should mention Docker/daemon/compose: %s", err.Error())
	}
	// If no error, Docker is running and no containers to stop (which is fine)
}

func TestDownCommand_WithSlugArgument_AcceptsSlug(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Test that the command accepts a slug argument
	cmd := &cobra.Command{
		Use:  "down [slug]",
		RunE: downCmd.RunE,
	}
	cmd.Flags().Bool("all", false, "")
	cmd.Flags().Bool("orphans", false, "")
	cmd.Flags().String("project", "", "")
	cmd.Flags().Bool("verbose", false, "")

	// Execute with a slug argument
	err := cmd.RunE(cmd, []string{"my-feature"})

	// Should either succeed (no containers) or fail with Docker error
	// We're mainly testing that the slug argument is accepted
	if err != nil {
		errMsg := strings.ToLower(err.Error())
		// Should not error on argument parsing
		assert.NotContains(t, errMsg, "unknown flag", "should not error on unknown flag")
		assert.NotContains(t, errMsg, "too many arguments", "should not error on too many arguments")
	}
}

func TestLoadInProgressSlugs_MissingRoadmap_ReturnsError(t *testing.T) {
	// Create temp dir without roadmap
	tmpDir := t.TempDir()
	oldDir, err := os.Getwd()
	require.NoError(t, err)
	defer func() { _ = os.Chdir(oldDir) }()
	err = os.Chdir(tmpDir)
	require.NoError(t, err)

	// Should return error when roadmap doesn't exist
	slugs, err := loadInProgressSlugs()
	require.Error(t, err, "should error when roadmap.json doesn't exist")
	assert.Nil(t, slugs, "slugs should be nil on error")
	assert.Contains(t, err.Error(), "roadmap.json", "error should mention roadmap.json")
}

func TestLoadInProgressSlugs_ArrayFormat(t *testing.T) {
	// Create temp dir with roadmap in array format
	tmpDir := t.TempDir()
	roadmapContent := `[
		{"slug": "feature-1", "status": "in_progress"},
		{"slug": "feature-2", "status": "planned"},
		{"slug": "feature-3", "status": "in_progress"}
	]`

	// Create directory first
	err := os.MkdirAll(tmpDir+"/.muster", 0755) //nolint:gosec // G301: Test directory permissions
	require.NoError(t, err)
	err = os.WriteFile(tmpDir+"/.muster/roadmap.json", []byte(roadmapContent), 0644) //nolint:gosec // G306: Test file permissions
	require.NoError(t, err)

	oldDir, err := os.Getwd()
	require.NoError(t, err)
	defer func() { _ = os.Chdir(oldDir) }()
	err = os.Chdir(tmpDir)
	require.NoError(t, err)

	slugs, err := loadInProgressSlugs()
	require.NoError(t, err, "should parse array format roadmap")
	assert.Len(t, slugs, 2, "should have 2 in_progress slugs")
	assert.True(t, slugs["feature-1"], "feature-1 should be in_progress")
	assert.False(t, slugs["feature-2"], "feature-2 should not be in_progress")
	assert.True(t, slugs["feature-3"], "feature-3 should be in_progress")
}

func TestLoadInProgressSlugs_WrapperFormat(t *testing.T) {
	// Create temp dir with roadmap in wrapper format
	tmpDir := t.TempDir()
	roadmapContent := `{
		"items": [
			{"slug": "task-1", "status": "in_progress"},
			{"slug": "task-2", "status": "completed"}
		]
	}`

	err := os.MkdirAll(tmpDir+"/.muster", 0755) //nolint:gosec // G301: Test directory permissions
	require.NoError(t, err)
	err = os.WriteFile(tmpDir+"/.muster/roadmap.json", []byte(roadmapContent), 0644) //nolint:gosec // G306: Test file permissions
	require.NoError(t, err)

	oldDir, err := os.Getwd()
	require.NoError(t, err)
	defer func() { _ = os.Chdir(oldDir) }()
	err = os.Chdir(tmpDir)
	require.NoError(t, err)

	slugs, err := loadInProgressSlugs()
	require.NoError(t, err, "should parse wrapper format roadmap")
	assert.Len(t, slugs, 1, "should have 1 in_progress slug")
	assert.True(t, slugs["task-1"], "task-1 should be in_progress")
	assert.False(t, slugs["task-2"], "task-2 should not be in_progress")
}

func TestLoadInProgressSlugs_LegacyLocation(t *testing.T) {
	// Create temp dir with roadmap in legacy location
	tmpDir := t.TempDir()
	roadmapContent := `[{"slug": "legacy", "status": "in_progress"}]`

	err := os.WriteFile(tmpDir+"/.roadmap.json", []byte(roadmapContent), 0644) //nolint:gosec // G306: Test file permissions
	require.NoError(t, err)

	oldDir, err := os.Getwd()
	require.NoError(t, err)
	defer func() { _ = os.Chdir(oldDir) }()
	err = os.Chdir(tmpDir)
	require.NoError(t, err)

	slugs, err := loadInProgressSlugs()
	require.NoError(t, err, "should parse legacy location roadmap")
	assert.Len(t, slugs, 1, "should have 1 in_progress slug")
	assert.True(t, slugs["legacy"], "legacy should be in_progress")
}

func TestMapKeys_ReturnsAllKeys(t *testing.T) {
	m := map[string]bool{
		"key1": true,
		"key2": false,
		"key3": true,
	}

	keys := mapKeys(m)
	assert.Len(t, keys, 3, "should return all keys")
	assert.Contains(t, keys, "key1")
	assert.Contains(t, keys, "key2")
	assert.Contains(t, keys, "key3")
}

func TestMapKeys_EmptyMap_ReturnsEmptySlice(t *testing.T) {
	m := map[string]bool{}
	keys := mapKeys(m)
	assert.Len(t, keys, 0, "should return empty slice for empty map")
	assert.NotNil(t, keys, "should not return nil")
}
