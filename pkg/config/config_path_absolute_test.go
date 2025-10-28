package config

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestInitCliConfig_AbsolutePathHandling tests how InitCliConfig handles absolute paths
// particularly when terraform.base_path is absolute, which can cause path duplication.
func TestInitCliConfig_AbsolutePathHandling(t *testing.T) {
	tests := []struct {
		name                        string
		basePath                    string
		terraformBasePath           string
		expectedTerraformDirAbsPath string
		description                 string
		skipOnWindows               bool
	}{
		{
			name: "BasePath with absolute terraform.base_path",
			// This configuration causes the bug
			basePath:                    "/home/runner/_work/infrastructure/infrastructure",
			terraformBasePath:           "/home/runner/_work/infrastructure/infrastructure/atmos/components/terraform",
			expectedTerraformDirAbsPath: "/home/runner/_work/infrastructure/infrastructure/atmos/components/terraform",
			description:                 "When terraform.base_path is absolute, it should not be joined with base_path",
			skipOnWindows:               true,
		},
		{
			name:                        "BasePath with relative terraform.base_path",
			basePath:                    "/home/runner/_work/infrastructure/infrastructure",
			terraformBasePath:           "atmos/components/terraform",
			expectedTerraformDirAbsPath: "/home/runner/_work/infrastructure/infrastructure/atmos/components/terraform",
			description:                 "Normal case with relative terraform.base_path",
			skipOnWindows:               true,
		},
		{
			name:                        "Relative BasePath with relative terraform.base_path",
			basePath:                    ".",
			terraformBasePath:           "components/terraform",
			expectedTerraformDirAbsPath: "", // Will be computed relative to current directory
			description:                 "Both paths are relative",
			skipOnWindows:               false,
		},
		{
			name:                        "BasePath with ./ prefix in terraform.base_path",
			basePath:                    "/home/runner/_work/infrastructure/infrastructure",
			terraformBasePath:           "./atmos/components/terraform",
			expectedTerraformDirAbsPath: "/home/runner/_work/infrastructure/infrastructure/atmos/components/terraform",
			description:                 "Relative path with ./ prefix should be handled correctly",
			skipOnWindows:               true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.skipOnWindows && runtime.GOOS == "windows" {
				t.Skipf("Skipping Unix path test on Windows")
			}

			// Create a minimal AtmosConfiguration to test the actual atmosConfigAbsolutePaths behavior
			atmosConfig := schema.AtmosConfiguration{
				BasePath: tt.basePath,
				Components: schema.Components{
					Terraform: schema.Terraform{
						BasePath: tt.terraformBasePath,
					},
					Helmfile: schema.Helmfile{
						BasePath: "helmfile", // Set a default for the test
					},
					Packer: schema.Packer{
						BasePath: "packer", // Set a default for the test
					},
				},
				Stacks: schema.Stacks{
					BasePath: "stacks",
				},
			}

			// Call the actual AtmosConfigAbsolutePaths function (from InitCliConfig)
			err := AtmosConfigAbsolutePaths(&atmosConfig)
			require.NoError(t, err)

			t.Logf("Input: BasePath=%q, Terraform.BasePath=%q", tt.basePath, tt.terraformBasePath)
			t.Logf("Output: TerraformDirAbsolutePath=%q", atmosConfig.TerraformDirAbsolutePath)

			// Check for path duplication in the result
			assert.NotContains(t, atmosConfig.TerraformDirAbsolutePath,
				"/home/runner/_work/infrastructure/infrastructure/home/runner/_work/infrastructure/infrastructure",
				"Absolute path should not contain duplicated base path")
			assert.NotContains(t, atmosConfig.TerraformDirAbsolutePath, "/.//",
				"Absolute path should not contain /.// pattern")
			assert.NotContains(t, atmosConfig.TerraformDirAbsolutePath, "//",
				"Absolute path should not contain // pattern")

			// For the expected result, check if we need special handling
			if tt.expectedTerraformDirAbsPath != "" && filepath.IsAbs(tt.expectedTerraformDirAbsPath) {
				// If terraform.base_path is absolute, the result should be that absolute path
				assert.Equal(t, tt.expectedTerraformDirAbsPath, atmosConfig.TerraformDirAbsolutePath,
					"When terraform.base_path is absolute, it should be used as-is")
			}
		})
	}
}

// TestConfigPathJoining_EdgeCases tests various edge cases in path joining
// that might lead to the path duplication bug.
func TestConfigPathJoining_EdgeCases(t *testing.T) {
	tests := []struct {
		name             string
		basePath         string
		componentPath    string
		expectedBehavior string
		skipOnWindows    bool
	}{
		{
			name:             "Two absolute paths on Unix",
			basePath:         "/absolute/base",
			componentPath:    "/absolute/component",
			expectedBehavior: "Second path should be treated specially",
			skipOnWindows:    true,
		},
		{
			name:             "Absolute base with relative component",
			basePath:         "/absolute/base",
			componentPath:    "relative/component",
			expectedBehavior: "Should join normally",
			skipOnWindows:    true,
		},
		{
			name:             "Base with trailing slash",
			basePath:         "/absolute/base/",
			componentPath:    "relative/component",
			expectedBehavior: "Trailing slash should not affect result",
			skipOnWindows:    true,
		},
		{
			name:             "Component with leading ./",
			basePath:         "/absolute/base",
			componentPath:    "./relative/component",
			expectedBehavior: "./ should be normalized",
			skipOnWindows:    true,
		},
		{
			name:             "Both paths with dots",
			basePath:         "/absolute/base/.",
			componentPath:    "./relative/component",
			expectedBehavior: "Dots should be normalized",
			skipOnWindows:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.skipOnWindows && runtime.GOOS == "windows" {
				t.Skipf("Skipping Unix path test on Windows")
			}

			// Test filepath.Join behavior
			joined := filepath.Join(tt.basePath, tt.componentPath)
			t.Logf("filepath.Join(%q, %q) = %q", tt.basePath, tt.componentPath, joined)

			// Clean path should equal joined path (filepath.Join should clean)
			assert.Equal(t, filepath.Clean(joined), joined,
				"filepath.Join should produce a clean path")

			// Test for problematic patterns
			assert.NotContains(t, joined, "//",
				"Joined path should not contain double slashes")
			assert.NotContains(t, joined, "/./",
				"Joined path should not contain /./ pattern")
			assert.NotContains(t, joined, "/../",
				"Joined path should not contain /../ pattern")

			// Special case: two absolute paths
			if filepath.IsAbs(tt.basePath) && filepath.IsAbs(tt.componentPath) {
				// Document the actual filepath.Join behavior with two absolute paths
				// filepath.Join treats the second absolute path as relative by stripping leading separator
				if runtime.GOOS != "windows" {
					expected := filepath.Clean(tt.basePath + tt.componentPath)
					assert.Equal(t, expected, joined,
						"filepath.Join with two absolute Unix paths strips leading slash from second path")
				}
			}

			// Test filepath.Abs behavior
			absPath, err := filepath.Abs(joined)
			require.NoError(t, err)
			t.Logf("filepath.Abs(%q) = %q", joined, absPath)

			// Abs path should be clean
			assert.Equal(t, filepath.Clean(absPath), absPath,
				"filepath.Abs should produce a clean path")
		})
	}
}

// TestCorrectPathHandling demonstrates the correct way to handle paths
// to avoid the duplication bug.
func TestCorrectPathHandling(t *testing.T) {
	tests := []struct {
		name          string
		basePath      string
		componentPath string
		skipOnWindows bool
	}{
		{
			name:          "Absolute component path should be used as-is",
			basePath:      "/home/runner/_work/infrastructure/infrastructure",
			componentPath: "/home/runner/_work/infrastructure/infrastructure/atmos/components/terraform",
			skipOnWindows: true,
		},
		{
			name:          "Relative component path should be joined",
			basePath:      "/home/runner/_work/infrastructure/infrastructure",
			componentPath: "atmos/components/terraform",
			skipOnWindows: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.skipOnWindows && runtime.GOOS == "windows" {
				t.Skipf("Skipping Unix path test on Windows")
			}

			// Correct way to handle paths
			var resultPath string
			if filepath.IsAbs(tt.componentPath) {
				// If component path is absolute, use it as-is
				resultPath = tt.componentPath
			} else {
				// If component path is relative, join with base path
				resultPath = filepath.Join(tt.basePath, tt.componentPath)
			}

			t.Logf("Correct handling: %q", resultPath)

			// The result should not have duplication
			assert.NotContains(t, resultPath,
				"/home/runner/_work/infrastructure/infrastructure/home/runner/_work/infrastructure/infrastructure",
				"Correctly handled path should not have duplication")

			// Convert to absolute (if needed)
			absPath, err := filepath.Abs(resultPath)
			require.NoError(t, err)

			// Should still be clean
			assert.Equal(t, filepath.Clean(absPath), absPath,
				"Absolute path should be clean")

			// Should not contain problematic patterns
			assert.NotContains(t, absPath, "/.//",
				"Should not contain /.// pattern")
			assert.NotContains(t, absPath, "//",
				"Should not contain // pattern")
		})
	}
}
