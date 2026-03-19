package docker

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/abenz1267/muster/internal/config"
)

// setTestHome sets the home directory for tests, handling both Unix and Windows.
func setTestHome(t *testing.T, dir string) {
	t.Helper()
	t.Setenv("HOME", dir)
	if runtime.GOOS == "windows" {
		t.Setenv("USERPROFILE", dir)
	}
}

func TestDetectBedrockAuth(t *testing.T) {
	tests := []struct {
		name          string
		setupEnv      map[string]string
		setupFiles    map[string]string
		expectError   bool
		errorContains string
	}{
		{
			name: "missing bedrock env var",
			setupEnv: map[string]string{
				"CLAUDE_CODE_USE_BEDROCK": "",
			},
			expectError:   true,
			errorContains: "bedrock requires CLAUDE_CODE_USE_BEDROCK=1",
		},
		{
			name: "missing settings.json",
			setupEnv: map[string]string{
				"CLAUDE_CODE_USE_BEDROCK": "1",
			},
			expectError:   true,
			errorContains: "bedrock auth requires ~/.claude/settings.json",
		},
		{
			name: "valid settings with explicit profile",
			setupEnv: map[string]string{
				"CLAUDE_CODE_USE_BEDROCK": "1",
				"AWS_PROFILE":             "test-profile",
				"AWS_REGION":              "us-west-2",
			},
			setupFiles: map[string]string{
				".claude/settings.json": `{"aws":{"profile":"default","region":"us-east-1"}}`,
			},
			expectError:   true, // Will fail because AWS CLI is not mocked
			errorContains: "bedrock auth requires ~/.aws directory",
		},
		{
			name: "settings with defaults",
			setupEnv: map[string]string{
				"CLAUDE_CODE_USE_BEDROCK": "1",
			},
			setupFiles: map[string]string{
				".claude/settings.json": `{"aws":{"profile":"bedrock-profile","region":"eu-west-1"}}`,
			},
			expectError:   true, // Will fail because AWS CLI is not mocked
			errorContains: "bedrock auth requires ~/.aws directory",
		},
		{
			name: "invalid json",
			setupEnv: map[string]string{
				"CLAUDE_CODE_USE_BEDROCK": "1",
			},
			setupFiles: map[string]string{
				".claude/settings.json": `{invalid json}`,
			},
			expectError:   true,
			errorContains: "failed to parse settings.json",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp directory for test
			tempDir := t.TempDir()

			// Setup environment
			for k, v := range tt.setupEnv {
				t.Setenv(k, v)
			}
			setTestHome(t, tempDir)

			// Setup files
			for path, content := range tt.setupFiles {
				fullPath := filepath.Join(tempDir, path)
				if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil { //nolint:gosec // G301: Test directory permissions are appropriate
					t.Fatalf("failed to create directory: %v", err)
				}
				if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil { //nolint:gosec // G306: Test file permissions are appropriate
					t.Fatalf("failed to write file: %v", err)
				}
			}

			// Run test
			auth, err := DetectBedrockAuth()

			// Verify results
			if tt.expectError {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tt.errorContains)
				} else if tt.errorContains != "" && !contains(err.Error(), tt.errorContains) {
					t.Errorf("expected error containing %q, got %q", tt.errorContains, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if auth == nil {
					t.Error("expected auth to be non-nil")
				}
			}
		})
	}
}

func TestDetectMaxAuth(t *testing.T) {
	tests := []struct {
		name          string
		setupFiles    map[string]string
		expectError   bool
		errorContains string
	}{
		{
			name:          "missing credentials file",
			expectError:   true,
			errorContains: "max auth requires ~/.claude/.credentials.json",
		},
		{
			name: "credentials file exists",
			//nolint:gosec // G101: Test fixture with hardcoded token for testing purposes only
			setupFiles: map[string]string{
				".claude/.credentials.json": `{"token":"test-token"}`,
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp directory for test
			tempDir := t.TempDir()
			setTestHome(t, tempDir)

			// Setup files
			for path, content := range tt.setupFiles {
				fullPath := filepath.Join(tempDir, path)
				if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil { //nolint:gosec // G301: Test directory permissions are appropriate
					t.Fatalf("failed to create directory: %v", err)
				}
				if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil { //nolint:gosec // G306: Test file permissions are appropriate
					t.Fatalf("failed to write file: %v", err)
				}
			}

			// Run test
			auth, err := DetectMaxAuth()

			// Verify results
			if tt.expectError {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tt.errorContains)
				} else if tt.errorContains != "" && !contains(err.Error(), tt.errorContains) {
					t.Errorf("expected error containing %q, got %q", tt.errorContains, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if auth == nil {
					t.Error("expected auth to be non-nil")
				}
				if auth != nil && auth.CredentialsPath == "" {
					t.Error("expected CredentialsPath to be non-empty")
				}
			}
		})
	}
}

func TestDetectAPIKeyAuth(t *testing.T) {
	tests := []struct {
		name          string
		envVar        string
		envValue      string
		expectError   bool
		errorContains string
	}{
		{
			name:          "env var not set",
			envVar:        "ANTHROPIC_API_KEY",
			envValue:      "",
			expectError:   true,
			errorContains: "provider requires ANTHROPIC_API_KEY",
		},
		{
			name:        "env var set",
			envVar:      "ANTHROPIC_API_KEY",
			envValue:    "sk-ant-test-key",
			expectError: false,
		},
		{
			name:        "custom env var",
			envVar:      "CUSTOM_API_KEY",
			envValue:    "custom-value",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envValue != "" {
				t.Setenv(tt.envVar, tt.envValue)
			}

			// Run test
			auth, err := DetectAPIKeyAuth(tt.envVar)

			// Verify results
			if tt.expectError {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tt.errorContains)
				} else if tt.errorContains != "" && !contains(err.Error(), tt.errorContains) {
					t.Errorf("expected error containing %q, got %q", tt.errorContains, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if auth == nil {
					t.Error("expected auth to be non-nil")
				}
				if auth != nil {
					if auth.EnvVar != tt.envVar {
						t.Errorf("expected EnvVar=%q, got %q", tt.envVar, auth.EnvVar)
					}
					if auth.Value != tt.envValue {
						t.Errorf("expected Value=%q, got %q", tt.envValue, auth.Value)
					}
				}
			}
		})
	}
}

func TestDetectLocalProvider(t *testing.T) {
	tests := []struct {
		name            string
		baseURL         string
		expectError     bool
		errorContains   string
		expectedRewrite string
	}{
		{
			name:          "empty URL",
			baseURL:       "",
			expectError:   true,
			errorContains: "base URL is required",
		},
		{
			name:            "localhost with port",
			baseURL:         "http://localhost:8080",
			expectError:     false,
			expectedRewrite: "http://host.docker.internal:8080",
		},
		{
			name:            "localhost with path",
			baseURL:         "http://localhost:8080/api/v1",
			expectError:     false,
			expectedRewrite: "http://host.docker.internal:8080/api/v1",
		},
		{
			name:            "127.0.0.1 with port",
			baseURL:         "http://127.0.0.1:3000",
			expectError:     false,
			expectedRewrite: "http://host.docker.internal:3000",
		},
		{
			name:            "ipv6 localhost",
			baseURL:         "http://[::1]:5000",
			expectError:     false,
			expectedRewrite: "http://host.docker.internal:5000",
		},
		{
			name:            "localhost without port",
			baseURL:         "http://localhost",
			expectError:     false,
			expectedRewrite: "http://host.docker.internal",
		},
		{
			name:            "remote URL unchanged",
			baseURL:         "http://example.com:8080",
			expectError:     false,
			expectedRewrite: "http://example.com:8080",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Run test
			auth, err := DetectLocalProvider(tt.baseURL)

			// Verify results
			if tt.expectError {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tt.errorContains)
				} else if tt.errorContains != "" && !contains(err.Error(), tt.errorContains) {
					t.Errorf("expected error containing %q, got %q", tt.errorContains, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if auth == nil {
					t.Error("expected auth to be non-nil")
				}
				if auth != nil {
					if auth.OriginalURL != tt.baseURL {
						t.Errorf("expected OriginalURL=%q, got %q", tt.baseURL, auth.OriginalURL)
					}
					if auth.BaseURL != tt.expectedRewrite {
						t.Errorf("expected BaseURL=%q, got %q", tt.expectedRewrite, auth.BaseURL)
					}
				}
			}
		})
	}
}

func TestRewriteLocalhostURL(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "localhost with port",
			input:    "http://localhost:8080",
			expected: "http://host.docker.internal:8080",
		},
		{
			name:     "localhost with path",
			input:    "http://localhost:8080/api/v1",
			expected: "http://host.docker.internal:8080/api/v1",
		},
		{
			name:     "localhost without port",
			input:    "http://localhost",
			expected: "http://host.docker.internal",
		},
		{
			name:     "localhost with slash",
			input:    "http://localhost/api",
			expected: "http://host.docker.internal/api",
		},
		{
			name:     "127.0.0.1 with port",
			input:    "http://127.0.0.1:3000",
			expected: "http://host.docker.internal:3000",
		},
		{
			name:     "127.0.0.1 with path",
			input:    "http://127.0.0.1:3000/api",
			expected: "http://host.docker.internal:3000/api",
		},
		{
			name:     "127.0.0.1 without port",
			input:    "http://127.0.0.1",
			expected: "http://host.docker.internal",
		},
		{
			name:     "ipv6 localhost with port",
			input:    "http://[::1]:5000",
			expected: "http://host.docker.internal:5000",
		},
		{
			name:     "ipv6 localhost with path",
			input:    "http://[::1]:5000/api",
			expected: "http://host.docker.internal:5000/api",
		},
		{
			name:     "ipv6 localhost without port",
			input:    "http://[::1]",
			expected: "http://host.docker.internal",
		},
		{
			name:     "https localhost",
			input:    "https://localhost:8443",
			expected: "https://host.docker.internal:8443",
		},
		{
			name:     "remote URL unchanged",
			input:    "http://example.com:8080",
			expected: "http://example.com:8080",
		},
		{
			name:     "remote IP unchanged",
			input:    "http://192.168.1.1:8080",
			expected: "http://192.168.1.1:8080",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := rewriteLocalhostURL(tt.input)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestCollectAuthRequirements(t *testing.T) {
	tests := []struct {
		name          string
		config        *config.Config
		steps         []string
		expectBedrock bool
		expectMax     bool
		expectAPIKeys int
		expectLocal   int
		expectErrors  bool
	}{
		{
			name: "interactive with API key provider",
			config: &config.Config{
				User: &config.UserConfig{
					Defaults: &config.DefaultsConfig{
						Provider: strPtr("anthropic"),
					},
					Providers: map[string]*config.ProviderConfig{
						"anthropic": {
							APIKeyEnv: strPtr("ANTHROPIC_API_KEY"),
						},
					},
				},
			},
			steps:         []string{"interactive"},
			expectAPIKeys: 0,    // Auth not added when detection fails
			expectErrors:  true, // Will fail because ANTHROPIC_API_KEY is not set
		},
		{
			name: "interactive with local provider",
			config: &config.Config{
				User: &config.UserConfig{
					Defaults: &config.DefaultsConfig{
						Provider: strPtr("local-ollama"),
					},
					Providers: map[string]*config.ProviderConfig{
						"local-ollama": {
							BaseURL: strPtr("http://localhost:11434"),
						},
					},
				},
			},
			steps:        []string{"interactive"},
			expectLocal:  1,
			expectErrors: false,
		},
		{
			name: "empty steps uses default",
			config: &config.Config{
				User: &config.UserConfig{
					Defaults: &config.DefaultsConfig{
						Provider: strPtr("test-provider"),
					},
					Providers: map[string]*config.ProviderConfig{
						"test-provider": {
							APIKeyEnv: strPtr("TEST_API_KEY"),
						},
					},
				},
			},
			steps:         []string{},
			expectAPIKeys: 0,    // Auth not added when detection fails
			expectErrors:  true, // Will fail because TEST_API_KEY is not set
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Run test
			reqs, errs := CollectAuthRequirements(tt.config, tt.steps)

			// Verify results
			if tt.expectErrors {
				if len(errs) == 0 {
					t.Error("expected errors but got none")
				}
			}

			if reqs == nil {
				t.Fatal("expected non-nil requirements")
			}

			if tt.expectBedrock && reqs.Bedrock == nil {
				t.Error("expected Bedrock auth but got nil")
			}
			if !tt.expectBedrock && reqs.Bedrock != nil {
				t.Error("expected no Bedrock auth but got one")
			}

			if tt.expectMax && reqs.Max == nil {
				t.Error("expected Max auth but got nil")
			}
			if !tt.expectMax && reqs.Max != nil {
				t.Error("expected no Max auth but got one")
			}

			if len(reqs.APIKeys) != tt.expectAPIKeys {
				t.Errorf("expected %d API keys, got %d", tt.expectAPIKeys, len(reqs.APIKeys))
			}

			if len(reqs.Local) != tt.expectLocal {
				t.Errorf("expected %d local providers, got %d", tt.expectLocal, len(reqs.Local))
			}
		})
	}

	// Add a successful test case with environment variable set
	t.Run("interactive_with_API_key_provider_success", func(t *testing.T) {
		t.Setenv("ANTHROPIC_API_KEY", "sk-ant-test-key")

		cfg := &config.Config{
			User: &config.UserConfig{
				Defaults: &config.DefaultsConfig{
					Provider: strPtr("anthropic"),
				},
				Providers: map[string]*config.ProviderConfig{
					"anthropic": {
						APIKeyEnv: strPtr("ANTHROPIC_API_KEY"),
					},
				},
			},
		}

		reqs, errs := CollectAuthRequirements(cfg, []string{"interactive"})

		if len(errs) > 0 {
			t.Errorf("unexpected errors: %v", errs)
		}

		if reqs == nil {
			t.Fatal("expected non-nil requirements")
		}

		if len(reqs.APIKeys) != 1 {
			t.Errorf("expected 1 API key, got %d", len(reqs.APIKeys))
		}

		if auth, ok := reqs.APIKeys["anthropic"]; !ok || auth == nil {
			t.Error("expected anthropic API key")
		} else if auth.Value != "sk-ant-test-key" {
			t.Errorf("expected API key value 'sk-ant-test-key', got %q", auth.Value)
		}
	})
}

// Helper functions

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && containsSubstring(s, substr))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func strPtr(s string) *string {
	return &s
}
