package cmd

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/abenz1267/muster/internal/ui"
	"github.com/stretchr/testify/assert"
)

func TestVersionVars_AreSet(t *testing.T) {
	// Check that version, commit, date variables are all non-empty strings
	assert.NotEmpty(t, version, "version variable should not be empty")
	assert.NotEmpty(t, commit, "commit variable should not be empty")
	assert.NotEmpty(t, date, "date variable should not be empty")
}

func TestVersionCommand_Exists(t *testing.T) {
	// Just verify the command exists and can be retrieved
	assert.NotNil(t, versionCmd, "version command should exist")
	assert.Equal(t, "version", versionCmd.Use, "command use should be 'version'")
}

func TestFormatVersion_TableMode(t *testing.T) {
	originalMode := ui.GetOutputMode()
	t.Cleanup(func() { ui.SetOutputMode(originalMode) })

	ui.SetOutputMode(ui.TableMode)
	info := ui.VersionInfo{
		Version:   "v1.0.0",
		Commit:    "abc123",
		Date:      "2024-01-01",
		GoVersion: "go1.23",
		Platform:  "linux/amd64",
	}
	output, err := ui.FormatVersion(info)
	assert.NoError(t, err)
	assert.Contains(t, output, "Version:")
	assert.Contains(t, output, "v1.0.0")
}

func TestFormatVersion_JSONMode(t *testing.T) {
	originalMode := ui.GetOutputMode()
	t.Cleanup(func() { ui.SetOutputMode(originalMode) })

	ui.SetOutputMode(ui.JSONMode)
	info := ui.VersionInfo{
		Version:   "v1.0.0",
		Commit:    "abc123",
		Date:      "2024-01-01",
		GoVersion: "go1.23",
		Platform:  "linux/amd64",
	}
	output, err := ui.FormatVersion(info)
	assert.NoError(t, err)
	assert.Contains(t, output, `"version"`)
	assert.Contains(t, output, `"v1.0.0"`)
}

func TestVersionCommand_RunE_ExecutesWithoutError(t *testing.T) {
	// Test that the RunE function executes without error
	buf := new(bytes.Buffer)
	versionCmd.SetOut(buf)

	err := versionCmd.RunE(versionCmd, []string{})
	assert.NoError(t, err, "RunE should execute without error")

	// Verify output contains expected fields
	output := buf.String()
	assert.NotEmpty(t, output, "output should not be empty")
	assert.Contains(t, output, "dev", "output should contain version")
	assert.Contains(t, output, "none", "output should contain commit")
	assert.Contains(t, output, "unknown", "output should contain date")
}

func TestVersionCommand_WithFormatJSON(t *testing.T) {
	// Save and restore output mode
	originalMode := ui.GetOutputMode()
	defer ui.SetOutputMode(originalMode)

	// Set JSON mode
	ui.SetOutputMode(ui.JSONMode)

	// Create a buffer to capture output
	buf := new(bytes.Buffer)
	versionCmd.SetOut(buf)

	// Execute command's RunE function directly
	err := versionCmd.RunE(versionCmd, []string{})
	assert.NoError(t, err, "command should execute without error")

	// Verify output is valid JSON
	output := buf.String()
	assert.NotEmpty(t, output, "output should not be empty")

	// Parse JSON to verify it's valid
	var info ui.VersionInfo
	err = json.Unmarshal([]byte(output), &info)
	assert.NoError(t, err, "output should be valid JSON")

	// Verify expected fields are present
	assert.NotEmpty(t, info.Version, "version should be present")
	assert.NotEmpty(t, info.Commit, "commit should be present")
	assert.NotEmpty(t, info.Date, "date should be present")
	assert.NotEmpty(t, info.GoVersion, "goVersion should be present")
	assert.NotEmpty(t, info.Platform, "platform should be present")
}

func TestVersionCommand_WithFormatTable(t *testing.T) {
	// Save and restore output mode
	originalMode := ui.GetOutputMode()
	defer ui.SetOutputMode(originalMode)

	// Set table mode
	ui.SetOutputMode(ui.TableMode)

	// Create a buffer to capture output
	buf := new(bytes.Buffer)
	versionCmd.SetOut(buf)

	// Execute command's RunE function directly
	err := versionCmd.RunE(versionCmd, []string{})
	assert.NoError(t, err, "command should execute without error")

	// Verify output contains table-formatted fields
	output := buf.String()
	assert.NotEmpty(t, output, "output should not be empty")
	assert.Contains(t, output, "Version:", "output should contain Version field")
	assert.Contains(t, output, "Commit:", "output should contain Commit field")
	assert.Contains(t, output, "Date:", "output should contain Date field")
	assert.Contains(t, output, "Go:", "output should contain Go field")
	assert.Contains(t, output, "Platform:", "output should contain Platform field")
}
