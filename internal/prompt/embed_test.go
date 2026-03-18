package prompt

import (
	"io/fs"
	"strings"
	"testing"
	"text/template"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPromptFS_PlaceholderExists(t *testing.T) {
	content, err := fs.ReadFile(Prompts, "prompts/test/example.md.tmpl")
	require.NoError(t, err)
	assert.NotEmpty(t, content)
	assert.Equal(t, "Hello {{.Tool}}\n", string(content))
}

func TestPromptFS_CanWalk(t *testing.T) {
	fileCount := 0
	foundExpectedFile := false
	err := fs.WalkDir(Prompts, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			fileCount++
			// Verify expected file pattern exists
			if strings.HasSuffix(path, ".md.tmpl") {
				foundExpectedFile = true
			}
		}
		return nil
	})
	require.NoError(t, err)
	assert.GreaterOrEqual(t, fileCount, 14, "Expected at least 14 .tmpl files from Phase 0.5")
	assert.True(t, foundExpectedFile, "Expected to find at least one .md.tmpl file in embedded FS")
}

func TestPromptFS_CanParseTemplate(t *testing.T) {
	content, err := fs.ReadFile(Prompts, "prompts/test/example.md.tmpl")
	require.NoError(t, err)

	tmpl, err := template.New("test").Parse(string(content))
	require.NoError(t, err)
	assert.NotNil(t, tmpl)

	// Verify template can execute
	var buf strings.Builder
	err = tmpl.Execute(&buf, map[string]string{"Tool": "World"})
	require.NoError(t, err)
	assert.Equal(t, "Hello World\n", buf.String())
}

func TestPromptFS_ExpectedPathsExist(t *testing.T) {
	// Verify expected paths from Phase 0.5 are embedded
	expectedPaths := []string{
		"prompts/plan-feature/SKILL.md.tmpl",
		"prompts/execute-plan/SKILL.md.tmpl",
		"prompts/review-implementation/SKILL.md.tmpl",
	}

	for _, path := range expectedPaths {
		content, err := fs.ReadFile(Prompts, path)
		require.NoError(t, err, "Expected path %s to exist in embedded FS", path)
		assert.NotEmpty(t, content, "Expected path %s to have content", path)
	}
}

func TestPromptFS_ReadDirCount(t *testing.T) {
	// Verify fs.ReadDir returns expected count
	// We expect at least 14 .tmpl files from Phase 0.5
	tmplCount := 0

	err := fs.WalkDir(Prompts, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(path, ".tmpl") {
			tmplCount++
		}
		return nil
	})

	require.NoError(t, err)
	assert.GreaterOrEqual(t, tmplCount, 14, "Expected at least 14 .tmpl files from Phase 0.5")
}
