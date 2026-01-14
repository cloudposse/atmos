package show

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/profile"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestRenderProfile tests the RenderProfile function.
func TestRenderProfile(t *testing.T) {
	tests := []struct {
		name                string
		profileInfo         *profile.ProfileInfo
		expectedContains    []string
		expectedNotContains []string
	}{
		{
			name: "profile with all metadata fields",
			profileInfo: &profile.ProfileInfo{
				Name:         "production",
				Path:         "/path/to/profiles/production",
				LocationType: "project",
				Files: []string{
					"atmos.yaml",
					"stacks/vpc.yaml",
					"stacks/eks.yaml",
				},
				Metadata: &schema.ConfigMetadata{
					Name:        "Production Environment",
					Description: "Production deployment configuration",
					Version:     "2.1.0",
					Tags:        []string{"production", "aws", "critical"},
					Deprecated:  false,
				},
			},
			expectedContains: []string{
				"PROFILE: production",
				"Location Type:",
				"project",
				"Path:",
				"/path/to/profiles/production",
				"METADATA",
				"Name:",
				"Production Environment",
				"Description:",
				"Production deployment configuration",
				"Version:",
				"2.1.0",
				"Tags:",
				"production, aws, critical",
				"FILES",
				"atmos.yaml",
				"stacks/vpc.yaml",
				"stacks/eks.yaml",
				"Use with: atmos --profile production <command>",
			},
			expectedNotContains: []string{
				"DEPRECATED",
			},
		},
		{
			name: "deprecated profile",
			profileInfo: &profile.ProfileInfo{
				Name:         "legacy",
				Path:         "/path/to/profiles/legacy",
				LocationType: "xdg",
				Files: []string{
					"atmos.yaml",
				},
				Metadata: &schema.ConfigMetadata{
					Name:        "Legacy Profile",
					Description: "Deprecated - use modern profile instead",
					Deprecated:  true,
				},
			},
			expectedContains: []string{
				"PROFILE: legacy",
				"Location Type:",
				"xdg",
				"METADATA",
				"Name:",
				"Legacy Profile",
				"Description:",
				"Deprecated - use modern profile instead",
				"Status:",
				"DEPRECATED",
				"FILES",
				"atmos.yaml",
			},
			expectedNotContains: []string{
				"Version:",
				"Tags:",
			},
		},
		{
			name: "profile with no metadata",
			profileInfo: &profile.ProfileInfo{
				Name:         "dev",
				Path:         "/path/to/profiles/dev",
				LocationType: "project-hidden",
				Files: []string{
					"atmos.yaml",
					"config.yaml",
				},
				Metadata: nil,
			},
			expectedContains: []string{
				"PROFILE: dev",
				"Location Type:",
				"project-hidden",
				"Path:",
				"/path/to/profiles/dev",
				"FILES",
				"atmos.yaml",
				"config.yaml",
				"Use with: atmos --profile dev <command>",
			},
			expectedNotContains: []string{
				"METADATA",
				"Name:",
				"Description:",
				"Version:",
				"Tags:",
				"DEPRECATED",
			},
		},
		{
			name: "profile with no files",
			profileInfo: &profile.ProfileInfo{
				Name:         "empty",
				Path:         "/path/to/profiles/empty",
				LocationType: "configurable",
				Files:        []string{},
				Metadata:     nil,
			},
			expectedContains: []string{
				"PROFILE: empty",
				"Location Type:",
				"configurable",
				"FILES",
				"No configuration files found",
				"Use with: atmos --profile empty <command>",
			},
			expectedNotContains: []string{
				"METADATA",
			},
		},
		{
			name: "profile with partial metadata",
			profileInfo: &profile.ProfileInfo{
				Name:         "staging",
				Path:         "/path/to/profiles/staging",
				LocationType: "project",
				Files: []string{
					"atmos.yaml",
				},
				Metadata: &schema.ConfigMetadata{
					Name:        "Staging Environment",
					Description: "",
					Version:     "",
					Tags:        nil,
					Deprecated:  false,
				},
			},
			expectedContains: []string{
				"PROFILE: staging",
				"METADATA",
				"Name:",
				"Staging Environment",
				"FILES",
				"atmos.yaml",
			},
			expectedNotContains: []string{
				"Description:",
				"Version:",
				"Tags:",
				"DEPRECATED",
			},
		},
		{
			name: "profile with only tags in metadata",
			profileInfo: &profile.ProfileInfo{
				Name:         "tagged",
				Path:         "/path/to/profiles/tagged",
				LocationType: "project",
				Files:        []string{"atmos.yaml"},
				Metadata: &schema.ConfigMetadata{
					Tags: []string{"test", "ci", "automated"},
				},
			},
			expectedContains: []string{
				"PROFILE: tagged",
				"METADATA",
				"Tags:",
				"test, ci, automated",
			},
			expectedNotContains: []string{
				"Name:",
				"Description:",
				"Version:",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output, err := RenderProfile(tt.profileInfo)

			require.NoError(t, err)
			require.NotEmpty(t, output)

			// Check for expected content.
			for _, expected := range tt.expectedContains {
				assert.Contains(t, output, expected,
					"Output should contain: %s", expected)
			}

			// Check for unexpected content.
			for _, notExpected := range tt.expectedNotContains {
				assert.NotContains(t, output, notExpected,
					"Output should not contain: %s", notExpected)
			}
		})
	}
}

// TestRenderProfile_StructuralChecks tests the structure of rendered output.
func TestRenderProfile_StructuralChecks(t *testing.T) {
	profileInfo := &profile.ProfileInfo{
		Name:         "test",
		Path:         "/test/path",
		LocationType: "project",
		Files:        []string{"file1.yaml", "file2.yaml"},
		Metadata: &schema.ConfigMetadata{
			Name:        "Test Profile",
			Description: "Test description",
			Version:     "1.0.0",
			Tags:        []string{"test"},
			Deprecated:  false,
		},
	}

	output, err := RenderProfile(profileInfo)
	require.NoError(t, err)

	// Verify output is not empty.
	assert.NotEmpty(t, output)

	// Verify sections appear in correct order.
	profileIdx := strings.Index(output, "PROFILE:")
	metadataIdx := strings.Index(output, "METADATA")
	filesIdx := strings.Index(output, "FILES")
	usageIdx := strings.Index(output, "Use with:")

	assert.Greater(t, metadataIdx, profileIdx, "METADATA should come after PROFILE")
	assert.Greater(t, filesIdx, metadataIdx, "FILES should come after METADATA")
	assert.Greater(t, usageIdx, filesIdx, "Usage hint should come after FILES")

	// Verify each file is listed.
	for _, file := range profileInfo.Files {
		assert.Contains(t, output, file)
	}
}

// TestRenderProfile_EdgeCases tests edge cases and boundary conditions.
func TestRenderProfile_EdgeCases(t *testing.T) {
	tests := []struct {
		name        string
		profileInfo *profile.ProfileInfo
	}{
		{
			name: "profile with very long path",
			profileInfo: &profile.ProfileInfo{
				Name:         "long-path",
				Path:         "/very/long/path/to/profiles/that/might/need/truncation/in/some/displays/long-path",
				LocationType: "project",
				Files:        []string{"atmos.yaml"},
			},
		},
		{
			name: "profile with special characters in name",
			profileInfo: &profile.ProfileInfo{
				Name:         "profile-with-dashes_and_underscores",
				Path:         "/path/to/profile",
				LocationType: "project",
				Files:        []string{"atmos.yaml"},
			},
		},
		{
			name: "profile with many files",
			profileInfo: &profile.ProfileInfo{
				Name:         "many-files",
				Path:         "/path/to/profile",
				LocationType: "project",
				Files: []string{
					"file1.yaml", "file2.yaml", "file3.yaml",
					"file4.yaml", "file5.yaml", "file6.yaml",
					"file7.yaml", "file8.yaml", "file9.yaml",
					"file10.yaml",
				},
			},
		},
		{
			name: "profile with many tags",
			profileInfo: &profile.ProfileInfo{
				Name:         "many-tags",
				Path:         "/path/to/profile",
				LocationType: "project",
				Files:        []string{"atmos.yaml"},
				Metadata: &schema.ConfigMetadata{
					Tags: []string{
						"tag1", "tag2", "tag3", "tag4", "tag5",
						"tag6", "tag7", "tag8", "tag9", "tag10",
					},
				},
			},
		},
		{
			name: "profile with unicode characters in metadata",
			profileInfo: &profile.ProfileInfo{
				Name:         "unicode",
				Path:         "/path/to/profile",
				LocationType: "project",
				Files:        []string{"atmos.yaml"},
				Metadata: &schema.ConfigMetadata{
					Name:        "Profile with émojis 🚀",
					Description: "Testing unicode: ñ, 中文, العربية",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output, err := RenderProfile(tt.profileInfo)

			require.NoError(t, err)
			assert.NotEmpty(t, output)

			// Basic validation that output contains profile name.
			assert.Contains(t, output, tt.profileInfo.Name)
		})
	}
}

// TestRenderMetadata tests the renderMetadata helper function indirectly.
func TestRenderMetadata(t *testing.T) {
	tests := []struct {
		name             string
		metadata         *schema.ConfigMetadata
		expectedContains []string
	}{
		{
			name: "all fields populated",
			metadata: &schema.ConfigMetadata{
				Name:        "Full Metadata",
				Description: "Complete description",
				Version:     "3.2.1",
				Tags:        []string{"a", "b", "c"},
				Deprecated:  false,
			},
			expectedContains: []string{
				"METADATA",
				"Name:",
				"Full Metadata",
				"Description:",
				"Complete description",
				"Version:",
				"3.2.1",
				"Tags:",
				"a, b, c",
			},
		},
		{
			name: "only name",
			metadata: &schema.ConfigMetadata{
				Name: "Just Name",
			},
			expectedContains: []string{
				"METADATA",
				"Name:",
				"Just Name",
			},
		},
		{
			name: "deprecated with description",
			metadata: &schema.ConfigMetadata{
				Description: "Old profile",
				Deprecated:  true,
			},
			expectedContains: []string{
				"METADATA",
				"Description:",
				"Old profile",
				"Status:",
				"DEPRECATED",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			profileInfo := &profile.ProfileInfo{
				Name:         "test",
				Path:         "/test",
				LocationType: "project",
				Files:        []string{"atmos.yaml"},
				Metadata:     tt.metadata,
			}

			output, err := RenderProfile(profileInfo)
			require.NoError(t, err)

			for _, expected := range tt.expectedContains {
				assert.Contains(t, output, expected)
			}
		})
	}
}
