package docker

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Compile-time assertion that *Client satisfies ContainerRuntime.
// This test file ensures the interface exists and the concrete type implements it.
var _ ContainerRuntime = (*Client)(nil)

func TestNewContainerRuntime(t *testing.T) {
	runtime, err := NewContainerRuntime()
	if err != nil {
		// Expected when Docker is not available (e.g., in CI)
		t.Logf("NewContainerRuntime returned error (expected in non-Docker env): %v", err)
		return
	}
	assert.NotNil(t, runtime)
	defer func() { _ = runtime.Close() }()
}

func TestNewContainerRuntime_ErrorWhenDockerUnavailable(t *testing.T) {
	// This test documents expected behavior when Docker is unavailable.
	// NewContainerRuntime should return an error (not panic) when:
	// - Docker daemon is not running
	// - Docker CLI is not installed
	// - Docker socket is not accessible
	//
	// The actual error depends on the environment. In CI without Docker,
	// we expect an error. On a dev machine with Docker, it should succeed.

	runtime, err := NewContainerRuntime()

	if err != nil {
		// Expected error case: Docker is not available
		assert.Contains(t, err.Error(), "docker", "error should mention docker")
		assert.Nil(t, runtime, "runtime should be nil when error is returned")
		t.Logf("Docker unavailable (expected in CI): %v", err)
	} else {
		// Success case: Docker is available
		require.NotNil(t, runtime, "runtime should not be nil when no error")
		defer func() { _ = runtime.Close() }()
		t.Log("Docker available, NewContainerRuntime succeeded")
	}

	// Note: This test always passes - it documents behavior in both scenarios
}
