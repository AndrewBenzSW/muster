package docker

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	dockerclient "github.com/docker/docker/client"
)

// Client wraps Docker SDK for queries and docker compose CLI for orchestration.
type Client struct {
	sdk *dockerclient.Client
}

// ContainerInfo is a simplified view of a Docker container for muster's needs.
type ContainerInfo struct {
	ID      string
	Name    string
	Status  string
	Project string
	Slug    string
	Labels  map[string]string
}

// NewClient creates a Docker client with API version negotiation.
// Verifies Docker Compose v2+ is installed per review S10.
func NewClient() (*Client, error) {
	// First verify Docker Compose v2+ is available
	if err := verifyComposeVersion(); err != nil {
		return nil, err
	}

	// Initialize Docker SDK client
	sdk, err := dockerclient.NewClientWithOpts(
		dockerclient.FromEnv,
		dockerclient.WithAPIVersionNegotiation(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create Docker client: %w", err)
	}

	return &Client{sdk: sdk}, nil
}

// Close closes the Docker client connection.
func (c *Client) Close() error {
	if c.sdk != nil {
		return c.sdk.Close()
	}
	return nil
}

// verifyComposeVersion checks that Docker Compose v2+ is installed.
// Per review S10: check for v2, detect v1, provide actionable error messages.
func verifyComposeVersion() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Try "docker compose version" (v2)
	cmd := exec.CommandContext(ctx, "docker", "compose", "version")
	output, err := cmd.CombinedOutput()

	if err != nil {
		// Check if "docker-compose" (v1) exists
		cmdV1 := exec.CommandContext(ctx, "docker-compose", "--version")
		outputV1, errV1 := cmdV1.CombinedOutput()
		if errV1 == nil {
			// docker-compose exists, so v1 is installed but v2 is not
			//nolint:staticcheck // ST1005: Docker is a proper noun
			return fmt.Errorf("Docker Compose v1 detected but v2+ required; upgrade at https://docs.docker.com/compose/install/\nOutput: %s", string(outputV1))
		}

		// Neither v2 nor v1 found
		//nolint:staticcheck // ST1005: Docker is a proper noun
		return fmt.Errorf("Docker Compose v2+ required; install from https://docs.docker.com/compose/install/\nError: %w", err)
	}

	// Parse version to ensure it's v2+
	version := string(output)
	if !isComposeV2OrHigher(version) {
		//nolint:staticcheck // ST1005: Docker is a proper noun
		return fmt.Errorf("Docker Compose v2+ required, but found: %s\nUpgrade at https://docs.docker.com/compose/install/", strings.TrimSpace(version))
	}

	return nil
}

// isComposeV2OrHigher checks if the version string indicates v2 or higher.
func isComposeV2OrHigher(versionOutput string) bool {
	// Look for version number in format "v2.x.x" or "version v2.x.x" or "Docker Compose version v2.x.x"
	re := regexp.MustCompile(`v?(\d+)\.(\d+)\.(\d+)`)
	matches := re.FindStringSubmatch(versionOutput)
	if len(matches) < 2 {
		return false
	}

	major, err := strconv.Atoi(matches[1])
	if err != nil {
		return false
	}

	return major >= 2
}

// ComposeUp starts services from the compose file.
// Per review S6: uses context.WithTimeout (default 5 minutes).
func (c *Client) ComposeUp(ctx context.Context, composeFile, projectName string) error {
	// Default 5 minute timeout if no deadline set
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 5*time.Minute)
		defer cancel()
	}

	return c.runCompose(ctx, composeFile, projectName, "up", "-d")
}

// ComposeDown stops and removes all services.
// Per review S6: uses context.WithTimeout (default 5 minutes).
func (c *Client) ComposeDown(ctx context.Context, composeFile, projectName string) error {
	// Default 5 minute timeout if no deadline set
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 5*time.Minute)
		defer cancel()
	}

	return c.runCompose(ctx, composeFile, projectName, "down")
}

// ComposeExec runs a command inside a service container.
// Per review S6: no timeout for exec (interactive sessions).
func (c *Client) ComposeExec(ctx context.Context, composeFile, projectName, service string, cmd []string) error {
	args := append([]string{"exec", service}, cmd...)
	return c.runCompose(ctx, composeFile, projectName, args...)
}

// runCompose builds and runs a docker compose command with stdout/stderr/stdin passthrough.
func (c *Client) runCompose(ctx context.Context, composeFile, projectName string, args ...string) error {
	cmdArgs := []string{"compose", "-f", composeFile, "-p", projectName}
	cmdArgs = append(cmdArgs, args...)

	//nolint:gosec // G204: composeFile and args are from internal code, not user input
	command := exec.CommandContext(ctx, "docker", cmdArgs...)
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	command.Stdin = os.Stdin

	if err := command.Run(); err != nil {
		return fmt.Errorf("docker compose command failed: %w", err)
	}

	return nil
}

// ListContainers finds muster-managed containers, optionally filtered by project and slug.
// Per review S6: wraps SDK calls with timeout context.
func (c *Client) ListContainers(ctx context.Context, project, slug string) ([]ContainerInfo, error) {
	// Default 30 second timeout if no deadline set
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
	}

	f := filters.NewArgs()
	f.Add("label", LabelManaged+"=true")
	if project != "" {
		f.Add("label", fmt.Sprintf("%s=%s", LabelProject, project))
	}
	if slug != "" {
		f.Add("label", fmt.Sprintf("%s=%s", LabelSlug, slug))
	}

	containers, err := c.sdk.ContainerList(ctx, container.ListOptions{
		Filters: f,
		All:     true,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list containers: %w", err)
	}

	var result []ContainerInfo
	for _, ctr := range containers {
		name := ""
		if len(ctr.Names) > 0 {
			name = strings.TrimPrefix(ctr.Names[0], "/")
		}

		id := ctr.ID
		if len(id) > 12 {
			id = id[:12]
		}

		result = append(result, ContainerInfo{
			ID:      id,
			Name:    name,
			Status:  ctr.Status,
			Project: ctr.Labels[LabelProject],
			Slug:    ctr.Labels[LabelSlug],
			Labels:  ctr.Labels,
		})
	}

	return result, nil
}

// Ping checks if the Docker daemon is reachable and Docker Compose v2+ is installed.
// Per review S10: single call that checks both daemon and compose version.
func (c *Client) Ping(ctx context.Context) error {
	// Default 10 second timeout if no deadline set
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
	}

	// Check daemon reachability
	_, err := c.sdk.Ping(ctx)
	if err != nil {
		//nolint:staticcheck // ST1005: Docker is a proper noun
		return fmt.Errorf("Docker daemon is not running; start Docker Desktop or run 'sudo systemctl start docker': %w", err)
	}

	// Verify compose version
	if err := verifyComposeVersion(); err != nil {
		return err
	}

	return nil
}

// PingDaemon checks only if the Docker daemon is reachable (without compose version check).
func (c *Client) PingDaemon(ctx context.Context) error {
	// Default 10 second timeout if no deadline set
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
	}

	_, err := c.sdk.Ping(ctx)
	if err != nil {
		//nolint:staticcheck // ST1005: Docker is a proper noun
		return fmt.Errorf("Docker daemon is not running; start Docker Desktop or run 'sudo systemctl start docker': %w", err)
	}

	return nil
}

// ServerVersion returns the Docker daemon version information.
func (c *Client) ServerVersion(ctx context.Context) (types.Version, error) {
	// Default 10 second timeout if no deadline set
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
	}

	return c.sdk.ServerVersion(ctx)
}
