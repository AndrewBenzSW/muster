package docker

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateAllowlist(t *testing.T) {
	tests := []struct {
		name           string
		auth           *AuthRequirements
		devAgent       interface{}
		expectDomains  []string
		expectMissing  []string
		expectWarnings bool
	}{
		{
			name: "base domains only",
			auth: nil,
			devAgent: struct {
				AllowedDomains []string
				Env            map[string]string
				Volumes        []string
				Networks       []string
			}{
				AllowedDomains: nil,
			},
			expectDomains: []string{"github.com", ".github.com", ".githubusercontent.com"},
		},
		{
			name: "bedrock provider domains",
			auth: &AuthRequirements{
				Bedrock: &BedrockAuth{
					AWSProfile: "default",
					AWSRegion:  "us-east-1",
				},
			},
			expectDomains: []string{".amazonaws.com", ".aws.amazon.com"},
		},
		{
			name: "max provider domains",
			auth: &AuthRequirements{
				//nolint:gosec // G101: Test fixture with hardcoded path, not actual credentials
				Max: &MaxAuth{
					CredentialsPath: "/path/to/creds",
				},
			},
			expectDomains: []string{".anthropic.com", ".claude.ai"},
		},
		{
			name: "api key provider domains",
			auth: &AuthRequirements{
				APIKeys: map[string]*APIKeyAuth{
					"openrouter": {
						EnvVar: "OPENROUTER_API_KEY",
						Value:  "test",
					},
				},
			},
			expectDomains: []string{".openrouter.ai"},
		},
		{
			name: "local provider domains",
			auth: &AuthRequirements{
				Local: map[string]*LocalAuth{
					"local": {
						BaseURL: "http://localhost:8080",
					},
				},
			},
			expectDomains: []string{"host.docker.internal"},
		},
		{
			name: "project domains merged",
			auth: nil,
			devAgent: struct {
				AllowedDomains []string
				Env            map[string]string
				Volumes        []string
				Networks       []string
			}{
				AllowedDomains: []string{"example.com", ".example.org"},
			},
			expectDomains: []string{"example.com", ".example.org"},
		},
		{
			name: "deduplication",
			auth: &AuthRequirements{
				Bedrock: &BedrockAuth{
					AWSProfile: "default",
					AWSRegion:  "us-east-1",
				},
			},
			devAgent: struct {
				AllowedDomains []string
				Env            map[string]string
				Volumes        []string
				Networks       []string
			}{
				AllowedDomains: []string{".amazonaws.com", "github.com"},
			},
			expectDomains: []string{".amazonaws.com", "github.com"},
		},
		{
			name: "invalid domains filtered",
			auth: nil,
			devAgent: struct {
				AllowedDomains []string
				Env            map[string]string
				Volumes        []string
				Networks       []string
			}{
				AllowedDomains: []string{"", "invalid domain with spaces", "valid.com"},
			},
			expectDomains:  []string{"valid.com"},
			expectMissing:  []string{"", "invalid domain with spaces"},
			expectWarnings: true,
		},
		{
			name: "multiple provider types",
			auth: &AuthRequirements{
				Bedrock: &BedrockAuth{
					AWSProfile: "default",
					AWSRegion:  "us-east-1",
				},
				//nolint:gosec // G101: Test fixture with hardcoded path, not actual credentials
				Max: &MaxAuth{
					CredentialsPath: "/path/to/creds",
				},
				Local: map[string]*LocalAuth{
					"local": {BaseURL: "http://localhost:8080"},
				},
			},
			expectDomains: []string{
				".amazonaws.com",
				".aws.amazon.com",
				".anthropic.com",
				".claude.ai",
				"host.docker.internal",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp directory for output
			tmpDir := t.TempDir()

			// Generate allowlist
			allowlistPath, err := GenerateAllowlist(tmpDir, tt.auth, tt.devAgent)
			if err != nil {
				t.Fatalf("GenerateAllowlist failed: %v", err)
			}

			// Verify file was created
			if allowlistPath != filepath.Join(tmpDir, "allowed-domains.txt") {
				t.Errorf("expected path %s, got %s", filepath.Join(tmpDir, "allowed-domains.txt"), allowlistPath)
			}

			// Read generated file
			//nolint:gosec // G304: Reading test fixture file with known safe path
			content, err := os.ReadFile(allowlistPath)
			if err != nil {
				t.Fatalf("failed to read allowlist: %v", err)
			}

			contentStr := string(content)

			// Check that expected domains are present
			for _, domain := range tt.expectDomains {
				if !strings.Contains(contentStr, domain) {
					t.Errorf("expected domain %q not found in allowlist:\n%s", domain, contentStr)
				}
			}

			// Check that invalid domains are not present
			for _, domain := range tt.expectMissing {
				if domain != "" && strings.Contains(contentStr, domain) {
					t.Errorf("invalid domain %q should not be in allowlist:\n%s", domain, contentStr)
				}
			}

			// Parse domains from file (skip comments and empty lines)
			domains := parseDomains(contentStr)

			// Check for duplicates
			seen := make(map[string]bool)
			for _, domain := range domains {
				if seen[domain] {
					t.Errorf("duplicate domain found: %q", domain)
				}
				seen[domain] = true
			}
		})
	}
}

func TestIsValidDomain(t *testing.T) {
	tests := []struct {
		domain string
		valid  bool
	}{
		// Valid domains
		{"example.com", true},
		{".example.com", true},
		{"*.example.com", true},
		{"sub.example.com", true},
		{".sub.example.com", true},
		{"github.com", true},
		{".github.com", true},
		{".githubusercontent.com", true},
		{"host.docker.internal", true},
		{".amazonaws.com", true},
		{"api.openai.com", true},
		{".anthropic.com", true},

		// Invalid domains
		{"", false},
		{"invalid domain", false},
		{"invalid..domain", false},
		{"-invalid.com", false},
		{"invalid-.com", false},
		{".com", false},
		{".", false},
		{"*", false},
		{"*..", false},
		{"example.com/path", false},
		{"http://example.com", false},
		{"example com", false},
		{"example@com", false},
	}

	for _, tt := range tests {
		t.Run(tt.domain, func(t *testing.T) {
			result := isValidDomain(tt.domain)
			if result != tt.valid {
				t.Errorf("isValidDomain(%q) = %v, want %v", tt.domain, result, tt.valid)
			}
		})
	}
}

func TestParseDomains(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected []string
	}{
		{
			name: "simple list",
			content: `example.com
.example.org
github.com`,
			expected: []string{"example.com", ".example.org", "github.com"},
		},
		{
			name: "with comments",
			content: `# This is a comment
example.com
# Another comment
.example.org`,
			expected: []string{"example.com", ".example.org"},
		},
		{
			name: "with empty lines",
			content: `example.com

.example.org

github.com`,
			expected: []string{"example.com", ".example.org", "github.com"},
		},
		{
			name: "with mixed whitespace",
			content: `  example.com
	.example.org
github.com   `,
			expected: []string{"example.com", ".example.org", "github.com"},
		},
		{
			name:     "empty content",
			content:  "",
			expected: []string{},
		},
		{
			name: "only comments",
			content: `# Comment 1
# Comment 2
# Comment 3`,
			expected: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseDomains(tt.content)

			if len(result) != len(tt.expected) {
				t.Errorf("parseDomains() returned %d domains, want %d", len(result), len(tt.expected))
				return
			}

			for i, domain := range result {
				if domain != tt.expected[i] {
					t.Errorf("parseDomains()[%d] = %q, want %q", i, domain, tt.expected[i])
				}
			}
		})
	}
}
