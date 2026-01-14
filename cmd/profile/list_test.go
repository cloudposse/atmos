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

// TestProfileFormatFlagCompletion tests the format flag completion function.
func TestProfileFormatFlagCompletion(t *testing.T) {
	cmd := &cobra.Command{}
	args := []string{}
	toComplete := ""

	formats, directive := profileFormatFlagCompletion(cmd, args, toComplete)

	assert.Len(t, formats, 3)
	assert.Contains(t, formats, "table")
	assert.Contains(t, formats, "json")
	assert.Contains(t, formats, "yaml")
	assert.Equal(t, cobra.ShellCompDirectiveNoFileComp, directive)
}

// TestRenderProfilesJSON tests the JSON rendering function for profile lists.
func TestRenderProfilesJSON(t *testing.T) {
	tests := []struct {
		name     string
		profiles []profile.ProfileInfo
		validate func(t *testing.T, output string)
	}{
		{
			name:     "empty profile list",
			profiles: []profile.ProfileInfo{},
			validate: func(t *testing.T, output string) {
				assert.Equal(t, "[]\n", output)

				// Verify valid JSON.
				var result []map[string]interface{}
				err := json.Unmarshal([]byte(output), &result)
				require.NoError(t, err)
				assert.Empty(t, result)
			},
		},
		{
			name: "single profile",
			profiles: []profile.ProfileInfo{
				{
					Name:         "dev",
					Path:         "/path/to/dev",
					LocationType: "project",
					Files:        []string{"atmos.yaml"},
				},
			},
			validate: func(t *testing.T, output string) {
				assert.Contains(t, output, `"Name": "dev"`)
				assert.Contains(t, output, `"Path": "/path/to/dev"`)
				assert.Contains(t, output, `"LocationType": "project"`)

				// Verify valid JSON.
				var result []map[string]interface{}
				err := json.Unmarshal([]byte(output), &result)
				require.NoError(t, err)
				assert.Len(t, result, 1)
			},
		},
		{
			name: "multiple profiles with metadata",
			profiles: []profile.ProfileInfo{
				{
					Name:         "production",
					Path:         "/path/to/production",
					LocationType: "xdg",
					Files:        []string{"atmos.yaml", "vpc.yaml"},
					Metadata: &schema.ConfigMetadata{
						Name:        "Production",
						Description: "Production environment",
						Version:     "1.0.0",
						Tags:        []string{"prod", "aws"},
					},
				},
				{
					Name:         "staging",
					Path:         "/path/to/staging",
					LocationType: "project-hidden",
					Files:        []string{"atmos.yaml"},
				},
			},
			validate: func(t *testing.T, output string) {
				assert.Contains(t, output, `"Name": "production"`)
				assert.Contains(t, output, `"Name": "staging"`)
				assert.Contains(t, output, `"Production"`)

				// Verify valid JSON.
				var result []map[string]interface{}
				err := json.Unmarshal([]byte(output), &result)
				require.NoError(t, err)
				assert.Len(t, result, 2)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output, err := renderProfilesJSON(tt.profiles)

			require.NoError(t, err)
			require.NotEmpty(t, output)

			if tt.validate != nil {
				tt.validate(t, output)
			}
		})
	}
}

// TestRenderProfilesYAML tests the YAML rendering function for profile lists.
func TestRenderProfilesYAML(t *testing.T) {
	tests := []struct {
		name     string
		profiles []profile.ProfileInfo
		validate func(t *testing.T, output string)
	}{
		{
			name:     "empty profile list",
			profiles: []profile.ProfileInfo{},
			validate: func(t *testing.T, output string) {
				assert.Equal(t, "[]\n", output)

				// Verify valid YAML.
				var result []map[string]interface{}
				err := yaml.Unmarshal([]byte(output), &result)
				require.NoError(t, err)
				assert.Empty(t, result)
			},
		},
		{
			name: "single profile",
			profiles: []profile.ProfileInfo{
				{
					Name:         "dev",
					Path:         "/path/to/dev",
					LocationType: "project",
					Files:        []string{"atmos.yaml", "config.yaml"},
				},
			},
			validate: func(t *testing.T, output string) {
				assert.Contains(t, output, "name: dev")
				assert.Contains(t, output, "path: /path/to/dev")
				assert.Contains(t, output, "locationtype: project")

				// Verify valid YAML.
				var result []map[string]interface{}
				err := yaml.Unmarshal([]byte(output), &result)
				require.NoError(t, err)
				assert.Len(t, result, 1)
			},
		},
		{
			name: "multiple profiles",
			profiles: []profile.ProfileInfo{
				{
					Name:         "prod",
					Path:         "/path/to/prod",
					LocationType: "configurable",
					Files:        []string{"atmos.yaml"},
					Metadata: &schema.ConfigMetadata{
						Name:       "Production",
						Deprecated: false,
					},
				},
				{
					Name:         "legacy",
					Path:         "/path/to/legacy",
					LocationType: "project",
					Files:        []string{"atmos.yaml"},
					Metadata: &schema.ConfigMetadata{
						Deprecated: true,
					},
				},
			},
			validate: func(t *testing.T, output string) {
				assert.Contains(t, output, "name: prod")
				assert.Contains(t, output, "name: legacy")
				assert.Contains(t, output, "deprecated: true")

				// Verify valid YAML.
				var result []map[string]interface{}
				err := yaml.Unmarshal([]byte(output), &result)
				require.NoError(t, err)
				assert.Len(t, result, 2)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output, err := renderProfilesYAML(tt.profiles)

			require.NoError(t, err)
			require.NotEmpty(t, output)

			if tt.validate != nil {
				tt.validate(t, output)
			}
		})
	}
}

// TestBuildProfileDiscoveryError tests the profile discovery error builder.
func TestBuildProfileDiscoveryError(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		CliConfigPath: "/path/to/config",
		Profiles: schema.ProfilesConfig{
			BasePath: "custom/profiles",
		},
	}

	originalErr := errUtils.ErrProfileDiscovery

	err := buildProfileDiscoveryError(originalErr, atmosConfig)

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrProfileDiscovery)
	assert.Contains(t, err.Error(), "discover profiles")
}

// TestRenderProfileListOutput tests the format dispatcher function.
func TestRenderProfileListOutput(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		CliConfigPath: "/test",
		Profiles:      schema.ProfilesConfig{},
	}

	profiles := []profile.ProfileInfo{
		{
			Name:         "test",
			Path:         "/path/to/test",
			LocationType: "project",
			Files:        []string{"atmos.yaml"},
		},
	}

	tests := []struct {
		name        string
		format      string
		expectError bool
		validate    func(t *testing.T, output string, err error)
	}{
		{
			name:        "table format",
			format:      "table",
			expectError: false,
			validate: func(t *testing.T, output string, err error) {
				require.NoError(t, err)
				assert.NotEmpty(t, output)
				// Table should contain headers.
				assert.Contains(t, output, "PROFILES")
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
				var result []map[string]interface{}
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
				var result []map[string]interface{}
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
			name:        "empty format",
			format:      "",
			expectError: true,
			validate: func(t *testing.T, output string, err error) {
				require.Error(t, err)
				assert.ErrorIs(t, err, errUtils.ErrInvalidFormat)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output, err := renderProfileListOutput(atmosConfig, profiles, tt.format)

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

// TestRenderProfileListOutput_EmptyProfiles tests rendering with no profiles.
func TestRenderProfileListOutput_EmptyProfiles(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		CliConfigPath: "/test",
		Profiles:      schema.ProfilesConfig{},
	}

	profiles := []profile.ProfileInfo{}

	tests := []struct {
		name     string
		format   string
		validate func(t *testing.T, output string)
	}{
		{
			name:   "table format with no profiles",
			format: "table",
			validate: func(t *testing.T, output string) {
				assert.Contains(t, output, "No profiles configured")
			},
		},
		{
			name:   "json format with no profiles",
			format: "json",
			validate: func(t *testing.T, output string) {
				assert.Equal(t, "[]\n", output)

				// Verify valid JSON.
				var result []map[string]interface{}
				err := json.Unmarshal([]byte(output), &result)
				require.NoError(t, err)
				assert.Empty(t, result)
			},
		},
		{
			name:   "yaml format with no profiles",
			format: "yaml",
			validate: func(t *testing.T, output string) {
				assert.Equal(t, "[]\n", output)

				// Verify valid YAML.
				var result []map[string]interface{}
				err := yaml.Unmarshal([]byte(output), &result)
				require.NoError(t, err)
				assert.Empty(t, result)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output, err := renderProfileListOutput(atmosConfig, profiles, tt.format)

			require.NoError(t, err)
			require.NotEmpty(t, output)

			if tt.validate != nil {
				tt.validate(t, output)
			}
		})
	}
}

// TestRenderProfileListOutput_WithComplexProfiles tests rendering with complex profile structures.
func TestRenderProfileListOutput_WithComplexProfiles(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		CliConfigPath: "/test",
		Profiles:      schema.ProfilesConfig{},
	}

	profiles := []profile.ProfileInfo{
		{
			Name:         "production",
			Path:         "/very/long/path/to/production/environment/configuration/directory",
			LocationType: "configurable",
			Files: []string{
				"atmos.yaml",
				"stacks/prod.yaml",
				"stacks/prod-us-east-1.yaml",
				"stacks/prod-us-west-2.yaml",
				"components/vpc.yaml",
			},
			Metadata: &schema.ConfigMetadata{
				Name:        "Production Environment",
				Description: "Production deployment configuration for all regions",
				Version:     "2.1.0",
				Tags:        []string{"production", "aws", "multi-region", "critical"},
				Deprecated:  false,
			},
		},
		{
			Name:         "staging",
			Path:         "/path/to/staging",
			LocationType: "project-hidden",
			Files:        []string{"atmos.yaml"},
			Metadata: &schema.ConfigMetadata{
				Name:        "Staging",
				Description: "Staging environment",
			},
		},
		{
			Name:         "legacy",
			Path:         "/path/to/legacy",
			LocationType: "project",
			Files:        []string{"atmos.yaml"},
			Metadata: &schema.ConfigMetadata{
				Deprecated:  true,
				Description: "Deprecated - use 'production' instead",
			},
		},
	}

	tests := []struct {
		name   string
		format string
	}{
		{
			name:   "complex profiles as table",
			format: "table",
		},
		{
			name:   "complex profiles as json",
			format: "json",
		},
		{
			name:   "complex profiles as yaml",
			format: "yaml",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output, err := renderProfileListOutput(atmosConfig, profiles, tt.format)

			require.NoError(t, err)
			assert.NotEmpty(t, output)

			// Verify profile names are included (table may not show all rows in non-focused mode).
			if tt.format != "table" {
				assert.Contains(t, output, "production")
				assert.Contains(t, output, "staging")
				assert.Contains(t, output, "legacy")
			} else {
				// For table format, just verify headers are present.
				assert.Contains(t, output, "PROFILES")
				assert.Contains(t, output, "NAME")
			}
		})
	}
}

// TestExecuteProfileListCommand_Integration tests the full command execution flow.
func TestExecuteProfileListCommand_Integration(t *testing.T) {
	// Create temporary directory structure with profiles.
	tmpDir := t.TempDir()

	// Create profiles directory.
	profilesDir := filepath.Join(tmpDir, "profiles")
	require.NoError(t, os.MkdirAll(profilesDir, 0o755))

	// Create dev profile.
	devProfile := filepath.Join(profilesDir, "dev")
	require.NoError(t, os.MkdirAll(devProfile, 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(devProfile, "atmos.yaml"),
		[]byte("base_path: /stacks/dev"),
		0o644,
	))

	// Create prod profile.
	prodProfile := filepath.Join(profilesDir, "prod")
	require.NoError(t, os.MkdirAll(prodProfile, 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(prodProfile, "atmos.yaml"),
		[]byte(`
metadata:
  name: Production
  description: Production environment
  tags:
    - production
    - aws
`),
		0o644,
	))

	// This test would require setting up the full atmos config environment.
	// For now, we verify the helper functions work correctly.
	// Full integration tests are covered in the tests/ directory.

	t.Skip("Integration test requires full atmos config setup - covered in tests/")
}
