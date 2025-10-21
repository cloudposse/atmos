package exec

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSplitAndTrim(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		sep      string
		expected []string
	}{
		{
			name:     "comma separated with spaces",
			input:    "tag1, tag2, tag3",
			sep:      ",",
			expected: []string{"tag1", "tag2", "tag3"},
		},
		{
			name:     "comma separated without spaces",
			input:    "tag1,tag2,tag3",
			sep:      ",",
			expected: []string{"tag1", "tag2", "tag3"},
		},
		{
			name:     "single item",
			input:    "tag1",
			sep:      ",",
			expected: []string{"tag1"},
		},
		{
			name:     "empty string",
			input:    "",
			sep:      ",",
			expected: []string{""},
		},
		{
			name:     "with extra spaces",
			input:    " tag1 , tag2 , tag3 ",
			sep:      ",",
			expected: []string{"tag1", "tag2", "tag3"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := splitAndTrim(tt.input, tt.sep)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// splitString and trimString tests removed - now using standard library strings.Split and strings.TrimSpace.
