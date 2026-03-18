package ui

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFormatVersion_TableMode(t *testing.T) {
	tests := []struct {
		name string
		info VersionInfo
	}{
		{
			name: "complete version info",
			info: VersionInfo{
				Version:   "v1.0.0",
				Commit:    "abc123",
				Date:      "2024-01-01",
				GoVersion: "go1.23.0",
				Platform:  "linux/amd64",
			},
		},
		{
			name: "minimal version info",
			info: VersionInfo{
				Version:   "v0.1.0",
				Commit:    "def456",
				Date:      "2024-02-15",
				GoVersion: "go1.23.1",
				Platform:  "darwin/arm64",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prevMode := GetOutputMode()
			t.Cleanup(func() { SetOutputMode(prevMode) })

			SetOutputMode(TableMode)

			result, err := FormatVersion(tt.info)
			require.NoError(t, err)

			assert.Contains(t, result, tt.info.Version)
			assert.Contains(t, result, tt.info.Commit)
			assert.Contains(t, result, tt.info.Date)
			assert.Contains(t, result, tt.info.GoVersion)
			assert.Contains(t, result, tt.info.Platform)
			assert.Contains(t, result, "Version:")
			assert.Contains(t, result, "Commit:")
			assert.Contains(t, result, "Date:")
			assert.Contains(t, result, "Go:")
			assert.Contains(t, result, "Platform:")
		})
	}
}

func TestFormatVersion_JSONMode(t *testing.T) {
	tests := []struct {
		name string
		info VersionInfo
	}{
		{
			name: "complete version info",
			info: VersionInfo{
				Version:   "v1.0.0",
				Commit:    "abc123",
				Date:      "2024-01-01",
				GoVersion: "go1.23.0",
				Platform:  "linux/amd64",
			},
		},
		{
			name: "minimal version info",
			info: VersionInfo{
				Version:   "v0.1.0",
				Commit:    "def456",
				Date:      "2024-02-15",
				GoVersion: "go1.23.1",
				Platform:  "darwin/arm64",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prevMode := GetOutputMode()
			t.Cleanup(func() { SetOutputMode(prevMode) })

			SetOutputMode(JSONMode)

			result, err := FormatVersion(tt.info)
			require.NoError(t, err)

			// Verify it's valid JSON
			var parsed VersionInfo
			err = json.Unmarshal([]byte(result), &parsed)
			require.NoError(t, err, "output should be valid JSON")

			// Verify all fields match
			assert.Equal(t, tt.info.Version, parsed.Version)
			assert.Equal(t, tt.info.Commit, parsed.Commit)
			assert.Equal(t, tt.info.Date, parsed.Date)
			assert.Equal(t, tt.info.GoVersion, parsed.GoVersion)
			assert.Equal(t, tt.info.Platform, parsed.Platform)
		})
	}
}

func TestIsInteractive(t *testing.T) {
	// Call IsInteractive and verify it returns a boolean value without panicking
	result := IsInteractive()

	// In a test environment, stdout is typically not a TTY, so we expect false.
	// However, the exact value depends on how tests are run (e.g., with `go test`
	// vs in an actual terminal), so we document the expected behavior rather than
	// asserting a specific value.
	//
	// Expected: false when running under `go test` (stdout is a pipe, not a TTY)
	// Expected: true if running interactively in a terminal
	assert.IsType(t, false, result, "IsInteractive should return a boolean")

	// In the context of automated test runs (CI, `make test`), stdout is not a TTY
	assert.False(t, result, "IsInteractive should return false in test environment where stdout is not a TTY")
}

func TestSetOutputMode(t *testing.T) {
	tests := []struct {
		name string
		mode OutputMode
	}{
		{
			name: "set to TableMode",
			mode: TableMode,
		},
		{
			name: "set to JSONMode",
			mode: JSONMode,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prevMode := GetOutputMode()
			t.Cleanup(func() { SetOutputMode(prevMode) })

			SetOutputMode(tt.mode)
			assert.Equal(t, tt.mode, GetOutputMode())
		})
	}
}

func TestFormatVersion_JSONMarshalError(t *testing.T) {
	// This test verifies that FormatVersion properly propagates JSON marshaling errors.
	// NOTE: This is a defensive test. The current VersionInfo type uses only simple
	// string fields which cannot fail JSON marshaling. This test documents the error
	// handling path exists for future extensibility (e.g., if complex types are added
	// or custom MarshalJSON implementations that could fail).
	//
	// Since we cannot easily trigger json.MarshalIndent to fail with the current
	// VersionInfo structure, this test verifies the error path exists by checking
	// that FormatVersion returns the expected signature. If marshaling did fail,
	// the error would be properly propagated.

	SetOutputMode(JSONMode)
	info := VersionInfo{
		Version:   "v1.0.0",
		Commit:    "abc123",
		Date:      "2024-01-01",
		GoVersion: "go1.23.0",
		Platform:  "linux/amd64",
	}

	// Call FormatVersion and verify it returns both string and error
	result, err := FormatVersion(info)

	// With current implementation, this should not error
	require.NoError(t, err, "marshaling simple strings should not fail")
	assert.NotEmpty(t, result, "result should not be empty")

	// Verify the result is valid JSON
	var parsed VersionInfo
	err = json.Unmarshal([]byte(result), &parsed)
	require.NoError(t, err, "output should be valid JSON")
}

func TestSetOutputMode_ConcurrentAccess(t *testing.T) {
	// This test verifies that SetOutputMode and GetOutputMode can be called
	// concurrently without causing data races. This test should pass after
	// Task B3 adds mutex protection to the currentMode variable.

	const goroutines = 100
	const iterations = 100

	// Save and restore output mode
	prevMode := GetOutputMode()
	t.Cleanup(func() { SetOutputMode(prevMode) })

	// Create a channel to synchronize goroutine startup
	start := make(chan struct{})
	done := make(chan struct{}, goroutines)

	// Launch goroutines that concurrently set and get output mode
	for i := 0; i < goroutines; i++ {
		go func(id int) {
			// Wait for all goroutines to be ready
			<-start

			for j := 0; j < iterations; j++ {
				// Alternate between modes based on goroutine id and iteration
				if (id+j)%2 == 0 {
					SetOutputMode(TableMode)
				} else {
					SetOutputMode(JSONMode)
				}

				// Read the mode (may not match what we just set due to other goroutines)
				mode := GetOutputMode()

				// Verify we get a valid mode
				if mode != TableMode && mode != JSONMode {
					t.Errorf("GetOutputMode returned invalid mode: %v", mode)
				}
			}

			// Signal completion
			done <- struct{}{}
		}(i)
	}

	// Signal all goroutines to start
	close(start)

	// Wait for all goroutines to complete
	for i := 0; i < goroutines; i++ {
		<-done
	}

	// Note: The test framework will detect data races if they exist
	// We don't need to explicitly check for race conditions
}

func TestFormatVersion_EdgeCases(t *testing.T) {
	tests := []struct {
		name string
		info VersionInfo
		mode OutputMode
	}{
		{
			name: "all empty strings in table mode",
			info: VersionInfo{
				Version:   "",
				Commit:    "",
				Date:      "",
				GoVersion: "",
				Platform:  "",
			},
			mode: TableMode,
		},
		{
			name: "all empty strings in JSON mode",
			info: VersionInfo{
				Version:   "",
				Commit:    "",
				Date:      "",
				GoVersion: "",
				Platform:  "",
			},
			mode: JSONMode,
		},
		{
			name: "special characters in table mode",
			info: VersionInfo{
				Version:   "v1.0.0-beta+build.123",
				Commit:    "abc123!@#$%",
				Date:      "2024-01-01T12:34:56Z",
				GoVersion: "go1.23.0 (special: αβγ)",
				Platform:  "linux/amd64 [custom]",
			},
			mode: TableMode,
		},
		{
			name: "special characters in JSON mode",
			info: VersionInfo{
				Version:   "v1.0.0-beta+build.123",
				Commit:    "abc123!@#$%",
				Date:      "2024-01-01T12:34:56Z",
				GoVersion: "go1.23.0 (special: αβγ)",
				Platform:  "linux/amd64 [custom]",
			},
			mode: JSONMode,
		},
		{
			name: "unicode and newlines in table mode",
			info: VersionInfo{
				Version:   "v1.0.0 🚀",
				Commit:    "abc\n123",
				Date:      "2024-01-01\ttab",
				GoVersion: "go1.23 \"quoted\"",
				Platform:  "linux/amd64 'single'",
			},
			mode: TableMode,
		},
		{
			name: "unicode and newlines in JSON mode",
			info: VersionInfo{
				Version:   "v1.0.0 🚀",
				Commit:    "abc\n123",
				Date:      "2024-01-01\ttab",
				GoVersion: "go1.23 \"quoted\"",
				Platform:  "linux/amd64 'single'",
			},
			mode: JSONMode,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save and restore output mode
			prevMode := GetOutputMode()
			t.Cleanup(func() { SetOutputMode(prevMode) })

			SetOutputMode(tt.mode)
			result, err := FormatVersion(tt.info)
			require.NoError(t, err, "FormatVersion should not error on valid inputs")
			assert.NotEmpty(t, result, "result should not be empty")

			switch tt.mode {
			case JSONMode:
				// Verify output is valid JSON and can be unmarshaled
				var parsed VersionInfo
				err := json.Unmarshal([]byte(result), &parsed)
				require.NoError(t, err, "output should be valid JSON")

				// Verify all fields are preserved correctly
				assert.Equal(t, tt.info.Version, parsed.Version, "Version should match")
				assert.Equal(t, tt.info.Commit, parsed.Commit, "Commit should match")
				assert.Equal(t, tt.info.Date, parsed.Date, "Date should match")
				assert.Equal(t, tt.info.GoVersion, parsed.GoVersion, "GoVersion should match")
				assert.Equal(t, tt.info.Platform, parsed.Platform, "Platform should match")

			case TableMode:
				// Verify output contains expected labels
				assert.Contains(t, result, "Version:", "table should contain Version label")
				assert.Contains(t, result, "Commit:", "table should contain Commit label")
				assert.Contains(t, result, "Date:", "table should contain Date label")
				assert.Contains(t, result, "Go:", "table should contain Go label")
				assert.Contains(t, result, "Platform:", "table should contain Platform label")

				// Verify field values appear in output (even if empty)
				// Note: We don't check exact formatting, just that values are present
				if tt.info.Version != "" {
					assert.Contains(t, result, tt.info.Version, "table should contain Version value")
				}
				if tt.info.Commit != "" {
					assert.Contains(t, result, tt.info.Commit, "table should contain Commit value")
				}
			}
		})
	}
}
