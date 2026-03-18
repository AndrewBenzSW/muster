package docker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/abenz1267/muster/internal/config"
)

// Auth types for different provider authentication methods.

// BedrockAuth contains AWS Bedrock authentication configuration.
type BedrockAuth struct {
	// AWSProfile is the AWS profile name to use
	AWSProfile string
	// AWSRegion is the AWS region
	AWSRegion string
	// Settings is the merged settings.json content
	Settings map[string]interface{}
}

// MaxAuth contains Claude Max authentication configuration.
type MaxAuth struct {
	// CredentialsPath is the path to the credentials file
	CredentialsPath string
}

// APIKeyAuth contains API key authentication configuration.
type APIKeyAuth struct {
	// EnvVar is the environment variable name containing the API key
	EnvVar string
	// Value is the actual API key value
	Value string
}

// LocalAuth contains local provider authentication configuration.
type LocalAuth struct {
	// BaseURL is the rewritten base URL (with host.docker.internal)
	BaseURL string
	// OriginalURL is the original base URL
	OriginalURL string
}

// AuthRequirements contains all detected authentication for a command.
type AuthRequirements struct {
	// Bedrock contains Bedrock auth if needed
	Bedrock *BedrockAuth
	// Max contains Max auth if needed
	Max *MaxAuth
	// APIKeys contains API key auth by provider name
	APIKeys map[string]*APIKeyAuth
	// Local contains local provider auth by provider name
	Local map[string]*LocalAuth
}

// ClaudeSettings represents the structure of ~/.claude/settings.json.
type ClaudeSettings struct {
	AWS *struct {
		Profile string `json:"profile"`
		Region  string `json:"region"`
	} `json:"aws"`
}

// DetectBedrockAuth detects and validates AWS Bedrock authentication.
// It reads ~/.claude/settings.json, checks for CLAUDE_CODE_USE_BEDROCK,
// extracts AWS profile/region from settings or environment variables,
// and pre-resolves credentials via aws CLI.
func DetectBedrockAuth() (*BedrockAuth, error) {
	// Check if Bedrock is enabled
	if os.Getenv("CLAUDE_CODE_USE_BEDROCK") != "1" {
		return nil, errors.New("bedrock requires CLAUDE_CODE_USE_BEDROCK=1")
	}

	// Read settings.json
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	settingsPath := filepath.Join(homeDir, ".claude", "settings.json")
	settingsData, err := os.ReadFile(settingsPath) //nolint:gosec // G304: Reading settings file from user's home directory is intended behavior
	if err != nil {
		return nil, fmt.Errorf("bedrock auth requires ~/.claude/settings.json; create it with AWS profile/region configuration")
	}

	var settings ClaudeSettings
	if err := json.Unmarshal(settingsData, &settings); err != nil {
		return nil, fmt.Errorf("failed to parse settings.json: %w", err)
	}

	// Extract AWS profile and region (env vars take precedence)
	profile := os.Getenv("AWS_PROFILE")
	if profile == "" && settings.AWS != nil {
		profile = settings.AWS.Profile
	}
	if profile == "" {
		profile = "default"
	}

	region := os.Getenv("AWS_REGION")
	if region == "" && settings.AWS != nil {
		region = settings.AWS.Region
	}
	if region == "" {
		region = "us-east-1"
	}

	// Pre-resolve AWS credentials via aws CLI
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	//nolint:gosec // G204: AWS CLI invocation with validated profile from settings.json or environment
	cmd := exec.CommandContext(ctx, "aws", "configure", "export-credentials", "--profile", profile)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("bedrock auth requires ~/.aws directory; run AWS CLI setup:\n  aws configure sso\n  aws sso login --profile %s\nError: %v\nOutput: %s", profile, err, string(output))
	}

	// Verify AccessKeyId is present in the output
	var creds struct {
		AccessKeyId string `json:"AccessKeyId"`
	}
	if err := json.Unmarshal(output, &creds); err != nil {
		return nil, fmt.Errorf("failed to parse AWS credentials: %w", err)
	}
	if creds.AccessKeyId == "" {
		return nil, fmt.Errorf("AWS credentials missing AccessKeyId for profile %s", profile)
	}

	// Parse full settings.json as generic map for merging
	var settingsMap map[string]interface{}
	if err := json.Unmarshal(settingsData, &settingsMap); err != nil {
		return nil, fmt.Errorf("failed to parse settings.json as map: %w", err)
	}

	return &BedrockAuth{
		AWSProfile: profile,
		AWSRegion:  region,
		Settings:   settingsMap,
	}, nil
}

// DetectMaxAuth detects and validates Claude Max authentication.
// It checks that ~/.claude/.credentials.json exists.
func DetectMaxAuth() (*MaxAuth, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	credPath := filepath.Join(homeDir, ".claude", ".credentials.json")
	if _, err := os.Stat(credPath); err != nil {
		if os.IsNotExist(err) {
			return nil, errors.New("max auth requires ~/.claude/.credentials.json; log in with Claude Desktop to generate credentials")
		}
		return nil, fmt.Errorf("failed to check credentials file: %w", err)
	}

	return &MaxAuth{
		CredentialsPath: credPath,
	}, nil
}

// DetectAPIKeyAuth detects and validates API key authentication.
// It checks that the specified environment variable is set and non-empty.
func DetectAPIKeyAuth(envVar string) (*APIKeyAuth, error) {
	value := os.Getenv(envVar)
	if value == "" {
		return nil, fmt.Errorf("provider requires %s; set it in your shell:\n  export %s=<value>", envVar, envVar)
	}

	return &APIKeyAuth{
		EnvVar: envVar,
		Value:  value,
	}, nil
}

// DetectLocalProvider detects and validates local provider authentication.
// It rewrites localhost/127.0.0.1/::1 URLs to host.docker.internal for use
// inside Docker containers.
//
// Reachability validation only runs when the local provider is actually used
// by the current command (not globally), as per review B5.
func DetectLocalProvider(baseURL string) (*LocalAuth, error) {
	if baseURL == "" {
		return nil, errors.New("base URL is required for local provider")
	}

	// Parse and rewrite the URL
	originalURL := baseURL
	rewrittenURL := rewriteLocalhostURL(baseURL)

	return &LocalAuth{
		BaseURL:     rewrittenURL,
		OriginalURL: originalURL,
	}, nil
}

// ValidateLocalProvider checks that a local provider is reachable.
// This should only be called when the local provider is actually used.
func ValidateLocalProvider(baseURL string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Extract host:port from URL
	// Simple parsing - assume format is http(s)://host:port
	url := strings.TrimPrefix(strings.TrimPrefix(baseURL, "https://"), "http://")
	if idx := strings.Index(url, "/"); idx != -1 {
		url = url[:idx]
	}

	// Try to connect
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", url)
	if err != nil {
		return fmt.Errorf("local provider at %s is not reachable: %w\nEnsure the service is running", baseURL, err)
	}
	_ = conn.Close()

	return nil
}

// rewriteLocalhostURL rewrites localhost/127.0.0.1/::1 to host.docker.internal.
func rewriteLocalhostURL(baseURL string) string {
	// Handle various localhost patterns
	patterns := []string{
		"://localhost:",
		"://localhost/",
		"://127.0.0.1:",
		"://127.0.0.1/",
		"://[::1]:",
		"://[::1]/",
	}

	for _, pattern := range patterns {
		if strings.Contains(baseURL, pattern) {
			// Replace the host part
			if strings.Contains(pattern, "localhost") {
				baseURL = strings.Replace(baseURL, "localhost", "host.docker.internal", 1)
			} else if strings.Contains(pattern, "127.0.0.1") {
				baseURL = strings.Replace(baseURL, "127.0.0.1", "host.docker.internal", 1)
			} else if strings.Contains(pattern, "[::1]") {
				baseURL = strings.Replace(baseURL, "[::1]", "host.docker.internal", 1)
			}
			return baseURL
		}
	}

	// Also handle URLs without explicit port or path
	if strings.HasSuffix(baseURL, "://localhost") {
		return strings.Replace(baseURL, "localhost", "host.docker.internal", 1)
	}
	if strings.HasSuffix(baseURL, "://127.0.0.1") {
		return strings.Replace(baseURL, "127.0.0.1", "host.docker.internal", 1)
	}
	if strings.HasSuffix(baseURL, "://[::1]") {
		return strings.Replace(baseURL, "[::1]", "host.docker.internal", 1)
	}

	return baseURL
}

// CollectAuthRequirements collects all authentication requirements for the given steps.
//
// Per review B3: Phase 2 only supports interactive mode, so cmd/code.go passes
// steps := []string{"interactive"} which resolves using pipeline.defaults and user.default.
// Pipeline step scanning is deferred to Phase 6.
//
// This function:
// - Resolves provider configuration for each step
// - Deduplicates providers across steps
// - Calls detection functions for each unique provider
// - Collects all errors (does not fail on first error)
// - Returns actionable error messages per review S1
func CollectAuthRequirements(cfg *config.Config, steps []string) (*AuthRequirements, []error) {
	requirements := &AuthRequirements{
		APIKeys: make(map[string]*APIKeyAuth),
		Local:   make(map[string]*LocalAuth),
	}
	var errs []error

	// For Phase 2, we only support "interactive" step
	// In the future, this will scan pipeline steps
	if len(steps) == 0 {
		steps = []string{"interactive"}
	}

	// Track unique providers to deduplicate
	providersSeen := make(map[string]bool)

	// For each step, resolve the provider
	for _, step := range steps {
		// Resolve the configuration for this step
		// For now, we use defaults since we only support "interactive"
		resolved, err := cfg.Resolve(step)
		if err != nil {
			errs = append(errs, fmt.Errorf("failed to resolve config for step %s: %w", step, err))
			continue
		}

		providerName := resolved.Provider
		if providersSeen[providerName] {
			continue
		}
		providersSeen[providerName] = true

		// Get provider config
		var providerCfg *config.ProviderConfig
		if cfg.User != nil && cfg.User.Providers != nil {
			providerCfg = cfg.User.Providers[providerName]
		}

		// Detect auth based on provider type
		switch {
		case strings.Contains(strings.ToLower(providerName), "bedrock"):
			auth, err := DetectBedrockAuth()
			if err != nil {
				errs = append(errs, fmt.Errorf("bedrock auth detection failed: %w", err))
			} else {
				requirements.Bedrock = auth
			}

		case strings.Contains(strings.ToLower(providerName), "max"):
			auth, err := DetectMaxAuth()
			if err != nil {
				errs = append(errs, fmt.Errorf("max auth detection failed: %w", err))
			} else {
				requirements.Max = auth
			}

		case strings.Contains(strings.ToLower(providerName), "local"):
			// For local providers, get the base URL from provider config
			if providerCfg == nil || providerCfg.BaseURL == nil {
				errs = append(errs, fmt.Errorf("local provider %s requires base_url in config", providerName))
				continue
			}
			auth, err := DetectLocalProvider(*providerCfg.BaseURL)
			if err != nil {
				errs = append(errs, fmt.Errorf("local provider detection failed: %w", err))
			} else {
				requirements.Local[providerName] = auth
			}

		default:
			// API key provider
			if providerCfg == nil || providerCfg.APIKeyEnv == nil {
				errs = append(errs, fmt.Errorf("provider %s requires api_key_env in config", providerName))
				continue
			}
			auth, err := DetectAPIKeyAuth(*providerCfg.APIKeyEnv)
			if err != nil {
				errs = append(errs, fmt.Errorf("provider %s auth detection failed: %w", providerName, err))
			} else {
				requirements.APIKeys[providerName] = auth
			}
		}
	}

	return requirements, errs
}
