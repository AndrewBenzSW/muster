package docker

import (
	"testing"
	"time"
)

func TestLabels(t *testing.T) {
	tests := []struct {
		name        string
		project     string
		slug        string
		wantManaged string
		wantProject string
		wantSlug    bool
	}{
		{
			name:        "with slug",
			project:     "myproject",
			slug:        "feature-branch",
			wantManaged: "true",
			wantProject: "myproject",
			wantSlug:    true,
		},
		{
			name:        "without slug",
			project:     "myproject",
			slug:        "",
			wantManaged: "true",
			wantProject: "myproject",
			wantSlug:    false,
		},
		{
			name:        "empty project with slug",
			project:     "",
			slug:        "test-slug",
			wantManaged: "true",
			wantProject: "",
			wantSlug:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			labels := Labels(tt.project, tt.slug)

			// Check managed label
			if got := labels[LabelManaged]; got != tt.wantManaged {
				t.Errorf("Labels()[%s] = %v, want %v", LabelManaged, got, tt.wantManaged)
			}

			// Check project label
			if got := labels[LabelProject]; got != tt.wantProject {
				t.Errorf("Labels()[%s] = %v, want %v", LabelProject, got, tt.wantProject)
			}

			// Check slug label presence
			_, hasSlug := labels[LabelSlug]
			if hasSlug != tt.wantSlug {
				t.Errorf("Labels() slug presence = %v, want %v", hasSlug, tt.wantSlug)
			}

			if tt.wantSlug && labels[LabelSlug] != tt.slug {
				t.Errorf("Labels()[%s] = %v, want %v", LabelSlug, labels[LabelSlug], tt.slug)
			}

			// Check created timestamp is present and valid RFC3339
			created, ok := labels[LabelCreated]
			if !ok {
				t.Errorf("Labels() missing %s label", LabelCreated)
			}

			// Verify timestamp is valid RFC3339
			_, err := time.Parse(time.RFC3339, created)
			if err != nil {
				t.Errorf("Labels()[%s] = %v is not valid RFC3339: %v", LabelCreated, created, err)
			}

			// Verify timestamp is recent (within last 5 seconds)
			parsedTime, _ := time.Parse(time.RFC3339, created)
			if time.Since(parsedTime) > 5*time.Second {
				t.Errorf("Labels()[%s] timestamp is too old: %v", LabelCreated, parsedTime)
			}
		})
	}
}

func TestLabelsTimestampFormat(t *testing.T) {
	labels := Labels("test-project", "test-slug")
	created := labels[LabelCreated]

	// Parse the timestamp
	parsedTime, err := time.Parse(time.RFC3339, created)
	if err != nil {
		t.Fatalf("Failed to parse timestamp: %v", err)
	}

	// Verify it's in UTC
	if parsedTime.Location() != time.UTC {
		t.Errorf("Timestamp location = %v, want UTC", parsedTime.Location())
	}

	// Verify re-formatting matches (idempotent)
	reformatted := parsedTime.Format(time.RFC3339)
	if reformatted != created {
		t.Errorf("Re-formatted timestamp = %v, want %v", reformatted, created)
	}
}

func TestFormatLabels(t *testing.T) {
	input := map[string]string{
		"key1": "value1",
		"key2": "value2",
	}

	output := FormatLabels(input)

	// Should return the same map
	if len(output) != len(input) {
		t.Errorf("FormatLabels() length = %v, want %v", len(output), len(input))
	}

	for k, v := range input {
		if output[k] != v {
			t.Errorf("FormatLabels()[%s] = %v, want %v", k, output[k], v)
		}
	}
}
