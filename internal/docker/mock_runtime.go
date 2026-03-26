package docker

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
)

// MockContainerRuntime is a file-based mock implementation of ContainerRuntime.
// Each method call writes JSON to a numbered file in callsDir and optionally
// reads a response from responsesDir. Call counters are maintained per method.
//
// This mock is not safe for concurrent use. Each test must create its own mock
// instance with separate t.TempDir() directories. Do not share mock instances
// across t.Parallel() subtests.
//
// Does not validate input parameters - tests should verify correct config by
// inspecting call files.
type MockContainerRuntime struct {
	callsDir     string
	responsesDir string

	// Per-method call counters (atomic for thread safety)
	composeUpCount   atomic.Int32
	composeDownCount atomic.Int32
	composeExecCount atomic.Int32
	listCount        atomic.Int32
	pingCount        atomic.Int32
}

// NewMockContainerRuntime creates a new MockContainerRuntime that writes call
// records to callsDir and reads optional responses from responsesDir.
func NewMockContainerRuntime(callsDir, responsesDir string) *MockContainerRuntime {
	return &MockContainerRuntime{
		callsDir:     callsDir,
		responsesDir: responsesDir,
	}
}

// ComposeUp records the call and returns nil or error from response file.
func (m *MockContainerRuntime) ComposeUp(ctx context.Context, composeFile, projectName string) error {
	count := m.composeUpCount.Add(1)

	call := map[string]interface{}{
		"method":      "ComposeUp",
		"composeFile": composeFile,
		"projectName": projectName,
	}

	if err := m.writeCall("compose-up", int(count), call); err != nil {
		return &MockDockerInfraError{Op: "ComposeUp", Err: err}
	}

	return m.readResponse("compose-up", int(count))
}

// ComposeDown records the call and returns nil or error from response file.
func (m *MockContainerRuntime) ComposeDown(ctx context.Context, composeFile, projectName string) error {
	count := m.composeDownCount.Add(1)

	call := map[string]interface{}{
		"method":      "ComposeDown",
		"composeFile": composeFile,
		"projectName": projectName,
	}

	if err := m.writeCall("compose-down", int(count), call); err != nil {
		return &MockDockerInfraError{Op: "ComposeDown", Err: err}
	}

	return m.readResponse("compose-down", int(count))
}

// ComposeExec records the call and returns nil or error from response file.
func (m *MockContainerRuntime) ComposeExec(ctx context.Context, composeFile, projectName, service string, cmd []string) error {
	count := m.composeExecCount.Add(1)

	call := map[string]interface{}{
		"method":      "ComposeExec",
		"composeFile": composeFile,
		"projectName": projectName,
		"service":     service,
		"cmd":         cmd,
	}

	if err := m.writeCall("compose-exec", int(count), call); err != nil {
		return &MockDockerInfraError{Op: "ComposeExec", Err: err}
	}

	return m.readResponse("compose-exec", int(count))
}

// ListContainers records the call and returns containers from response file or error.
func (m *MockContainerRuntime) ListContainers(ctx context.Context, project, slug string) ([]ContainerInfo, error) {
	count := m.listCount.Add(1)

	call := map[string]interface{}{
		"method":  "ListContainers",
		"project": project,
		"slug":    slug,
	}

	if err := m.writeCall("list-containers", int(count), call); err != nil {
		return nil, &MockDockerInfraError{Op: "ListContainers", Err: err}
	}

	// Read response file for containers
	responsePath := filepath.Join(m.responsesDir, fmt.Sprintf("%03d-list-containers.json", count))
	//nolint:gosec // G304: responsePath is constructed from test fixture directory, not user input
	data, err := os.ReadFile(responsePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, &MockDockerInfraError{Op: "ListContainers", Err: fmt.Errorf("missing response file for ListContainers call #%d (expected %s)", count, responsePath)}
		}
		return nil, &MockDockerInfraError{Op: "ListContainers", Err: err}
	}

	var response struct {
		Containers []ContainerInfo `json:"containers"`
		Error      string          `json:"error"`
	}

	if err := json.Unmarshal(data, &response); err != nil {
		return nil, &MockDockerInfraError{Op: "ListContainers", Err: fmt.Errorf("invalid response JSON: %w", err)}
	}

	if response.Error != "" {
		return nil, &SimulatedDockerError{Message: response.Error}
	}

	return response.Containers, nil
}

// Ping records the call and returns nil or error from response file.
func (m *MockContainerRuntime) Ping(ctx context.Context) error {
	count := m.pingCount.Add(1)

	call := map[string]interface{}{
		"method": "Ping",
	}

	if err := m.writeCall("ping", int(count), call); err != nil {
		return &MockDockerInfraError{Op: "Ping", Err: err}
	}

	return m.readResponse("ping", int(count))
}

// Close is a no-op that returns nil.
func (m *MockContainerRuntime) Close() error {
	return nil
}

// writeCall writes the call data as JSON to the calls directory.
func (m *MockContainerRuntime) writeCall(methodName string, count int, call map[string]interface{}) error {
	data, err := json.MarshalIndent(call, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal call: %w", err)
	}

	filename := fmt.Sprintf("%03d-%s.json", count, methodName)
	path := filepath.Join(m.callsDir, filename)

	//nolint:gosec // G306: Test file permissions are acceptable
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write call file: %w", err)
	}

	return nil
}

// readResponse reads an optional response file and returns error if specified.
func (m *MockContainerRuntime) readResponse(methodName string, count int) error {
	filename := fmt.Sprintf("%03d-%s.json", count, methodName)
	path := filepath.Join(m.responsesDir, filename)

	//nolint:gosec // G304: path is constructed from test fixture directory, not user input
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// No response file means success (nil error)
			return nil
		}
		return &MockDockerInfraError{Op: methodName, Err: err}
	}

	var response struct {
		Error string `json:"error"`
	}

	if err := json.Unmarshal(data, &response); err != nil {
		return &MockDockerInfraError{Op: methodName, Err: fmt.Errorf("invalid response JSON: %w", err)}
	}

	if response.Error != "" {
		return &SimulatedDockerError{Message: response.Error}
	}

	return nil
}

// MockDockerInfraError represents an infrastructure error in the mock (e.g., file I/O).
// This is distinct from simulated Docker errors controlled by response fixtures.
type MockDockerInfraError struct {
	Op  string // The operation that failed (e.g., "ComposeUp", "ListContainers")
	Err error  // The underlying error
}

func (e *MockDockerInfraError) Error() string {
	return fmt.Sprintf("mock infrastructure error in %s: %v", e.Op, e.Err)
}

func (e *MockDockerInfraError) Unwrap() error {
	return e.Err
}

// SimulatedDockerError represents a Docker error controlled by test fixtures.
type SimulatedDockerError struct {
	Message string
}

func (e *SimulatedDockerError) Error() string {
	return e.Message
}
