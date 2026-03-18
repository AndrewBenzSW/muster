package docker

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

// ExtractAssets extracts embedded Docker assets to ~/.cache/muster/docker-assets/{hash}/
// versioned by content hash. Returns the extraction directory path.
// Skips extraction if the hash directory already exists (already extracted).
// Uses atomic operations: extract to temp dir, then rename. Uses filesystem lock for concurrency safety.
func ExtractAssets() (string, error) {
	// Compute SHA-256 hash of embedded FS
	hash, err := computeAssetsHash()
	if err != nil {
		return "", fmt.Errorf("failed to compute assets hash: %w", err)
	}

	// Determine cache directory
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get user home directory: %w", err)
	}
	cacheDir := filepath.Join(home, ".cache", "muster", "docker-assets")
	targetDir := filepath.Join(cacheDir, hash)

	// If final directory exists, skip extraction
	if _, err := os.Stat(targetDir); err == nil {
		return targetDir, nil
	}

	// Create cache directory if needed
	//nolint:gosec // G301: Directory permissions 0755 are appropriate for cache directory
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return "", fmt.Errorf("failed to create cache directory: %w", err)
	}

	// Acquire filesystem lock for concurrency safety
	lockPath := filepath.Join(cacheDir, hash+".lock")
	//nolint:gosec // G304: Lock file path is constructed from trusted hash, not user input
	lockFile, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		if os.IsExist(err) {
			// Another process is extracting; check if target directory exists
			if _, err := os.Stat(targetDir); err == nil {
				return targetDir, nil
			}
			return "", fmt.Errorf("extraction in progress by another process; try again in a moment")
		}
		return "", fmt.Errorf("failed to acquire lock: %w", err)
	}
	defer func() { _ = os.Remove(lockPath) }()
	defer func() { _ = lockFile.Close() }()

	// Extract to temp directory
	tempDir := filepath.Join(cacheDir, hash+".tmp")
	if err := os.RemoveAll(tempDir); err != nil {
		return "", fmt.Errorf("failed to clean temp directory: %w", err)
	}
	//nolint:gosec // G301: Directory permissions 0755 are appropriate for cache directory
	if err := os.MkdirAll(tempDir, 0o755); err != nil {
		return "", fmt.Errorf("failed to create temp directory: %w", err)
	}

	// Extract all files from embedded FS
	if err := extractFS(Assets, "docker", tempDir); err != nil {
		_ = os.RemoveAll(tempDir) // Clean up on failure
		return "", fmt.Errorf("failed to extract assets: %w", err)
	}

	// Atomic rename
	if err := os.Rename(tempDir, targetDir); err != nil {
		_ = os.RemoveAll(tempDir) // Clean up on failure
		return "", fmt.Errorf("failed to rename temp directory: %w", err)
	}

	return targetDir, nil
}

// computeAssetsHash computes a SHA-256 hash of the embedded FS content.
func computeAssetsHash() (string, error) {
	h := sha256.New()

	// Walk the embedded FS in sorted order for deterministic hash
	err := fs.WalkDir(Assets, "docker", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}

		// Hash the file path and content
		h.Write([]byte(path))

		f, err := Assets.Open(path)
		if err != nil {
			return fmt.Errorf("failed to open %s: %w", path, err)
		}
		defer func() { _ = f.Close() }()

		if _, err := io.Copy(h, f); err != nil {
			return fmt.Errorf("failed to read %s: %w", path, err)
		}

		return nil
	})
	if err != nil {
		return "", err
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}

// extractFS extracts all files from an embedded FS to a target directory.
func extractFS(fsys fs.FS, srcDir, targetDir string) error {
	return fs.WalkDir(fsys, srcDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Compute relative path
		relPath, err := filepath.Rel(srcDir, path)
		if err != nil {
			return fmt.Errorf("failed to compute relative path for %s: %w", path, err)
		}

		targetPath := filepath.Join(targetDir, relPath)

		if d.IsDir() {
			// Create directory
			//nolint:gosec // G301: Directory permissions 0755 are appropriate for asset directories
			return os.MkdirAll(targetPath, 0o755)
		}

		// Create parent directory
		//nolint:gosec // G301: Directory permissions 0755 are appropriate for asset directories
		if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
			return fmt.Errorf("failed to create parent directory for %s: %w", targetPath, err)
		}

		// Copy file
		src, err := fsys.Open(path)
		if err != nil {
			return fmt.Errorf("failed to open source file %s: %w", path, err)
		}
		defer func() { _ = src.Close() }()

		//nolint:gosec // G304: Target path is constructed from embedded assets, not user input
		dst, err := os.Create(targetPath)
		if err != nil {
			return fmt.Errorf("failed to create target file %s: %w", targetPath, err)
		}
		defer func() { _ = dst.Close() }()

		if _, err := io.Copy(dst, src); err != nil {
			return fmt.Errorf("failed to copy %s: %w", path, err)
		}

		// Preserve executable bit if present
		if info, err := d.Info(); err == nil {
			if err := dst.Chmod(info.Mode()); err != nil {
				return fmt.Errorf("failed to set permissions for %s: %w", targetPath, err)
			}
		}

		return nil
	})
}
