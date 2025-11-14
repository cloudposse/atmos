package profile

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/profile"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestProfileShowFormatFlagCompletion tests the format flag completion function.
func TestProfileShowFormatFlagCompletion(t *testing.T) {
	cmd := &cobra.Command{}
	args := []string{}
	toComplete := ""

	formats, directive := profileShowFormatFlagCompletion(cmd, args, toComplete)

	assert.Len(t, formats, 3)
	assert.Contains(t, formats, "text")
	assert.Contains(t, formats, "json")
	assert.Contains(t, formats, "yaml")
	assert.Equal(t, cobra.ShellCompDirectiveNoFileComp, directive)
}

// TestProfileNameCompletion tests the profile name completion function.
func TestProfileNameCompletion(t *testing.T) {
	tests := []struct {
		name           string
		args           []string
		expectedCount  int
		shouldComplete bool
	}{
		{
			name:           "no args provided - should complete",
			args:           []string{},
			expectedCount:  0, // Will vary based on test environment.
			shouldComplete: true,
		},
		{
			name:           "profile already provided - should not complete",
			args:           []string{"dev"},
			expectedCount:  0,
			shouldComplete: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &cobra.Command{}
			toComplete := ""

			names, directive := profileNameCompletion(cmd, tt.args, toComplete)

			assert.Equal(t, cobra.ShellCompDirectiveNoFileComp, directive)

			if !tt.shouldComplete {
				assert.Empty(t, names)
			}
			// Note: When shouldComplete is true, the actual names depend on config.
			// We just verify it doesn't error.
		})
	}
}

// TestBuildProfileNotFoundError tests the profile not found error builder.
func TestBuildProfileNotFoundError(t *testing.T) {
	profileName := "nonexistent"

	err := buildProfileNotFoundError(profileName)

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrProfileNotFound)
}

// TestBuildInvalidFormatError tests the invalid format error builder.
func TestBuildInvalidFormatError(t *testing.T) {
	format := "invalid"

	err := buildInvalidFormatError(format)

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrInvalidFormat)
}

// TestRenderProfileJSON tests the JSON rendering function.
func TestRenderProfileJSON(t *testing.T) {
	tests := []struct {
		name        string
		profileInfo *profile.ProfileInfo
		validate    func(t *testing.T, output string)
	}{
		{
			name: "basic profile",
			profileInfo: &profile.ProfileInfo{
				Name:         "dev",
				Path:         "/path/to/dev",
				LocationType: "project",
				Files:        []string{"atmos.yaml"},
			},
			validate: func(t *testing.T, output string) {
				assert.Contains(t, output, `"Name": "dev"`)
				assert.Contains(t, output, `"Path": "/path/to/dev"`)
				assert.Contains(t, output, `"LocationType": "project"`)

				// Verify valid JSON.
				var result map[string]interface{}
				err := json.Unmarshal([]byte(output), &result)
				require.NoError(t, err)
			},
		},
		{
			name: "profile with metadata",
			profileInfo: &profile.ProfileInfo{
				Name:         "production",
				Path:         "/path/to/production",
				LocationType: "xdg",
				Files:        []string{"atmos.yaml", "vpc.yaml"},
				Metadata: &schema.ConfigMetadata{
					Name:        "Production",
					Description: "Production environment",
					Version:     "1.0.0",
					Tags:        []string{"prod", "aws"},
					Deprecated:  false,
				},
			},
			validate: func(t *testing.T, output string) {
				assert.Contains(t, output, `"Name": "production"`)
				assert.Contains(t, output, `"Production"`)
				assert.Contains(t, output, `"Production environment"`)

				// Verify valid JSON.
				var result map[string]interface{}
				err := json.Unmarshal([]byte(output), &result)
				require.NoError(t, err)

				// Verify metadata is included.
				metadata, ok := result["Metadata"].(map[string]interface{})
				require.True(t, ok)
				assert.Equal(t, "Production", metadata["name"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output, err := renderProfileJSON(tt.profileInfo)

			require.NoError(t, err)
			require.NotEmpty(t, output)

			if tt.validate != nil {
				tt.validate(t, output)
			}
		})
	}
}

// TestRenderProfileYAML tests the YAML rendering function.
func TestRenderProfileYAML(t *testing.T) {
	tests := []struct {
		name        string
		profileInfo *profile.ProfileInfo
		validate    func(t *testing.T, output string)
	}{
		{
			name: "basic profile",
			profileInfo: &profile.ProfileInfo{
				Name:         "staging",
				Path:         "/path/to/staging",
				LocationType: "project-hidden",
				Files:        []string{"atmos.yaml", "config.yaml"},
			},
			validate: func(t *testing.T, output string) {
				assert.Contains(t, output, "name: staging")
				assert.Contains(t, output, "path: /path/to/staging")
				assert.Contains(t, output, "locationtype: project-hidden")

				// Verify valid YAML.
				var result map[string]interface{}
				err := yaml.Unmarshal([]byte(output), &result)
				require.NoError(t, err)
			},
		},
		{
			name: "profile with deprecated metadata",
			profileInfo: &profile.ProfileInfo{
				Name:         "legacy",
				Path:         "/path/to/legacy",
				LocationType: "project",
				Files:        []string{"atmos.yaml"},
				Metadata: &schema.ConfigMetadata{
					Name:        "Legacy",
					Description: "Deprecated profile",
					Deprecated:  true,
				},
			},
			validate: func(t *testing.T, output string) {
				assert.Contains(t, output, "name: legacy")
				assert.Contains(t, output, "deprecated: true")

				// Verify valid YAML.
				var result map[string]interface{}
				err := yaml.Unmarshal([]byte(output), &result)
				require.NoError(t, err)

				// Verify metadata is included.
				metadata, ok := result["metadata"].(map[string]interface{})
				require.True(t, ok)
				assert.Equal(t, true, metadata["deprecated"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output, err := renderProfileYAML(tt.profileInfo)

			require.NoError(t, err)
			require.NotEmpty(t, output)

			if tt.validate != nil {
				tt.validate(t, output)
			}
		})
	}
}

// TestRenderProfileOutput tests the format dispatcher function.
func TestRenderProfileOutput(t *testing.T) {
	profileInfo := &profile.ProfileInfo{
		Name:         "test",
		Path:         "/path/to/test",
		LocationType: "project",
		Files:        []string{"atmos.yaml"},
	}

	tests := []struct {
		name        string
		format      string
		expectError bool
		validate    func(t *testing.T, output string, err error)
	}{
		{
			name:        "text format",
			format:      "text",
			expectError: false,
			validate: func(t *testing.T, output string, err error) {
				require.NoError(t, err)
				assert.NotEmpty(t, output)
			},
		},
		{
			name:        "json format",
			format:      "json",
			expectError: false,
			validate: func(t *testing.T, output string, err error) {
				require.NoError(t, err)
				assert.Contains(t, output, `"Name": "test"`)

				// Verify valid JSON.
				var result map[string]interface{}
				jsonErr := json.Unmarshal([]byte(output), &result)
				require.NoError(t, jsonErr)
			},
		},
		{
			name:        "yaml format",
			format:      "yaml",
			expectError: false,
			validate: func(t *testing.T, output string, err error) {
				require.NoError(t, err)
				assert.Contains(t, output, "name: test")

				// Verify valid YAML.
				var result map[string]interface{}
				yamlErr := yaml.Unmarshal([]byte(output), &result)
				require.NoError(t, yamlErr)
			},
		},
		{
			name:        "invalid format",
			format:      "invalid",
			expectError: true,
			validate: func(t *testing.T, output string, err error) {
				require.Error(t, err)
				assert.ErrorIs(t, err, errUtils.ErrInvalidFormat)
				assert.Empty(t, output)
			},
		},
		{
			name:        "empty format defaults to text",
			format:      "",
			expectError: true,
			validate: func(t *testing.T, output string, err error) {
				// Empty format is invalid - should error.
				require.Error(t, err)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output, err := renderProfileOutput(profileInfo, tt.format)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			if tt.validate != nil {
				tt.validate(t, output, err)
			}
		})
	}
}

// TestGetProfileInfo tests the profile info retrieval function.
func TestGetProfileInfo(t *testing.T) {
	tests := []struct {
		name        string
		profileName string
		setupEnv    func(t *testing.T) (*schema.AtmosConfiguration, func())
		expectError bool
		errorType   error
	}{
		{
			name:        "profile exists",
			profileName: "dev",
			setupEnv: func(t *testing.T) (*schema.AtmosConfiguration, func()) {
				// Create temporary directory structure.
				tmpDir := t.TempDir()
				profileDir := filepath.Join(tmpDir, "profiles", "dev")
				require.NoError(t, os.MkdirAll(profileDir, 0o755))

				// Create atmos.yaml in profile.
				atmosYaml := filepath.Join(profileDir, "atmos.yaml")
				require.NoError(t, os.WriteFile(atmosYaml, []byte("base_path: /stacks"), 0o644))

				atmosConfig := &schema.AtmosConfiguration{
					CliConfigPath: tmpDir,
					Profiles: schema.ProfilesConfig{
						BasePath: "",
					},
				}

				return atmosConfig, func() {}
			},
			expectError: false,
		},
		{
			name:        "profile does not exist",
			profileName: "nonexistent",
			setupEnv: func(t *testing.T) (*schema.AtmosConfiguration, func()) {
				tmpDir := t.TempDir()

				atmosConfig := &schema.AtmosConfiguration{
					CliConfigPath: tmpDir,
					Profiles: schema.ProfilesConfig{
						BasePath: "",
					},
				}

				return atmosConfig, func() {}
			},
			expectError: true,
			errorType:   errUtils.ErrProfileNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			atmosConfig, cleanup := tt.setupEnv(t)
			defer cleanup()

			profileInfo, err := getProfileInfo(atmosConfig, tt.profileName)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, profileInfo)

				if tt.errorType != nil {
					assert.ErrorIs(t, err, tt.errorType)
				}
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, profileInfo)
				assert.Equal(t, tt.profileName, profileInfo.Name)
			}
		})
	}
}

// TestRenderProfile_EdgeCases tests edge cases for profile rendering.
func TestRenderProfile_EdgeCases(t *testing.T) {
	tests := []struct {
		name        string
		profileInfo *profile.ProfileInfo
		format      string
		expectError bool
	}{
		{
			name: "profile with no files",
			profileInfo: &profile.ProfileInfo{
				Name:         "empty",
				Path:         "/path/to/empty",
				LocationType: "project",
				Files:        []string{},
			},
			format:      "json",
			expectError: false,
		},
		{
			name: "profile with many files",
			profileInfo: &profile.ProfileInfo{
				Name:         "complex",
				Path:         "/path/to/complex",
				LocationType: "configurable",
				Files: []string{
					"atmos.yaml",
					"stacks/dev.yaml",
					"stacks/prod.yaml",
					"components/vpc.yaml",
					"components/eks.yaml",
				},
			},
			format:      "yaml",
			expectError: false,
		},
		{
			name: "profile with nil metadata",
			profileInfo: &profile.ProfileInfo{
				Name:         "no-metadata",
				Path:         "/path/to/no-metadata",
				LocationType: "project",
				Files:        []string{"atmos.yaml"},
				Metadata:     nil,
			},
			format:      "json",
			expectError: false,
		},
		{
			name: "profile with empty metadata",
			profileInfo: &profile.ProfileInfo{
				Name:         "empty-metadata",
				Path:         "/path/to/empty-metadata",
				LocationType: "xdg",
				Files:        []string{"atmos.yaml"},
				Metadata:     &schema.ConfigMetadata{},
			},
			format:      "yaml",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output, err := renderProfileOutput(tt.profileInfo, tt.format)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.NotEmpty(t, output)
			}
		})
	}
}
