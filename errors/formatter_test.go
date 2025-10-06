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

	// The new formatter uses structured markdown sections.
	// Wrapping is handled by the Glamour markdown renderer.
	assert.Contains(t, result, "# Error")
	assert.Contains(t, result, longMsg)
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

	// Context IS shown in non-verbose mode (new structured markdown format).
	assert.Contains(t, result, "## Context")
	assert.Contains(t, result, "component")
	assert.Contains(t, result, "vpc")

	// Should NOT contain stack trace in non-verbose mode.
	assert.NotContains(t, result, "## Stack Trace")
}

func TestFormat_WithExplanation(t *testing.T) {
	err := Build(errors.New("test error")).
		WithExplanation("This is a detailed explanation of what went wrong.").
		Err()

	config := DefaultFormatterConfig()
	config.Color = "never"

	result := Format(err, config)

	// Should contain the Error header.
	assert.Contains(t, result, "# Error")
	// Should contain the error message.
	assert.Contains(t, result, "test error")
	// Should contain the Explanation section.
	assert.Contains(t, result, "## Explanation")
	assert.Contains(t, result, "This is a detailed explanation")
}

func TestFormat_WithExample(t *testing.T) {
	exampleContent := "```yaml\nworkflows:\n  deploy:\n    steps:\n      - command: terraform apply\n```"
	err := Build(errors.New("invalid workflow")).
		WithExampleFile(exampleContent).
		Err()

	config := DefaultFormatterConfig()
	config.Color = "never"

	result := Format(err, config)

	// Should contain the Error header.
	assert.Contains(t, result, "# Error")
	// Should contain the Example section.
	assert.Contains(t, result, "## Example")
	// Should contain the example content.
	assert.Contains(t, result, "workflows:")
	assert.Contains(t, result, "deploy:")
}

func TestFormat_WithAllSections(t *testing.T) {
	exampleContent := "```yaml\ntest: example\n```"
	err := Build(errors.New("test error")).
		WithExplanation("Detailed explanation of the error.").
		WithExampleFile(exampleContent).
		WithHint("First hint").
		WithHint("Second hint").
		WithContext("component", "vpc").
		WithContext("stack", "prod").
		Err()

	config := DefaultFormatterConfig()
	config.Color = "never"
	config.Verbose = true

	result := Format(err, config)

	// Section 1: Error.
	assert.Contains(t, result, "# Error")
	assert.Contains(t, result, "test error")

	// Section 2: Explanation.
	assert.Contains(t, result, "## Explanation")
	assert.Contains(t, result, "Detailed explanation")

	// Section 3: Example.
	assert.Contains(t, result, "## Example")
	assert.Contains(t, result, "test: example")

	// Section 4: Hints.
	assert.Contains(t, result, "## Hints")
	assert.Contains(t, result, "💡 First hint")
	assert.Contains(t, result, "💡 Second hint")

	// Section 5: Context.
	assert.Contains(t, result, "## Context")
	assert.Contains(t, result, "component")
	assert.Contains(t, result, "vpc")
	assert.Contains(t, result, "stack")
	assert.Contains(t, result, "prod")

	// Section 6: Stack Trace (verbose mode).
	assert.Contains(t, result, "## Stack Trace")
}

func TestFormat_SectionOrder(t *testing.T) {
	exampleContent := "example code"
	err := Build(errors.New("test error")).
		WithExplanation("explanation").
		WithExampleFile(exampleContent).
		WithHint("hint").
		WithContext("key", "value").
		Err()

	config := DefaultFormatterConfig()
	config.Color = "never"
	config.Verbose = true

	result := Format(err, config)

	// Find positions of each section header.
	errorPos := strings.Index(result, "# Error")
	explanationPos := strings.Index(result, "## Explanation")
	examplePos := strings.Index(result, "## Example")
	hintsPos := strings.Index(result, "## Hints")
	contextPos := strings.Index(result, "## Context")
	stackPos := strings.Index(result, "## Stack Trace")

	// Verify correct order.
	assert.True(t, errorPos < explanationPos, "Error should come before Explanation")
	assert.True(t, explanationPos < examplePos, "Explanation should come before Example")
	assert.True(t, examplePos < hintsPos, "Example should come before Hints")
	assert.True(t, hintsPos < contextPos, "Hints should come before Context")
	assert.True(t, contextPos < stackPos, "Context should come before Stack Trace")
}

func TestFormat_ExampleAndHintsSeparation(t *testing.T) {
	exampleContent := "example code here"
	err := Build(errors.New("test error")).
		WithHint("Regular hint 1").
		WithExampleFile(exampleContent).
		WithHint("Regular hint 2").
		Err()

	config := DefaultFormatterConfig()
	config.Color = "never"

	result := Format(err, config)

	// Should have separate Example and Hints sections.
	assert.Contains(t, result, "## Example")
	assert.Contains(t, result, "## Hints")

	// Example section should not have hint emoji.
	exampleStart := strings.Index(result, "## Example")
	hintsStart := strings.Index(result, "## Hints")
	exampleSection := result[exampleStart:hintsStart]
	assert.NotContains(t, exampleSection, "💡", "Example section should not contain hint emoji")

	// Hints section should only have regular hints.
	hintsSection := result[hintsStart:]
	assert.Contains(t, hintsSection, "💡 Regular hint 1")
	assert.Contains(t, hintsSection, "💡 Regular hint 2")
	assert.NotContains(t, hintsSection, "EXAMPLE:", "Hints section should not contain EXAMPLE: prefix")
}

func TestFormat_NoExplanation_NoSection(t *testing.T) {
	err := Build(errors.New("test error")).
		WithHint("Just a hint").
		Err()

	config := DefaultFormatterConfig()
	config.Color = "never"

	result := Format(err, config)

	// Should NOT have Explanation section if no explanation provided.
	assert.NotContains(t, result, "## Explanation")
	// But should have Error header and Hints.
	assert.Contains(t, result, "# Error")
	assert.Contains(t, result, "## Hints")
}

func TestFormat_NoExample_NoSection(t *testing.T) {
	err := Build(errors.New("test error")).
		WithHint("Just a hint").
		Err()

	config := DefaultFormatterConfig()
	config.Color = "never"

	result := Format(err, config)

	// Should NOT have Example section if no example provided.
	assert.NotContains(t, result, "## Example")
}

func TestFormat_NoHints_NoSection(t *testing.T) {
	err := Build(errors.New("test error")).
		WithExplanation("Just an explanation").
		Err()

	config := DefaultFormatterConfig()
	config.Color = "never"

	result := Format(err, config)

	// Should NOT have Hints section if no hints provided.
	assert.NotContains(t, result, "## Hints")
	// But should have Error and Explanation.
	assert.Contains(t, result, "# Error")
	assert.Contains(t, result, "## Explanation")
}

func TestFormat_ContextMarkdownTable(t *testing.T) {
	err := Build(errors.New("test error")).
		WithContext("component", "vpc").
		WithContext("stack", "prod").
		WithContext("region", "us-east-1").
		Err()

	config := DefaultFormatterConfig()
	config.Color = "never"
	config.Verbose = true

	result := Format(err, config)

	// Should have Context section (Glamour renders markdown tables with box-drawing chars).
	assert.Contains(t, result, "## Context")
	// Check for the actual context values (table structure may be rendered differently by Glamour).
	assert.Contains(t, result, "component")
	assert.Contains(t, result, "vpc")
	assert.Contains(t, result, "stack")
	assert.Contains(t, result, "prod")
	assert.Contains(t, result, "region")
	assert.Contains(t, result, "us-east-1")
}

func TestFormatContextForMarkdown(t *testing.T) {
	err := Build(errors.New("test error")).
		WithContext("component", "vpc").
		WithContext("stack", "prod").
		Err()

	result := formatContextForMarkdown(err)

	// Should return markdown table format.
	assert.Contains(t, result, "| Key | Value |")
	assert.Contains(t, result, "|-----|-------|")
	assert.Contains(t, result, "| component | vpc |")
	assert.Contains(t, result, "| stack | prod |")
}

func TestFormatContextForMarkdown_NoContext(t *testing.T) {
	err := errors.New("test error")

	result := formatContextForMarkdown(err)

	// Should return empty string when no context.
	assert.Empty(t, result)
}

func TestFormat_VerboseStackTrace(t *testing.T) {
	err := Build(errors.New("test error")).
		WithHint("A hint").
		Err()

	config := DefaultFormatterConfig()
	config.Color = "never"
	config.Verbose = true

	result := Format(err, config)

	// Should contain Stack Trace section in verbose mode.
	// Glamour renders code fences as styled blocks, so we just check for section header and content.
	assert.Contains(t, result, "## Stack Trace")
	assert.Contains(t, result, "test error")
	// Stack trace should contain error chain info.
	assert.Contains(t, result, "stack trace")
}

func TestFormat_NonVerboseNoStackTrace(t *testing.T) {
	err := Build(errors.New("test error")).
		WithHint("A hint").
		Err()

	config := DefaultFormatterConfig()
	config.Color = "never"
	config.Verbose = false

	result := Format(err, config)

	// Should NOT contain Stack Trace section in non-verbose mode.
	assert.NotContains(t, result, "## Stack Trace")
}
