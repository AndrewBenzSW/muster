package roadmap

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// LoadRoadmap loads the roadmap from .muster/roadmap.json (preferred) or
// .roadmap.json (legacy fallback). Returns an empty Roadmap if neither file
// exists. If the preferred file exists but is malformed, returns an error
// immediately without trying the fallback.
func LoadRoadmap(dir string) (*Roadmap, error) {
	newPath := filepath.Join(dir, ".muster", "roadmap.json")
	roadmap, err := LoadRoadmapFile(newPath)
	if err != nil {
		if !os.IsNotExist(err) {
			// Parse/read error on preferred file -- do NOT fall back
			return nil, fmt.Errorf("failed to load roadmap from %s: %w", newPath, err)
		}

		// Preferred file does not exist -- try legacy location
		legacyPath := filepath.Join(dir, ".roadmap.json")
		roadmap, err = LoadRoadmapFile(legacyPath)
		if err != nil {
			if os.IsNotExist(err) {
				// Neither file exists -- return empty roadmap
				return &Roadmap{Items: []RoadmapItem{}}, nil
			}
			return nil, fmt.Errorf("failed to load roadmap from %s: %w", legacyPath, err)
		}
	}

	return roadmap, nil
}

// LoadRoadmapFile reads and parses a single roadmap file.
// Transparently handles both wrapper format ({"items": [...]}) and
// legacy array format ([...]).
func LoadRoadmapFile(path string) (*Roadmap, error) {
	data, err := os.ReadFile(path) //nolint:gosec // G304: Reading roadmap file from project directory
	if err != nil {
		return nil, err // preserves os.PathError (IsNotExist works)
	}

	// Try wrapper format first (new canonical format)
	var wrapper Roadmap
	if err := json.Unmarshal(data, &wrapper); err == nil && wrapper.Items != nil {
		return &wrapper, nil
	}

	// Try array format (legacy)
	var items []RoadmapItem
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, fmt.Errorf("failed to parse %s: %w: %w", path, ErrRoadmapParse, err)
	}

	return &Roadmap{Items: items}, nil
}

// SaveRoadmap writes the roadmap to .muster/roadmap.json in wrapper format.
// Creates the .muster/ directory if it does not exist.
// Always writes wrapper format regardless of the format that was loaded.
func SaveRoadmap(dir string, roadmap *Roadmap) error {
	musterDir := filepath.Join(dir, ".muster")
	if err := os.MkdirAll(musterDir, 0755); err != nil { //nolint:gosec // G301: Standard directory permissions for .muster
		return fmt.Errorf("failed to create .muster directory: %w", err)
	}

	data, err := json.MarshalIndent(roadmap, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal roadmap: %w", err)
	}

	// Append trailing newline for POSIX compliance
	data = append(data, '\n')

	path := filepath.Join(musterDir, "roadmap.json")
	if err := os.WriteFile(path, data, 0644); err != nil { //nolint:gosec // G306: Standard file permissions for roadmap file
		return fmt.Errorf("failed to write roadmap to %s: %w", path, err)
	}

	return nil
}
