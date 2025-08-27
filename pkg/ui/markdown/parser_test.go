package markdown

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSplitMarkdownContent(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "Simple split with details and suggestion",
			input:    "This is the detail part\n\nThis is the suggestion part",
			expected: []string{"This is the detail part", "This is the suggestion part"},
		},
		{
			name:     "Multiple paragraphs - first becomes details, rest becomes suggestion",
			input:    "First paragraph\n\nSecond paragraph\n\nThird paragraph",
			expected: []string{"First paragraph", "Second paragraph\n\nThird paragraph"},
		},
		{
			name:     "Leading empty lines are ignored",
			input:    "\n\n\n\nFirst real content\n\nSecond part",
			expected: []string{"First real content", "Second part"},
		},
		{
			name:     "Single paragraph only - no suggestion",
			input:    "Only one part here",
			expected: []string{"Only one part here"},
		},
		{
			name:     "Empty string returns nil slice",
			input:    "",
			expected: nil,
		},
		{
			name:     "Only whitespace returns nil slice",
			input:    "   \n\n   \n\n   ",
			expected: nil,
		},
		{
			name:     "Preserves formatting in suggestion part",
			input:    "Error details\n\n## Suggestion\n- Try this\n- Or that\n\nMore info here",
			expected: []string{"Error details", "## Suggestion\n- Try this\n- Or that\n\nMore info here"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SplitMarkdownContent(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}