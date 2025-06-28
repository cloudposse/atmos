package exec

import (
	"strings"
	"testing"
)

func TestGetStringAfterTag(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		tag           string
		expected      string
		expectedError string
	}{
		{
			name:     "basic tag",
			input:    "!template path/to/file.yaml",
			tag:      "!template",
			expected: "path/to/file.yaml",
		},
		{
			name:     "tag with spaces",
			input:    "!template    path/with/spaces.yaml   ",
			tag:      "!template",
			expected: "path/with/spaces.yaml",
		},
		{
			name:     "tag with special characters",
			input:    "!template@# path/with/special@#.yaml",
			tag:      "!template@#",
			expected: "path/with/special@#.yaml",
		},
		{
			name:          "empty input",
			input:         "",
			tag:           "!template",
			expectedError: "invalid Atmos YAML function: ",
		},
		{
			name:     "tag not at start",
			input:    "some prefix !template path/to/file.yaml",
			tag:      "!template",
			expected: "some prefix !template path/to/file.yaml",
		},
		{
			name:     "multiple spaces after tag",
			input:    "!template    multiple   spaces.yaml",
			tag:      "!template",
			expected: "multiple   spaces.yaml",
		},
		{
			name:     "tag with newline",
			input:    "!template\npath/with/newline.yaml",
			tag:      "!template",
			expected: "path/with/newline.yaml",
		},
		{
			name:     "tag with tab",
			input:    "!template\tpath/with/tab.yaml",
			tag:      "!template",
			expected: "path/with/tab.yaml",
		},
		{
			name:     "empty tag",
			input:    "!template path/to/file.yaml",
			tag:      "",
			expected: "!template path/to/file.yaml",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := getStringAfterTag(tt.input, tt.tag)

			// Check error cases
			if tt.expectedError != "" {
				if err == nil {
					t.Errorf("expected error but got nil")
					return
				}
				if !strings.Contains(err.Error(), tt.expectedError) {
					t.Errorf("expected error to contain %q, got %q", tt.expectedError, err.Error())
				}
				return
			}

			// Check normal cases
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestGetStringAfterTag_EdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		tag      string
		expected string
	}{
		{
			name:     "tag not found",
			input:    "some string",
			tag:      "!template",
			expected: "some string",
		},
		{
			name:     "unicode characters",
			input:    "!templaté pâth/with/ünïcødé.yml",
			tag:      "!templaté",
			expected: "pâth/with/ünïcødé.yml",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := getStringAfterTag(tt.input, tt.tag)

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestSkipFunc(t *testing.T) {
	tests := []struct {
		name     string
		skip     []string
		function string
		expected bool
	}{
		{
			name:     "empty skip list",
			skip:     []string{},
			function: "!template",
			expected: false,
		},
		{
			name:     "function not in skip list",
			skip:     []string{"!exec", "!store"},
			function: "!template",
			expected: false,
		},
		{
			name:     "function with negation in skip list",
			skip:     []string{"!exec", "!!template", "!store"},
			function: "!template",
			expected: false,
		},
		{
			name:     "empty function",
			skip:     []string{"!exec", "!template", "!store"},
			function: "",
			expected: false,
		},
		{
			name:     "case sensitive match",
			skip:     []string{"!Template", "!Exec"},
			function: "!template",
			expected: false,
		},
		{
			name:     "exact match required",
			skip:     []string{"!template"},
			function: "!template:extra",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := skipFunc(tt.skip, tt.function)
			if result != tt.expected {
				t.Errorf("expected %v, got %v for function %q and skip list %v",
					tt.expected, result, tt.function, tt.skip)
			}
		})
	}
}

func TestSkipFunc_EdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		skip     []string
		function string
		expected bool
	}{
		{
			name:     "nil skip list",
			skip:     nil,
			function: "!template",
			expected: false,
		},
		{
			name:     "function with leading/trailing spaces",
			skip:     []string{"  !template  "},
			function: "!template",
			expected: false, // Because "  !template  " != "!template"
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := skipFunc(tt.skip, tt.function)
			if result != tt.expected {
				t.Errorf("expected %v, got %v for function %q and skip list %v",
					tt.expected, result, tt.function, tt.skip)
			}
		})
	}
}
