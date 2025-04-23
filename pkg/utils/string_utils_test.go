package utils

import "testing"

func TestSplitStringByDelimiter(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		delimiter rune
		expected  []string
		expectErr bool
	}{
		{
			name:      "Simple split by space",
			input:     "foo bar baz",
			delimiter: ' ',
			expected:  []string{"foo", "bar", "baz"},
			expectErr: false,
		},
		{
			name:      "Split with quoted sections",
			input:     `"foo bar" baz`,
			delimiter: ' ',
			expected:  []string{"foo bar", "baz"},
			expectErr: false,
		},
		{
			name:      "Empty input string",
			input:     "",
			delimiter: ' ',
			expected:  []string{""},
			expectErr: true,
		},
		{
			name:      "Delimiter not present",
			input:     "foobar",
			delimiter: ',',
			expected:  []string{"foobar"},
			expectErr: false,
		},
		{
			name:      "Multiple spaces as delimiter",
			input:     "foo: !env      FOO",
			delimiter: ' ',
			expected:  []string{"foo:", "!env", "FOO"},
			expectErr: false,
		},

		{
			name:      "Error case with invalid CSV format",
			input:     `"foo,bar`,
			delimiter: ',',
			expected:  nil,
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := SplitStringByDelimiter(tt.input, tt.delimiter)
			if (err != nil) != tt.expectErr {
				t.Errorf("expected error: %v, got: %v", tt.expectErr, err)
			}
			if !tt.expectErr && !equalSlices(t, result, tt.expected) {
				t.Errorf("expected: %v, got: %v", tt.expected, result)
			}
		})
	}
}

func equalSlices(t *testing.T, a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			t.Logf("mismatch at index %d: expected %s, got %s", i, b[i], a[i])
			return false
		}
	}
	return true
}
