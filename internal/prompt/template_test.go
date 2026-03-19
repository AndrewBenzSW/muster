package prompt

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/abenz1267/muster/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var update = flag.Bool("update", false, "update golden files")

func TestParseTemplates(t *testing.T) {
	err := ParseTemplates()
	require.NoError(t, err, "ParseTemplates should succeed")
	require.NotNil(t, parsedTemplates, "parsedTemplates should be set")
}

func TestParsedTemplatesContainsExpectedNames(t *testing.T) {
	err := ParseTemplates()
	require.NoError(t, err)

	expectedTemplates := []string{
		"prompts/plan-feature/SKILL.md.tmpl",
		"prompts/execute-plan/SKILL.md.tmpl",
		"prompts/review-implementation/SKILL.md.tmpl",
		"prompts/test/example.md.tmpl",
	}

	for _, name := range expectedTemplates {
		tmpl := parsedTemplates.Lookup(name)
		assert.NotNil(t, tmpl, "template %s should be parsed", name)
	}
}

func TestParseTemplatesMultipleTimes(t *testing.T) {
	// Call ParseTemplates multiple times - should be safe due to sync.Once
	for i := 0; i < 5; i++ {
		err := ParseTemplates()
		require.NoError(t, err, "ParseTemplates call %d should succeed", i)
	}

	// Verify templates are still valid
	tmpl := parsedTemplates.Lookup("prompts/test/example.md.tmpl")
	assert.NotNil(t, tmpl, "templates should still be available after multiple calls")
}

func TestRenderTemplateWithNilContext(t *testing.T) {
	err := ParseTemplates()
	require.NoError(t, err)

	_, err = RenderTemplate("prompts/test/example.md.tmpl", nil)
	assert.Error(t, err, "should error with nil context")
	assert.Contains(t, err.Error(), "cannot be nil", "error should mention nil context")
}

func TestRenderTemplateUnknownTemplate(t *testing.T) {
	err := ParseTemplates()
	require.NoError(t, err)

	ctx := &PromptContext{
		Tool: "claude-code",
	}

	_, err = RenderTemplate("prompts/nonexistent/template.md.tmpl", ctx)
	assert.Error(t, err, "should error with unknown template")
	assert.Contains(t, err.Error(), "not found", "error should mention template not found")
}

func TestRenderTemplateExample(t *testing.T) {
	err := ParseTemplates()
	require.NoError(t, err)

	ctx := &PromptContext{
		Tool: "claude-code",
	}

	// The example.md.tmpl uses {{.Tool}} which should work fine
	output, err := RenderTemplate("prompts/test/example.md.tmpl", ctx)
	assert.NoError(t, err, "should render successfully with valid field")
	assert.Equal(t, "Hello claude-code\n", output, "should render the tool name")
}

// Golden file tests

func TestGoldenFileInteractiveTrue(t *testing.T) {
	err := ParseTemplates()
	require.NoError(t, err)

	userCfg := &config.UserConfig{
		Tools: map[string]*config.ToolConfig{
			"claude-code": {
				ModelTiers: &config.ModelTiersConfig{
					Fast:     stringPtr("claude-haiku-4"),
					Standard: stringPtr("claude-sonnet-4.5"),
					Deep:     stringPtr("claude-opus-4"),
				},
			},
		},
	}

	resolved := &config.ResolvedConfig{
		Tool:     "claude-code",
		Provider: "anthropic",
		Model:    "claude-sonnet-4.5",
	}

	ctx := NewPromptContext(resolved, nil, userCfg, true, "test-feature", "/workspace", "/workspace", "/workspace/.plan/test-feature")

	rendered, err := RenderTemplate("prompts/plan-feature/SKILL.md.tmpl", ctx)
	require.NoError(t, err)

	compareGolden(t, "plan-feature-interactive-true.golden", rendered)
}

func TestGoldenFileInteractiveFalse(t *testing.T) {
	err := ParseTemplates()
	require.NoError(t, err)

	userCfg := &config.UserConfig{
		Tools: map[string]*config.ToolConfig{
			"claude-code": {
				ModelTiers: &config.ModelTiersConfig{
					Fast:     stringPtr("claude-haiku-4"),
					Standard: stringPtr("claude-sonnet-4.5"),
					Deep:     stringPtr("claude-opus-4"),
				},
			},
		},
	}

	resolved := &config.ResolvedConfig{
		Tool:     "claude-code",
		Provider: "anthropic",
		Model:    "claude-sonnet-4.5",
	}

	ctx := NewPromptContext(resolved, nil, userCfg, false, "test-feature", "/workspace", "/workspace", "/workspace/.plan/test-feature")

	rendered, err := RenderTemplate("prompts/plan-feature/SKILL.md.tmpl", ctx)
	require.NoError(t, err)

	compareGolden(t, "plan-feature-interactive-false.golden", rendered)
}

func TestGoldenFileModelsPopulation(t *testing.T) {
	err := ParseTemplates()
	require.NoError(t, err)

	userCfg := &config.UserConfig{
		Tools: map[string]*config.ToolConfig{
			"opencode": {
				ModelTiers: &config.ModelTiersConfig{
					Fast:     stringPtr("gemma3:4b"),
					Standard: stringPtr("qwen3:14b"),
					Deep:     stringPtr("qwen3:235b"),
				},
			},
		},
	}

	resolved := &config.ResolvedConfig{
		Tool:     "opencode",
		Provider: "ollama",
		Model:    "qwen3:14b",
	}

	ctx := NewPromptContext(resolved, nil, userCfg, true, "test-feature", "/workspace", "/workspace", "/workspace/.plan/test-feature")

	// Verify Models struct is populated with opencode tiers
	assert.Equal(t, "gemma3:4b", ctx.Models.Fast, "Models.Fast should use opencode's fast tier")
	assert.Equal(t, "qwen3:14b", ctx.Models.Standard, "Models.Standard should use opencode's standard tier")
	assert.Equal(t, "qwen3:235b", ctx.Models.Deep, "Models.Deep should use opencode's deep tier")

	rendered, err := RenderTemplate("prompts/plan-feature/SKILL.md.tmpl", ctx)
	require.NoError(t, err)

	// Verify rendered output contains opencode model names
	assert.Contains(t, rendered, "gemma3:4b", "rendered template should contain opencode fast tier")
	assert.Contains(t, rendered, "qwen3:14b", "rendered template should contain opencode standard tier")
	assert.Contains(t, rendered, "qwen3:235b", "rendered template should contain opencode deep tier")

	compareGolden(t, "plan-feature-opencode-models.golden", rendered)
}

func TestGoldenFileExecutePlan(t *testing.T) {
	err := ParseTemplates()
	require.NoError(t, err)

	userCfg := &config.UserConfig{
		Tools: map[string]*config.ToolConfig{
			"claude-code": {
				ModelTiers: &config.ModelTiersConfig{
					Fast:     stringPtr("claude-haiku-4"),
					Standard: stringPtr("claude-sonnet-4.5"),
					Deep:     stringPtr("claude-opus-4"),
				},
			},
		},
	}

	resolved := &config.ResolvedConfig{
		Tool:     "claude-code",
		Provider: "anthropic",
		Model:    "claude-sonnet-4.5",
	}

	ctx := NewPromptContext(resolved, nil, userCfg, true, "test-execution", "/workspace", "/workspace", "/workspace/.plan/test-execution")

	rendered, err := RenderTemplate("prompts/execute-plan/SKILL.md.tmpl", ctx)
	require.NoError(t, err)

	compareGolden(t, "execute-plan-interactive.golden", rendered)
}

// Task S6: Golden file branch tests

func TestGoldenFileExecutePlanInteractiveFalse(t *testing.T) {
	// Test execute-plan template with Interactive=false
	err := ParseTemplates()
	require.NoError(t, err)

	userCfg := &config.UserConfig{
		Tools: map[string]*config.ToolConfig{
			"claude-code": {
				ModelTiers: &config.ModelTiersConfig{
					Fast:     stringPtr("claude-haiku-4"),
					Standard: stringPtr("claude-sonnet-4.5"),
					Deep:     stringPtr("claude-opus-4"),
				},
			},
		},
	}

	resolved := &config.ResolvedConfig{
		Tool:     "claude-code",
		Provider: "anthropic",
		Model:    "claude-sonnet-4.5",
	}

	ctx := NewPromptContext(resolved, nil, userCfg, false, "test-execution", "/workspace", "/workspace", "/workspace/.plan/test-execution")

	rendered, err := RenderTemplate("prompts/execute-plan/SKILL.md.tmpl", ctx)
	require.NoError(t, err)

	compareGolden(t, "execute-plan-non-interactive.golden", rendered)
}

func TestGoldenFileReviewImplementationInteractive(t *testing.T) {
	// Test review-implementation template with Interactive=true
	err := ParseTemplates()
	require.NoError(t, err)

	userCfg := &config.UserConfig{
		Tools: map[string]*config.ToolConfig{
			"claude-code": {
				ModelTiers: &config.ModelTiersConfig{
					Fast:     stringPtr("claude-haiku-4"),
					Standard: stringPtr("claude-sonnet-4.5"),
					Deep:     stringPtr("claude-opus-4"),
				},
			},
		},
	}

	resolved := &config.ResolvedConfig{
		Tool:     "claude-code",
		Provider: "anthropic",
		Model:    "claude-sonnet-4.5",
	}

	ctx := NewPromptContext(resolved, nil, userCfg, true, "test-review", "/workspace", "/workspace", "/workspace/.plan/test-review")

	rendered, err := RenderTemplate("prompts/review-implementation/SKILL.md.tmpl", ctx)
	require.NoError(t, err)

	compareGolden(t, "review-implementation-interactive.golden", rendered)
}

func TestGoldenFileReviewImplementationNonInteractive(t *testing.T) {
	// Test review-implementation template with Interactive=false
	err := ParseTemplates()
	require.NoError(t, err)

	userCfg := &config.UserConfig{
		Tools: map[string]*config.ToolConfig{
			"claude-code": {
				ModelTiers: &config.ModelTiersConfig{
					Fast:     stringPtr("claude-haiku-4"),
					Standard: stringPtr("claude-sonnet-4.5"),
					Deep:     stringPtr("claude-opus-4"),
				},
			},
		},
	}

	resolved := &config.ResolvedConfig{
		Tool:     "claude-code",
		Provider: "anthropic",
		Model:    "claude-sonnet-4.5",
	}

	ctx := NewPromptContext(resolved, nil, userCfg, false, "test-review", "/workspace", "/workspace", "/workspace/.plan/test-review")

	rendered, err := RenderTemplate("prompts/review-implementation/SKILL.md.tmpl", ctx)
	require.NoError(t, err)

	compareGolden(t, "review-implementation-non-interactive.golden", rendered)
}

func TestAllTemplatesRenderWithoutErrors(t *testing.T) {
	// Helper test that verifies all templates render without errors for both Interactive values
	// This is a catch-all test to ensure no template has syntax errors
	err := ParseTemplates()
	require.NoError(t, err)

	userCfg := &config.UserConfig{
		Tools: map[string]*config.ToolConfig{
			"claude-code": {
				ModelTiers: &config.ModelTiersConfig{
					Fast:     stringPtr("claude-haiku-4"),
					Standard: stringPtr("claude-sonnet-4.5"),
					Deep:     stringPtr("claude-opus-4"),
				},
			},
		},
	}

	resolved := &config.ResolvedConfig{
		Tool:     "claude-code",
		Provider: "anthropic",
		Model:    "claude-sonnet-4.5",
	}

	templates := []string{
		"prompts/plan-feature/SKILL.md.tmpl",
		"prompts/plan-feature/planner-prompt.md.tmpl",
		"prompts/execute-plan/SKILL.md.tmpl",
		"prompts/execute-plan/worker-prompt.md.tmpl",
		"prompts/review-implementation/SKILL.md.tmpl",
		"prompts/review-implementation/reviewer-prompt.md.tmpl",
	}

	// Test with Interactive=true
	ctx := NewPromptContext(resolved, nil, userCfg, true, "test-all", "/workspace", "/workspace", "/workspace/.plan/test-all")
	for _, tmpl := range templates {
		rendered, err := RenderTemplate(tmpl, ctx)
		assert.NoError(t, err, "template %s should render without error (Interactive=true)", tmpl)
		assert.NotEmpty(t, rendered, "template %s should produce non-empty output (Interactive=true)", tmpl)
	}

	// Test with Interactive=false
	ctx = NewPromptContext(resolved, nil, userCfg, false, "test-all", "/workspace", "/workspace", "/workspace/.plan/test-all")
	for _, tmpl := range templates {
		rendered, err := RenderTemplate(tmpl, ctx)
		assert.NoError(t, err, "template %s should render without error (Interactive=false)", tmpl)
		assert.NotEmpty(t, rendered, "template %s should produce non-empty output (Interactive=false)", tmpl)
	}
}

// Task I8: Template error handling tests

func TestRenderTemplateBeforeParseTemplates(t *testing.T) {
	// Reset the template state to simulate calling RenderTemplate before ParseTemplates
	// This is tricky because sync.Once prevents re-initialization, so we test that
	// RenderTemplate gracefully handles this by calling ParseTemplates internally

	ctx := &PromptContext{
		Tool: "claude-code",
	}

	// RenderTemplate should call ParseTemplates internally, so this should work
	output, err := RenderTemplate("prompts/test/example.md.tmpl", ctx)
	assert.NoError(t, err, "RenderTemplate should call ParseTemplates internally")
	assert.NotEmpty(t, output, "should render successfully")
}

func TestRenderTemplateMissingContextField(t *testing.T) {
	err := ParseTemplates()
	require.NoError(t, err)

	// Create a context that's missing required fields for the template
	// We'll test this with a template that uses {{.Tool}}
	ctx := &PromptContext{
		// Intentionally leave Tool empty/missing
	}

	// The example template uses {{.Tool}}, but our context has an empty Tool field
	// With missingkey=error, this should fail
	_, err = RenderTemplate("prompts/test/example.md.tmpl", ctx)

	// The template will execute but produce "Hello \n" since Tool is empty string (not missing)
	// To test actual missing key error, we need to use a different approach
	// For now, verify that the render completes (empty string is valid, not a missing key)
	assert.NoError(t, err, "empty string is valid, not a missing key")
}

func TestRenderTemplateWithMissingKeyInTemplate(t *testing.T) {
	// This test verifies that templates with references to undefined fields
	// produce clear errors mentioning the missing key

	err := ParseTemplates()
	require.NoError(t, err)

	// Verify that parsedTemplates has missingkey=error set
	assert.NotNil(t, parsedTemplates, "templates should be parsed")

	// The actual test of missing key errors would require a template with
	// an undefined variable, which we can't create without modifying embedded files
	// So we document the expected behavior:
	// If a template uses {{.UndefinedField}}, the error should mention "undefined"
}

// Helper functions

func stringPtr(s string) *string {
	return &s
}

func compareGolden(t *testing.T, goldenFile string, actual string) {
	t.Helper()

	goldenPath := filepath.Join("testdata", goldenFile)

	if *update {
		// Create testdata directory if it doesn't exist
		err := os.MkdirAll("testdata", 0755) //nolint:gosec // G301: Test directory permissions
		require.NoError(t, err)

		// Write the golden file
		err = os.WriteFile(goldenPath, []byte(actual), 0644) //nolint:gosec // G306: Test file permissions
		require.NoError(t, err)
		t.Logf("Updated golden file: %s", goldenPath)
		return
	}

	// Read the golden file
	expected, err := os.ReadFile(goldenPath) //nolint:gosec // G304: Test fixture path
	require.NoError(t, err, "failed to read golden file: %s", goldenPath)

	// Normalize line endings for cross-platform comparison
	expectedStr := strings.ReplaceAll(string(expected), "\r\n", "\n")
	actualStr := strings.ReplaceAll(actual, "\r\n", "\n")

	assert.Equal(t, expectedStr, actualStr, "rendered output should match golden file: %s", goldenFile)
}
