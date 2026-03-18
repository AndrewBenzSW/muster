package docker

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

// providerDomains maps provider types to their required domains.
var providerDomains = map[string][]string{
	"bedrock": {
		".amazonaws.com",
		".aws.amazon.com",
	},
	"max": {
		".anthropic.com",
		".claude.ai",
	},
	"anthropic": {
		".anthropic.com",
		".claude.ai",
	},
	"openrouter": {
		".openrouter.ai",
	},
	"local": {
		"host.docker.internal",
	},
}

// domainPattern validates domain names and wildcards.
// Allows:
// - example.com (exact domain)
// - .example.com (wildcard subdomain)
// - *.example.com (explicit wildcard)
var domainPattern = regexp.MustCompile(`^(\*\.)?\.?[a-zA-Z0-9]([a-zA-Z0-9-]*[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9-]*[a-zA-Z0-9])?)*$`)

// GenerateAllowlist merges the base embedded allowed-domains.txt with
// provider-specific domains and project-configured domains.
// Returns the path to the written merged file.
//
// Per review S5: validates each domain (non-empty, valid hostname/wildcard),
// deduplicates before writing.
func GenerateAllowlist(assetDir string, auth *AuthRequirements, devAgentRaw interface{}) (string, error) {
	// Read base allowlist from embedded assets
	baseData, err := Assets.ReadFile("docker/allowed-domains.txt")
	if err != nil {
		return "", fmt.Errorf("failed to read embedded allowlist: %w", err)
	}

	// Parse base domains
	domains := parseDomains(string(baseData))

	// Add provider-specific domains
	if auth != nil {
		if auth.Bedrock != nil {
			domains = append(domains, providerDomains["bedrock"]...)
		}
		if auth.Max != nil {
			domains = append(domains, providerDomains["max"]...)
		}
		for providerName := range auth.APIKeys {
			lowerName := strings.ToLower(providerName)
			if providerDoms, ok := providerDomains[lowerName]; ok {
				domains = append(domains, providerDoms...)
			}
		}
		for range auth.Local {
			domains = append(domains, providerDomains["local"]...)
		}
	}

	// Add project-configured domains
	if devAgentRaw != nil {
		// Type assert to get the config (avoiding import cycle by using interface{})
		if devAgent, ok := devAgentRaw.(struct {
			AllowedDomains []string
			Env            map[string]string
			Volumes        []string
			Networks       []string
		}); ok {
			domains = append(domains, devAgent.AllowedDomains...)
		}
	}

	// Validate and deduplicate domains
	validDomains := make([]string, 0, len(domains))
	seen := make(map[string]bool)

	for _, domain := range domains {
		domain = strings.TrimSpace(domain)

		// Skip empty strings and comments
		if domain == "" || strings.HasPrefix(domain, "#") {
			continue
		}

		// Validate domain format
		if !isValidDomain(domain) {
			// Warning but continue - don't fail the entire generation
			fmt.Fprintf(os.Stderr, "Warning: skipping invalid domain in allowlist: %q\n", domain)
			continue
		}

		// Deduplicate
		if seen[domain] {
			continue
		}
		seen[domain] = true
		validDomains = append(validDomains, domain)
	}

	// Write merged allowlist
	allowlistPath := fmt.Sprintf("%s/allowed-domains.txt", assetDir)
	content := "# Merged domain allowlist for muster proxy\n"
	content += "# Generated at runtime - do not edit manually\n\n"
	content += strings.Join(validDomains, "\n") + "\n"

	//nolint:gosec // G306: File permissions 0644 are appropriate for non-sensitive domain allowlist
	if err := os.WriteFile(allowlistPath, []byte(content), 0644); err != nil {
		return "", fmt.Errorf("failed to write allowlist: %w", err)
	}

	return allowlistPath, nil
}

// parseDomains parses a domain list file, skipping comments and empty lines.
func parseDomains(content string) []string {
	var domains []string
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		domains = append(domains, line)
	}
	return domains
}

// isValidDomain checks if a domain string is valid.
// Allows exact domains, wildcard subdomains (.example.com), and explicit wildcards (*.example.com).
func isValidDomain(domain string) bool {
	// Empty strings are invalid
	if domain == "" {
		return false
	}

	// Special case: host.docker.internal is valid
	if domain == "host.docker.internal" {
		return true
	}

	// Check against regex pattern
	if !domainPattern.MatchString(domain) {
		return false
	}

	// Additional check: reject TLDs (e.g., ".com", "com")
	// Must have at least one dot and something before the TLD
	trimmed := strings.TrimPrefix(strings.TrimPrefix(domain, "*."), ".")
	parts := strings.Split(trimmed, ".")
	return len(parts) >= 2
}
