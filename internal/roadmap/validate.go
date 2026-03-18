package roadmap

import (
	"fmt"
	"strings"
)

// Validate validates the Roadmap and returns an error if validation fails.
// It checks:
// - Required fields (slug, title, priority, status, context) are non-empty
// - Slugs are unique across all items
// - Priority and Status enum values are valid
// Returns nil if the roadmap is valid (including empty roadmaps).
func (r *Roadmap) Validate() error {
	if r == nil {
		return nil
	}

	// Empty roadmap is valid
	if len(r.Items) == 0 {
		return nil
	}

	// Track slugs to check for duplicates
	seenSlugs := make(map[string]bool)

	for i, item := range r.Items {
		// Check required fields
		if strings.TrimSpace(item.Slug) == "" {
			return fmt.Errorf("item at index %d: slug is required", i)
		}
		if strings.TrimSpace(item.Title) == "" {
			return fmt.Errorf("item at index %d (%s): title is required", i, item.Slug)
		}
		if strings.TrimSpace(string(item.Priority)) == "" {
			return fmt.Errorf("item at index %d (%s): priority is required", i, item.Slug)
		}
		if strings.TrimSpace(string(item.Status)) == "" {
			return fmt.Errorf("item at index %d (%s): status is required", i, item.Slug)
		}
		if strings.TrimSpace(item.Context) == "" {
			return fmt.Errorf("item at index %d (%s): context is required", i, item.Slug)
		}

		// Check for duplicate slugs
		if seenSlugs[item.Slug] {
			return fmt.Errorf("duplicate slug: %s", item.Slug)
		}
		seenSlugs[item.Slug] = true

		// Validate enum values
		if !item.Priority.IsValid() {
			return fmt.Errorf("item at index %d (%s): invalid priority %q", i, item.Slug, item.Priority)
		}
		if !item.Status.IsValid() {
			return fmt.Errorf("item at index %d (%s): invalid status %q", i, item.Slug, item.Status)
		}
	}

	return nil
}

// FindBySlug finds a roadmap item by its slug.
// Returns nil if no item with the given slug is found.
func (r *Roadmap) FindBySlug(slug string) *RoadmapItem {
	if r == nil {
		return nil
	}

	for i := range r.Items {
		if r.Items[i].Slug == slug {
			return &r.Items[i]
		}
	}

	return nil
}

// AddItem adds a new item to the roadmap after validating it.
// It checks:
// - Required fields (slug, title, priority, status, context) are non-empty
// - Priority and Status enum values are valid via IsValid()
// - Slug is not already in use (no duplicates)
// Returns a descriptive error if validation fails.
func (r *Roadmap) AddItem(item RoadmapItem) error {
	if r == nil {
		return fmt.Errorf("cannot add item to nil roadmap")
	}

	// Validate required fields
	if strings.TrimSpace(item.Slug) == "" {
		return fmt.Errorf("slug is required")
	}
	if strings.TrimSpace(item.Title) == "" {
		return fmt.Errorf("title is required")
	}
	if strings.TrimSpace(string(item.Priority)) == "" {
		return fmt.Errorf("priority is required")
	}
	if strings.TrimSpace(string(item.Status)) == "" {
		return fmt.Errorf("status is required")
	}
	if strings.TrimSpace(item.Context) == "" {
		return fmt.Errorf("context is required")
	}

	// Validate enum values
	if !item.Priority.IsValid() {
		return fmt.Errorf("invalid priority: %s", item.Priority)
	}
	if !item.Status.IsValid() {
		return fmt.Errorf("invalid status: %s", item.Status)
	}

	// Check for duplicate slug
	if r.FindBySlug(item.Slug) != nil {
		return fmt.Errorf("duplicate slug: %s", item.Slug)
	}

	// Add the item
	r.Items = append(r.Items, item)

	return nil
}
