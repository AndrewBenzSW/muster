package ui

import (
	"errors"
	"testing"

	"github.com/charmbracelet/huh"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockPicker is a test implementation of the Picker interface
type MockPicker struct {
	ShowFunc func(title string, options []PickerOption, cfg PickerConfig) (string, error)
}

// Show delegates to the ShowFunc if set
func (m *MockPicker) Show(title string, options []PickerOption, cfg PickerConfig) (string, error) {
	if m.ShowFunc != nil {
		return m.ShowFunc(title, options, cfg)
	}
	return "", errors.New("MockPicker.ShowFunc not set")
}

func TestMockPicker_Show_ReturnsSelectedValue(t *testing.T) {
	mock := &MockPicker{
		ShowFunc: func(title string, options []PickerOption, cfg PickerConfig) (string, error) {
			// Simulate user selecting the second option
			return options[1].Value, nil
		},
	}

	options := []PickerOption{
		{Label: "First", Value: "first"},
		{Label: "Second", Value: "second"},
		{Label: "Third", Value: "third"},
	}

	result, err := mock.Show("Test Title", options, DefaultPickerConfig())

	require.NoError(t, err)
	assert.Equal(t, "second", result)
}

func TestMockPicker_Show_EmptyOptionsError(t *testing.T) {
	mock := &MockPicker{
		ShowFunc: func(title string, options []PickerOption, cfg PickerConfig) (string, error) {
			if len(options) == 0 {
				return "", errors.New("cannot show picker with empty options")
			}
			return options[0].Value, nil
		},
	}

	result, err := mock.Show("Test Title", []PickerOption{}, DefaultPickerConfig())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty options")
	assert.Equal(t, "", result)
}

func TestMockPicker_Show_CancellationError(t *testing.T) {
	mock := &MockPicker{
		ShowFunc: func(title string, options []PickerOption, cfg PickerConfig) (string, error) {
			// Simulate user cancellation
			return "", errors.New("picker cancelled or error: user cancelled")
		},
	}

	options := []PickerOption{
		{Label: "First", Value: "first"},
	}

	result, err := mock.Show("Test Title", options, DefaultPickerConfig())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "cancelled")
	assert.Equal(t, "", result)
}

func TestHuhPicker_EmptyOptions(t *testing.T) {
	picker := &HuhPicker{}

	result, err := picker.Show("Test Title", []PickerOption{}, DefaultPickerConfig())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty options")
	assert.Equal(t, "", result)
}

func TestHuhPicker_OptionBuilding(t *testing.T) {
	// Test that options are correctly converted to huh.Option[string]
	options := []PickerOption{
		{Label: "Option One", Value: "value1"},
		{Label: "Option Two", Value: "value2"},
		{Label: "Option Three", Value: "value3"},
	}

	// Verify that huh.Option can be created with our values
	huhOptions := make([]huh.Option[string], len(options))
	for i, opt := range options {
		huhOptions[i] = huh.NewOption(opt.Label, opt.Value)
		// Verify the option was created successfully
		assert.NotNil(t, huhOptions[i])
	}

	assert.Len(t, huhOptions, 3)
}

func TestDefaultPickerConfig(t *testing.T) {
	cfg := DefaultPickerConfig()

	assert.Equal(t, 10, cfg.Height)
	assert.Equal(t, "", cfg.DefaultValue)
	assert.Equal(t, "Filter:", cfg.FilteringLabel)
}

func TestDefaultPicker_IsSet(t *testing.T) {
	// Verify that DefaultPicker is set and implements Picker interface
	assert.NotNil(t, DefaultPicker)
	assert.Implements(t, (*Picker)(nil), DefaultPicker)
}

func TestPickerConfig_CustomValues(t *testing.T) {
	cfg := PickerConfig{
		Height:         20,
		DefaultValue:   "default123",
		FilteringLabel: "Search:",
	}

	assert.Equal(t, 20, cfg.Height)
	assert.Equal(t, "default123", cfg.DefaultValue)
	assert.Equal(t, "Search:", cfg.FilteringLabel)
}

func TestPickerOption_Creation(t *testing.T) {
	opt := PickerOption{
		Label: "Test Label",
		Value: "test_value",
	}

	assert.Equal(t, "Test Label", opt.Label)
	assert.Equal(t, "test_value", opt.Value)
}

func TestMockPicker_MultipleInvocations(t *testing.T) {
	callCount := 0
	mock := &MockPicker{
		ShowFunc: func(title string, options []PickerOption, cfg PickerConfig) (string, error) {
			callCount++
			return options[0].Value, nil
		},
	}

	options := []PickerOption{
		{Label: "First", Value: "first"},
	}

	// Call multiple times
	_, _ = mock.Show("Title 1", options, DefaultPickerConfig())
	_, _ = mock.Show("Title 2", options, DefaultPickerConfig())
	_, _ = mock.Show("Title 3", options, DefaultPickerConfig())

	assert.Equal(t, 3, callCount)
}

func TestMockPicker_CanVerifyArguments(t *testing.T) {
	var capturedTitle string
	var capturedOptions []PickerOption
	var capturedConfig PickerConfig

	mock := &MockPicker{
		ShowFunc: func(title string, options []PickerOption, cfg PickerConfig) (string, error) {
			capturedTitle = title
			capturedOptions = options
			capturedConfig = cfg
			return options[0].Value, nil
		},
	}

	expectedOptions := []PickerOption{
		{Label: "Test", Value: "test"},
	}
	expectedConfig := PickerConfig{Height: 15, DefaultValue: "test", FilteringLabel: "Custom:"}

	_, err := mock.Show("Expected Title", expectedOptions, expectedConfig)

	require.NoError(t, err)
	assert.Equal(t, "Expected Title", capturedTitle)
	assert.Equal(t, expectedOptions, capturedOptions)
	assert.Equal(t, expectedConfig, capturedConfig)
}
