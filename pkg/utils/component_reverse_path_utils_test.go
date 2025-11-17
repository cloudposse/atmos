package utils

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestExtractComponentInfoFromPath(t *testing.T) {
	// Create a temporary directory structure for testing
	tmpDir := t.TempDir()
	terraformBase := filepath.Join(tmpDir, "components", "terraform")
	helmfileBase := filepath.Join(tmpDir, "components", "helmfile")

	// Create test directories
	require.NoError(t, os.MkdirAll(filepath.Join(terraformBase, "vpc", "security-group"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(terraformBase, "networking", "vpc"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(terraformBase, "simple"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(helmfileBase, "app"), 0o755))

	tests := []struct {
		name          string
		atmosConfig   *schema.AtmosConfiguration
		path          string
		want          *ComponentInfo
		wantErr       bool
		errorContains string
	}{
		{
			name: "terraform component with folder prefix",
			atmosConfig: &schema.AtmosConfiguration{
				BasePath:                 tmpDir,
				TerraformDirAbsolutePath: terraformBase,
				Components: schema.Components{
					Terraform: schema.Terraform{
						BasePath: "components/terraform",
					},
				},
			},
			path: filepath.Join(terraformBase, "vpc", "security-group"),
			want: &ComponentInfo{
				ComponentType: "terraform",
				FolderPrefix:  "vpc",
				ComponentName: "security-group",
				FullComponent: "vpc/security-group",
			},
			wantErr: false,
		},
		{
			name: "terraform component with nested folder prefix",
			atmosConfig: &schema.AtmosConfiguration{
				BasePath:                 tmpDir,
				TerraformDirAbsolutePath: terraformBase,
				Components: schema.Components{
					Terraform: schema.Terraform{
						BasePath: "components/terraform",
					},
				},
			},
			path: filepath.Join(terraformBase, "networking", "vpc"),
			want: &ComponentInfo{
				ComponentType: "terraform",
				FolderPrefix:  "networking",
				ComponentName: "vpc",
				FullComponent: "networking/vpc",
			},
			wantErr: false,
		},
		{
			name: "terraform component without folder prefix",
			atmosConfig: &schema.AtmosConfiguration{
				BasePath:                 tmpDir,
				TerraformDirAbsolutePath: terraformBase,
				Components: schema.Components{
					Terraform: schema.Terraform{
						BasePath: "components/terraform",
					},
				},
			},
			path: filepath.Join(terraformBase, "simple"),
			want: &ComponentInfo{
				ComponentType: "terraform",
				FolderPrefix:  "",
				ComponentName: "simple",
				FullComponent: "simple",
			},
			wantErr: false,
		},
		{
			name: "helmfile component",
			atmosConfig: &schema.AtmosConfiguration{
				BasePath:                 tmpDir,
				TerraformDirAbsolutePath: terraformBase, // Need this so terraform type doesn't match
				HelmfileDirAbsolutePath:  helmfileBase,
				Components: schema.Components{
					Terraform: schema.Terraform{
						BasePath: "components/terraform",
					},
					Helmfile: schema.Helmfile{
						BasePath: "components/helmfile",
					},
				},
			},
			path: filepath.Join(helmfileBase, "app"),
			want: &ComponentInfo{
				ComponentType: "helmfile",
				FolderPrefix:  "",
				ComponentName: "app",
				FullComponent: "app",
			},
			wantErr: false,
		},
		{
			name: "path not in component directory",
			atmosConfig: &schema.AtmosConfiguration{
				BasePath:                 tmpDir,
				TerraformDirAbsolutePath: terraformBase,
				Components: schema.Components{
					Terraform: schema.Terraform{
						BasePath: "components/terraform",
					},
				},
			},
			path:          "/tmp/random",
			wantErr:       true,
			errorContains: "not within Atmos component directories",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ExtractComponentInfoFromPath(tt.atmosConfig, tt.path)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want.ComponentType, got.ComponentType)
			assert.Equal(t, tt.want.FolderPrefix, got.FolderPrefix)
			assert.Equal(t, tt.want.ComponentName, got.ComponentName)
			assert.Equal(t, tt.want.FullComponent, got.FullComponent)
		})
	}
}

func TestExtractComponentInfoFromPath_CurrentDirectory(t *testing.T) {
	// Create a temporary directory structure
	tmpDir := t.TempDir()
	terraformBase := filepath.Join(tmpDir, "components", "terraform")
	componentDir := filepath.Join(terraformBase, "vpc")

	require.NoError(t, os.MkdirAll(componentDir, 0o755))

	// Change to the component directory (t.Chdir automatically restores on cleanup)
	t.Chdir(componentDir)

	atmosConfig := &schema.AtmosConfiguration{
		BasePath:                 tmpDir,
		TerraformDirAbsolutePath: terraformBase,
		Components: schema.Components{
			Terraform: schema.Terraform{
				BasePath: "components/terraform",
			},
		},
	}

	// Test with "." (current directory)
	got, err := ExtractComponentInfoFromPath(atmosConfig, ".")
	require.NoError(t, err)

	assert.Equal(t, "terraform", got.ComponentType)
	assert.Equal(t, "", got.FolderPrefix)
	assert.Equal(t, "vpc", got.ComponentName)
	assert.Equal(t, "vpc", got.FullComponent)
}

func TestNormalizePathForResolution(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		wantAbs bool
		wantErr bool
	}{
		{
			name:    "absolute path",
			path:    "/tmp/test",
			wantAbs: true,
			wantErr: false,
		},
		{
			name:    "relative path",
			path:    "./test",
			wantAbs: true,
			wantErr: false,
		},
		{
			name:    "current directory",
			path:    ".",
			wantAbs: true,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizePathForResolution(tt.path)

			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			if tt.wantAbs {
				assert.True(t, filepath.IsAbs(got), "path should be absolute")
			}
		})
	}
}

func TestExtractComponentInfoFromPath_EnvironmentVariableOverride(t *testing.T) {
	// Create a temporary directory structure
	tmpDir := t.TempDir()
	customBase := filepath.Join(tmpDir, "custom", "tf")
	componentDir := filepath.Join(customBase, "vpc")

	require.NoError(t, os.MkdirAll(componentDir, 0o755))

	// Set environment variable override
	t.Setenv("ATMOS_COMPONENTS_TERRAFORM_BASE_PATH", customBase)

	atmosConfig := &schema.AtmosConfiguration{
		BasePath:                 tmpDir,
		TerraformDirAbsolutePath: "", // Will be overridden by env var
		Components: schema.Components{
			Terraform: schema.Terraform{
				BasePath: "components/terraform", // This should be overridden
			},
		},
	}

	got, err := ExtractComponentInfoFromPath(atmosConfig, componentDir)
	require.NoError(t, err)

	assert.Equal(t, "terraform", got.ComponentType)
	assert.Equal(t, "", got.FolderPrefix)
	assert.Equal(t, "vpc", got.ComponentName)
	assert.Equal(t, "vpc", got.FullComponent)
}
