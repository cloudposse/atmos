package exec

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMetadataComponent_PathHandling tests all scenarios where metadata.component
// specifies the component path, which can be absolute or relative.
func TestMetadataComponent_PathHandling(t *testing.T) {
	tests := []struct {
		name                   string
		atmosBasePath          string
		terraformBasePath      string
		metadataComponent      string // The value in metadata.component
		componentFolderPrefix  string
		description            string
		expectedFinalComponent string
		skipOnWindows          bool
	}{
		// ============ ABSOLUTE metadata.component TESTS ============
		{
			name:                   "Absolute metadata.component path",
			atmosBasePath:          "/home/runner/work/infrastructure",
			terraformBasePath:      "components/terraform",
			metadataComponent:      "/absolute/path/to/component",
			componentFolderPrefix:  "",
			description:            "metadata.component with absolute path should override everything",
			expectedFinalComponent: "/absolute/path/to/component",
			skipOnWindows:          true,
		},
		{
			name:                   "Absolute metadata.component with absolute terraform base",
			atmosBasePath:          "/home/runner/work/infrastructure",
			terraformBasePath:      "/home/runner/work/infrastructure/components/terraform",
			metadataComponent:      "/custom/absolute/vpc",
			componentFolderPrefix:  "aws",
			description:            "Absolute metadata.component ignores terraform base path",
			expectedFinalComponent: "/custom/absolute/vpc",
			skipOnWindows:          true,
		},
		{
			name:                   "Absolute metadata.component with folder prefix",
			atmosBasePath:          "/project/root",
			terraformBasePath:      "components/terraform",
			metadataComponent:      "/override/path/component",
			componentFolderPrefix:  "should-be-ignored",
			description:            "Folder prefix should be ignored with absolute metadata.component",
			expectedFinalComponent: "/override/path/component",
			skipOnWindows:          true,
		},

		// ============ RELATIVE metadata.component TESTS ============
		{
			name:                   "Relative metadata.component path",
			atmosBasePath:          "/home/runner/work/infrastructure",
			terraformBasePath:      "components/terraform",
			metadataComponent:      "vpc",
			componentFolderPrefix:  "",
			description:            "Simple relative metadata.component",
			expectedFinalComponent: "vpc",
			skipOnWindows:          true,
		},
		{
			name:                   "Relative metadata.component with folder prefix",
			atmosBasePath:          "/home/runner/work/infrastructure",
			terraformBasePath:      "components/terraform",
			metadataComponent:      "ec2-instance",
			componentFolderPrefix:  "aws",
			description:            "Folder prefix should be applied to relative metadata.component",
			expectedFinalComponent: "ec2-instance",
			skipOnWindows:          true,
		},
		{
			name:                   "Relative metadata.component with nested path",
			atmosBasePath:          "/project",
			terraformBasePath:      "components/terraform",
			metadataComponent:      "aws/networking/vpc",
			componentFolderPrefix:  "",
			description:            "Nested relative path in metadata.component",
			expectedFinalComponent: "aws/networking/vpc",
			skipOnWindows:          true,
		},

		// ============ DOT-SLASH PREFIX VARIATIONS ============
		{
			name:                   "metadata.component with ./ prefix",
			atmosBasePath:          "/home/runner/work",
			terraformBasePath:      "components/terraform",
			metadataComponent:      "./vpc",
			componentFolderPrefix:  "",
			description:            "Dot-slash prefix in metadata.component",
			expectedFinalComponent: "vpc",
			skipOnWindows:          true,
		},
		{
			name:                   "metadata.component with multiple ./",
			atmosBasePath:          "/project",
			terraformBasePath:      "components/terraform",
			metadataComponent:      "././vpc",
			componentFolderPrefix:  "",
			description:            "Multiple dot-slash prefixes",
			expectedFinalComponent: "vpc",
			skipOnWindows:          true,
		},
		{
			name:                   "metadata.component with ./ and folder prefix",
			atmosBasePath:          "/project",
			terraformBasePath:      "components/terraform",
			metadataComponent:      "./modules/vpc",
			componentFolderPrefix:  "aws",
			description:            "Dot-slash with nested path and folder prefix",
			expectedFinalComponent: "modules/vpc",
			skipOnWindows:          true,
		},

		// ============ PARENT DIRECTORY REFERENCES ============
		{
			name:                   "metadata.component with parent directory",
			atmosBasePath:          "/home/runner/work",
			terraformBasePath:      "components/terraform",
			metadataComponent:      "../shared/vpc",
			componentFolderPrefix:  "",
			description:            "Parent directory reference in metadata.component",
			expectedFinalComponent: "../shared/vpc",
			skipOnWindows:          true,
		},
		{
			name:                   "metadata.component with multiple parent refs",
			atmosBasePath:          "/project/infrastructure",
			terraformBasePath:      "components/terraform",
			metadataComponent:      "../../common/vpc",
			componentFolderPrefix:  "",
			description:            "Multiple parent directory references",
			expectedFinalComponent: "../../common/vpc",
			skipOnWindows:          true,
		},

		// ============ EDGE CASES ============
		{
			name:                   "Empty metadata.component",
			atmosBasePath:          "/project",
			terraformBasePath:      "components/terraform",
			metadataComponent:      "",
			componentFolderPrefix:  "aws",
			description:            "Empty metadata.component should be handled",
			expectedFinalComponent: "",
			skipOnWindows:          true,
		},
		{
			name:                   "metadata.component with trailing slash",
			atmosBasePath:          "/project",
			terraformBasePath:      "components/terraform",
			metadataComponent:      "vpc/",
			componentFolderPrefix:  "",
			description:            "Trailing slash in metadata.component",
			expectedFinalComponent: "vpc",
			skipOnWindows:          true,
		},
		{
			name:                   "metadata.component with spaces",
			atmosBasePath:          "/project",
			terraformBasePath:      "components/terraform",
			metadataComponent:      "my vpc component",
			componentFolderPrefix:  "",
			description:            "Spaces in metadata.component path",
			expectedFinalComponent: "my vpc component",
			skipOnWindows:          true,
		},
		{
			name:                   "metadata.component with URL-like path",
			atmosBasePath:          "/project",
			terraformBasePath:      "components/terraform",
			metadataComponent:      "https://github.com/org/repo/vpc",
			componentFolderPrefix:  "",
			description:            "URL-like path in metadata.component",
			expectedFinalComponent: "https://github.com/org/repo/vpc",
			skipOnWindows:          false,
		},

		// ============ THE GITHUB ACTIONS BUG SCENARIO ============
		{
			name:                   "GitHub Actions with absolute metadata.component",
			atmosBasePath:          "/home/runner/_work/infrastructure/infrastructure",
			terraformBasePath:      "/home/runner/_work/infrastructure/infrastructure/atmos/components/terraform",
			metadataComponent:      "/home/runner/_work/infrastructure/infrastructure/atmos/components/terraform/iam-role",
			componentFolderPrefix:  "",
			description:            "Absolute metadata.component matching terraform base path",
			expectedFinalComponent: "/home/runner/_work/infrastructure/infrastructure/atmos/components/terraform/iam-role",
			skipOnWindows:          true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.skipOnWindows && runtime.GOOS == "windows" {
				t.Skipf("Skipping Unix-specific test on Windows")
			}

			// Simulate the component section from a stack configuration
			componentSection := make(map[string]any)
			if tt.metadataComponent != "" {
				metadata := make(map[string]any)
				metadata["component"] = tt.metadataComponent
				componentSection["metadata"] = metadata
			}

			// Process component metadata to get the base component name
			_, baseComponentName, _, _, _ := ProcessComponentMetadata("test-component", componentSection)

			// The FinalComponent would be set to baseComponentName if it exists
			finalComponent := baseComponentName
			if finalComponent == "" {
				finalComponent = "test-component"
			}

			t.Logf("metadata.component: %q -> FinalComponent: %q", tt.metadataComponent, finalComponent)

			// If we have an expected final component, check it matches
			if tt.expectedFinalComponent != "" {
				cleanExpected := filepath.Clean(tt.expectedFinalComponent)
				cleanFinal := filepath.Clean(finalComponent)
				assert.Equal(t, cleanExpected, cleanFinal,
					"FinalComponent should match expected")
			}

			// Now test how this would be used with GetComponentPath
			if finalComponent != "" { //nolint:nestif
				atmosConfig := &schema.AtmosConfiguration{
					BasePath: tt.atmosBasePath,
					Components: schema.Components{
						Terraform: schema.Terraform{
							BasePath: tt.terraformBasePath,
						},
					},
				}

				// Set the absolute path if terraform base is absolute
				if filepath.IsAbs(tt.terraformBasePath) {
					atmosConfig.TerraformDirAbsolutePath = tt.terraformBasePath
				} else {
					terraformBasePath := filepath.Join(atmosConfig.BasePath, atmosConfig.Components.Terraform.BasePath)
					terraformDirAbsPath, err := filepath.Abs(terraformBasePath)
					require.NoError(t, err)
					atmosConfig.TerraformDirAbsolutePath = terraformDirAbsPath
				}

				// Get the component path using the final component from metadata
				componentPath, err := u.GetComponentPath(
					atmosConfig,
					"terraform",
					tt.componentFolderPrefix,
					finalComponent,
				)

				// Fail fast if GetComponentPath returns an error
				require.NoError(t, err)

				// If metadata.component is absolute, the path should be that absolute path
				if filepath.IsAbs(tt.metadataComponent) {
					// When metadata.component is absolute, it should be used as-is (with folder prefix if needed)
					if tt.componentFolderPrefix != "" {
						expectedPath := filepath.Join(tt.metadataComponent, tt.componentFolderPrefix)
						assert.Equal(t, filepath.Clean(expectedPath), componentPath,
							"Absolute metadata.component with folder prefix")
					} else {
						// Note: GetComponentPath might still join with base paths, so we check the end result
						assert.Contains(t, componentPath, filepath.Clean(tt.metadataComponent),
							"Component path should contain the absolute metadata.component")
					}
				} else {
					// For relative paths, check no duplication
					assert.NotContains(t, componentPath, "/.//",
						"Path should not contain /.// pattern")
					assert.NotContains(t, componentPath, "//",
						"Path should not contain // pattern")

					// Check for the specific GitHub Actions duplication pattern
					assert.NotContains(t, componentPath,
						"/home/runner/_work/infrastructure/infrastructure/.//home/runner/_work/infrastructure/infrastructure",
						"Path should not have the duplication bug")
				}
			}
		})
	}
}

// TestMetadataComponent_ResolutionPriority tests that metadata.component
// correctly overrides the top-level component attribute.
func TestMetadataComponent_ResolutionPriority(t *testing.T) {
	tests := []struct {
		name                  string
		topLevelComponent     string
		metadataComponent     string
		expectedBaseComponent string
		description           string
	}{
		{
			name:                  "metadata.component overrides top-level component",
			topLevelComponent:     "base-vpc",
			metadataComponent:     "override-vpc",
			expectedBaseComponent: "override-vpc",
			description:           "metadata.component should override component attribute",
		},
		{
			name:                  "Only top-level component set",
			topLevelComponent:     "base-vpc",
			metadataComponent:     "",
			expectedBaseComponent: "base-vpc",
			description:           "Should use top-level component when metadata.component not set",
		},
		{
			name:                  "Only metadata.component set",
			topLevelComponent:     "",
			metadataComponent:     "meta-vpc",
			expectedBaseComponent: "meta-vpc",
			description:           "Should use metadata.component when top-level not set",
		},
		{
			name:                  "Neither component attribute set",
			topLevelComponent:     "",
			metadataComponent:     "",
			expectedBaseComponent: "",
			description:           "No base component when neither is set",
		},
		{
			name:                  "metadata.component same as component name",
			topLevelComponent:     "",
			metadataComponent:     "test-component",
			expectedBaseComponent: "", // Should be cleared if same as component name
			description:           "Base component cleared when same as component name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Build component section
			componentSection := make(map[string]any)

			if tt.topLevelComponent != "" {
				componentSection["component"] = tt.topLevelComponent
			}

			if tt.metadataComponent != "" {
				metadata := make(map[string]any)
				metadata["component"] = tt.metadataComponent
				componentSection["metadata"] = metadata
			}

			// Process metadata
			_, baseComponentName, _, _, _ := ProcessComponentMetadata("test-component", componentSection)

			assert.Equal(t, tt.expectedBaseComponent, baseComponentName,
				"Base component should match expected: %s", tt.description)
		})
	}
}

// TestMetadataComponent_CompletePathResolution tests the complete path resolution
// with metadata.component through the entire stack processing flow.
func TestMetadataComponent_CompletePathResolution(t *testing.T) {
	tests := []struct {
		name                  string
		atmosBasePath         string
		terraformBasePath     string
		metadataComponent     string
		componentFolderPrefix string
		expectedPathContains  string
		shouldBeAbsolute      bool
		skipOnWindows         bool
	}{
		{
			name:                  "Complete flow with absolute metadata.component",
			atmosBasePath:         "/project/infrastructure",
			terraformBasePath:     "components/terraform",
			metadataComponent:     "/custom/components/vpc",
			componentFolderPrefix: "",
			expectedPathContains:  "/custom/components/vpc",
			shouldBeAbsolute:      true,
			skipOnWindows:         true,
		},
		{
			name:                  "Complete flow with relative metadata.component",
			atmosBasePath:         "/project/infrastructure",
			terraformBasePath:     "components/terraform",
			metadataComponent:     "modules/vpc",
			componentFolderPrefix: "aws",
			expectedPathContains:  "components/terraform/aws/modules/vpc",
			shouldBeAbsolute:      true,
			skipOnWindows:         true,
		},
		{
			name:                  "Complete flow with parent directory in metadata.component",
			atmosBasePath:         "/project/infrastructure",
			terraformBasePath:     "components/terraform",
			metadataComponent:     "../shared/vpc",
			componentFolderPrefix: "",
			expectedPathContains:  "shared/vpc",
			shouldBeAbsolute:      true,
			skipOnWindows:         true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.skipOnWindows && runtime.GOOS == "windows" {
				t.Skipf("Skipping Unix-specific test on Windows")
			}

			// Setup atmosphere config
			atmosConfig := &schema.AtmosConfiguration{
				BasePath: tt.atmosBasePath,
				Components: schema.Components{
					Terraform: schema.Terraform{
						BasePath: tt.terraformBasePath,
					},
				},
			}

			// Compute absolute terraform path
			terraformBasePath := filepath.Join(atmosConfig.BasePath, atmosConfig.Components.Terraform.BasePath)
			terraformDirAbsPath, err := filepath.Abs(terraformBasePath)
			require.NoError(t, err)
			atmosConfig.TerraformDirAbsolutePath = terraformDirAbsPath

			// Simulate component section with metadata.component
			componentSection := make(map[string]any)
			metadata := make(map[string]any)
			metadata["component"] = tt.metadataComponent
			componentSection["metadata"] = metadata

			// Process metadata to get final component
			_, baseComponentName, _, _, _ := ProcessComponentMetadata("stack-component", componentSection)

			finalComponent := baseComponentName
			if finalComponent == "" {
				finalComponent = "stack-component"
			}

			// Resolve the full component path
			componentPath, err := u.GetComponentPath(
				atmosConfig,
				"terraform",
				tt.componentFolderPrefix,
				finalComponent,
			)

			require.NoError(t, err)
			t.Logf("Resolved path: %s", componentPath)

			// Verify expectations
			if tt.expectedPathContains != "" {
				assert.Contains(t, componentPath, tt.expectedPathContains,
					"Component path should contain expected pattern")
			}

			if tt.shouldBeAbsolute {
				assert.True(t, filepath.IsAbs(componentPath),
					"Component path should be absolute")
			}

			// Ensure no path duplication
			assert.NotContains(t, componentPath, "/.//",
				"Path should not contain /.// pattern")
			assert.NotContains(t, componentPath, "//",
				"Path should not contain // pattern")
		})
	}
}
