package vendoring

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseSemVer(t *testing.T) {
	tests := []struct {
		name        string
		version     string
		expectError bool
		expected    string
	}{
		{
			name:        "version with v prefix",
			version:     "v1.2.3",
			expectError: false,
			expected:    "1.2.3",
		},
		{
			name:        "version with V prefix",
			version:     "V2.0.0",
			expectError: false,
			expected:    "2.0.0",
		},
		{
			name:        "version without prefix",
			version:     "1.5.0",
			expectError: false,
			expected:    "1.5.0",
		},
		{
			name:        "invalid version",
			version:     "not-a-version",
			expectError: true,
		},
		{
			name:        "version with prerelease",
			version:     "v1.0.0-alpha.1",
			expectError: false,
			expected:    "1.0.0-alpha.1",
		},
		{
			name:        "version with build metadata",
			version:     "1.0.0+build.123",
			expectError: false,
			expected:    "1.0.0+build.123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ver, err := parseSemVer(tt.version)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, ver.String())
			}
		})
	}
}

func TestFindLatestSemVerTag(t *testing.T) {
	tests := []struct {
		name            string
		tags            []string
		expectedVersion string
		expectedTag     string
	}{
		{
			name:            "mixed semantic versions",
			tags:            []string{"v1.0.0", "v1.2.3", "v1.1.0", "v2.0.0", "v1.5.0"},
			expectedVersion: "2.0.0",
			expectedTag:     "v2.0.0",
		},
		{
			name:            "versions with and without v prefix",
			tags:            []string{"1.0.0", "v2.0.0", "1.5.0"},
			expectedVersion: "2.0.0",
			expectedTag:     "v2.0.0",
		},
		{
			name:            "versions with prerelease",
			tags:            []string{"v1.0.0", "v1.1.0-alpha", "v1.0.5"},
			expectedVersion: "1.1.0-alpha",
			expectedTag:     "v1.1.0-alpha",
		},
		{
			name:            "non-semantic version tags ignored",
			tags:            []string{"latest", "main", "v1.0.0", "dev", "v0.5.0"},
			expectedVersion: "1.0.0",
			expectedTag:     "v1.0.0",
		},
		{
			name:            "no semantic versions",
			tags:            []string{"latest", "main", "dev"},
			expectedVersion: "",
			expectedTag:     "",
		},
		{
			name:            "empty tag list",
			tags:            []string{},
			expectedVersion: "",
			expectedTag:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ver, tag := findLatestSemVerTag(tt.tags)

			if tt.expectedVersion == "" {
				assert.Nil(t, ver)
				assert.Empty(t, tag)
			} else {
				require.NotNil(t, ver)
				assert.Equal(t, tt.expectedVersion, ver.String())
				assert.Equal(t, tt.expectedTag, tag)
			}
		})
	}
}

func TestIsValidCommitSHA(t *testing.T) {
	tests := []struct {
		name     string
		ref      string
		expected bool
	}{
		{
			name:     "full SHA",
			ref:      "a1b2c3d4e5f61728192021222324252627282930",
			expected: true,
		},
		{
			name:     "short SHA - 7 chars",
			ref:      "a1b2c3d",
			expected: true,
		},
		{
			name:     "short SHA - 8 chars",
			ref:      "a1b2c3d4",
			expected: true,
		},
		{
			name:     "too short - 6 chars",
			ref:      "a1b2c3",
			expected: false,
		},
		{
			name:     "too long - 41 chars",
			ref:      "a1b2c3d4e5f617281920212223242526272829301",
			expected: false,
		},
		{
			name:     "contains uppercase",
			ref:      "A1B2C3D",
			expected: false,
		},
		{
			name:     "contains invalid characters",
			ref:      "xyz123g",
			expected: false,
		},
		{
			name:     "not a SHA",
			ref:      "v1.0.0",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isValidCommitSHA(tt.ref)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExtractGitURI(t *testing.T) {
	tests := []struct {
		name     string
		source   string
		expected string
	}{
		{
			name:     "git:: prefix",
			source:   "git::https://github.com/cloudposse/terraform-aws-components",
			expected: "https://github.com/cloudposse/terraform-aws-components",
		},
		{
			name:     "github.com shorthand",
			source:   "github.com/cloudposse/terraform-aws-components",
			expected: "https://github.com/cloudposse/terraform-aws-components",
		},
		{
			name:     "https URL",
			source:   "https://github.com/cloudposse/terraform-aws-components.git",
			expected: "https://github.com/cloudposse/terraform-aws-components",
		},
		{
			name:     "git@ SSH URL",
			source:   "git@github.com:cloudposse/terraform-aws-components.git",
			expected: "git@github.com:cloudposse/terraform-aws-components",
		},
		{
			name:     "URL with query parameters",
			source:   "https://github.com/cloudposse/terraform-aws-components?ref=main",
			expected: "https://github.com/cloudposse/terraform-aws-components",
		},
		{
			name:     "complex git:: URL with ref",
			source:   "git::https://github.com/cloudposse/terraform-aws-components.git?ref=tags/0.1.0",
			expected: "https://github.com/cloudposse/terraform-aws-components",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractGitURI(tt.source)
			assert.Equal(t, tt.expected, result)
		})
	}
}
