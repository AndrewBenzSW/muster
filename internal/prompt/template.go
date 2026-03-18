package prompt

import (
	"bytes"
	"fmt"
	"io/fs"
	"path/filepath"
	"sync"
	"text/template"
)

// parsedTemplates holds the parsed template tree
var parsedTemplates *template.Template

// parseOnce ensures templates are parsed only once
var parseOnce sync.Once

// parseErr stores any error encountered during template parsing
var parseErr error

// ParseTemplates parses all embedded templates from the prompts directory.
// This function uses sync.Once to ensure parsing happens exactly once,
// making it safe to call multiple times.
func ParseTemplates() error {
	parseOnce.Do(func() {
		parsedTemplates = template.New("").Option("missingkey=error")

		// Walk the embedded FS and parse each .tmpl file
		// We need to preserve the full path as the template name
		parseErr = fs.WalkDir(Prompts, "prompts", func(path string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}

			// Skip directories and non-.tmpl files
			if d.IsDir() || filepath.Ext(path) != ".tmpl" {
				return nil
			}

			// Read the file content
			content, readErr := fs.ReadFile(Prompts, path)
			if readErr != nil {
				return fmt.Errorf("failed to read %s: %w", path, readErr)
			}

			// Parse and add to our template tree with the full path as the name
			_, parseErr := parsedTemplates.New(path).Parse(string(content))
			if parseErr != nil {
				return fmt.Errorf("failed to parse %s: %w", path, parseErr)
			}

			return nil
		})
	})
	return parseErr
}

// RenderTemplate renders a template by name with the given context.
// The template name should be the full path within the embedded filesystem,
// e.g., "prompts/plan-feature/SKILL.md.tmpl".
func RenderTemplate(name string, ctx *PromptContext) (string, error) {
	if ctx == nil {
		return "", fmt.Errorf("RenderTemplate: context cannot be nil")
	}

	// Ensure templates are parsed
	if err := ParseTemplates(); err != nil {
		return "", fmt.Errorf("failed to parse templates: %w", err)
	}

	// Look up the template
	tmpl := parsedTemplates.Lookup(name)
	if tmpl == nil {
		return "", fmt.Errorf("template not found: %s", name)
	}

	// Execute the template
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, ctx); err != nil {
		return "", fmt.Errorf("failed to render template %s: %w: %w", name, ErrTemplateRender, err)
	}

	return buf.String(), nil
}
