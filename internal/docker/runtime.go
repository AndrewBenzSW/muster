package docker

import "context"

// ContainerRuntime is the interface for Docker container operations.
// It abstracts Docker SDK calls and docker compose CLI invocations,
// enabling test mocks and alternative implementations.
//
// Default timeouts per method:
//   - ComposeUp: 5 minutes (if context has no deadline)
//   - ComposeDown: 5 minutes (if context has no deadline)
//   - ComposeExec: no timeout (supports interactive sessions)
//   - ListContainers: 30 seconds (if context has no deadline)
//   - Ping: 10 seconds (if context has no deadline)
//
// All methods respect context deadlines when provided.
type ContainerRuntime interface {
	// ComposeUp starts services from the compose file in detached mode.
	// Default timeout: 5 minutes (if context has no deadline).
	ComposeUp(ctx context.Context, composeFile, projectName string) error

	// ComposeDown stops and removes all services.
	// Default timeout: 5 minutes (if context has no deadline).
	ComposeDown(ctx context.Context, composeFile, projectName string) error

	// ComposeExec runs a command inside a service container.
	// No default timeout (supports interactive sessions).
	ComposeExec(ctx context.Context, composeFile, projectName, service string, cmd []string) error

	// ListContainers finds muster-managed containers, optionally filtered by project and slug.
	// Default timeout: 30 seconds (if context has no deadline).
	ListContainers(ctx context.Context, project, slug string) ([]ContainerInfo, error)

	// Ping checks if the Docker daemon is reachable and Docker Compose v2+ is installed.
	// Default timeout: 10 seconds (if context has no deadline).
	Ping(ctx context.Context) error

	// Close closes the Docker client connection.
	Close() error
}

// Compile-time assertion that *Client satisfies ContainerRuntime.
var _ ContainerRuntime = (*Client)(nil)

// NewContainerRuntime creates a new ContainerRuntime using the Docker SDK client.
// This is the production constructor that delegates to NewClient().
func NewContainerRuntime() (ContainerRuntime, error) {
	return NewClient()
}
