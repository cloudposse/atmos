package exec

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestConstructTerraformComponentWorkingDir_AbsolutePathHandling tests the constructTerraformComponentWorkingDir function
// from path_utils.go to ensure it correctly handles absolute paths without duplication.
func TestConstructTerraformComponentWorkingDir_AbsolutePathHandling(t *testing.T) {
	tests := []struct {
		name                      string
		basePath                  string
		terraformBasePath         string
		terraformDirAbsolutePath  string
		componentFolderPrefix     string
		finalComponent            string
		expectedPathContains      string
		shouldNotContainDuplicate bool
	}{
		{
			name: "GitHub Actions environment with absolute BasePath",
			// Simulating the GitHub Actions environment where BasePath is already absolute
			basePath:                  "/home/runner/_work/infrastructure/infrastructure/atmos",
			terraformBasePath:         "components/terraform",
			terraformDirAbsolutePath:  "/home/runner/_work/infrastructure/infrastructure/atmos/components/terraform",
			componentFolderPrefix:     "",
			finalComponent:            "iam-role-legacy",
			expectedPathContains:      "components/terraform/iam-role-legacy",
			shouldNotContainDuplicate: true,
		},
		{
			name:                      "GitHub Actions with component folder prefix",
			basePath:                  "/home/runner/_work/infrastructure/infrastructure",
			terraformBasePath:         "atmos/components/terraform",
			terraformDirAbsolutePath:  "/home/runner/_work/infrastructure/infrastructure/atmos/components/terraform",
			componentFolderPrefix:     "aws",
			finalComponent:            "vpc",
			expectedPathContains:      "components/terraform/aws/vpc",
			shouldNotContainDuplicate: true,
		},
		{
			name:                      "Relative BasePath (normal case)",
			basePath:                  ".",
			terraformBasePath:         "components/terraform",
			terraformDirAbsolutePath:  "", // Will be constructed
			componentFolderPrefix:     "",
			finalComponent:            "vpc",
			expectedPathContains:      "components/terraform/vpc",
			shouldNotContainDuplicate: true,
		},
		{
			name:                      "Complex absolute path with dots",
			basePath:                  "/home/runner/_work/infrastructure/infrastructure/./atmos",
			terraformBasePath:         "components/terraform",
			terraformDirAbsolutePath:  "/home/runner/_work/infrastructure/infrastructure/atmos/components/terraform",
			componentFolderPrefix:     "",
			finalComponent:            "test-component",
			expectedPathContains:      "components/terraform/test-component",
			shouldNotContainDuplicate: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Skip tests with absolute paths on Windows as they're Unix-specific
			if runtime.GOOS == "windows" && filepath.IsAbs(tt.basePath) {
				t.Skipf("Skipping Unix absolute path test on Windows")
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

			info := &schema.ConfigAndStacksInfo{
				ComponentFolderPrefix: tt.componentFolderPrefix,
				FinalComponent:        tt.finalComponent,
			}

			// Construct the working directory
			workingDir := constructTerraformComponentWorkingDir(atmosConfig, info)

			// Check that the path contains expected components
			assert.Contains(t, workingDir, tt.finalComponent,
				"Working directory should contain the component name")

			// Check for path duplication
			if tt.shouldNotContainDuplicate {
				checkForPathDuplication(t, workingDir, tt.basePath)
			}

			// Verify the path is clean (no double slashes, no unnecessary dots)
			cleanPath := filepath.Clean(workingDir)
			assert.Equal(t, cleanPath, workingDir,
				"Working directory should be a clean path without redundant elements")
		})
	}
}

// TestConstructTerraformComponentWorkingDir_ConsistencyWithGetComponentPath tests that
// constructTerraformComponentWorkingDir produces consistent results with GetComponentPath.
func TestConstructTerraformComponentWorkingDir_ConsistencyWithGetComponentPath(t *testing.T) {
	tests := []struct {
		name                  string
		basePath              string
		terraformBasePath     string
		componentFolderPrefix string
		finalComponent        string
		description           string
		skipOnWindows         bool
	}{
		{
			name:                  "GitHub Actions absolute path scenario",
			basePath:              "/home/runner/_work/infrastructure/infrastructure",
			terraformBasePath:     "atmos/components/terraform",
			componentFolderPrefix: "",
			finalComponent:        "iam-role-legacy",
			description:           "Reproduces the exact GitHub Actions error case",
			skipOnWindows:         true,
		},
		{
			name:                  "Absolute path with dots",
			basePath:              "/home/runner/_work/infrastructure/infrastructure/.",
			terraformBasePath:     "atmos/components/terraform",
			componentFolderPrefix: "",
			finalComponent:        "vpc",
			description:           "Absolute path ending with dot",
			skipOnWindows:         true,
		},
		{
			name:                  "Relative path (should work fine)",
			basePath:              ".",
			terraformBasePath:     "components/terraform",
			componentFolderPrefix: "",
			finalComponent:        "vpc",
			description:           "Normal relative path case",
		},
		{
			name:                  "Complex nested absolute path",
			basePath:              "/usr/local/project/infrastructure",
			terraformBasePath:     "./components/terraform",
			componentFolderPrefix: "aws",
			finalComponent:        "ecs/cluster",
			description:           "Nested component with relative terraform base path",
			skipOnWindows:         true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.skipOnWindows && runtime.GOOS == "windows" {
				t.Skipf("Skipping Unix path test on Windows")
			}

			// Setup atmosphere config
			atmosConfig := &schema.AtmosConfiguration{
				BasePath: tt.basePath,
				Components: schema.Components{
					Terraform: schema.Terraform{
						BasePath: tt.terraformBasePath,
					},
				},
			}

			// Simulate what InitCliConfig does: compute absolute paths
			terraformBasePath := filepath.Join(atmosConfig.BasePath, atmosConfig.Components.Terraform.BasePath)
			terraformDirAbsPath, err := filepath.Abs(terraformBasePath)
			require.NoError(t, err)
			atmosConfig.TerraformDirAbsolutePath = terraformDirAbsPath

			info := &schema.ConfigAndStacksInfo{
				ComponentFolderPrefix: tt.componentFolderPrefix,
				FinalComponent:        tt.finalComponent,
			}

			// Get paths using both methods
			// Method 1: Using GetComponentPath (as used for ExecuteShellCommand)
			componentPath, err := u.GetComponentPath(atmosConfig, "terraform", info.ComponentFolderPrefix, info.FinalComponent)
			require.NoError(t, err)

			// Method 2: Using constructTerraformComponentWorkingDir (as used for backend config)
			workingDir := constructTerraformComponentWorkingDir(atmosConfig, info)

			// Both paths should be identical
			assert.Equal(t, componentPath, workingDir,
				"GetComponentPath and constructTerraformComponentWorkingDir should produce the same path")

			// Check for path duplication patterns
			checkForPathDuplication(t, componentPath, tt.basePath)
			checkForPathDuplication(t, workingDir, tt.basePath)

			// Both paths should be clean (no double slashes, no unnecessary dots)
			assert.Equal(t, filepath.Clean(componentPath), componentPath,
				"componentPath should be clean")
			assert.Equal(t, filepath.Clean(workingDir), workingDir,
				"workingDir should be clean")

			// Ensure paths contain the expected components
			assert.Contains(t, componentPath, tt.finalComponent,
				"Path should contain the final component")
			assert.Contains(t, workingDir, tt.finalComponent,
				"Path should contain the final component")
		})
	}
}

// Helper function to check for path duplication patterns.
func checkForPathDuplication(t *testing.T, path string, basePath string) {
	// Check for common duplication patterns
	assert.NotContains(t, path, "/.//",
		"Path should not contain /.// pattern")
	assert.NotContains(t, path, "//",
		"Path should not contain // pattern")
	assert.NotContains(t, path, "././",
		"Path should not contain ././ pattern")

	// If basePath is absolute, check it's not duplicated
	if filepath.IsAbs(basePath) {
		cleanBase := filepath.Clean(basePath)
		if cleanBase != "/" {
			// Check for patterns like /path/.//path or /path//path
			assert.NotContains(t, path, cleanBase+"/."+cleanBase,
				"Path should not contain duplicated base with dot separator")
			assert.NotContains(t, path, cleanBase+"//"+cleanBase,
				"Path should not contain duplicated base with double slash")
			assert.NotContains(t, path, cleanBase+"/./"+cleanBase,
				"Path should not contain duplicated base with dot path")
		}

		// Check for the specific duplication pattern from the bug
		if strings.Contains(basePath, "/home/runner/_work/infrastructure/infrastructure") {
			assert.NotContains(t, path,
				"/home/runner/_work/infrastructure/infrastructure/.//home/runner/_work/infrastructure/infrastructure",
				"Path should not contain the exact duplication pattern from the GitHub Actions bug")
		}
	}
}
