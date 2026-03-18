package ui

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"

	"golang.org/x/term"
)

// OutputMode represents the output format mode
type OutputMode string

const (
	// TableMode outputs in human-readable table format
	TableMode OutputMode = "table"
	// JSONMode outputs in JSON format
	JSONMode OutputMode = "json"
)

// VersionInfo contains version information for the application
type VersionInfo struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	Date      string `json:"date"`
	GoVersion string `json:"goVersion"`
	Platform  string `json:"platform"`
}

var (
	modeMux     sync.Mutex
	currentMode OutputMode = TableMode
)

// IsInteractive returns true if stdout is connected to a terminal
func IsInteractive() bool {
	return term.IsTerminal(int(os.Stdout.Fd())) //nolint:gosec // G115: Safe conversion for terminal detection
}

// SetOutputMode sets the current output mode
func SetOutputMode(mode OutputMode) {
	modeMux.Lock()
	defer modeMux.Unlock()
	currentMode = mode
}

// GetOutputMode returns the current output mode
func GetOutputMode() OutputMode {
	modeMux.Lock()
	defer modeMux.Unlock()
	return currentMode
}

// FormatVersion formats version information according to the current output mode
func FormatVersion(info VersionInfo) (string, error) {
	modeMux.Lock()
	mode := currentMode
	modeMux.Unlock()

	switch mode {
	case JSONMode:
		data, err := json.MarshalIndent(info, "", "  ")
		if err != nil {
			return "", err
		}
		return string(data), nil
	case TableMode:
		fallthrough
	default:
		return fmt.Sprintf(`Version:    %s
Commit:     %s
Date:       %s
Go:         %s
Platform:   %s`,
			info.Version,
			info.Commit,
			info.Date,
			info.GoVersion,
			info.Platform), nil
	}
}
