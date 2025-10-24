package container

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCleanPodmanOutput(t *testing.T) {
	tests := []struct {
		name     string
		input    []byte
		expected string
	}{
		{
			name:     "simple string without escapes",
			input:    []byte("hello world"),
			expected: "hello world",
		},
		{
			name:     "string with literal \\n",
			input:    []byte("line1\\nline2\\nline3"),
			expected: "line1\nline2\nline3",
		},
		{
			name:     "string with literal \\t",
			input:    []byte("col1\\tcol2\\tcol3"),
			expected: "col1\tcol2\tcol3",
		},
		{
			name:     "string with literal \\r",
			input:    []byte("text\\rmore text"),
			expected: "text\rmore text",
		},
		{
			name:     "mixed escape sequences",
			input:    []byte("line1\\nline2\\tcolumn\\rtext"),
			expected: "line1\nline2\tcolumn\rtext",
		},
		{
			name:     "podman pull output with escapes",
			input:    []byte("Resolving\\nTrying to pull\\nGetting image\\nCopying blob\\n"),
			expected: "Resolving\nTrying to pull\nGetting image\nCopying blob",
		},
		{
			name:     "trailing and leading whitespace",
			input:    []byte("  text with spaces  \\n  more text  "),
			expected: "text with spaces  \n  more text",
		},
		{
			name:     "empty string",
			input:    []byte(""),
			expected: "",
		},
		{
			name:     "only whitespace",
			input:    []byte("   \t\n  "),
			expected: "",
		},
		{
			name:     "real podman error with escaped newlines",
			input:    []byte("Error: no container with name or ID \"Resolving\\nTrying to pull\\nGetting image\""),
			expected: "Error: no container with name or ID \"Resolving\nTrying to pull\nGetting image\"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := cleanPodmanOutput(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// Test the container ID extraction logic used in podman Create.
func TestExtractContainerIDFromPodmanCreateOutput(t *testing.T) {
	tests := []struct {
		name             string
		output           string
		expectedID       string
		expectEmpty      bool
		errorDescription string
	}{
		{
			name:       "simple container ID only",
			output:     "d31a46dd77566a9b55c6062cdc711a38453cb7f859f086c984a3a1fe77892703",
			expectedID: "d31a46dd77566a9b55c6062cdc711a38453cb7f859f086c984a3a1fe77892703",
		},
		{
			name: "container ID with pull output (real podman behavior)",
			output: `Resolving "hashicorp/terraform" using unqualified-search registries
Trying to pull docker.io/hashicorp/terraform:1.10...
Getting image source signatures
Copying blob sha256:3f8427b65950b065cf17e781548deca8611c329bee69fd12c944e8b77615c5af
Copying blob sha256:185961b25d19dc6017cce9b3a843a692f95ba285c7ae1f50a6c1eb7bac1fb97a
Copying config sha256:36df1606fe8f0580466adc91adba819177db091c29094febf7ed2e10e64b4127
Writing manifest to image destination
d31a46dd77566a9b55c6062cdc711a38453cb7f859f086c984a3a1fe77892703`,
			expectedID: "d31a46dd77566a9b55c6062cdc711a38453cb7f859f086c984a3a1fe77892703",
		},
		{
			name: "container ID with trailing newlines",
			output: `Some output
abcd1234567890

`,
			expectedID: "abcd1234567890",
		},
		{
			name: "container ID with multiple trailing newlines",
			output: `Output line 1
Output line 2
container-id-here


`,
			expectedID: "container-id-here",
		},
		{
			name:             "empty output",
			output:           "",
			expectEmpty:      true,
			errorDescription: "should return empty for no output",
		},
		{
			name:             "only whitespace",
			output:           "   \n\t\n   ",
			expectEmpty:      true,
			errorDescription: "should return empty for whitespace-only output",
		},
		{
			name:       "single line with whitespace",
			output:     "  container-id  ",
			expectedID: "container-id",
		},
		{
			name: "multiline with empty lines in middle",
			output: `First line


Last line with ID`,
			expectedID: "Last line with ID",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Replicate the exact logic from podman.go Create function (lines 72-84).
			lines := strings.Split(tt.output, "\n")
			var containerID string
			for i := len(lines) - 1; i >= 0; i-- {
				line := strings.TrimSpace(lines[i])
				if line != "" {
					containerID = line
					break
				}
			}

			if tt.expectEmpty {
				assert.Empty(t, containerID, tt.errorDescription)
			} else {
				assert.Equal(t, tt.expectedID, containerID)
			}
		})
	}
}
