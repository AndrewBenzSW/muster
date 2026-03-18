package docker

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractAssets(t *testing.T) {
	// First extraction creates directory
	dir1, err := ExtractAssets()
	require.NoError(t, err, "First extraction should succeed")
	require.DirExists(t, dir1, "Extraction directory should exist")

	// Verify directory structure
	assert.DirExists(t, dir1, "Asset directory should exist")
	assert.FileExists(t, filepath.Join(dir1, "docker-compose.yml"), "docker-compose.yml should exist")
	assert.FileExists(t, filepath.Join(dir1, "agent.Dockerfile"), "agent.Dockerfile should exist")
	assert.FileExists(t, filepath.Join(dir1, "proxy.Dockerfile"), "proxy.Dockerfile should exist")
	assert.FileExists(t, filepath.Join(dir1, "entrypoint.sh"), "entrypoint.sh should exist")
	assert.FileExists(t, filepath.Join(dir1, "squid.conf"), "squid.conf should exist")
	assert.FileExists(t, filepath.Join(dir1, "allowed-domains.txt"), "allowed-domains.txt should exist")
	assert.FileExists(t, filepath.Join(dir1, "settings.json"), "settings.json should exist")
	assert.FileExists(t, filepath.Join(dir1, "settings.max.json"), "settings.max.json should exist")

	// Second extraction skips (returns same directory)
	dir2, err := ExtractAssets()
	require.NoError(t, err, "Second extraction should succeed")
	assert.Equal(t, dir1, dir2, "Second extraction should return same directory")
}

func TestExtractAssets_AtomicRename(t *testing.T) {
	// Extract once to get the hash-based directory
	dir, err := ExtractAssets()
	require.NoError(t, err, "Extraction should succeed")

	// Verify temp directory was cleaned up
	tempDir := dir + ".tmp"
	assert.NoDirExists(t, tempDir, "Temp directory should be removed after successful extraction")

	// Verify lock file was cleaned up
	lockFile := dir + ".lock"
	assert.NoFileExists(t, lockFile, "Lock file should be removed after extraction")
}

func TestComputeAssetsHash(t *testing.T) {
	// Test that hash computation is deterministic
	hash1, err := computeAssetsHash()
	require.NoError(t, err, "First hash computation should succeed")
	require.NotEmpty(t, hash1, "Hash should not be empty")

	hash2, err := computeAssetsHash()
	require.NoError(t, err, "Second hash computation should succeed")
	assert.Equal(t, hash1, hash2, "Hash should be deterministic")

	// Verify hash is hex-encoded SHA-256 (64 characters)
	assert.Len(t, hash1, 64, "SHA-256 hash should be 64 hex characters")
}

func TestExtractFS(t *testing.T) {
	tempDir := t.TempDir()

	// Extract the embedded FS to temp directory
	err := extractFS(Assets, "docker", tempDir)
	require.NoError(t, err, "extractFS should succeed")

	// Verify files were extracted
	assert.FileExists(t, filepath.Join(tempDir, "docker-compose.yml"), "docker-compose.yml should be extracted")
	assert.FileExists(t, filepath.Join(tempDir, "agent.Dockerfile"), "agent.Dockerfile should be extracted")
	assert.FileExists(t, filepath.Join(tempDir, "proxy.Dockerfile"), "proxy.Dockerfile should be extracted")

	// Verify file contents are preserved
	//nolint:gosec // G304: Reading test fixture file from temp directory
	content, err := os.ReadFile(filepath.Join(tempDir, "docker-compose.yml"))
	require.NoError(t, err, "Should read extracted file")
	assert.Contains(t, string(content), "version:", "File content should be preserved")
}

// Integration test: verify extracted files match embedded files
func TestExtractAssets_ContentMatches(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Extract assets
	dir, err := ExtractAssets()
	require.NoError(t, err, "Extraction should succeed")

	// Walk embedded FS and compare with extracted files
	err = fs.WalkDir(Assets, "docker", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}

		// Read embedded file
		embeddedContent, err := fs.ReadFile(Assets, path)
		require.NoError(t, err, "Should read embedded file %s", path)

		// Compute extracted file path
		relPath, err := filepath.Rel("docker", path)
		require.NoError(t, err, "Should compute relative path")
		extractedPath := filepath.Join(dir, relPath)

		// Read extracted file
		//nolint:gosec // G304: Reading test fixture file from controlled extraction directory
		extractedContent, err := os.ReadFile(extractedPath)
		require.NoError(t, err, "Should read extracted file %s", extractedPath)

		// Compare contents
		assert.Equal(t, embeddedContent, extractedContent,
			"Content mismatch for %s", path)

		return nil
	})
	require.NoError(t, err, "Should walk embedded FS successfully")
}

func TestExtractAssets_ConcurrencySafe(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping concurrency test in short mode")
	}

	// Try extracting from multiple goroutines
	const numGoroutines = 5
	results := make(chan struct {
		dir string
		err error
	}, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			dir, err := ExtractAssets()
			results <- struct {
				dir string
				err error
			}{dir, err}
		}()
	}

	// Collect results
	var dirs []string
	for i := 0; i < numGoroutines; i++ {
		result := <-results
		require.NoError(t, result.err, "Concurrent extraction should not error")
		dirs = append(dirs, result.dir)
	}

	// All should return the same directory
	for i := 1; i < len(dirs); i++ {
		assert.Equal(t, dirs[0], dirs[i], "All extractions should return same directory")
	}
}
