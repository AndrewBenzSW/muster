package docker

import (
	"io/fs"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDockerFS_PlaceholderExists(t *testing.T) {
	content, err := fs.ReadFile(Assets, "docker/README.md")
	require.NoError(t, err)
	assert.NotEmpty(t, content)
	assert.True(t, strings.Contains(string(content), "Phase 2"), "Expected content to mention 'Phase 2'")
}

func TestDockerFS_CanWalk(t *testing.T) {
	fileCount := 0
	foundDockerDir := false
	err := fs.WalkDir(Assets, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			fileCount++
		}
		// Verify expected directory structure
		if strings.HasPrefix(path, "docker/") {
			foundDockerDir = true
		}
		return nil
	})
	require.NoError(t, err)
	assert.GreaterOrEqual(t, fileCount, 1, "Expected at least one file in embedded FS")
	assert.True(t, foundDockerDir, "Expected to find docker/ directory in embedded FS")
}
