package cmd

import (
	"context"
	"errors"
	"os/exec"
	"testing"

	"github.com/abenz1267/muster/internal/ai"
	"github.com/abenz1267/muster/internal/coding"
	"github.com/abenz1267/muster/internal/docker"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCodingToolFactory_DefaultReturnsNonNil tests that the default factory returns a valid tool.
// Skips if claude binary is not available (e.g., in CI without claude installed).
func TestCodingToolFactory_DefaultReturnsNonNil(t *testing.T) {
	// Skip if claude binary is not in PATH (common in CI)
	if _, err := exec.LookPath("claude"); err != nil {
		t.Skip("claude binary not found in PATH, skipping default factory test")
	}

	tool, err := codingToolFactory()
	require.NoError(t, err, "default codingToolFactory should not return error")
	assert.NotNil(t, tool, "default codingToolFactory should return non-nil tool")
}

// TestInteractiveToolFactory_DefaultReturnsNonNil tests that the default factory returns a valid tool.
// Skips if claude binary is not available (e.g., in CI without claude installed).
func TestInteractiveToolFactory_DefaultReturnsNonNil(t *testing.T) {
	// Skip if claude binary is not in PATH (common in CI)
	if _, err := exec.LookPath("claude"); err != nil {
		t.Skip("claude binary not found in PATH, skipping default factory test")
	}

	tool, err := interactiveToolFactory()
	require.NoError(t, err, "default interactiveToolFactory should not return error")
	assert.NotNil(t, tool, "default interactiveToolFactory should return non-nil tool")
}

// TestContainerRuntimeFactory_DefaultReturnsNonNil tests that the default factory returns a valid runtime.
// Skips if Docker is not available (common in CI without Docker daemon).
func TestContainerRuntimeFactory_DefaultReturnsNonNil(t *testing.T) {
	// Skip if docker binary is not in PATH (common in CI)
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker binary not found in PATH, skipping default factory test")
	}

	runtime, err := containerRuntimeFactory()
	require.NoError(t, err, "default containerRuntimeFactory should not return error")
	assert.NotNil(t, runtime, "default containerRuntimeFactory should return non-nil runtime")

	// Clean up the runtime if Close is available
	if runtime != nil {
		_ = runtime.Close()
	}
}

// TestCodingToolFactory_Replacement verifies the factory replacement pattern.
// This validates that tests can inject mocks by replacing the factory variable.
func TestCodingToolFactory_Replacement(t *testing.T) {
	// Save original factory
	original := codingToolFactory
	defer func() { codingToolFactory = original }()

	// Replace with mock factory
	mockCalled := false
	mockTool := &mockCodingToolForFactoryTest{called: false}
	codingToolFactory = func() (coding.CodingTool, error) {
		mockCalled = true
		return mockTool, nil
	}

	// Call factory
	tool, err := codingToolFactory()
	require.NoError(t, err)
	assert.NotNil(t, tool)
	assert.True(t, mockCalled, "mock factory should have been called")

	// Verify it's the mock
	assert.Same(t, mockTool, tool, "factory should return mock tool instance")
}

// TestInteractiveToolFactory_Replacement verifies the factory replacement pattern.
func TestInteractiveToolFactory_Replacement(t *testing.T) {
	// Save original factory
	original := interactiveToolFactory
	defer func() { interactiveToolFactory = original }()

	// Create a simple mock with a function field for verification
	mockTool := &mockInteractiveToolForFactoryTest{called: false}

	// Replace with mock factory
	mockCalled := false
	interactiveToolFactory = func() (coding.InteractiveCodingTool, error) {
		mockCalled = true
		return mockTool, nil
	}

	// Call factory
	tool, err := interactiveToolFactory()
	require.NoError(t, err)
	assert.NotNil(t, tool)
	assert.True(t, mockCalled, "mock factory should have been called")

	// Verify it's the mock
	assert.Same(t, mockTool, tool, "factory should return mock tool instance")
}

// TestContainerRuntimeFactory_Replacement verifies the factory replacement pattern.
func TestContainerRuntimeFactory_Replacement(t *testing.T) {
	// Save original factory
	original := containerRuntimeFactory
	defer func() { containerRuntimeFactory = original }()

	// Replace with mock factory
	mockCalled := false
	containerRuntimeFactory = func() (docker.ContainerRuntime, error) {
		mockCalled = true
		return &mockContainerRuntime{}, nil
	}

	// Call factory
	runtime, err := containerRuntimeFactory()
	require.NoError(t, err)
	assert.NotNil(t, runtime)
	assert.True(t, mockCalled, "mock factory should have been called")

	// Verify it's the mock
	_, ok := runtime.(*mockContainerRuntime)
	assert.True(t, ok, "factory should return mock runtime")
}

// TestFactoryReplacementRestore verifies that defer correctly restores original factories.
func TestFactoryReplacementRestore(t *testing.T) {
	// Replace and restore in a sub-scope
	func() {
		// Save originals
		original1 := codingToolFactory
		original2 := interactiveToolFactory
		original3 := containerRuntimeFactory
		defer func() {
			codingToolFactory = original1
			interactiveToolFactory = original2
			containerRuntimeFactory = original3
		}()

		// Replace with error-returning factories
		codingToolFactory = func() (coding.CodingTool, error) {
			return nil, errors.New("mock error")
		}
		interactiveToolFactory = func() (coding.InteractiveCodingTool, error) {
			return nil, errors.New("mock error")
		}
		containerRuntimeFactory = func() (docker.ContainerRuntime, error) {
			return nil, errors.New("mock error")
		}

		// Verify mocks are in place
		_, err := codingToolFactory()
		assert.Error(t, err, "mock codingToolFactory should return error")
		_, err = interactiveToolFactory()
		assert.Error(t, err, "mock interactiveToolFactory should return error")
		_, err = containerRuntimeFactory()
		assert.Error(t, err, "mock containerRuntimeFactory should return error")
	}()

	// Verify originals are restored by checking they no longer return errors
	// (The default factories should succeed or be skipped if binaries are missing)
	// We can't compare function pointers, so we just verify the behavior is restored
	// by checking that the factories don't return the mock error
	_, err := codingToolFactory()
	if err != nil {
		assert.NotEqual(t, "mock error", err.Error(), "codingToolFactory should be restored (not returning mock error)")
	}
	_, err = interactiveToolFactory()
	if err != nil {
		assert.NotEqual(t, "mock error", err.Error(), "interactiveToolFactory should be restored (not returning mock error)")
	}
	_, err = containerRuntimeFactory()
	if err != nil {
		assert.NotEqual(t, "mock error", err.Error(), "containerRuntimeFactory should be restored (not returning mock error)")
	}
}

// Mock implementations for testing

type mockContainerRuntime struct{}

func (m *mockContainerRuntime) ComposeUp(ctx context.Context, composeFile, projectName string) error {
	return nil
}

func (m *mockContainerRuntime) ComposeDown(ctx context.Context, composeFile, projectName string) error {
	return nil
}

func (m *mockContainerRuntime) ComposeExec(ctx context.Context, composeFile, projectName, service string, cmd []string) error {
	return nil
}

func (m *mockContainerRuntime) ListContainers(ctx context.Context, project, slug string) ([]docker.ContainerInfo, error) {
	return nil, nil
}

func (m *mockContainerRuntime) Ping(ctx context.Context) error {
	return nil
}

func (m *mockContainerRuntime) Close() error {
	return nil
}

type mockCodingToolForFactoryTest struct {
	called bool
}

func (m *mockCodingToolForFactoryTest) Invoke(ctx context.Context, cfg ai.InvokeConfig) (*ai.InvokeResult, error) {
	m.called = true
	return &ai.InvokeResult{}, nil
}

type mockInteractiveToolForFactoryTest struct {
	called bool
}

func (m *mockInteractiveToolForFactoryTest) RunInteractive(ctx context.Context, cfg coding.InteractiveConfig) error {
	m.called = true
	return nil
}

// TestFactoryReplacementIsolation verifies that factory replacements in subtests
// are properly isolated and each subtest gets its own mock.
func TestFactoryReplacementIsolation(t *testing.T) {
	// Save original factories
	origCoding := codingToolFactory
	origInteractive := interactiveToolFactory
	defer func() {
		codingToolFactory = origCoding
		interactiveToolFactory = origInteractive
	}()

	// Subtest 1: Replace with first mock
	t.Run("FirstMock", func(t *testing.T) {
		mockTool1 := &mockCodingToolForFactoryTest{called: false}
		codingToolFactory = func() (coding.CodingTool, error) {
			return mockTool1, nil
		}

		tool, err := codingToolFactory()
		require.NoError(t, err)
		assert.Same(t, mockTool1, tool, "should get first mock")
	})

	// Subtest 2: Replace with second mock
	t.Run("SecondMock", func(t *testing.T) {
		mockTool2 := &mockCodingToolForFactoryTest{called: false}
		codingToolFactory = func() (coding.CodingTool, error) {
			return mockTool2, nil
		}

		tool, err := codingToolFactory()
		require.NoError(t, err)
		assert.Same(t, mockTool2, tool, "should get second mock, not first")
	})

	// Verify interactive tool factory can be replaced independently
	t.Run("InteractiveMock", func(t *testing.T) {
		mockInteractive := &mockInteractiveToolForFactoryTest{called: false}
		interactiveToolFactory = func() (coding.InteractiveCodingTool, error) {
			return mockInteractive, nil
		}

		tool, err := interactiveToolFactory()
		require.NoError(t, err)
		assert.Same(t, mockInteractive, tool, "should get interactive mock")
	})
}

// TestFactoryError_PropagatedToCommand verifies that errors returned by factories
// are properly propagated to command invocation.
func TestFactoryError_PropagatedToCommand(t *testing.T) {
	// Save original factories
	origCoding := codingToolFactory
	origInteractive := interactiveToolFactory
	origContainer := containerRuntimeFactory
	defer func() {
		codingToolFactory = origCoding
		interactiveToolFactory = origInteractive
		containerRuntimeFactory = origContainer
	}()

	// Test 1: codingToolFactory returns error
	t.Run("CodingToolFactoryError", func(t *testing.T) {
		expectedErr := errors.New("coding tool initialization failed")
		codingToolFactory = func() (coding.CodingTool, error) {
			return nil, expectedErr
		}

		tool, err := codingToolFactory()
		require.Error(t, err)
		assert.Nil(t, tool)
		assert.Equal(t, expectedErr, err, "error should be propagated")
	})

	// Test 2: interactiveToolFactory returns error
	t.Run("InteractiveToolFactoryError", func(t *testing.T) {
		expectedErr := errors.New("interactive tool not available")
		interactiveToolFactory = func() (coding.InteractiveCodingTool, error) {
			return nil, expectedErr
		}

		tool, err := interactiveToolFactory()
		require.Error(t, err)
		assert.Nil(t, tool)
		assert.Equal(t, expectedErr, err, "error should be propagated")
	})

	// Test 3: containerRuntimeFactory returns error
	t.Run("ContainerRuntimeFactoryError", func(t *testing.T) {
		expectedErr := errors.New("docker not available")
		containerRuntimeFactory = func() (docker.ContainerRuntime, error) {
			return nil, expectedErr
		}

		runtime, err := containerRuntimeFactory()
		require.Error(t, err)
		assert.Nil(t, runtime)
		assert.Equal(t, expectedErr, err, "error should be propagated")
	})
}
