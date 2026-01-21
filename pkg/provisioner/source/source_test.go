package source

import (
	"context"
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
			expectedDir:     filepath.Join("components", "terraform", "vpc"),
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
			expectedDir:     filepath.Join("components", "helmfile", "nginx"),
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

// TestProvision_NilParams tests that Provision returns an error when params is nil.
func TestProvision_NilParams(t *testing.T) {
	ctx := context.Background()

	err := Provision(ctx, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrNilParam)
}

// TestProvision_NoSource tests that Provision returns nil when no source is configured.
func TestProvision_NoSource(t *testing.T) {
	ctx := context.Background()

	params := &ProvisionParams{
		AtmosConfig: &schema.AtmosConfiguration{
			Components: schema.Components{
				Terraform: schema.Terraform{
					BasePath: "components/terraform",
				},
			},
		},
		ComponentType:   "terraform",
		Component:       "vpc",
		Stack:           "dev",
		ComponentConfig: map[string]any{}, // No source configured.
		Force:           false,
	}

	err := Provision(ctx, params)
	assert.NoError(t, err, "Provision should return nil when no source is configured")
}

// TestProvision_InvalidSource tests that Provision returns an error for invalid source spec.
func TestProvision_InvalidSource(t *testing.T) {
	ctx := context.Background()

	params := &ProvisionParams{
		AtmosConfig: &schema.AtmosConfiguration{
			Components: schema.Components{
				Terraform: schema.Terraform{
					BasePath: "components/terraform",
				},
			},
		},
		ComponentType: "terraform",
		Component:     "vpc",
		Stack:         "dev",
		ComponentConfig: map[string]any{
			// Invalid: source is a number, not a string or map.
			"source": 12345,
		},
		Force: false,
	}

	err := Provision(ctx, params)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrSourceProvision)
}

// TestProvision_TargetDirectoryError tests that Provision returns an error when target directory cannot be determined.
func TestProvision_TargetDirectoryError(t *testing.T) {
	ctx := context.Background()

	params := &ProvisionParams{
		AtmosConfig: &schema.AtmosConfiguration{
			Components: schema.Components{
				Terraform: schema.Terraform{
					BasePath: "", // Empty base path should cause error.
				},
			},
		},
		ComponentType: "terraform",
		Component:     "vpc",
		Stack:         "dev",
		ComponentConfig: map[string]any{
			"source": map[string]any{
				"uri": "github.com/cloudposse/terraform-aws-vpc",
			},
		},
		Force: false,
	}

	err := Provision(ctx, params)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrSourceProvision)
}

// TestProvision_AlreadyExists tests that Provision skips when target already exists and force=false.
func TestProvision_AlreadyExists(t *testing.T) {
	ctx := context.Background()

	// Create a temp directory with content.
	tempDir := t.TempDir()
	targetDir := filepath.Join(tempDir, "vpc")
	err := os.MkdirAll(targetDir, 0o755)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(targetDir, "main.tf"), []byte("# existing"), 0o644)
	require.NoError(t, err)

	params := &ProvisionParams{
		AtmosConfig: &schema.AtmosConfiguration{
			Components: schema.Components{
				Terraform: schema.Terraform{
					BasePath: tempDir,
				},
			},
		},
		ComponentType: "terraform",
		Component:     "vpc",
		Stack:         "dev",
		ComponentConfig: map[string]any{
			"source": map[string]any{
				"uri": "github.com/cloudposse/terraform-aws-vpc",
			},
		},
		Force: false, // Not forcing re-vendor.
	}

	// Should not error - just skip.
	err = Provision(ctx, params)
	assert.NoError(t, err, "Provision should skip when target exists and force=false")

	// Verify the existing file was not modified.
	content, err := os.ReadFile(filepath.Join(targetDir, "main.tf"))
	require.NoError(t, err)
	assert.Equal(t, "# existing", string(content), "Existing file should not be modified")
}

// TestProvision_ForceOverwritesExisting tests that Provision re-vendors when Force=true.
func TestProvision_ForceOverwritesExisting(t *testing.T) {
	ctx := context.Background()

	// Create a temp directory with existing content.
	tempDir := t.TempDir()
	targetDir := filepath.Join(tempDir, "vpc")
	err := os.MkdirAll(targetDir, 0o755)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(targetDir, "main.tf"), []byte("# old content"), 0o644)
	require.NoError(t, err)

	params := &ProvisionParams{
		AtmosConfig: &schema.AtmosConfiguration{
			Components: schema.Components{
				Terraform: schema.Terraform{
					BasePath: tempDir,
				},
			},
		},
		ComponentType: "terraform",
		Component:     "vpc",
		Stack:         "dev",
		ComponentConfig: map[string]any{
			"source": map[string]any{
				"uri": "github.com/cloudposse/terraform-aws-vpc",
			},
		},
		Force: true, // Force re-vendor even if target exists.
	}

	// Provision will attempt to download (which will fail in tests without network).
	// The key validation is that it doesn't skip due to existing directory.
	err = Provision(ctx, params)

	// We expect an error because go-getter can't actually download in unit tests.
	// But the error should be a download error, not a "skipped" situation.
	// This confirms Force=true triggers the download path instead of skipping.
	require.Error(t, err, "Expected error from download attempt, not skip")
	assert.ErrorIs(t, err, errUtils.ErrSourceProvision, "Error should be from provisioning attempt")
}

// TestGetComponentBasePath_Packer tests packer component type base path.
func TestGetComponentBasePath_Packer(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		Components: schema.Components{
			Packer: schema.Packer{
				BasePath: "components/packer",
			},
		},
	}

	result := getComponentBasePath(atmosConfig, "packer")
	assert.Equal(t, "components/packer", result)
}

// TestDetermineTargetDirectory_Packer tests target directory for packer components.
func TestDetermineTargetDirectory_Packer(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		Components: schema.Components{
			Packer: schema.Packer{
				BasePath: "components/packer",
			},
		},
	}

	result, err := DetermineTargetDirectory(atmosConfig, "packer", "ami", map[string]any{})
	require.NoError(t, err)
	assert.Equal(t, filepath.Join("components", "packer", "ami"), result)
}

// TestDetermineTargetDirectory_WorkdirEnabled tests workdir path when provision.workdir.enabled is true.
func TestDetermineTargetDirectory_WorkdirEnabled(t *testing.T) {
	tempDir := t.TempDir()
	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tempDir,
		Components: schema.Components{
			Terraform: schema.Terraform{
				BasePath: "components/terraform",
			},
		},
	}

	componentConfig := map[string]any{
		"atmos_stack": "dev",
		"provision": map[string]any{
			"workdir": map[string]any{
				"enabled": true,
			},
		},
	}

	result, err := DetermineTargetDirectory(atmosConfig, "terraform", "vpc", componentConfig)
	require.NoError(t, err)
	// Expecting: <tempDir>/.workdir/terraform/dev-vpc/
	expected := filepath.Join(tempDir, WorkdirPath, "terraform", "dev-vpc")
	assert.Equal(t, expected, result)
}

// TestDetermineTargetDirectory_WorkdirEnabledNoStack tests workdir path error when stack is missing.
func TestDetermineTargetDirectory_WorkdirEnabledNoStack(t *testing.T) {
	tempDir := t.TempDir()
	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tempDir,
		Components: schema.Components{
			Terraform: schema.Terraform{
				BasePath: "components/terraform",
			},
		},
	}

	// workdir enabled but no atmos_stack in config.
	componentConfig := map[string]any{
		"provision": map[string]any{
			"workdir": map[string]any{
				"enabled": true,
			},
		},
	}

	_, err := DetermineTargetDirectory(atmosConfig, "terraform", "vpc", componentConfig)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrSourceProvision)
}

// TestGetResolvedAbsPath tests retrieval of pre-resolved absolute paths.
func TestGetResolvedAbsPath(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		TerraformDirAbsolutePath: "/abs/path/terraform",
		HelmfileDirAbsolutePath:  "/abs/path/helmfile",
		PackerDirAbsolutePath:    "/abs/path/packer",
	}

	assert.Equal(t, "/abs/path/terraform", getResolvedAbsPath(atmosConfig, "terraform"))
	assert.Equal(t, "/abs/path/helmfile", getResolvedAbsPath(atmosConfig, "helmfile"))
	assert.Equal(t, "/abs/path/packer", getResolvedAbsPath(atmosConfig, "packer"))
	assert.Equal(t, "", getResolvedAbsPath(atmosConfig, "unknown"))
}

// TestDetermineTargetDirectory_PreResolvedAbsPath tests using pre-resolved absolute paths.
func TestDetermineTargetDirectory_PreResolvedAbsPath(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		TerraformDirAbsolutePath: "/abs/path/terraform",
	}

	result, err := DetermineTargetDirectory(atmosConfig, "terraform", "vpc", map[string]any{})
	require.NoError(t, err)
	assert.Equal(t, "/abs/path/terraform/vpc", result)
}

// TestBuildComponentPath_AbsolutePath tests building path when config base path is absolute.
func TestBuildComponentPath_AbsolutePath(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		Components: schema.Components{
			Terraform: schema.Terraform{
				BasePath: "/absolute/path/components/terraform",
			},
		},
	}

	result, err := buildComponentPath(atmosConfig, "terraform")
	require.NoError(t, err)
	assert.Equal(t, "/absolute/path/components/terraform", result)
}

// TestBuildComponentPath_RelativePathWithBasePath tests building path with relative config and base path.
func TestBuildComponentPath_RelativePathWithBasePath(t *testing.T) {
	tempDir := t.TempDir()
	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tempDir,
		Components: schema.Components{
			Terraform: schema.Terraform{
				BasePath: "components/terraform",
			},
		},
	}

	result, err := buildComponentPath(atmosConfig, "terraform")
	require.NoError(t, err)
	expected := filepath.Join(tempDir, "components", "terraform")
	assert.Equal(t, expected, result)
}

// TestBuildComponentPath_RelativePathNoBasePath tests building path with relative config and no base path.
func TestBuildComponentPath_RelativePathNoBasePath(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		BasePath: "",
		Components: schema.Components{
			Terraform: schema.Terraform{
				BasePath: "components/terraform",
			},
		},
	}

	result, err := buildComponentPath(atmosConfig, "terraform")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(".", "components", "terraform"), result)
}
