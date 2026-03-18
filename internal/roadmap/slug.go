package roadmap

import (
	"regexp"
	"strings"
)

var (
	reNonAlphaNumHyphen  = regexp.MustCompile(`[^a-z0-9-]`)
	reConsecutiveHyphens = regexp.MustCompile(`-+`)
)

// GenerateSlug converts a title string into a URL-friendly slug.
//
// The function applies the following transformations:
// - Converts to lowercase
// - Replaces spaces and underscores with hyphens
// - Strips all characters not matching [a-z0-9-]
// - Collapses consecutive hyphens into a single hyphen
// - Trims leading and trailing hyphens
// - Truncates to 40 characters maximum
//
// Returns an empty string if the input is empty.
func GenerateSlug(title string) string {
	if title == "" {
		return ""
	}

	// Convert to lowercase
	slug := strings.ToLower(title)

	// Replace spaces and underscores with hyphens
	slug = strings.ReplaceAll(slug, " ", "-")
	slug = strings.ReplaceAll(slug, "_", "-")

	// Strip all chars not matching [a-z0-9-]
	slug = reNonAlphaNumHyphen.ReplaceAllString(slug, "")

	// Collapse consecutive hyphens
	slug = reConsecutiveHyphens.ReplaceAllString(slug, "-")

	// Trim leading and trailing hyphens
	slug = strings.Trim(slug, "-")

	// Truncate to 40 chars
	if len(slug) > 40 {
		slug = slug[:40]
	}

	// Trim trailing hyphen if truncation created one
	slug = strings.TrimRight(slug, "-")

	return slug
}
