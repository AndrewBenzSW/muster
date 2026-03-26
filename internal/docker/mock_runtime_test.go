package docker

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMockContainerRuntime_Counters(t *testing.T) {
	callsDir := t.TempDir()
	responsesDir := t.TempDir()

	mock := NewMockContainerRuntime(callsDir, responsesDir)
	ctx := context.Background()

	// Call ComposeUp twice
	err := mock.ComposeUp(ctx, "compose.yml", "test-project")
	require.NoError(t, err)

	err = mock.ComposeUp(ctx, "compose2.yml", "test-project-2")
	require.NoError(t, err)

	// Call ListContainers once (will fail due to missing response, but should still write call file)
	_, _ = mock.ListContainers(ctx, "test-project", "test-slug")

	// Call ComposeDown once
	err = mock.ComposeDown(ctx, "compose.yml", "test-project")
	require.NoError(t, err)

	// Verify call files exist with correct names
	files, err := os.ReadDir(callsDir)
	require.NoError(t, err)
	require.Len(t, files, 4, "expected 4 call files")

	// Verify filenames match expected pattern
	expectedFiles := []string{
		"001-compose-up.json",
		"002-compose-up.json",
		"001-list-containers.json",
		"001-compose-down.json",
	}

	actualFiles := make([]string, len(files))
	for i, f := range files {
		actualFiles[i] = f.Name()
	}

	assert.ElementsMatch(t, expectedFiles, actualFiles)

	// Verify per-method independence: ListContainers was called once, so it should be 001-list-containers.json
	// NOT 003-list-containers.json (which would happen if counters were shared)
	listContainersPath := filepath.Join(callsDir, "001-list-containers.json")
	assert.FileExists(t, listContainersPath, "ListContainers should use its own counter starting at 001")

	// Verify the second ComposeUp call is 002, not 003
	composeUp2Path := filepath.Join(callsDir, "002-compose-up.json")
	assert.FileExists(t, composeUp2Path, "Second ComposeUp should be 002, not affected by other method calls")

	// Verify counter values by checking internal state (via file count per method)
	composeUpCount := 0
	listContainersCount := 0
	composeDownCount := 0
	for _, f := range files {
		switch {
		case f.Name() == "001-compose-up.json" || f.Name() == "002-compose-up.json":
			composeUpCount++
		case f.Name() == "001-list-containers.json":
			listContainersCount++
		case f.Name() == "001-compose-down.json":
			composeDownCount++
		}
	}

	// This validates per-method independence: each method has its own counter
	assert.Equal(t, 2, composeUpCount, "ComposeUp should have been called twice (001, 002)")
	assert.Equal(t, 1, listContainersCount, "ListContainers should have been called once (001)")
	assert.Equal(t, 1, composeDownCount, "ComposeDown should have been called once (001)")
}

func TestMockContainerRuntime_ListContainersResponse(t *testing.T) {
	callsDir := t.TempDir()
	responsesDir := t.TempDir()

	mock := NewMockContainerRuntime(callsDir, responsesDir)
	ctx := context.Background()

	// Create response fixture
	response := struct {
		Containers []ContainerInfo `json:"containers"`
		Error      string          `json:"error"`
	}{
		Containers: []ContainerInfo{
			{
				ID:      "abc123",
				Name:    "test-container",
				Status:  "running",
				Project: "test-project",
				Slug:    "test-slug",
				Labels: map[string]string{
					"muster.managed": "true",
					"muster.project": "test-project",
					"muster.slug":    "test-slug",
				},
			},
		},
		Error: "",
	}

	data, err := json.MarshalIndent(response, "", "  ")
	require.NoError(t, err)

	responseFile := filepath.Join(responsesDir, "001-list-containers.json")
	//nolint:gosec // G306: Test file permissions are acceptable
	err = os.WriteFile(responseFile, data, 0644)
	require.NoError(t, err)

	// Call ListContainers
	containers, err := mock.ListContainers(ctx, "test-project", "test-slug")
	require.NoError(t, err)
	require.Len(t, containers, 1)

	assert.Equal(t, "abc123", containers[0].ID)
	assert.Equal(t, "test-container", containers[0].Name)
	assert.Equal(t, "running", containers[0].Status)
	assert.Equal(t, "test-project", containers[0].Project)
	assert.Equal(t, "test-slug", containers[0].Slug)
}

func TestMockContainerRuntime_Close(t *testing.T) {
	callsDir := t.TempDir()
	responsesDir := t.TempDir()

	mock := NewMockContainerRuntime(callsDir, responsesDir)

	// Close should return nil (no-op)
	err := mock.Close()
	assert.NoError(t, err)
}

func TestMockContainerRuntime_MissingResponse(t *testing.T) {
	callsDir := t.TempDir()
	responsesDir := t.TempDir()

	mock := NewMockContainerRuntime(callsDir, responsesDir)
	ctx := context.Background()

	// Call ListContainers without a response file
	containers, err := mock.ListContainers(ctx, "test-project", "test-slug")

	// Should return MockDockerInfraError
	require.Error(t, err)
	assert.Nil(t, containers)

	var mockErr *MockDockerInfraError
	assert.ErrorAs(t, err, &mockErr)
	assert.Equal(t, "ListContainers", mockErr.Op)
}

func TestMockContainerRuntime_SimulatedDockerError(t *testing.T) {
	callsDir := t.TempDir()
	responsesDir := t.TempDir()

	mock := NewMockContainerRuntime(callsDir, responsesDir)
	ctx := context.Background()

	// Write a response file with an error field to simulate a Docker failure
	response := struct {
		Error string `json:"error"`
	}{
		Error: "simulated docker failure",
	}
	data, err := json.MarshalIndent(response, "", "  ")
	require.NoError(t, err)

	responseFile := filepath.Join(responsesDir, "001-compose-up.json")
	//nolint:gosec // G306: Test file permissions are acceptable
	require.NoError(t, os.WriteFile(responseFile, data, 0644))

	// Call ComposeUp -- should return SimulatedDockerError
	err = mock.ComposeUp(ctx, "compose.yml", "test-project")
	require.Error(t, err)

	var simErr *SimulatedDockerError
	assert.True(t, errors.As(err, &simErr), "error should be SimulatedDockerError")
	assert.Equal(t, "simulated docker failure", simErr.Message)
}

func TestMockContainerRuntime_ErrorDistinction(t *testing.T) {
	callsDir := t.TempDir()
	responsesDir := t.TempDir()

	mock := NewMockContainerRuntime(callsDir, responsesDir)
	ctx := context.Background()

	// Test 1: Missing response file returns MockDockerInfraError (not SimulatedDockerError)
	_, err := mock.ListContainers(ctx, "test-project", "test-slug")
	require.Error(t, err)

	var mockErr *MockDockerInfraError
	var simErr *SimulatedDockerError
	assert.True(t, errors.As(err, &mockErr), "missing response should return MockDockerInfraError")
	assert.False(t, errors.As(err, &simErr), "missing response should not return SimulatedDockerError")

	// Verify error message contains useful details
	assert.Contains(t, mockErr.Op, "ListContainers", "error should identify the operation")
	assert.Contains(t, mockErr.Error(), "001-list-containers.json", "error message should include expected file path")

	// Test 2: Response with error field returns SimulatedDockerError (not MockDockerInfraError)
	response := struct {
		Error string `json:"error"`
	}{
		Error: "docker daemon not running",
	}
	data, err := json.MarshalIndent(response, "", "  ")
	require.NoError(t, err)

	responseFile := filepath.Join(responsesDir, "001-compose-up.json")
	//nolint:gosec // G306: Test file permissions are acceptable
	require.NoError(t, os.WriteFile(responseFile, data, 0644))

	err = mock.ComposeUp(ctx, "compose.yml", "test-project")
	require.Error(t, err)

	assert.True(t, errors.As(err, &simErr), "simulated error should return SimulatedDockerError")
	assert.False(t, errors.As(err, &mockErr), "simulated error should not return MockDockerInfraError")

	// Verify error message contains the simulated error text
	assert.Equal(t, "docker daemon not running", simErr.Message, "error message should contain simulated docker error")
}
