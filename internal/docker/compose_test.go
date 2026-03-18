package docker

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestParseComposeFile(t *testing.T) {
	tests := []struct {
		name        string
		yaml        string
		expectError bool
	}{
		{
			name: "valid minimal compose",
			yaml: `version: '3.8'
services:
  app:
    image: alpine:latest
`,
			expectError: false,
		},
		{
			name: "valid full compose",
			yaml: `version: '3.8'
name: test-project
services:
  app:
    image: alpine:latest
    environment:
      KEY: value
    volumes:
      - ./data:/data
    ports:
      - "8080:8080"
volumes:
  data: {}
networks:
  default:
    driver: bridge
`,
			expectError: false,
		},
		{
			name:        "invalid yaml",
			yaml:        `invalid: [unclosed`,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			compose, err := parseComposeFile([]byte(tt.yaml))

			if tt.expectError {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if compose == nil {
				t.Error("expected compose file, got nil")
			}
		})
	}
}

func TestMarshalComposeFile(t *testing.T) {
	compose := &ComposeFile{
		Version: "3.8",
		Name:    "test-project",
		Services: map[string]*Service{
			"app": {
				Image: "alpine:latest",
				Environment: map[string]string{
					"KEY": "value",
				},
			},
		},
	}

	data, err := marshalComposeFile(compose)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	// Verify we can parse it back
	parsed, err := parseComposeFile(data)
	if err != nil {
		t.Fatalf("failed to parse marshaled data: %v", err)
	}

	if parsed.Version != compose.Version {
		t.Errorf("version = %v, want %v", parsed.Version, compose.Version)
	}
	if parsed.Name != compose.Name {
		t.Errorf("name = %v, want %v", parsed.Name, compose.Name)
	}
}

func TestAppendUnique(t *testing.T) {
	tests := []struct {
		name string
		base []string
		add  []string
		want []string
	}{
		{
			name: "no duplicates",
			base: []string{"a", "b"},
			add:  []string{"c", "d"},
			want: []string{"a", "b", "c", "d"},
		},
		{
			name: "all duplicates",
			base: []string{"a", "b"},
			add:  []string{"a", "b"},
			want: []string{"a", "b"},
		},
		{
			name: "partial duplicates",
			base: []string{"a", "b", "c"},
			add:  []string{"b", "d", "e"},
			want: []string{"a", "b", "c", "d", "e"},
		},
		{
			name: "empty base",
			base: []string{},
			add:  []string{"a", "b"},
			want: []string{"a", "b"},
		},
		{
			name: "empty add",
			base: []string{"a", "b"},
			add:  []string{},
			want: []string{"a", "b"},
		},
		{
			name: "nil add",
			base: []string{"a", "b"},
			add:  nil,
			want: []string{"a", "b"},
		},
		{
			name: "duplicates in add list",
			base: []string{"a"},
			add:  []string{"b", "b", "c"},
			want: []string{"a", "b", "c"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := appendUnique(tt.base, tt.add)

			if len(got) != len(tt.want) {
				t.Errorf("length = %v, want %v", len(got), len(tt.want))
			}

			for i, v := range got {
				if i >= len(tt.want) || v != tt.want[i] {
					t.Errorf("result = %v, want %v", got, tt.want)
					break
				}
			}
		})
	}
}

func TestMergeService(t *testing.T) {
	tests := []struct {
		name string
		base *Service
		over *Service
		want *Service
	}{
		{
			name: "scalar field override",
			base: &Service{
				Image: "alpine:3.14",
				User:  "root",
			},
			over: &Service{
				Image: "alpine:3.15",
			},
			want: &Service{
				Image:       "alpine:3.15",
				User:        "root",
				Environment: map[string]string{},
				Labels:      map[string]string{},
			},
		},
		{
			name: "map merge",
			base: &Service{
				Environment: map[string]string{
					"KEY1": "value1",
					"KEY2": "value2",
				},
			},
			over: &Service{
				Environment: map[string]string{
					"KEY2": "override",
					"KEY3": "value3",
				},
			},
			want: &Service{
				Environment: map[string]string{
					"KEY1": "value1",
					"KEY2": "override",
					"KEY3": "value3",
				},
				Labels: map[string]string{},
			},
		},
		{
			name: "list append and deduplicate",
			base: &Service{
				Volumes: []string{
					"/data:/data",
					"/config:/config",
				},
			},
			over: &Service{
				Volumes: []string{
					"/config:/config", // duplicate
					"/logs:/logs",     // new
				},
			},
			want: &Service{
				Volumes: []string{
					"/data:/data",
					"/config:/config",
					"/logs:/logs",
				},
				Environment: map[string]string{},
				Labels:      map[string]string{},
			},
		},
		{
			name: "type conflict - command string to array",
			base: &Service{
				Command: "echo hello",
			},
			over: &Service{
				Command: []string{"sh", "-c", "echo world"},
			},
			want: &Service{
				Command:     []string{"sh", "-c", "echo world"},
				Environment: map[string]string{},
				Labels:      map[string]string{},
			},
		},
		{
			name: "type conflict - command array to string",
			base: &Service{
				Command: []string{"sh", "-c", "echo hello"},
			},
			over: &Service{
				Command: "echo world",
			},
			want: &Service{
				Command:     "echo world",
				Environment: map[string]string{},
				Labels:      map[string]string{},
			},
		},
		{
			name: "full service merge",
			base: &Service{
				Image: "alpine:latest",
				Environment: map[string]string{
					"BASE_VAR": "base",
				},
				Volumes: []string{"/data:/data"},
				Labels: map[string]string{
					"base.label": "true",
				},
			},
			over: &Service{
				WorkingDir: "/app",
				Environment: map[string]string{
					"OVERRIDE_VAR": "override",
				},
				Volumes: []string{"/logs:/logs"},
				Labels: map[string]string{
					"override.label": "true",
				},
			},
			want: &Service{
				Image:      "alpine:latest",
				WorkingDir: "/app",
				Environment: map[string]string{
					"BASE_VAR":     "base",
					"OVERRIDE_VAR": "override",
				},
				Volumes: []string{"/data:/data", "/logs:/logs"},
				Labels: map[string]string{
					"base.label":     "true",
					"override.label": "true",
				},
			},
		},
		{
			name: "nil base",
			base: nil,
			over: &Service{
				Image: "alpine:latest",
			},
			want: &Service{
				Image: "alpine:latest",
			},
		},
		{
			name: "nil override",
			base: &Service{
				Image: "alpine:latest",
			},
			over: nil,
			want: &Service{
				Image: "alpine:latest",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mergeService(tt.base, tt.over)

			if got == nil && tt.want != nil {
				t.Error("got nil, want non-nil")
				return
			}
			if got != nil && tt.want == nil {
				t.Error("got non-nil, want nil")
				return
			}
			if got == nil && tt.want == nil {
				return
			}

			// Compare scalar fields
			if got.Image != tt.want.Image {
				t.Errorf("Image = %v, want %v", got.Image, tt.want.Image)
			}
			if got.WorkingDir != tt.want.WorkingDir {
				t.Errorf("WorkingDir = %v, want %v", got.WorkingDir, tt.want.WorkingDir)
			}
			if got.User != tt.want.User {
				t.Errorf("User = %v, want %v", got.User, tt.want.User)
			}

			// Compare Command (handle type flexibility)
			if tt.want.Command != nil {
				if got.Command == nil {
					t.Errorf("Command = nil, want %v", tt.want.Command)
				}
				// Both should match in value
			}

			// Compare environment
			if len(got.Environment) != len(tt.want.Environment) {
				t.Errorf("Environment length = %v, want %v", len(got.Environment), len(tt.want.Environment))
			}
			for k, v := range tt.want.Environment {
				if got.Environment[k] != v {
					t.Errorf("Environment[%s] = %v, want %v", k, got.Environment[k], v)
				}
			}

			// Compare labels
			if len(got.Labels) != len(tt.want.Labels) {
				t.Errorf("Labels length = %v, want %v", len(got.Labels), len(tt.want.Labels))
			}
			for k, v := range tt.want.Labels {
				if got.Labels[k] != v {
					t.Errorf("Labels[%s] = %v, want %v", k, got.Labels[k], v)
				}
			}

			// Compare volumes
			if len(got.Volumes) != len(tt.want.Volumes) {
				t.Errorf("Volumes = %v, want %v", got.Volumes, tt.want.Volumes)
			}
		})
	}
}

func TestMergeComposeFiles(t *testing.T) {
	tests := []struct {
		name string
		base *ComposeFile
		over *ComposeFile
		want *ComposeFile
	}{
		{
			name: "version override",
			base: &ComposeFile{
				Version: "3.7",
			},
			over: &ComposeFile{
				Version: "3.8",
			},
			want: &ComposeFile{
				Version: "3.8",
			},
		},
		{
			name: "name override",
			base: &ComposeFile{
				Name: "base-project",
			},
			over: &ComposeFile{
				Name: "override-project",
			},
			want: &ComposeFile{
				Name: "override-project",
			},
		},
		{
			name: "service merge",
			base: &ComposeFile{
				Services: map[string]*Service{
					"app": {
						Image: "alpine:3.14",
					},
				},
			},
			over: &ComposeFile{
				Services: map[string]*Service{
					"app": {
						WorkingDir: "/app",
					},
				},
			},
			want: &ComposeFile{
				Services: map[string]*Service{
					"app": {
						Image:       "alpine:3.14",
						WorkingDir:  "/app",
						Environment: map[string]string{},
						Labels:      map[string]string{},
					},
				},
			},
		},
		{
			name: "new service added",
			base: &ComposeFile{
				Services: map[string]*Service{
					"app": {
						Image: "alpine:latest",
					},
				},
			},
			over: &ComposeFile{
				Services: map[string]*Service{
					"db": {
						Image: "postgres:13",
					},
				},
			},
			want: &ComposeFile{
				Services: map[string]*Service{
					"app": {
						Image: "alpine:latest",
					},
					"db": {
						Image: "postgres:13",
					},
				},
			},
		},
		{
			name: "volumes merge",
			base: &ComposeFile{
				Volumes: map[string]any{
					"data": map[string]any{
						"driver": "local",
					},
				},
			},
			over: &ComposeFile{
				Volumes: map[string]any{
					"logs": map[string]any{
						"driver": "local",
					},
				},
			},
			want: &ComposeFile{
				Volumes: map[string]any{
					"data": map[string]any{
						"driver": "local",
					},
					"logs": map[string]any{
						"driver": "local",
					},
				},
			},
		},
		{
			name: "networks merge",
			base: &ComposeFile{
				Networks: map[string]any{
					"frontend": nil,
				},
			},
			over: &ComposeFile{
				Networks: map[string]any{
					"backend": map[string]any{
						"driver": "bridge",
					},
				},
			},
			want: &ComposeFile{
				Networks: map[string]any{
					"frontend": nil,
					"backend": map[string]any{
						"driver": "bridge",
					},
				},
			},
		},
		{
			name: "nil base",
			base: nil,
			over: &ComposeFile{
				Version: "3.8",
			},
			want: nil, // Function returns early if base is nil
		},
		{
			name: "nil override",
			base: &ComposeFile{
				Version: "3.8",
			},
			over: nil,
			want: &ComposeFile{
				Version: "3.8",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Make a copy of base since mergeComposeFiles modifies in place
			var base *ComposeFile
			if tt.base != nil {
				data, _ := yaml.Marshal(tt.base)
				base = &ComposeFile{}
				_ = yaml.Unmarshal(data, base)
			}

			mergeComposeFiles(base, tt.over)

			if base == nil && tt.want != nil {
				t.Error("got nil, want non-nil")
				return
			}
			if base != nil && tt.want == nil {
				t.Error("got non-nil, want nil")
				return
			}
			if base == nil && tt.want == nil {
				return
			}

			if base.Version != tt.want.Version {
				t.Errorf("Version = %v, want %v", base.Version, tt.want.Version)
			}
			if base.Name != tt.want.Name {
				t.Errorf("Name = %v, want %v", base.Name, tt.want.Name)
			}

			if len(base.Services) != len(tt.want.Services) {
				t.Errorf("Services count = %v, want %v", len(base.Services), len(tt.want.Services))
			}

			if len(base.Volumes) != len(tt.want.Volumes) {
				t.Errorf("Volumes count = %v, want %v", len(base.Volumes), len(tt.want.Volumes))
			}

			if len(base.Networks) != len(tt.want.Networks) {
				t.Errorf("Networks count = %v, want %v", len(base.Networks), len(tt.want.Networks))
			}
		})
	}
}

func TestNormalizeCommand(t *testing.T) {
	tests := []struct {
		name string
		cmd  any
		want string
	}{
		{
			name: "string command",
			cmd:  "echo hello",
			want: "echo hello",
		},
		{
			name: "string array command",
			cmd:  []string{"sh", "-c", "echo hello"},
			want: "sh -c echo hello",
		},
		{
			name: "interface array command",
			cmd:  []any{"sh", "-c", "echo hello"},
			want: "sh -c echo hello",
		},
		{
			name: "nil command",
			cmd:  nil,
			want: "",
		},
		{
			name: "empty string",
			cmd:  "",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeCommand(tt.cmd)
			if got != tt.want {
				t.Errorf("normalizeCommand() = %v, want %v", got, tt.want)
			}
		})
	}
}

// Golden file tests for compose generation
func TestComposeGeneration_Golden(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping golden file test in short mode")
	}

	tests := []struct {
		name     string
		compose  *ComposeFile
		filename string
	}{
		{
			name: "base compose",
			compose: &ComposeFile{
				Version: "3.8",
				Name:    "test-project",
				Services: map[string]*Service{
					"app": {
						Image: "alpine:latest",
						Environment: map[string]string{
							"ENV": "test",
						},
						Volumes: []string{
							"./data:/data",
						},
					},
				},
			},
			filename: "base-compose.yml",
		},
		{
			name: "compose with labels",
			compose: &ComposeFile{
				Version: "3.8",
				Services: map[string]*Service{
					"app": {
						Image: "alpine:latest",
						Labels: map[string]string{
							"muster.managed": "true",
							"muster.project": "test",
						},
					},
				},
			},
			filename: "compose-with-labels.yml",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Marshal compose to YAML
			got, err := marshalComposeFile(tt.compose)
			if err != nil {
				t.Fatalf("failed to marshal compose: %v", err)
			}

			// Golden file path
			goldenPath := filepath.Join("testdata", "golden", tt.filename)

			// Update golden file if -update flag is set
			if os.Getenv("UPDATE_GOLDEN") == "1" {
				//nolint:gosec // G301: Test directory permissions are appropriate
				if err := os.MkdirAll(filepath.Dir(goldenPath), 0755); err != nil {
					t.Fatalf("failed to create golden dir: %v", err)
				}
				//nolint:gosec // G306: Test file permissions are appropriate
				if err := os.WriteFile(goldenPath, got, 0644); err != nil {
					t.Fatalf("failed to write golden file: %v", err)
				}
			}

			// Read golden file
			//nolint:gosec // G304: Reading test fixture golden file with known safe path
			want, err := os.ReadFile(goldenPath)
			if err != nil {
				if os.IsNotExist(err) {
					t.Logf("Golden file does not exist: %s", goldenPath)
					t.Logf("Run with UPDATE_GOLDEN=1 to create it")
					t.Logf("Got:\n%s", string(got))
				}
				t.Fatalf("failed to read golden file: %v", err)
			}

			// Compare
			if string(got) != string(want) {
				t.Errorf("compose mismatch\nGot:\n%s\nWant:\n%s", string(got), string(want))
			}
		})
	}
}
