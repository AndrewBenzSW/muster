package cmd

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCodeCommand_Exists(t *testing.T) {
	// Verify the command exists and can be retrieved
	assert.NotNil(t, codeCmd, "code command should exist")
	assert.Equal(t, "code", codeCmd.Use, "command use should be 'code'")
}

func TestCodeCommand_HasExpectedFlags(t *testing.T) {
	// Test tool flag
	toolFlag := codeCmd.PersistentFlags().Lookup("tool")
	assert.NotNil(t, toolFlag, "tool flag should exist")
	if toolFlag != nil {
		assert.Equal(t, "string", toolFlag.Value.Type(), "tool flag should be a string")
		assert.Equal(t, "", toolFlag.DefValue, "tool flag default should be empty string")
	}

	// Test no-plugin flag
	noPluginFlag := codeCmd.PersistentFlags().Lookup("no-plugin")
	assert.NotNil(t, noPluginFlag, "no-plugin flag should exist")
	if noPluginFlag != nil {
		assert.Equal(t, "bool", noPluginFlag.Value.Type(), "no-plugin flag should be a bool")
		assert.Equal(t, "false", noPluginFlag.DefValue, "no-plugin flag default should be false")
	}

	// Test keep-staged flag
	keepStagedFlag := codeCmd.PersistentFlags().Lookup("keep-staged")
	assert.NotNil(t, keepStagedFlag, "keep-staged flag should exist")
	if keepStagedFlag != nil {
		assert.Equal(t, "bool", keepStagedFlag.Value.Type(), "keep-staged flag should be a bool")
		assert.Equal(t, "false", keepStagedFlag.DefValue, "keep-staged flag default should be false")
	}

	// Test yolo flag (local flag)
	yoloFlag := codeCmd.Flags().Lookup("yolo")
	assert.NotNil(t, yoloFlag, "yolo flag should exist")
	if yoloFlag != nil {
		assert.Equal(t, "bool", yoloFlag.Value.Type(), "yolo flag should be a bool")
		assert.Equal(t, "false", yoloFlag.DefValue, "yolo flag default should be false")
	}
}

func TestCodeCommand_WithHelpFlag_Succeeds(t *testing.T) {
	// Create a fresh command instance to avoid state pollution
	cmd := &cobra.Command{
		Use:   "code",
		Short: "Launch Claude/OpenCode with workflow skills staged",
		RunE:  codeCmd.RunE,
	}

	// Copy flags from codeCmd
	cmd.PersistentFlags().AddFlagSet(codeCmd.PersistentFlags())
	cmd.Flags().AddFlagSet(codeCmd.Flags())

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
	assert.Contains(t, output, "code", "help should contain command name")
	assert.Contains(t, output, "Launch Claude/OpenCode", "help should contain description")
	assert.Contains(t, output, "--tool", "help should contain --tool flag")
	assert.Contains(t, output, "--no-plugin", "help should contain --no-plugin flag")
	assert.Contains(t, output, "--keep-staged", "help should contain --keep-staged flag")
	assert.Contains(t, output, "--yolo", "help should contain --yolo flag")
}

func TestCodeCommand_WithYoloFlag_RequiresDocker(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Create a fresh command instance to avoid state pollution
	cmd := &cobra.Command{
		Use:  "code",
		RunE: codeCmd.RunE,
	}
	cmd.Flags().Bool("yolo", false, "Run in sandboxed container mode")
	cmd.Flags().Bool("verbose", false, "Enable verbose output")

	// Set the yolo flag
	err := cmd.Flags().Set("yolo", "true")
	require.NoError(t, err, "setting yolo flag should not error")

	// Execute the command - should fail if Docker not running or config missing
	err = cmd.RunE(cmd, []string{})
	require.Error(t, err, "command should return error with --yolo flag without proper config")

	// Verify error is actionable (mentions Docker or config)
	errMsg := err.Error()
	hasDockerError := strings.Contains(strings.ToLower(errMsg), "docker") ||
		strings.Contains(strings.ToLower(errMsg), "daemon")
	hasConfigError := strings.Contains(strings.ToLower(errMsg), "config")

	assert.True(t, hasDockerError || hasConfigError,
		"error should mention Docker daemon or config: %s", errMsg)
}

func TestCodeCommand_YoloWithoutConfig_ReturnsActionableError(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Test that --yolo without config returns actionable error
	cmd := &cobra.Command{
		Use:  "code",
		RunE: codeCmd.RunE,
	}
	cmd.Flags().Bool("yolo", false, "")
	cmd.Flags().Bool("verbose", false, "")
	_ = cmd.Flags().Set("yolo", "true")

	// Create temp dir without config
	tmpDir := t.TempDir()
	oldDir, err := os.Getwd()
	require.NoError(t, err)
	defer func() { _ = os.Chdir(oldDir) }()
	err = os.Chdir(tmpDir)
	require.NoError(t, err)

	err = cmd.RunE(cmd, []string{})
	require.Error(t, err, "command should error without config")

	// Error should be actionable
	errMsg := err.Error()
	assert.NotEmpty(t, errMsg, "error message should not be empty")
}

func TestCodeCommand_FlagParsing_ToolOverride(t *testing.T) {
	// Test that --tool flag can be set without error
	err := codeCmd.PersistentFlags().Set("tool", "opencode")
	assert.NoError(t, err, "setting tool flag should not error")

	// Verify the flag value
	toolValue, err := codeCmd.PersistentFlags().GetString("tool")
	assert.NoError(t, err, "getting tool flag should not error")
	assert.Equal(t, "opencode", toolValue, "tool flag should have the set value")

	// Reset to default
	err = codeCmd.PersistentFlags().Set("tool", "")
	assert.NoError(t, err, "resetting tool flag should not error")
}

func TestCodeCommand_FlagParsing_NoPlugin(t *testing.T) {
	// Test that --no-plugin flag can be set without error
	err := codeCmd.PersistentFlags().Set("no-plugin", "true")
	assert.NoError(t, err, "setting no-plugin flag should not error")

	// Verify the flag value
	noPluginValue, err := codeCmd.PersistentFlags().GetBool("no-plugin")
	assert.NoError(t, err, "getting no-plugin flag should not error")
	assert.True(t, noPluginValue, "no-plugin flag should be true")

	// Reset to default
	err = codeCmd.PersistentFlags().Set("no-plugin", "false")
	assert.NoError(t, err, "resetting no-plugin flag should not error")
}

func TestCodeCommand_FlagParsing_KeepStaged(t *testing.T) {
	// Test that --keep-staged flag can be set without error
	err := codeCmd.PersistentFlags().Set("keep-staged", "true")
	assert.NoError(t, err, "setting keep-staged flag should not error")

	// Verify the flag value
	keepStagedValue, err := codeCmd.PersistentFlags().GetBool("keep-staged")
	assert.NoError(t, err, "getting keep-staged flag should not error")
	assert.True(t, keepStagedValue, "keep-staged flag should be true")

	// Reset to default
	err = codeCmd.PersistentFlags().Set("keep-staged", "false")
	assert.NoError(t, err, "resetting keep-staged flag should not error")
}

func TestCodeCommand_Description_Contains_KeyFeatures(t *testing.T) {
	// Verify the command description contains key features
	longDesc := codeCmd.Long

	assert.Contains(t, longDesc, "Loads project and user configuration", "description should mention config loading")
	assert.Contains(t, longDesc, "Resolves the tool, provider, and model", "description should mention resolution")
	assert.Contains(t, longDesc, "Stages workflow skill templates", "description should mention staging")
	assert.Contains(t, longDesc, "Executes the resolved tool", "description should mention execution")
	assert.Contains(t, longDesc, "workflow skills", "description should mention workflow skills")
}

func TestCodeCommand_ShortDescription_IsConcise(t *testing.T) {
	// Verify the short description is actually short and descriptive
	shortDesc := codeCmd.Short

	assert.NotEmpty(t, shortDesc, "short description should not be empty")
	assert.Less(t, len(shortDesc), 100, "short description should be less than 100 characters")
	assert.Contains(t, strings.ToLower(shortDesc), "claude", "short description should mention Claude")
	assert.Contains(t, strings.ToLower(shortDesc), "workflow", "short description should mention workflow")
}

func TestCodeCommand_HasRunEFunction(t *testing.T) {
	// Verify the command has a RunE function
	assert.NotNil(t, codeCmd.RunE, "code command should have a RunE function")
}

func TestCodeCommand_IsAddedToRootCommand(t *testing.T) {
	// Verify the code command is added as a subcommand of root
	found := false
	for _, cmd := range rootCmd.Commands() {
		if cmd.Name() == "code" {
			found = true
			break
		}
	}
	assert.True(t, found, "code command should be added to root command")
}

func TestCodeCommand_FlagTypes_AreCorrect(t *testing.T) {
	tests := []struct {
		name         string
		flagName     string
		expectedType string
		isPersistent bool
	}{
		{
			name:         "tool flag is string",
			flagName:     "tool",
			expectedType: "string",
			isPersistent: true,
		},
		{
			name:         "no-plugin flag is bool",
			flagName:     "no-plugin",
			expectedType: "bool",
			isPersistent: true,
		},
		{
			name:         "keep-staged flag is bool",
			flagName:     "keep-staged",
			expectedType: "bool",
			isPersistent: true,
		},
		{
			name:         "yolo flag is bool",
			flagName:     "yolo",
			expectedType: "bool",
			isPersistent: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var flag *pflag.Flag
			if tt.isPersistent {
				flag = codeCmd.PersistentFlags().Lookup(tt.flagName)
			} else {
				flag = codeCmd.Flags().Lookup(tt.flagName)
			}

			require.NotNil(t, flag, "flag %s should exist", tt.flagName)
			assert.Equal(t, tt.expectedType, flag.Value.Type(), "flag %s should have type %s", tt.flagName, tt.expectedType)
		})
	}
}

func TestCodeCommand_AllFlags_HaveUsageText(t *testing.T) {
	// Verify all flags have non-empty usage text
	flags := []*pflag.Flag{
		codeCmd.PersistentFlags().Lookup("tool"),
		codeCmd.PersistentFlags().Lookup("no-plugin"),
		codeCmd.PersistentFlags().Lookup("keep-staged"),
		codeCmd.Flags().Lookup("yolo"),
	}

	for _, flag := range flags {
		require.NotNil(t, flag, "flag should exist")
		assert.NotEmpty(t, flag.Usage, "flag %s should have usage text", flag.Name)
	}
}

// Integration Tests

func TestCodeCommand_ConfigLoadingFailure_MalformedYAML(t *testing.T) {
	// Create a command with a malformed config file
	cmd := &cobra.Command{
		Use:  "code",
		RunE: codeCmd.RunE,
	}
	cmd.PersistentFlags().String("tool", "", "")
	cmd.PersistentFlags().Bool("no-plugin", false, "")
	cmd.PersistentFlags().Bool("keep-staged", false, "")
	cmd.Flags().Bool("yolo", false, "")
	cmd.Flags().Bool("verbose", false, "")

	// Create a temporary directory with a malformed config
	tmpDir := t.TempDir()
	malformedConfig := tmpDir + "/.muster/config.yml"
	err := os.MkdirAll(tmpDir+"/.muster", 0755) //nolint:gosec // G301: Test directory permissions
	require.NoError(t, err)

	malformedContent := []byte("defaults:\n  tool: [invalid yaml")
	err = os.WriteFile(malformedConfig, malformedContent, 0644) //nolint:gosec // G306: Test file permissions
	require.NoError(t, err)

	// Change to temp directory
	oldDir, err := os.Getwd()
	require.NoError(t, err)
	defer func() { _ = os.Chdir(oldDir) }() // Error in defer is not critical for test cleanup

	err = os.Chdir(tmpDir)
	require.NoError(t, err)

	// Execute the command - should fail with parse error
	err = cmd.RunE(cmd, []string{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "config file malformed", "error should indicate malformed config")
}

func TestCodeCommand_ToolNotFound_ErrorMessage(t *testing.T) {
	// Skip this test if we can't mock exec.Command easily
	// This is a placeholder showing the test structure
	t.Skip("Requires mocking exec.Command - implementation depends on test infrastructure")
}

func TestCodeCommand_NoPlugin_SkipsStaging(t *testing.T) {
	// Test that --no-plugin prevents staging
	// This would require mocking or a test binary
	t.Skip("Requires mocking StageSkills or test binary - implementation depends on test infrastructure")
}

func TestCodeCommand_KeepStaged_PreservesDirectory(t *testing.T) {
	// Test that --keep-staged prevents cleanup
	// This would require integration with actual staging
	t.Skip("Requires integration with staging system - implementation depends on test infrastructure")
}

func TestCodeCommand_ToolOverride_UsesSpecifiedTool(t *testing.T) {
	// Test that --tool flag overrides config
	// This would require mocking exec.Command
	t.Skip("Requires mocking exec.Command - implementation depends on test infrastructure")
}
