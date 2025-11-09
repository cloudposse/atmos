package exec

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestResolveComponentFromPath(t *testing.T) {
	// Skip if running in a minimal test environment
	if os.Getenv("ATMOS_TEST_SKIP_STACK_LOADING") != "" {
		t.Skip("Skipping test that requires stack loading")
	}

	tests := []struct {
		name                string
		path                string
		stack               string
		componentType       string
		want                string
		wantErr             bool
		errorIs             error
		setupStacks         bool
		skipStackValidation bool
	}{
		{
			name:                "resolve without stack validation",
			path:                ".",
			stack:               "", // No stack = skip validation
			componentType:       "terraform",
			skipStackValidation: true,
			// Can't test actual resolution without a real component directory
			wantErr: true, // Will fail because "." is not in component directory
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create minimal config for testing
			atmosConfig := &schema.AtmosConfiguration{
				BasePath: t.TempDir(),
				Components: schema.Components{
					Terraform: schema.Terraform{
						BasePath: "components/terraform",
					},
					Helmfile: schema.Helmfile{
						BasePath: "components/helmfile",
					},
					Packer: schema.Packer{
						BasePath: "components/packer",
					},
				},
			}

			got, err := ResolveComponentFromPath(
				atmosConfig,
				tt.path,
				tt.stack,
				tt.componentType,
			)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errorIs != nil {
					assert.ErrorIs(t, err, tt.errorIs)
				}
				return
			}

			require.NoError(t, err)
			if tt.want != "" {
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func TestResolveComponentFromPath_ComponentTypeMismatch(t *testing.T) {
	// Create a temporary directory structure
	tmpDir := t.TempDir()
	terraformBase := filepath.Join(tmpDir, "components", "terraform")
	componentDir := filepath.Join(terraformBase, "vpc")

	require.NoError(t, os.MkdirAll(componentDir, 0o755))

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tmpDir,
		Components: schema.Components{
			Terraform: schema.Terraform{
				BasePath: "components/terraform",
			},
			Helmfile: schema.Helmfile{
				BasePath: "components/helmfile",
			},
		},
	}

	// Try to resolve a terraform path as helmfile component
	_, err := ResolveComponentFromPath(
		atmosConfig,
		componentDir,
		"", // No stack validation
		"helmfile",
	)

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrComponentTypeMismatch)
	assert.Contains(t, err.Error(), "terraform")
	assert.Contains(t, err.Error(), "helmfile")
}

func TestResolveComponentFromPath_PathNotInComponentDir(t *testing.T) {
	tmpDir := t.TempDir()

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tmpDir,
		Components: schema.Components{
			Terraform: schema.Terraform{
				BasePath: "components/terraform",
			},
		},
	}

	// Try to resolve a path outside component directories
	_, err := ResolveComponentFromPath(
		atmosConfig,
		"/tmp/random",
		"",
		"terraform",
	)

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrPathResolutionFailed)
	assert.Contains(t, err.Error(), "not within Atmos component directories")
}

func TestResolveComponentFromPathWithoutTypeCheck(t *testing.T) {
	// Create a temporary directory structure
	tmpDir := t.TempDir()
	terraformBase := filepath.Join(tmpDir, "components", "terraform")
	helmfileBase := filepath.Join(tmpDir, "components", "helmfile")
	vpcDir := filepath.Join(terraformBase, "vpc")
	appDir := filepath.Join(helmfileBase, "app")

	require.NoError(t, os.MkdirAll(vpcDir, 0o755))
	require.NoError(t, os.MkdirAll(appDir, 0o755))

	atmosConfig := &schema.AtmosConfiguration{
		BasePath:                 tmpDir,
		TerraformDirAbsolutePath: terraformBase,
		HelmfileDirAbsolutePath:  helmfileBase,
		Components: schema.Components{
			Terraform: schema.Terraform{
				BasePath: "components/terraform",
			},
			Helmfile: schema.Helmfile{
				BasePath: "components/helmfile",
			},
		},
	}

	tests := []struct {
		name          string
		path          string
		stack         string
		want          string
		wantErr       bool
		errorContains string
	}{
		{
			name:    "resolve terraform component without stack validation",
			path:    vpcDir,
			stack:   "", // No stack validation
			want:    "vpc",
			wantErr: false,
		},
		{
			name:    "resolve helmfile component without stack validation",
			path:    appDir,
			stack:   "", // No stack validation
			want:    "app",
			wantErr: false,
		},
		{
			name:          "path not in component directory",
			path:          "/tmp/random",
			stack:         "",
			wantErr:       true,
			errorContains: "not within Atmos component directories",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ResolveComponentFromPathWithoutTypeCheck(
				atmosConfig,
				tt.path,
				tt.stack,
			)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
