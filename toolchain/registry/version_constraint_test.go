package registry

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test the version override application logic.
func TestApplyVersionOverride(t *testing.T) {
	tests := []struct {
		name             string
		tool             *Tool
		version          string
		expectedAsset    string
		expectedFormat   string
		expectOverride   bool
	}{
		{
			name: "simple true constraint matches",
			tool: &Tool{
				Asset:  "default-asset",
				Format: "tar.gz",
				VersionOverrides: []VersionOverride{
					{
						VersionConstraint: "true",
						Asset:             "override-asset",
						Format:            "zip",
					},
				},
			},
			version:          "1.2.3",
			expectedAsset:    "override-asset",
			expectedFormat:   "zip",
			expectOverride:   true,
		},
		{
			name: "exact version match",
			tool: &Tool{
				Asset:  "default-asset",
				Format: "tar.gz",
				VersionOverrides: []VersionOverride{
					{
						VersionConstraint: `Version == "v1.2.3"`,
						Asset:             "exact-match-asset",
						Format:            "raw",
					},
				},
			},
			version:          "v1.2.3",
			expectedAsset:    "exact-match-asset",
			expectedFormat:   "raw",
			expectOverride:   true,
		},
		{
			name: "exact version no match keeps defaults",
			tool: &Tool{
				Asset:  "default-asset",
				Format: "tar.gz",
				VersionOverrides: []VersionOverride{
					{
						VersionConstraint: `Version == "v1.2.3"`,
						Asset:             "exact-match-asset",
						Format:            "raw",
					},
				},
			},
			version:          "v1.2.4",
			expectedAsset:    "default-asset",
			expectedFormat:   "tar.gz",
			expectOverride:   false,
		},
		{
			name: "semver constraint matches",
			tool: &Tool{
				Asset:  "default-asset",
				Format: "tar.gz",
				VersionOverrides: []VersionOverride{
					{
						VersionConstraint: `semver(">= 1.0.0")`,
						Asset:             "new-format-asset",
						Format:            "zip",
					},
				},
			},
			version:          "1.5.0",
			expectedAsset:    "new-format-asset",
			expectedFormat:   "zip",
			expectOverride:   true,
		},
		{
			name: "first matching override wins",
			tool: &Tool{
				Asset:  "default-asset",
				Format: "tar.gz",
				VersionOverrides: []VersionOverride{
					{
						VersionConstraint: `semver("<= 1.5.0")`,
						Asset:             "old-format",
						Format:            "zip",
					},
					{
						VersionConstraint: "true",
						Asset:             "catch-all",
						Format:            "tar.gz",
					},
				},
			},
			version:          "1.2.0",
			expectedAsset:    "old-format",
			expectedFormat:   "zip",
			expectOverride:   true,
		},
		{
			name: "catch-all after specific constraint",
			tool: &Tool{
				Asset:  "default-asset",
				Format: "tar.gz",
				VersionOverrides: []VersionOverride{
					{
						VersionConstraint: `semver("<= 1.5.0")`,
						Asset:             "old-format",
						Format:            "zip",
					},
					{
						VersionConstraint: "true",
						Asset:             "catch-all",
						Format:            "tar.gz",
					},
				},
			},
			version:          "2.0.0",
			expectedAsset:    "catch-all",
			expectedFormat:   "tar.gz",
			expectOverride:   true,
		},
		{
			name: "partial override only changes asset",
			tool: &Tool{
				Asset:  "default-asset",
				Format: "tar.gz",
				VersionOverrides: []VersionOverride{
					{
						VersionConstraint: "true",
						Asset:             "override-asset",
						// Format not specified
					},
				},
			},
			version:          "1.2.3",
			expectedAsset:    "override-asset",
			expectedFormat:   "tar.gz", // Should keep original
			expectOverride:   true,
		},
		{
			name: "no overrides uses defaults",
			tool: &Tool{
				Asset:            "default-asset",
				Format:           "tar.gz",
				VersionOverrides: []VersionOverride{},
			},
			version:          "1.2.3",
			expectedAsset:    "default-asset",
			expectedFormat:   "tar.gz",
			expectOverride:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Make a copy of the tool to avoid test interference.
			toolCopy := *tt.tool

			err := applyVersionOverride(&toolCopy, tt.version)
			require.NoError(t, err, "applyVersionOverride should not return error")

			assert.Equal(t, tt.expectedAsset, toolCopy.Asset, "asset should match expected")
			assert.Equal(t, tt.expectedFormat, toolCopy.Format, "format should match expected")
		})
	}
}

// Test real-world scenarios from Aqua registry.
func TestApplyVersionOverride_RealWorld(t *testing.T) {
	tests := []struct {
		name    string
		tool    *Tool
		version string
		want    struct {
			asset  string
			format string
		}
	}{
		{
			name: "terraform-backend-git raw binary",
			tool: &Tool{
				VersionOverrides: []VersionOverride{
					{
						VersionConstraint: "true",
						Asset:             "terraform-backend-git-{{.OS}}-{{.Arch}}",
						Format:            "raw",
					},
				},
			},
			version: "0.1.8",
			want: struct {
				asset  string
				format string
			}{
				asset:  "terraform-backend-git-{{.OS}}-{{.Arch}}",
				format: "raw",
			},
		},
		{
			name: "opentofu versioned format",
			tool: &Tool{
				VersionOverrides: []VersionOverride{
					{
						VersionConstraint: `semver("<= 1.6.0-beta4")`,
						Asset:             "tofu_{{trimV .Version}}_{{.OS}}_{{.Arch}}.{{.Format}}",
						Format:            "zip",
					},
					{
						VersionConstraint: "true",
						Asset:             "tofu_{{trimV .Version}}_{{.OS}}_{{.Arch}}.{{.Format}}",
						Format:            "tar.gz",
					},
				},
			},
			version: "1.10.7",
			want: struct {
				asset  string
				format string
			}{
				asset:  "tofu_{{trimV .Version}}_{{.OS}}_{{.Arch}}.{{.Format}}",
				format: "tar.gz",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			toolCopy := *tt.tool
			err := applyVersionOverride(&toolCopy, tt.version)
			require.NoError(t, err)

			assert.Equal(t, tt.want.asset, toolCopy.Asset)
			assert.Equal(t, tt.want.format, toolCopy.Format)
		})
	}
}
