package utils

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestGetComponentPath(t *testing.T) {
	// Note: We don't need to save/restore env vars as t.Setenv in subtests handles cleanup

	tests := []struct {
		name               string
		setupConfig        func() *schema.AtmosConfiguration
		componentType      string
		componentFolder    string
		component          string
		envVars            map[string]string
		expectedPathSuffix string // We'll check the path ends with this to handle absolute paths.
		expectError        bool
		skipWindows        bool
	}{
		{
			name: "terraform with standard path",
			setupConfig: func() *schema.AtmosConfiguration {
				return &schema.AtmosConfiguration{
					BasePath: "/workspace",
					Components: schema.Components{
						Terraform: schema.Terraform{
							BasePath: "components/terraform",
						},
					},
				}
			},
			componentType:      "terraform",
			component:          "vpc",
			expectedPathSuffix: filepath.Join("workspace", "components", "terraform", "vpc"),
		},
		{
			name: "terraform with custom opentofu path",
			setupConfig: func() *schema.AtmosConfiguration {
				return &schema.AtmosConfiguration{
					BasePath: "/workspace",
					Components: schema.Components{
						Terraform: schema.Terraform{
							BasePath: "infrastructure/opentofu",
						},
					},
				}
			},
			componentType:      "terraform",
			component:          "vpc",
			expectedPathSuffix: filepath.Join("workspace", "infrastructure", "opentofu", "vpc"),
		},
		{
			name: "terraform with environment override",
			setupConfig: func() *schema.AtmosConfiguration {
				return &schema.AtmosConfiguration{
					BasePath: "/workspace",
					Components: schema.Components{
						Terraform: schema.Terraform{
							BasePath: "components/terraform",
						},
					},
				}
			},
			componentType: "terraform",
			component:     "vpc",
			envVars: map[string]string{
				"ATMOS_COMPONENTS_TERRAFORM_BASE_PATH": "/custom/tf-modules",
			},
			expectedPathSuffix: filepath.Join("custom", "tf-modules", "vpc"),
		},
		{
			name: "terraform with component folder prefix",
			setupConfig: func() *schema.AtmosConfiguration {
				return &schema.AtmosConfiguration{
					BasePath: "/workspace",
					Components: schema.Components{
						Terraform: schema.Terraform{
							BasePath: "components/terraform",
						},
					},
				}
			},
			componentType:      "terraform",
			componentFolder:    "infra",
			component:          "vpc",
			expectedPathSuffix: filepath.Join("workspace", "components", "terraform", "infra", "vpc"),
		},
		{
			name: "terraform with absolute path in config",
			setupConfig: func() *schema.AtmosConfiguration {
				return &schema.AtmosConfiguration{
					BasePath: "/workspace",
					Components: schema.Components{
						Terraform: schema.Terraform{
							BasePath: "components/terraform",
						},
					},
					TerraformDirAbsolutePath: "/absolute/terraform/path",
				}
			},
			componentType:      "terraform",
			component:          "vpc",
			expectedPathSuffix: filepath.Join("absolute", "terraform", "path", "vpc"),
		},
		{
			name: "helmfile with standard path",
			setupConfig: func() *schema.AtmosConfiguration {
				return &schema.AtmosConfiguration{
					BasePath: "/workspace",
					Components: schema.Components{
						Helmfile: schema.Helmfile{
							BasePath: "components/helmfile",
						},
					},
				}
			},
			componentType:      "helmfile",
			component:          "nginx",
			expectedPathSuffix: filepath.Join("workspace", "components", "helmfile", "nginx"),
		},
		{
			name: "helmfile with environment override",
			setupConfig: func() *schema.AtmosConfiguration {
				return &schema.AtmosConfiguration{
					BasePath: "/workspace",
					Components: schema.Components{
						Helmfile: schema.Helmfile{
							BasePath: "components/helmfile",
						},
					},
				}
			},
			componentType: "helmfile",
			component:     "nginx",
			envVars: map[string]string{
				"ATMOS_COMPONENTS_HELMFILE_BASE_PATH": "/custom/helm",
			},
			expectedPathSuffix: filepath.Join("custom", "helm", "nginx"),
		},
		{
			name: "packer with standard path",
			setupConfig: func() *schema.AtmosConfiguration {
				return &schema.AtmosConfiguration{
					BasePath: "/workspace",
					Components: schema.Components{
						Packer: schema.Packer{
							BasePath: "components/packer",
						},
					},
				}
			},
			componentType:      "packer",
			component:          "ami",
			expectedPathSuffix: filepath.Join("workspace", "components", "packer", "ami"),
		},
		{
			name: "unknown component type",
			setupConfig: func() *schema.AtmosConfiguration {
				return &schema.AtmosConfiguration{
					BasePath: "/workspace",
				}
			},
			componentType: "unknown",
			component:     "test",
			expectError:   true,
		},
		{
			name: "sandbox scenario with absolute override",
			setupConfig: func() *schema.AtmosConfiguration {
				return &schema.AtmosConfiguration{
					BasePath: "/original",
					Components: schema.Components{
						Terraform: schema.Terraform{
							BasePath: "deployment/terragrunt",
						},
					},
				}
			},
			componentType: "terraform",
			component:     "app",
			envVars: map[string]string{
				"ATMOS_COMPONENTS_TERRAFORM_BASE_PATH": "/tmp/sandbox-123/components/terraform",
			},
			expectedPathSuffix: filepath.Join("tmp", "sandbox-123", "components", "terraform", "app"),
			skipWindows:        true, // /tmp doesn't exist on Windows.
		},
		{
			name: "relative base path gets resolved to absolute",
			setupConfig: func() *schema.AtmosConfiguration {
				return &schema.AtmosConfiguration{
					BasePath: ".", // Relative path.
					Components: schema.Components{
						Terraform: schema.Terraform{
							BasePath: "components/terraform",
						},
					},
				}
			},
			componentType: "terraform",
			component:     "vpc",
			// The path should be absolute even though we started with relative.
			expectedPathSuffix: filepath.Join("components", "terraform", "vpc"),
		},
		{
			name: "empty component returns base path",
			setupConfig: func() *schema.AtmosConfiguration {
				return &schema.AtmosConfiguration{
					BasePath: "/workspace",
					Components: schema.Components{
						Terraform: schema.Terraform{
							BasePath: "components/terraform",
						},
					},
				}
			},
			componentType:      "terraform",
			component:          "",
			expectedPathSuffix: filepath.Join("workspace", "components", "terraform"),
		},
		{
			name: "environment variable with relative path gets resolved",
			setupConfig: func() *schema.AtmosConfiguration {
				return &schema.AtmosConfiguration{
					BasePath: "/workspace",
					Components: schema.Components{
						Terraform: schema.Terraform{
							BasePath: "components/terraform",
						},
					},
				}
			},
			componentType: "terraform",
			component:     "vpc",
			envVars: map[string]string{
				"ATMOS_COMPONENTS_TERRAFORM_BASE_PATH": "./custom/terraform",
			},
			expectedPathSuffix: filepath.Join("custom", "terraform", "vpc"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.skipWindows && runtime.GOOS == "windows" {
				t.Skipf("Skipping test on Windows")
			}

			// Set test env vars (t.Setenv handles cleanup automatically).
			for k, v := range tt.envVars {
				t.Setenv(k, v)
			}

			cfg := tt.setupConfig()
			path, err := GetComponentPath(cfg, tt.componentType, tt.componentFolder, tt.component)

			if tt.expectError {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.NotEmpty(t, path)

			// Verify path is absolute (including UNC paths).
			isAbsolute := filepath.IsAbs(path) || strings.HasPrefix(path, `\\`)
			assert.True(t, isAbsolute, "Expected absolute path, got: %s", path)

			// Verify path contains expected suffix (handles platform differences).
			// We check if the path ends with the expected suffix
			expectedSuffix := filepath.FromSlash(tt.expectedPathSuffix)
			assert.True(t,
				strings.HasSuffix(path, expectedSuffix),
				"Path %s should end with %s", path, expectedSuffix)

			// Verify path is clean (no redundant separators).
			assert.Equal(t, filepath.Clean(path), path)
		})
	}
}

func TestGetComponentBasePath(t *testing.T) {
	cfg := &schema.AtmosConfiguration{
		BasePath: "/workspace",
		Components: schema.Components{
			Terraform: schema.Terraform{
				BasePath: "components/terraform",
			},
		},
	}

	path, err := GetComponentBasePath(cfg, "terraform")
	require.NoError(t, err)
	assert.NotEmpty(t, path)
	assert.True(t, filepath.IsAbs(path))
	assert.True(t, strings.HasSuffix(path, filepath.Join("workspace", "components", "terraform")))
}

func TestGetComponentPathCrossPlatform(t *testing.T) {
	cfg := &schema.AtmosConfiguration{
		BasePath: "/workspace",
		Components: schema.Components{
			Terraform: schema.Terraform{
				BasePath: "components/terraform",
			},
		},
	}

	path, err := GetComponentPath(cfg, "terraform", "", "vpc")
	require.NoError(t, err)

	// Path should use correct separator for the platform.
	if runtime.GOOS == "windows" {
		// On Windows, absolute paths would have drive letter.
		// Since we're starting with /workspace, filepath will handle it appropriately.
		assert.Contains(t, path, string(os.PathSeparator))
	} else {
		assert.Contains(t, path, "/")
	}

	// Path should be clean regardless of platform.
	assert.Equal(t, filepath.Clean(path), path)
}

func TestGetBasePathForComponentType(t *testing.T) {
	tests := []struct {
		name          string
		componentType string
		setupConfig   func() *schema.AtmosConfiguration
		setupEnv      map[string]string
		expectedPath  string
		expectError   bool
	}{
		{
			name:          "terraform_with_env_override",
			componentType: "terraform",
			setupConfig: func() *schema.AtmosConfiguration {
				return &schema.AtmosConfiguration{
					BasePath: "/workspace",
					Components: schema.Components{
						Terraform: schema.Terraform{
							BasePath: "components/terraform",
						},
					},
				}
			},
			setupEnv: map[string]string{
				"ATMOS_COMPONENTS_TERRAFORM_BASE_PATH": "/custom/terraform",
			},
			expectedPath: "/custom/terraform",
		},
		{
			name:          "helmfile_with_resolved_path",
			componentType: "helmfile",
			setupConfig: func() *schema.AtmosConfiguration {
				return &schema.AtmosConfiguration{
					BasePath: "/workspace",
					Components: schema.Components{
						Helmfile: schema.Helmfile{
							BasePath: "components/helmfile",
						},
					},
					HelmfileDirAbsolutePath: "/resolved/helmfile/path",
				}
			},
			expectedPath: "/resolved/helmfile/path",
		},
		{
			name:          "packer_constructed_path",
			componentType: "packer",
			setupConfig: func() *schema.AtmosConfiguration {
				return &schema.AtmosConfiguration{
					BasePath: "/workspace",
					Components: schema.Components{
						Packer: schema.Packer{
							BasePath: "components/packer",
						},
					},
				}
			},
			expectedPath: "/workspace/components/packer",
		},
		{
			name:          "unknown_component_type",
			componentType: "unknown",
			setupConfig: func() *schema.AtmosConfiguration {
				return &schema.AtmosConfiguration{
					BasePath: "/workspace",
				}
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set test env vars (t.Setenv handles cleanup automatically).
			for k, v := range tt.setupEnv {
				t.Setenv(k, v)
			}

			cfg := tt.setupConfig()
			basePath, envVarName, err := getBasePathForComponentType(cfg, tt.componentType)

			if tt.expectError {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.NotEmpty(t, envVarName)

			if strings.Contains(tt.expectedPath, "/") && !strings.HasPrefix(tt.expectedPath, `\\`) {
				// For Unix-style paths, normalize for comparison.
				expectedNormalized := filepath.FromSlash(tt.expectedPath)
				actualNormalized := filepath.FromSlash(basePath)
				assert.True(t, strings.HasSuffix(actualNormalized, expectedNormalized) || actualNormalized == expectedNormalized,
					"Expected path %s to match or end with %s", actualNormalized, expectedNormalized)
			} else {
				// For UNC paths or exact matches, compare directly.
				assert.Equal(t, tt.expectedPath, basePath)
			}
		})
	}
}
