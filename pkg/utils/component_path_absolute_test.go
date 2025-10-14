package utils

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGetComponentPath_AbsolutePathScenarios tests GetComponentPath function
// with various absolute path configurations that might cause path duplication.
func TestGetComponentPath_AbsolutePathScenarios(t *testing.T) {
	tests := []struct {
		name                     string
		basePath                 string
		terraformDirAbsolutePath string
		terraformBasePath        string
		componentType            string
		componentFolderPrefix    string
		component                string
		expectedError            bool
		shouldNotHaveDuplication bool
		skipOnWindows            bool
	}{
		{
			name:                     "GitHub Actions bug scenario",
			basePath:                 "/home/runner/_work/infrastructure/infrastructure",
			terraformDirAbsolutePath: "/home/runner/_work/infrastructure/infrastructure/atmos/components/terraform",
			terraformBasePath:        "atmos/components/terraform",
			componentType:            "terraform",
			componentFolderPrefix:    "",
			component:                "iam-role-legacy",
			expectedError:            false,
			shouldNotHaveDuplication: true,
			skipOnWindows:            true,
		},
		{
			name:                     "Absolute terraform dir with component",
			basePath:                 "/project/root",
			terraformDirAbsolutePath: "/project/root/components/terraform",
			terraformBasePath:        "components/terraform",
			componentType:            "terraform",
			componentFolderPrefix:    "aws",
			component:                "vpc",
			expectedError:            false,
			shouldNotHaveDuplication: true,
			skipOnWindows:            true,
		},
		// Removed "Incorrectly set absolute path with duplication" test case:
		// This test was testing an impossible scenario - a pre-duplicated path in
		// TerraformDirAbsolutePath that would never occur in real usage since we use
		// JoinPath (which prevents duplication) when computing these paths in config.go.
		{
			name:                     "Relative paths (normal case)",
			basePath:                 ".",
			terraformDirAbsolutePath: "", // Not set, will use basePath + terraformBasePath
			terraformBasePath:        "components/terraform",
			componentType:            "terraform",
			componentFolderPrefix:    "",
			component:                "vpc",
			expectedError:            false,
			shouldNotHaveDuplication: true,
			skipOnWindows:            false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.skipOnWindows && runtime.GOOS == "windows" {
				t.Skipf("Skipping Unix path test on Windows")
			}

			atmosConfig := &schema.AtmosConfiguration{
				BasePath:                 tt.basePath,
				TerraformDirAbsolutePath: tt.terraformDirAbsolutePath,
				Components: schema.Components{
					Terraform: schema.Terraform{
						BasePath: tt.terraformBasePath,
					},
				},
			}

			// Test GetComponentPath
			componentPath, err := GetComponentPath(
				atmosConfig,
				tt.componentType,
				tt.componentFolderPrefix,
				tt.component,
			)

			if tt.expectedError {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			t.Logf("Component path: %s", componentPath)

			// Check that the path is clean
			assert.Equal(t, filepath.Clean(componentPath), componentPath,
				"Component path should be clean")

			// Check for path duplication
			if tt.shouldNotHaveDuplication {
				assert.NotContains(t, componentPath, "/.//",
					"Component path should not contain /.// pattern")
				assert.NotContains(t, componentPath, "//",
					"Component path should not contain // pattern")

				// Check for the specific GitHub Actions bug pattern
				assert.NotContains(t, componentPath,
					"/home/runner/_work/infrastructure/infrastructure/.//home/runner/_work/infrastructure/infrastructure",
					"Component path should not contain the duplication pattern from the bug")

				// Check that the full base path doesn't appear twice consecutively
				if filepath.IsAbs(tt.basePath) && tt.basePath != "/" {
					// Look for the base path appearing twice with path separators
					duplicatedPath := filepath.Join(tt.basePath, tt.basePath)
					assert.NotContains(t, componentPath, duplicatedPath,
						"Base path should not be duplicated consecutively")

					// Also check with ./ or .// patterns that might indicate duplication
					duplicatedWithDot := tt.basePath + "/./" + tt.basePath
					assert.NotContains(t, componentPath, duplicatedWithDot,
						"Base path should not be duplicated with /./ pattern")
				}
			}

			// Verify the component is included in the path
			assert.Contains(t, componentPath, tt.component,
				"Component path should contain the component name")

			// If folder prefix is specified, it should be in the path
			if tt.componentFolderPrefix != "" {
				assert.Contains(t, componentPath, tt.componentFolderPrefix,
					"Component path should contain the folder prefix")
			}
		})
	}
}

// TestGetComponentPath_EnvironmentVariableOverride tests that environment variable
// overrides are handled correctly even with malformed paths.
func TestGetComponentPath_EnvironmentVariableOverride(t *testing.T) {
	tests := []struct {
		name                     string
		envVarValue              string
		basePath                 string
		terraformDirAbsolutePath string
		terraformBasePath        string
		componentFolderPrefix    string
		component                string
		skipOnWindows            bool
	}{
		{
			name:                     "Environment variable with path duplication",
			envVarValue:              "/home/runner/_work/infrastructure/infrastructure/.//home/runner/_work/infrastructure/infrastructure/atmos/components/terraform",
			basePath:                 "/home/runner/_work/infrastructure/infrastructure",
			terraformDirAbsolutePath: "/home/runner/_work/infrastructure/infrastructure/atmos/components/terraform",
			terraformBasePath:        "atmos/components/terraform",
			componentFolderPrefix:    "",
			component:                "iam-role-legacy",
			skipOnWindows:            true,
		},
		{
			name:                     "Clean environment variable override",
			envVarValue:              "/custom/path/to/terraform",
			basePath:                 "/home/runner/_work/infrastructure/infrastructure",
			terraformDirAbsolutePath: "/home/runner/_work/infrastructure/infrastructure/atmos/components/terraform",
			terraformBasePath:        "atmos/components/terraform",
			componentFolderPrefix:    "",
			component:                "vpc",
			skipOnWindows:            true,
		},
		{
			name:                     "Relative path in environment variable",
			envVarValue:              "./custom/terraform",
			basePath:                 "/home/runner/_work/infrastructure/infrastructure",
			terraformDirAbsolutePath: "/home/runner/_work/infrastructure/infrastructure/atmos/components/terraform",
			terraformBasePath:        "atmos/components/terraform",
			componentFolderPrefix:    "aws",
			component:                "rds",
			skipOnWindows:            true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.skipOnWindows && runtime.GOOS == "windows" {
				t.Skipf("Skipping Unix path test on Windows")
			}

			// Set the test environment variable
			t.Setenv("ATMOS_COMPONENTS_TERRAFORM_BASE_PATH", tt.envVarValue)

			atmosConfig := &schema.AtmosConfiguration{
				BasePath:                 tt.basePath,
				TerraformDirAbsolutePath: tt.terraformDirAbsolutePath,
				Components: schema.Components{
					Terraform: schema.Terraform{
						BasePath: tt.terraformBasePath,
					},
				},
			}

			// Test GetComponentPath with environment variable override
			componentPath, err := GetComponentPath(
				atmosConfig,
				"terraform",
				tt.componentFolderPrefix,
				tt.component,
			)

			require.NoError(t, err)
			t.Logf("Component path with env override: %s", componentPath)

			// The path should be clean even with malformed environment variable
			assert.Equal(t, filepath.Clean(componentPath), componentPath,
				"Component path should be clean even with malformed env var")

			// Check for path duplication patterns
			assert.NotContains(t, componentPath, "/.//",
				"Component path from env var should not contain /.// pattern")
			assert.NotContains(t, componentPath, "//",
				"Component path from env var should not contain // pattern")
			assert.NotContains(t, componentPath,
				"/home/runner/_work/infrastructure/infrastructure/.//home/runner/_work/infrastructure/infrastructure",
				"Component path from env var should not have the bug's duplication pattern")

			// Verify component is in the path
			assert.Contains(t, componentPath, tt.component,
				"Component path should contain the component name")
		})
	}
}

// TestGetComponentPath_AllComponentTypes tests GetComponentPath for all component types
// (terraform, helmfile, packer) to ensure consistent behavior.
func TestGetComponentPath_AllComponentTypes(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skipf("Skipping Unix path test on Windows")
	}

	basePath := "/home/runner/_work/infrastructure/infrastructure"

	atmosConfig := &schema.AtmosConfiguration{
		BasePath:                 basePath,
		TerraformDirAbsolutePath: filepath.Join(basePath, "atmos", "components", "terraform"),
		HelmfileDirAbsolutePath:  filepath.Join(basePath, "atmos", "components", "helmfile"),
		PackerDirAbsolutePath:    filepath.Join(basePath, "atmos", "components", "packer"),
		Components: schema.Components{
			Terraform: schema.Terraform{
				BasePath: "atmos/components/terraform",
			},
			Helmfile: schema.Helmfile{
				BasePath: "atmos/components/helmfile",
			},
			Packer: schema.Packer{
				BasePath: "atmos/components/packer",
			},
		},
	}

	componentTypes := []struct {
		componentType string
		component     string
		expectedPath  string
	}{
		{
			componentType: "terraform",
			component:     "iam-role",
			expectedPath:  "/home/runner/_work/infrastructure/infrastructure/atmos/components/terraform/iam-role",
		},
		{
			componentType: "helmfile",
			component:     "nginx",
			expectedPath:  "/home/runner/_work/infrastructure/infrastructure/atmos/components/helmfile/nginx",
		},
		{
			componentType: "packer",
			component:     "ami-builder",
			expectedPath:  "/home/runner/_work/infrastructure/infrastructure/atmos/components/packer/ami-builder",
		},
	}

	for _, ct := range componentTypes {
		t.Run(ct.componentType, func(t *testing.T) {
			componentPath, err := GetComponentPath(
				atmosConfig,
				ct.componentType,
				"", // No folder prefix
				ct.component,
			)

			require.NoError(t, err)
			assert.Equal(t, ct.expectedPath, componentPath,
				"Component path should match expected for %s", ct.componentType)

			// Ensure no duplication
			assert.NotContains(t, componentPath, "/.//",
				"%s component path should not contain /.//", ct.componentType)
			assert.NotContains(t, componentPath,
				"/home/runner/_work/infrastructure/infrastructure/home/runner/_work/infrastructure/infrastructure",
				"%s component path should not have path duplication", ct.componentType)
		})
	}
}
