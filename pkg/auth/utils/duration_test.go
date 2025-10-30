package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseDurationFlexible(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		expected      int64
		shouldError   bool
		errorContains string
	}{
		// Integer formats (seconds)
		{
			name:     "integer seconds - 3600",
			input:    "3600",
			expected: 3600,
		},
		{
			name:     "integer seconds - 43200 (12 hours)",
			input:    "43200",
			expected: 43200,
		},
		{
			name:     "integer seconds - 86400 (24 hours)",
			input:    "86400",
			expected: 86400,
		},
		{
			name:          "integer seconds - zero",
			input:         "0",
			shouldError:   true,
			errorContains: "duration must be positive",
		},
		{
			name:          "integer seconds - negative",
			input:         "-100",
			shouldError:   true,
			errorContains: "duration must be positive",
		},

		// Go duration formats
		{
			name:     "go duration - 1h",
			input:    "1h",
			expected: 3600,
		},
		{
			name:     "go duration - 12h",
			input:    "12h",
			expected: 43200,
		},
		{
			name:     "go duration - 24h",
			input:    "24h",
			expected: 86400,
		},
		{
			name:     "go duration - 1h30m",
			input:    "1h30m",
			expected: 5400,
		},
		{
			name:     "go duration - 90m",
			input:    "90m",
			expected: 5400,
		},
		{
			name:     "go duration - 5400s",
			input:    "5400s",
			expected: 5400,
		},
		{
			name:     "go duration - 15m",
			input:    "15m",
			expected: 900,
		},
		{
			name:     "go duration - 900s",
			input:    "900s",
			expected: 900,
		},
		{
			name:     "go duration - complex 1h30m45s",
			input:    "1h30m45s",
			expected: 5445,
		},

		// Days format (not supported by Go's ParseDuration)
		{
			name:     "days - 1d",
			input:    "1d",
			expected: 86400,
		},
		{
			name:     "days - 2d",
			input:    "2d",
			expected: 172800,
		},
		{
			name:     "days - 7d",
			input:    "7d",
			expected: 604800,
		},

		// Edge cases
		{
			name:     "whitespace trimmed",
			input:    "  12h  ",
			expected: 43200,
		},
		{
			name:          "empty string",
			input:         "",
			shouldError:   true,
			errorContains: "duration string is empty",
		},
		{
			name:          "whitespace only",
			input:         "   ",
			shouldError:   true,
			errorContains: "duration string is empty",
		},
		{
			name:          "invalid format - letters only",
			input:         "abc",
			shouldError:   true,
			errorContains: "invalid duration format",
		},
		{
			name:          "invalid format - invalid unit",
			input:         "12x",
			shouldError:   true,
			errorContains: "invalid duration format",
		},
		{
			name:          "negative go duration",
			input:         "-1h",
			shouldError:   true,
			errorContains: "duration must be positive",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseDurationFlexible(tt.input)

			if tt.shouldError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}
