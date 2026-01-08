package github

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConvertToRawURL(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expected    string
		expectError bool
	}{
		// GitHub blob URLs
		{
			name:     "github blob URL with file path",
			input:    "https://github.com/owner/repo/blob/main/path/to/file.yaml",
			expected: "https://raw.githubusercontent.com/owner/repo/main/path/to/file.yaml",
		},
		{
			name:     "github blob URL with branch",
			input:    "https://github.com/owner/repo/blob/develop/file.yaml",
			expected: "https://raw.githubusercontent.com/owner/repo/develop/file.yaml",
		},
		{
			name:     "github blob URL with tag",
			input:    "https://github.com/aquaproj/aqua-registry/blob/v4.155.1/pkgs/hashicorp/terraform/registry.yaml",
			expected: "https://raw.githubusercontent.com/aquaproj/aqua-registry/v4.155.1/pkgs/hashicorp/terraform/registry.yaml",
		},
		{
			name:     "github blob URL with nested path",
			input:    "https://github.com/owner/repo/blob/main/deep/nested/path/file.yaml",
			expected: "https://raw.githubusercontent.com/owner/repo/main/deep/nested/path/file.yaml",
		},

		// GitHub tree URLs
		{
			name:     "github tree URL",
			input:    "https://github.com/owner/repo/tree/main/path",
			expected: "https://raw.githubusercontent.com/owner/repo/main/path",
		},
		{
			name:     "github tree URL with tag",
			input:    "https://github.com/aquaproj/aqua-registry/tree/v4.155.1/pkgs",
			expected: "https://raw.githubusercontent.com/aquaproj/aqua-registry/v4.155.1/pkgs",
		},

		// Owner/repo only (defaults to main)
		{
			name:     "owner/repo only defaults to main",
			input:    "https://github.com/owner/repo",
			expected: "https://raw.githubusercontent.com/owner/repo/main",
		},

		// Already raw URLs
		{
			name:     "already raw URL returns unchanged",
			input:    "https://raw.githubusercontent.com/owner/repo/main/file.yaml",
			expected: "https://raw.githubusercontent.com/owner/repo/main/file.yaml",
		},

		// GitHub scheme URLs (github://)
		{
			name:     "github scheme with ref",
			input:    "github://owner/repo/path/file.yaml@main",
			expected: "https://raw.githubusercontent.com/owner/repo/main/path/file.yaml",
		},
		{
			name:     "github scheme with tag",
			input:    "github://aquaproj/aqua-registry/pkgs@v4.155.1",
			expected: "https://raw.githubusercontent.com/aquaproj/aqua-registry/v4.155.1/pkgs",
		},
		{
			name:     "github scheme without ref defaults to main",
			input:    "github://owner/repo/file.yaml",
			expected: "https://raw.githubusercontent.com/owner/repo/main/file.yaml",
		},
		{
			name:     "github scheme with only owner/repo",
			input:    "github://owner/repo@develop",
			expected: "https://raw.githubusercontent.com/owner/repo/develop",
		},
		{
			name:     "github scheme minimal",
			input:    "github://owner/repo",
			expected: "https://raw.githubusercontent.com/owner/repo/main",
		},

		// Error cases
		{
			name:        "invalid URL",
			input:       "not a url",
			expectError: true,
		},
		{
			name:        "unsupported host",
			input:       "https://gitlab.com/owner/repo/blob/main/file.yaml",
			expectError: true,
		},
		{
			name:        "missing owner/repo",
			input:       "https://github.com/",
			expectError: true,
		},
		{
			name:        "only owner, no repo",
			input:       "https://github.com/owner",
			expectError: true,
		},
		{
			name:        "invalid blob/tree indicator",
			input:       "https://github.com/owner/repo/invalid/main/file.yaml",
			expectError: true,
		},
		{
			name:        "github scheme with only owner",
			input:       "github://owner",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ConvertToRawURL(tt.input)

			if tt.expectError {
				assert.Error(t, err)
				assert.Empty(t, result)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestConvertToRawURL_RealWorldExamples(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Aqua registry main branch",
			input:    "https://github.com/aquaproj/aqua-registry/tree/main/pkgs",
			expected: "https://raw.githubusercontent.com/aquaproj/aqua-registry/main/pkgs",
		},
		{
			name:     "Aqua registry specific tool",
			input:    "https://github.com/aquaproj/aqua-registry/blob/main/pkgs/hashicorp/terraform/registry.yaml",
			expected: "https://raw.githubusercontent.com/aquaproj/aqua-registry/main/pkgs/hashicorp/terraform/registry.yaml",
		},
		{
			name:     "Corporate registry with custom scheme",
			input:    "github://mycompany/toolchain-registry/registry.yaml@v1.0.0",
			expected: "https://raw.githubusercontent.com/mycompany/toolchain-registry/v1.0.0/registry.yaml",
		},
		{
			name:     "Atmos config from GitHub",
			input:    "https://github.com/cloudposse/atmos/blob/main/examples/quick-start/atmos.yaml",
			expected: "https://raw.githubusercontent.com/cloudposse/atmos/main/examples/quick-start/atmos.yaml",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ConvertToRawURL(tt.input)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}
