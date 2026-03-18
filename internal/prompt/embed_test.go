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
	assert.Equal(t, "Hello {{.Name}}\n", string(content))
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
	assert.GreaterOrEqual(t, fileCount, 1, "Expected at least one file in embedded FS")
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
	err = tmpl.Execute(&buf, map[string]string{"Name": "World"})
	require.NoError(t, err)
	assert.Equal(t, "Hello World\n", buf.String())
}
