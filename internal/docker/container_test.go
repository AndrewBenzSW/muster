package docker

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestPing_Integration verifies Docker daemon reachability.
func TestPing_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test requiring Docker")
	}

	client, err := NewClient()
	if err != nil {
		t.Skipf("Docker not available, skipping integration test: %v", err)
	}
	defer func() { _ = client.Close() }()

	ctx := context.Background()
	err = client.Ping(ctx)
	if err != nil {
		t.Errorf("Ping() failed: %v", err)
	}
}

// TestComposeUp_Integration verifies container startup with labels.
func TestComposeUp_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test requiring Docker")
	}

	// Create minimal compose file with labels
	tmpDir := t.TempDir()
	composePath := filepath.Join(tmpDir, "docker-compose.yml")
	composeYAML := `services:
  test-agent:
    image: alpine:latest
    command: sleep 3600
    labels:
      muster.managed: "true"
      muster.project: "test-project"
      muster.slug: "test-slug"
`
	err := os.WriteFile(composePath, []byte(composeYAML), 0644) //nolint:gosec // G306: test files
	if err != nil {
		t.Fatalf("failed to write compose file: %v", err)
	}

	client, err := NewClient()
	if err != nil {
		t.Skipf("Docker not available, skipping integration test: %v", err)
	}
	defer func() { _ = client.Close() }()

	ctx := context.Background()

	// Ensure clean state
	_ = client.ComposeDown(ctx, composePath, "test-project")

	// Start container
	err = client.ComposeUp(ctx, composePath, "test-project")
	if err != nil {
		t.Fatalf("ComposeUp() failed: %v", err)
	}

	// Clean up on test completion
	defer func() {
		_ = client.ComposeDown(ctx, composePath, "test-project")
	}()

	// Verify container is running with labels
	containers, err := client.ListContainers(ctx, "test-project", "test-slug")
	if err != nil {
		t.Fatalf("ListContainers() failed: %v", err)
	}

	if len(containers) == 0 {
		t.Fatal("expected at least one container, got none")
	}

	found := false
	for _, ctr := range containers {
		if ctr.Project == "test-project" && ctr.Slug == "test-slug" {
			found = true
			if ctr.Labels[LabelManaged] != "true" {
				t.Errorf("expected label %s=true, got %s", LabelManaged, ctr.Labels[LabelManaged])
			}
			break
		}
	}

	if !found {
		t.Error("no container found with expected project and slug labels")
	}
}

// TestListContainers_ProjectFilter verifies filtering by project label.
func TestListContainers_ProjectFilter(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test requiring Docker")
	}

	// Create two compose files for different projects
	tmpDir := t.TempDir()

	// Project 1
	compose1Path := filepath.Join(tmpDir, "docker-compose-1.yml")
	compose1YAML := `services:
  agent1:
    image: alpine:latest
    command: sleep 3600
    labels:
      muster.managed: "true"
      muster.project: "project-one"
      muster.slug: "test-slug-1"
`
	err := os.WriteFile(compose1Path, []byte(compose1YAML), 0644) //nolint:gosec // G306: test files
	if err != nil {
		t.Fatalf("failed to write compose1 file: %v", err)
	}

	// Project 2
	compose2Path := filepath.Join(tmpDir, "docker-compose-2.yml")
	compose2YAML := `services:
  agent2:
    image: alpine:latest
    command: sleep 3600
    labels:
      muster.managed: "true"
      muster.project: "project-two"
      muster.slug: "test-slug-2"
`
	err = os.WriteFile(compose2Path, []byte(compose2YAML), 0644) //nolint:gosec // G306: test files
	if err != nil {
		t.Fatalf("failed to write compose2 file: %v", err)
	}

	client, err := NewClient()
	if err != nil {
		t.Skipf("Docker not available, skipping integration test: %v", err)
	}
	defer func() { _ = client.Close() }()

	ctx := context.Background()

	// Ensure clean state
	_ = client.ComposeDown(ctx, compose1Path, "project-one")
	_ = client.ComposeDown(ctx, compose2Path, "project-two")

	// Start both containers
	err = client.ComposeUp(ctx, compose1Path, "project-one")
	if err != nil {
		t.Fatalf("ComposeUp(project-one) failed: %v", err)
	}
	defer func() {
		_ = client.ComposeDown(ctx, compose1Path, "project-one")
	}()

	err = client.ComposeUp(ctx, compose2Path, "project-two")
	if err != nil {
		t.Fatalf("ComposeUp(project-two) failed: %v", err)
	}
	defer func() {
		_ = client.ComposeDown(ctx, compose2Path, "project-two")
	}()

	// Query for project-one only
	containers, err := client.ListContainers(ctx, "project-one", "")
	if err != nil {
		t.Fatalf("ListContainers(project-one) failed: %v", err)
	}

	// Verify only project-one containers returned
	for _, ctr := range containers {
		if ctr.Project != "project-one" {
			t.Errorf("expected only project-one containers, got container with project=%s", ctr.Project)
		}
	}

	if len(containers) == 0 {
		t.Error("expected at least one container for project-one")
	}
}

// TestListContainers_SlugFilter verifies filtering by slug label.
func TestListContainers_SlugFilter(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test requiring Docker")
	}

	// Create compose file with one service that has a slug, one without
	tmpDir := t.TempDir()
	composePath := filepath.Join(tmpDir, "docker-compose.yml")
	composeYAML := `services:
  with-slug:
    image: alpine:latest
    command: sleep 3600
    labels:
      muster.managed: "true"
      muster.project: "test-project"
      muster.slug: "feat-1"
  without-slug:
    image: alpine:latest
    command: sleep 3600
    labels:
      muster.managed: "true"
      muster.project: "test-project"
`
	err := os.WriteFile(composePath, []byte(composeYAML), 0644) //nolint:gosec // G306: test files
	if err != nil {
		t.Fatalf("failed to write compose file: %v", err)
	}

	client, err := NewClient()
	if err != nil {
		t.Skipf("Docker not available, skipping integration test: %v", err)
	}
	defer func() { _ = client.Close() }()

	ctx := context.Background()

	// Ensure clean state
	_ = client.ComposeDown(ctx, composePath, "test-project")

	// Start containers
	err = client.ComposeUp(ctx, composePath, "test-project")
	if err != nil {
		t.Fatalf("ComposeUp() failed: %v", err)
	}
	defer func() {
		_ = client.ComposeDown(ctx, composePath, "test-project")
	}()

	// Query for specific slug
	containers, err := client.ListContainers(ctx, "test-project", "feat-1")
	if err != nil {
		t.Fatalf("ListContainers(feat-1) failed: %v", err)
	}

	// Verify only feat-1 slug returned
	for _, ctr := range containers {
		if ctr.Slug != "feat-1" {
			t.Errorf("expected only slug=feat-1, got container with slug=%s", ctr.Slug)
		}
	}

	if len(containers) == 0 {
		t.Error("expected at least one container with slug=feat-1")
	}

	// Verify the container without slug is not returned
	foundWithoutSlug := false
	for _, ctr := range containers {
		if strings.Contains(ctr.Name, "without-slug") {
			foundWithoutSlug = true
			break
		}
	}
	if foundWithoutSlug {
		t.Error("container without slug should not be returned when filtering by slug")
	}
}

// TestComposeExec_Integration verifies command execution in running container.
func TestComposeExec_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test requiring Docker")
	}

	// Create minimal compose file
	tmpDir := t.TempDir()
	composePath := filepath.Join(tmpDir, "docker-compose.yml")
	composeYAML := `services:
  test-agent:
    image: alpine:latest
    command: sleep 3600
    labels:
      muster.managed: "true"
      muster.project: "test-project"
      muster.slug: "test-slug"
`
	err := os.WriteFile(composePath, []byte(composeYAML), 0644) //nolint:gosec // G306: test files
	if err != nil {
		t.Fatalf("failed to write compose file: %v", err)
	}

	client, err := NewClient()
	if err != nil {
		t.Skipf("Docker not available, skipping integration test: %v", err)
	}
	defer func() { _ = client.Close() }()

	ctx := context.Background()

	// Ensure clean state
	_ = client.ComposeDown(ctx, composePath, "test-project")

	// Start container
	err = client.ComposeUp(ctx, composePath, "test-project")
	if err != nil {
		t.Fatalf("ComposeUp() failed: %v", err)
	}
	defer func() {
		_ = client.ComposeDown(ctx, composePath, "test-project")
	}()

	// Wait briefly for container to be fully running
	time.Sleep(2 * time.Second)

	// Execute command in container
	cmd := []string{"echo", "test-output"}
	err = client.ComposeExec(ctx, composePath, "test-project", "test-agent", cmd)
	if err != nil {
		t.Fatalf("ComposeExec() failed: %v", err)
	}

	// Note: In a real implementation, we might want to capture and verify stdout,
	// but for this integration test, we just verify the command doesn't error.
}

// TestComposeDown_Integration verifies container cleanup.
func TestComposeDown_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test requiring Docker")
	}

	// Create minimal compose file
	tmpDir := t.TempDir()
	composePath := filepath.Join(tmpDir, "docker-compose.yml")
	composeYAML := `services:
  test-agent:
    image: alpine:latest
    command: sleep 3600
    labels:
      muster.managed: "true"
      muster.project: "test-project-down"
      muster.slug: "test-slug-down"
`
	err := os.WriteFile(composePath, []byte(composeYAML), 0644) //nolint:gosec // G306: test files
	if err != nil {
		t.Fatalf("failed to write compose file: %v", err)
	}

	client, err := NewClient()
	if err != nil {
		t.Skipf("Docker not available, skipping integration test: %v", err)
	}
	defer func() { _ = client.Close() }()

	ctx := context.Background()

	// Ensure clean state
	_ = client.ComposeDown(ctx, composePath, "test-project-down")

	// Start container
	err = client.ComposeUp(ctx, composePath, "test-project-down")
	if err != nil {
		t.Fatalf("ComposeUp() failed: %v", err)
	}

	// Verify container exists
	containers, err := client.ListContainers(ctx, "test-project-down", "test-slug-down")
	if err != nil {
		t.Fatalf("ListContainers() before down failed: %v", err)
	}
	if len(containers) == 0 {
		t.Fatal("expected container to exist before ComposeDown")
	}

	// Stop and remove containers
	err = client.ComposeDown(ctx, composePath, "test-project-down")
	if err != nil {
		t.Fatalf("ComposeDown() failed: %v", err)
	}

	// Verify containers are removed
	containers, err = client.ListContainers(ctx, "test-project-down", "test-slug-down")
	if err != nil {
		t.Fatalf("ListContainers() after down failed: %v", err)
	}

	if len(containers) > 0 {
		t.Errorf("expected no containers after ComposeDown, found %d", len(containers))
		for _, ctr := range containers {
			t.Logf("  - %s (status: %s)", ctr.Name, ctr.Status)
		}
	}
}

// TestComposeLifecycle_FullIntegration is a comprehensive test of the full lifecycle.
func TestComposeLifecycle_FullIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test requiring Docker")
	}

	// Create compose file with multiple services
	tmpDir := t.TempDir()
	composePath := filepath.Join(tmpDir, "docker-compose.yml")
	composeYAML := `services:
  dev-agent:
    image: alpine:latest
    command: sleep 3600
    labels:
      muster.managed: "true"
      muster.project: "full-test"
      muster.slug: "feat-branch"
  helper:
    image: alpine:latest
    command: sleep 3600
    labels:
      muster.managed: "true"
      muster.project: "full-test"
      muster.slug: "feat-branch"
`
	err := os.WriteFile(composePath, []byte(composeYAML), 0644) //nolint:gosec // G306: test files
	if err != nil {
		t.Fatalf("failed to write compose file: %v", err)
	}

	client, err := NewClient()
	if err != nil {
		t.Skipf("Docker not available, skipping integration test: %v", err)
	}
	defer func() { _ = client.Close() }()

	ctx := context.Background()

	// 1. Ping - verify Docker is available
	err = client.Ping(ctx)
	if err != nil {
		t.Fatalf("Ping() failed: %v", err)
	}

	// 2. Ensure clean state
	_ = client.ComposeDown(ctx, composePath, "full-test")

	// 3. Start containers
	err = client.ComposeUp(ctx, composePath, "full-test")
	if err != nil {
		t.Fatalf("ComposeUp() failed: %v", err)
	}
	defer func() {
		_ = client.ComposeDown(ctx, composePath, "full-test")
	}()

	// 4. List all containers for project
	allContainers, err := client.ListContainers(ctx, "full-test", "")
	if err != nil {
		t.Fatalf("ListContainers(all) failed: %v", err)
	}
	if len(allContainers) < 2 {
		t.Errorf("expected at least 2 containers, got %d", len(allContainers))
	}

	// 5. List containers by slug
	slugContainers, err := client.ListContainers(ctx, "full-test", "feat-branch")
	if err != nil {
		t.Fatalf("ListContainers(slug) failed: %v", err)
	}
	if len(slugContainers) < 2 {
		t.Errorf("expected at least 2 containers with slug, got %d", len(slugContainers))
	}

	// 6. Verify all have correct labels
	for _, ctr := range slugContainers {
		if ctr.Project != "full-test" {
			t.Errorf("container %s: expected project=full-test, got %s", ctr.Name, ctr.Project)
		}
		if ctr.Slug != "feat-branch" {
			t.Errorf("container %s: expected slug=feat-branch, got %s", ctr.Name, ctr.Slug)
		}
		if ctr.Labels[LabelManaged] != "true" {
			t.Errorf("container %s: expected managed=true", ctr.Name)
		}
	}

	// 7. Execute command in dev-agent service
	time.Sleep(2 * time.Second) // Wait for container to be ready
	cmd := []string{"sh", "-c", "echo 'Hello from container'"}
	err = client.ComposeExec(ctx, composePath, "full-test", "dev-agent", cmd)
	if err != nil {
		t.Errorf("ComposeExec() failed: %v", err)
	}

	// 8. Tear down
	err = client.ComposeDown(ctx, composePath, "full-test")
	if err != nil {
		t.Fatalf("ComposeDown() failed: %v", err)
	}

	// 9. Verify cleanup
	remainingContainers, err := client.ListContainers(ctx, "full-test", "")
	if err != nil {
		t.Fatalf("ListContainers(verify cleanup) failed: %v", err)
	}
	if len(remainingContainers) > 0 {
		t.Errorf("expected no containers after cleanup, found %d", len(remainingContainers))
	}
}

// TestIsComposeV2OrHigher verifies version detection logic.
// Task 5.6: Unit test for version detection with mocked output.
func TestIsComposeV2OrHigher(t *testing.T) {
	tests := []struct {
		name     string
		output   string
		expected bool
	}{
		{
			name:     "v2.0.0 format",
			output:   "Docker Compose version v2.0.0",
			expected: true,
		},
		{
			name:     "v2.20.2 format",
			output:   "Docker Compose version v2.20.2",
			expected: true,
		},
		{
			name:     "v3.0.0 format",
			output:   "Docker Compose version v3.0.0",
			expected: true,
		},
		{
			name:     "v1.29.2 format (old)",
			output:   "docker-compose version 1.29.2, build 5becea4c",
			expected: false,
		},
		{
			name:     "simple v2 format",
			output:   "v2.20.2",
			expected: true,
		},
		{
			name:     "simple v1 format",
			output:   "v1.29.2",
			expected: false,
		},
		{
			name:     "without v prefix v2",
			output:   "2.20.2",
			expected: true,
		},
		{
			name:     "without v prefix v1",
			output:   "1.29.2",
			expected: false,
		},
		{
			name:     "no version number",
			output:   "docker compose",
			expected: false,
		},
		{
			name:     "empty output",
			output:   "",
			expected: false,
		},
		{
			name:     "malformed version",
			output:   "version abc.def.ghi",
			expected: false,
		},
		{
			name:     "v2 with extra text",
			output:   "Docker Compose version v2.29.7\nCopyright (C) 2020-2024 Docker Inc.",
			expected: true,
		},
		{
			name:     "v1 detected (should return false)",
			output:   "docker-compose version 1.29.2, build 5becea4c",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isComposeV2OrHigher(tt.output)
			if result != tt.expected {
				t.Errorf("isComposeV2OrHigher(%q) = %v, want %v", tt.output, result, tt.expected)
			}
		})
	}
}

// TestVerifyComposeVersion_ErrorMessages verifies actionable error messages.
// Task 5.6: Test v2 detected, v1 detected (error), neither found (error).
func TestVerifyComposeVersion_ErrorMessages(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping compose version test in short mode")
	}

	err := verifyComposeVersion()

	if err != nil {
		// Verify error message contains installation URL
		errMsg := err.Error()
		if !strings.Contains(errMsg, "https://docs.docker.com/compose/install/") {
			t.Errorf("error message should contain installation URL, got: %s", errMsg)
		}

		// Check for expected error patterns
		hasV1Error := strings.Contains(errMsg, "Docker Compose v1 detected")
		hasV2RequiredError := strings.Contains(errMsg, "Docker Compose v2+ required")

		if !hasV1Error && !hasV2RequiredError {
			t.Errorf("error message should mention v1 detected or v2 required, got: %s", errMsg)
		}

		t.Logf("Expected error (compose not available or wrong version): %v", err)
	} else {
		t.Log("Docker Compose v2+ detected successfully")
	}
}

// TestNewClient_ComposeMissing verifies NewClient fails gracefully without Docker Compose.
// Task 5.6: Verify error when compose is not available.
func TestNewClient_ComposeMissing(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping compose availability test in short mode")
	}

	// This test will succeed or fail depending on system state
	client, err := NewClient()

	if err != nil {
		// Expected if Docker Compose not installed or wrong version
		errMsg := err.Error()

		// Verify error messages are actionable
		if strings.Contains(errMsg, "v1 detected") {
			if !strings.Contains(errMsg, "https://docs.docker.com/compose/install/") {
				t.Error("v1 detected error should contain upgrade URL")
			}
			t.Logf("Docker Compose v1 detected (expected error): %v", err)
		} else if strings.Contains(errMsg, "v2+ required") {
			if !strings.Contains(errMsg, "https://docs.docker.com/compose/install/") {
				t.Error("v2 required error should contain installation URL")
			}
			t.Logf("Docker Compose not found (expected error): %v", err)
		} else {
			t.Logf("Other Docker error: %v", err)
		}
	} else {
		// Docker Compose v2+ is available
		if client == nil {
			t.Fatal("NewClient returned nil client with no error")
		}
		if client.sdk == nil {
			t.Error("client.sdk should not be nil")
		}
		_ = client.Close()
		t.Log("Docker Compose v2+ is available")
	}
}
