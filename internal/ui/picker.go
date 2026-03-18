package ui

import (
	"errors"
	"fmt"

	"github.com/charmbracelet/huh"
)

// PickerOption represents a single option in the picker
type PickerOption struct {
	Label string
	Value string
}

// PickerConfig holds configuration for the picker
type PickerConfig struct {
	Height         int
	DefaultValue   string
	FilteringLabel string
}

// DefaultPickerConfig returns a PickerConfig with sensible defaults
func DefaultPickerConfig() PickerConfig {
	return PickerConfig{
		Height:         10,
		DefaultValue:   "",
		FilteringLabel: "Filter:",
	}
}

// Picker is the interface for interactive item selection
type Picker interface {
	Show(title string, options []PickerOption, cfg PickerConfig) (string, error)
}

// HuhPicker implements Picker using charmbracelet/huh
type HuhPicker struct{}

// Show displays an interactive picker and returns the selected value
func (p *HuhPicker) Show(title string, options []PickerOption, cfg PickerConfig) (string, error) {
	if len(options) == 0 {
		return "", errors.New("cannot show picker with empty options")
	}

	// Convert PickerOptions to huh.Option[string]
	huhOptions := make([]huh.Option[string], len(options))
	for i, opt := range options {
		huhOptions[i] = huh.NewOption(opt.Label, opt.Value)
	}

	var selected string

	// Set default value if provided and valid
	if cfg.DefaultValue != "" {
		selected = cfg.DefaultValue
	}

	// Create the select input with filtering enabled
	selectInput := huh.NewSelect[string]().
		Title(title).
		Options(huhOptions...).
		Value(&selected).
		Filtering(true)

	// Set height if specified (height > 0)
	if cfg.Height > 0 {
		selectInput = selectInput.Height(cfg.Height)
	}

	// Create and run the form
	form := huh.NewForm(huh.NewGroup(selectInput))

	err := form.Run()
	if err != nil {
		// User cancelled (ESC/Ctrl+C) or other error
		return "", fmt.Errorf("picker cancelled or error: %w", err)
	}

	return selected, nil
}

// DefaultPicker is the package-level picker instance used by commands
var DefaultPicker Picker = &HuhPicker{}
