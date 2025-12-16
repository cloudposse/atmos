package source

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestNeedsVendoring(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(t *testing.T) string
		expected bool
	}{
		{
			name: "non-existent directory",
			setup: func(t *testing.T) string {
				return filepath.Join(t.TempDir(), "does-not-exist")
			},
			expected: true,
		},
		{
			name: "empty directory",
			setup: func(t *testing.T) string {
				dir := t.TempDir()
				emptyDir := filepath.Join(dir, "empty")
				err := os.MkdirAll(emptyDir, 0o755)
				require.NoError(t, err)
				return emptyDir
			},
			expected: true,
		},
		{
			name: "directory with files",
			setup: func(t *testing.T) string {
				dir := t.TempDir()
				populatedDir := filepath.Join(dir, "populated")
				err := os.MkdirAll(populatedDir, 0o755)
				require.NoError(t, err)
				err = os.WriteFile(filepath.Join(populatedDir, "main.tf"), []byte("# test"), 0o644)
				require.NoError(t, err)
				return populatedDir
			},
			expected: false,
		},
		{
			name: "file instead of directory",
			setup: func(t *testing.T) string {
				dir := t.TempDir()
				filePath := filepath.Join(dir, "file.txt")
				err := os.WriteFile(filePath, []byte("test"), 0o644)
				require.NoError(t, err)
				return filePath
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			targetDir := tt.setup(t)
			result := needsVendoring(targetDir)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDetermineTargetDirectory(t *testing.T) {
	tests := []struct {
		name            string
		atmosConfig     *schema.AtmosConfiguration
		componentType   string
		component       string
		componentConfig map[string]any
		expectedDir     string
		expectError     error
	}{
		{
			name: "working_directory in metadata takes priority",
			atmosConfig: &schema.AtmosConfiguration{
				Components: schema.Components{
					Terraform: schema.Terraform{
						BasePath: "components/terraform",
					},
				},
			},
			componentType: "terraform",
			component:     "vpc",
			componentConfig: map[string]any{
				"metadata": map[string]any{
					"working_directory": "/custom/path/vpc",
				},
			},
			expectedDir: "/custom/path/vpc",
			expectError: nil,
		},
		{
			name: "working_directory in settings takes priority over default",
			atmosConfig: &schema.AtmosConfiguration{
				Components: schema.Components{
					Terraform: schema.Terraform{
						BasePath: "components/terraform",
					},
				},
			},
			componentType: "terraform",
			component:     "vpc",
			componentConfig: map[string]any{
				"settings": map[string]any{
					"working_directory": "/settings/path/vpc",
				},
			},
			expectedDir: "/settings/path/vpc",
			expectError: nil,
		},
		{
			name: "default terraform base path",
			atmosConfig: &schema.AtmosConfiguration{
				Components: schema.Components{
					Terraform: schema.Terraform{
						BasePath: "components/terraform",
					},
				},
			},
			componentType:   "terraform",
			component:       "vpc",
			componentConfig: map[string]any{},
			expectedDir:     "components/terraform/vpc",
			expectError:     nil,
		},
		{
			name: "default helmfile base path",
			atmosConfig: &schema.AtmosConfiguration{
				Components: schema.Components{
					Helmfile: schema.Helmfile{
						BasePath: "components/helmfile",
					},
				},
			},
			componentType:   "helmfile",
			component:       "nginx",
			componentConfig: map[string]any{},
			expectedDir:     "components/helmfile/nginx",
			expectError:     nil,
		},
		{
			name: "no base path configured for terraform",
			atmosConfig: &schema.AtmosConfiguration{
				Components: schema.Components{
					Terraform: schema.Terraform{
						BasePath: "",
					},
				},
			},
			componentType:   "terraform",
			component:       "vpc",
			componentConfig: map[string]any{},
			expectedDir:     "",
			expectError:     errUtils.ErrInvalidConfig,
		},
		{
			name:            "nil atmos config",
			atmosConfig:     nil,
			componentType:   "terraform",
			component:       "vpc",
			componentConfig: map[string]any{},
			expectedDir:     "",
			expectError:     errUtils.ErrInvalidConfig,
		},
		{
			name: "unknown component type",
			atmosConfig: &schema.AtmosConfiguration{
				Components: schema.Components{
					Terraform: schema.Terraform{
						BasePath: "components/terraform",
					},
				},
			},
			componentType:   "unknown",
			component:       "test",
			componentConfig: map[string]any{},
			expectedDir:     "",
			expectError:     errUtils.ErrInvalidConfig,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := DetermineTargetDirectory(tt.atmosConfig, tt.componentType, tt.component, tt.componentConfig)

			if tt.expectError != nil {
				require.Error(t, err)
				assert.ErrorIs(t, err, tt.expectError)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expectedDir, result)
			}
		})
	}
}

func TestGetComponentBasePath(t *testing.T) {
	tests := []struct {
		name          string
		atmosConfig   *schema.AtmosConfiguration
		componentType string
		expected      string
	}{
		{
			name: "terraform component type",
			atmosConfig: &schema.AtmosConfiguration{
				Components: schema.Components{
					Terraform: schema.Terraform{
						BasePath: "components/terraform",
					},
				},
			},
			componentType: "terraform",
			expected:      "components/terraform",
		},
		{
			name: "helmfile component type",
			atmosConfig: &schema.AtmosConfiguration{
				Components: schema.Components{
					Helmfile: schema.Helmfile{
						BasePath: "components/helmfile",
					},
				},
			},
			componentType: "helmfile",
			expected:      "components/helmfile",
		},
		{
			name: "unknown component type",
			atmosConfig: &schema.AtmosConfiguration{
				Components: schema.Components{
					Terraform: schema.Terraform{
						BasePath: "components/terraform",
					},
				},
			},
			componentType: "packer",
			expected:      "",
		},
		{
			name:          "nil config",
			atmosConfig:   nil,
			componentType: "terraform",
			expected:      "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getComponentBasePath(tt.atmosConfig, tt.componentType)
			assert.Equal(t, tt.expected, result)
		})
	}
}
