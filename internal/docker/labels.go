package docker

import (
	"time"
)

// Label constants for container metadata tracking.
const (
	// LabelManaged marks containers managed by muster.
	LabelManaged = "muster.managed"

	// LabelProject identifies the project name.
	LabelProject = "muster.project"

	// LabelSlug identifies the specific instance (e.g., git branch slug).
	LabelSlug = "muster.slug"

	// LabelCreated records the container creation timestamp.
	LabelCreated = "muster.created"
)

// Labels builds a label map for Docker containers.
// Returns labels with project name, optional slug, and RFC3339 timestamp.
func Labels(project, slug string) map[string]string {
	labels := map[string]string{
		LabelManaged: "true",
		LabelProject: project,
		LabelCreated: time.Now().UTC().Format(time.RFC3339),
	}

	if slug != "" {
		labels[LabelSlug] = slug
	}

	return labels
}

// FormatLabels converts a label map to docker-compose YAML format.
func FormatLabels(labels map[string]string) map[string]string {
	// Return as-is for YAML marshaling.
	return labels
}
