package errors

import (
	"strings"
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/stretchr/testify/assert"
)

func TestDefaultFormatterConfig(t *testing.T) {
	config := DefaultFormatterConfig()

	assert.False(t, config.Verbose)
	assert.Equal(t, "auto", config.Color)
	assert.Equal(t, 80, config.MaxLineLength)
}

func TestFormat_NilError(t *testing.T) {
	config := DefaultFormatterConfig()
	result := Format(nil, config)

	assert.Empty(t, result)
}

func TestFormat_SimpleError(t *testing.T) {
	err := errors.New("test error")
	config := DefaultFormatterConfig()
	config.Color = "never" // Disable color for testing.

	result := Format(err, config)

	assert.Contains(t, result, "test error")
	assert.NotContains(t, result, "💡") // No hints.
}

func TestFormat_ErrorWithHint(t *testing.T) {
	err := errors.WithHint(
		errors.New("test error"),
		"Try running --help",
	)

	config := DefaultFormatterConfig()
	config.Color = "never"

	result := Format(err, config)

	assert.Contains(t, result, "test error")
	assert.Contains(t, result, "💡")
	assert.Contains(t, result, "Try running --help")
}

func TestFormat_ErrorWithMultipleHints(t *testing.T) {
	err := errors.WithHint(
		errors.WithHint(
			errors.New("test error"),
			"First hint",
		),
		"Second hint",
	)

	config := DefaultFormatterConfig()
	config.Color = "never"

	result := Format(err, config)

	assert.Contains(t, result, "test error")
	assert.Contains(t, result, "First hint")
	assert.Contains(t, result, "Second hint")

	// Count hint emojis.
	hintCount := strings.Count(result, "💡")
	assert.Equal(t, 2, hintCount)
}

func TestFormat_LongErrorMessage(t *testing.T) {
	longMsg := "This is a very long error message that exceeds the maximum line length and should be wrapped to multiple lines for better readability in the terminal output"
	err := errors.New(longMsg)

	config := DefaultFormatterConfig()
	config.Color = "never"
	config.MaxLineLength = 80

	result := Format(err, config)

	// Check that the message is wrapped.
	lines := strings.Split(result, "\n")
	for _, line := range lines {
		// Each line should be at or under max length (allowing for some flexibility).
		assert.LessOrEqual(t, len(line), config.MaxLineLength+20) // Buffer for words.
	}
}

func TestFormat_VerboseMode(t *testing.T) {
	err := errors.New("test error")

	config := DefaultFormatterConfig()
	config.Color = "never"
	config.Verbose = true

	result := Format(err, config)

	assert.Contains(t, result, "test error")
	// Verbose mode should include additional details.
	assert.Greater(t, len(result), len("test error"))
}

func TestFormat_ColorModes(t *testing.T) {
	err := errors.New("test error")

	tests := []struct {
		name      string
		colorMode string
	}{
		{"always", "always"},
		{"never", "never"},
		{"auto", "auto"},
		{"invalid", "invalid"}, // Should fallback to auto.
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := DefaultFormatterConfig()
			config.Color = tt.colorMode

			result := Format(err, config)
			assert.Contains(t, result, "test error")
		})
	}
}

func TestFormat_WithBuilder(t *testing.T) {
	err := Build(errors.New("database connection failed")).
		WithHint("Check database credentials in atmos.yaml").
		WithHintf("Verify network connectivity to %s", "db.example.com").
		WithContext("component", "vpc").
		WithContext("stack", "prod").
		WithExitCode(2).
		Err()

	config := DefaultFormatterConfig()
	config.Color = "never"

	result := Format(err, config)

	assert.Contains(t, result, "database connection failed")
	assert.Contains(t, result, "Check database credentials")
	assert.Contains(t, result, "Verify network connectivity to db.example.com")
	assert.Equal(t, 2, strings.Count(result, "💡"))
}

func TestShouldUseColor(t *testing.T) {
	tests := []struct {
		name      string
		colorMode string
		expected  bool
	}{
		{"always", "always", true},
		{"never", "never", false},
		// "auto" and "invalid" depend on TTY, so we don't test exact values.
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.colorMode == "always" || tt.colorMode == "never" {
				result := shouldUseColor(tt.colorMode)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestWrapText(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		width    int
		expected int // Number of lines expected.
	}{
		{
			name:     "short text",
			text:     "hello world",
			width:    80,
			expected: 1,
		},
		{
			name:     "long text wraps",
			text:     "This is a very long sentence that should wrap to multiple lines when the width is set to a small value",
			width:    40,
			expected: 3, // Should wrap to at least 3 lines.
		},
		{
			name:     "single long word",
			text:     "supercalifragilisticexpialidocious",
			width:    10,
			expected: 1, // Single word stays on one line.
		},
		{
			name:     "zero width uses default",
			text:     "hello world",
			width:    0,
			expected: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := wrapText(tt.text, tt.width)
			lines := strings.Split(result, "\n")

			assert.GreaterOrEqual(t, len(lines), tt.expected)

			// Verify each line is within width (allowing for single long words).
			for _, line := range lines {
				if strings.Contains(tt.text, " ") {
					// Only check width if text has spaces (can wrap).
					maxLen := tt.width
					if maxLen == 0 {
						maxLen = 80
					}
					// Allow some buffer for word boundaries.
					assert.LessOrEqual(t, len(line), maxLen+30)
				}
			}
		})
	}
}

func TestFormatStackTrace(t *testing.T) {
	err := errors.New("test error")

	// Test without color.
	result := formatStackTrace(err, false)
	assert.Contains(t, result, "test error")

	// Test with color.
	resultColor := formatStackTrace(err, true)
	assert.Contains(t, resultColor, "test error")
}

func TestFormatContextTable_NoContext(t *testing.T) {
	err := errors.New("test error")

	// Error without context should return empty string.
	result := formatContextTable(err, false)
	assert.Empty(t, result)
}

func TestFormatContextTable_WithContext(t *testing.T) {
	err := Build(errors.New("test error")).
		WithContext("component", "vpc").
		WithContext("stack", "prod").
		Err()

	// Test without color.
	result := formatContextTable(err, false)
	assert.NotEmpty(t, result)
	assert.Contains(t, result, "Context")
	assert.Contains(t, result, "Value")
	assert.Contains(t, result, "component")
	assert.Contains(t, result, "vpc")
	assert.Contains(t, result, "stack")
	assert.Contains(t, result, "prod")
}

func TestFormatContextTable_WithColorAndMultipleContext(t *testing.T) {
	err := Build(errors.New("test error")).
		WithContext("component", "vpc").
		WithContext("stack", "prod").
		WithContext("region", "us-east-1").
		Err()

	// Test with color.
	result := formatContextTable(err, true)
	assert.NotEmpty(t, result)
	assert.Contains(t, result, "component")
	assert.Contains(t, result, "vpc")
	assert.Contains(t, result, "stack")
	assert.Contains(t, result, "prod")
	assert.Contains(t, result, "region")
	assert.Contains(t, result, "us-east-1")
}

func TestFormat_VerboseWithContext(t *testing.T) {
	err := Build(errors.New("test error")).
		WithContext("component", "vpc").
		WithContext("stack", "prod").
		WithHint("Check the configuration").
		Err()

	config := DefaultFormatterConfig()
	config.Verbose = true
	config.Color = "never"

	result := Format(err, config)

	// Should contain error message.
	assert.Contains(t, result, "test error")

	// Should contain hints.
	assert.Contains(t, result, "💡")
	assert.Contains(t, result, "Check the configuration")

	// Should contain context table.
	assert.Contains(t, result, "Context")
	assert.Contains(t, result, "Value")
	assert.Contains(t, result, "component")
	assert.Contains(t, result, "vpc")
	assert.Contains(t, result, "stack")
	assert.Contains(t, result, "prod")

	// Should contain stack trace.
	assert.Contains(t, result, "stack trace")
}

func TestFormat_NonVerboseWithContext(t *testing.T) {
	err := Build(errors.New("test error")).
		WithContext("component", "vpc").
		WithContext("stack", "prod").
		WithHint("Check the configuration").
		Err()

	config := DefaultFormatterConfig()
	config.Verbose = false // Non-verbose mode.
	config.Color = "never"

	result := Format(err, config)

	// Should contain error message and hints.
	assert.Contains(t, result, "test error")
	assert.Contains(t, result, "💡")
	assert.Contains(t, result, "Check the configuration")

	// Should NOT contain context table in non-verbose mode.
	assert.NotContains(t, result, "Context")
	assert.NotContains(t, result, "Value")

	// Should NOT contain stack trace in non-verbose mode.
	assert.NotContains(t, result, "stack trace")
}
