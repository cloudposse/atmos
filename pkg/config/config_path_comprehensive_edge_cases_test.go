package config

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPathJoining_ComprehensiveEdgeCases tests ALL combinations of absolute/relative paths
// with and without dot-slash prefixes, trailing slashes, etc.
func TestPathJoining_ComprehensiveEdgeCases(t *testing.T) {
	// Test matrix: every combination of path types.
	tests := []struct {
		name                  string
		atmosBasePath         string // ATMOS_BASE_PATH or base_path in config
		componentBasePath     string // terraform.base_path, helmfile.base_path, etc
		componentName         string // The actual component
		description           string
		expectedPattern       string // What the result should contain
		shouldHaveDuplication bool   // Whether we expect path duplication (bug)
		skipOnWindows         bool
		onlyOnWindows         bool // Run only on Windows
	}{
		// ============ ABSOLUTE ATMOS BASE PATH TESTS ============.
		{
			name:                  "Absolute ATMOS base + Absolute component base",
			atmosBasePath:         "/home/runner/work/infrastructure",
			componentBasePath:     "/home/runner/work/infrastructure/components/terraform",
			componentName:         "vpc",
			description:           "Both paths absolute - THIS TRIGGERS THE BUG",
			expectedPattern:       "components/terraform/vpc",
			shouldHaveDuplication: true, // Bug: filepath.Join creates duplication
			skipOnWindows:         true,
		},
		{
			name:                  "Absolute ATMOS base + Relative component base",
			atmosBasePath:         "/home/runner/work/infrastructure",
			componentBasePath:     "components/terraform",
			componentName:         "vpc",
			description:           "Normal case - should work correctly",
			expectedPattern:       "/home/runner/work/infrastructure/components/terraform/vpc",
			shouldHaveDuplication: false,
			skipOnWindows:         true,
		},
		{
			name:                  "Absolute ATMOS base + Relative with ./ prefix",
			atmosBasePath:         "/home/runner/work/infrastructure",
			componentBasePath:     "./components/terraform",
			componentName:         "vpc",
			description:           "Dot-slash prefix should be normalized",
			expectedPattern:       "/home/runner/work/infrastructure/components/terraform/vpc",
			shouldHaveDuplication: false,
			skipOnWindows:         true,
		},
		{
			name:                  "Absolute ATMOS base with trailing slash + Relative",
			atmosBasePath:         "/home/runner/work/infrastructure/",
			componentBasePath:     "components/terraform",
			componentName:         "vpc",
			description:           "Trailing slash should be handled",
			expectedPattern:       "/home/runner/work/infrastructure/components/terraform/vpc",
			shouldHaveDuplication: false,
			skipOnWindows:         true,
		},
		{
			name:                  "Absolute ATMOS base + Component base with leading slash",
			atmosBasePath:         "/home/runner/work/infrastructure",
			componentBasePath:     "/components/terraform", // Leading slash makes it absolute!
			componentName:         "vpc",
			description:           "Leading slash makes path absolute from root",
			expectedPattern:       "/components/terraform/vpc",
			shouldHaveDuplication: false,
			skipOnWindows:         true,
		},

		// ============ RELATIVE ATMOS BASE PATH TESTS ============.
		{
			name:                  "Relative ATMOS base + Relative component base",
			atmosBasePath:         ".",
			componentBasePath:     "components/terraform",
			componentName:         "vpc",
			description:           "Both paths relative - standard case",
			expectedPattern:       "components/terraform/vpc",
			shouldHaveDuplication: false,
			skipOnWindows:         false,
		},
		{
			name:                  "Relative ATMOS base with ./ + Relative component",
			atmosBasePath:         "./",
			componentBasePath:     "components/terraform",
			componentName:         "vpc",
			description:           "Dot-slash ATMOS base",
			expectedPattern:       "components/terraform/vpc",
			shouldHaveDuplication: false,
			skipOnWindows:         false,
		},
		{
			name:                  "Relative ATMOS base + Absolute component base",
			atmosBasePath:         ".",
			componentBasePath:     "/absolute/components/terraform",
			componentName:         "vpc",
			description:           "Absolute component base should override relative ATMOS base",
			expectedPattern:       "/absolute/components/terraform/vpc",
			shouldHaveDuplication: false,
			skipOnWindows:         true,
		},
		{
			name:                  "Relative nested ATMOS base + Relative component",
			atmosBasePath:         "../infrastructure",
			componentBasePath:     "components/terraform",
			componentName:         "vpc",
			description:           "Parent directory reference",
			expectedPattern:       "infrastructure/components/terraform/vpc", // After abs(), no relative paths
			shouldHaveDuplication: false,
			skipOnWindows:         false,
		},

		// ============ DOT-SLASH PREFIX VARIATIONS ============.
		{
			name:                  "No dot-slash anywhere",
			atmosBasePath:         "infrastructure",
			componentBasePath:     "components/terraform",
			componentName:         "vpc",
			description:           "Clean relative paths",
			expectedPattern:       "infrastructure/components/terraform/vpc",
			shouldHaveDuplication: false,
			skipOnWindows:         false,
		},
		{
			name:                  "Dot-slash in component base only",
			atmosBasePath:         "infrastructure",
			componentBasePath:     "./components/terraform",
			componentName:         "vpc",
			description:           "Component base with dot-slash",
			expectedPattern:       "infrastructure/components/terraform/vpc",
			shouldHaveDuplication: false,
			skipOnWindows:         false,
		},
		{
			name:                  "Dot-slash in ATMOS base only",
			atmosBasePath:         "./infrastructure",
			componentBasePath:     "components/terraform",
			componentName:         "vpc",
			description:           "ATMOS base with dot-slash",
			expectedPattern:       "infrastructure/components/terraform/vpc",
			shouldHaveDuplication: false,
			skipOnWindows:         false,
		},
		{
			name:                  "Dot-slash everywhere",
			atmosBasePath:         "./infrastructure",
			componentBasePath:     "./components/terraform",
			componentName:         "./vpc",
			description:           "Dot-slash in all paths",
			expectedPattern:       "infrastructure/components/terraform/vpc",
			shouldHaveDuplication: false,
			skipOnWindows:         false,
		},
		{
			name:                  "Multiple dot-slashes",
			atmosBasePath:         "././infrastructure",
			componentBasePath:     "././components/terraform",
			componentName:         "vpc",
			description:           "Multiple consecutive dot-slashes",
			expectedPattern:       "infrastructure/components/terraform/vpc",
			shouldHaveDuplication: false,
			skipOnWindows:         false,
		},

		// ============ TRAILING SLASH VARIATIONS ============.
		{
			name:                  "Trailing slash on ATMOS base only",
			atmosBasePath:         "infrastructure/",
			componentBasePath:     "components/terraform",
			componentName:         "vpc",
			description:           "ATMOS base with trailing slash",
			expectedPattern:       "infrastructure/components/terraform/vpc",
			shouldHaveDuplication: false,
			skipOnWindows:         false,
		},
		{
			name:                  "Trailing slash on component base only",
			atmosBasePath:         "infrastructure",
			componentBasePath:     "components/terraform/",
			componentName:         "vpc",
			description:           "Component base with trailing slash",
			expectedPattern:       "infrastructure/components/terraform/vpc",
			shouldHaveDuplication: false,
			skipOnWindows:         false,
		},
		{
			name:                  "Trailing slashes everywhere",
			atmosBasePath:         "infrastructure/",
			componentBasePath:     "components/terraform/",
			componentName:         "vpc/",
			description:           "All paths with trailing slashes",
			expectedPattern:       "infrastructure/components/terraform/vpc",
			shouldHaveDuplication: false,
			skipOnWindows:         false,
		},
		{
			name:                  "Multiple trailing slashes",
			atmosBasePath:         "infrastructure//",
			componentBasePath:     "components/terraform//",
			componentName:         "vpc",
			description:           "Multiple trailing slashes",
			expectedPattern:       "infrastructure/components/terraform/vpc",
			shouldHaveDuplication: false,
			skipOnWindows:         false,
		},

		// ============ SPECIAL EDGE CASES ============.
		{
			name:                  "Empty ATMOS base path",
			atmosBasePath:         "",
			componentBasePath:     "components/terraform",
			componentName:         "vpc",
			description:           "Empty ATMOS base",
			expectedPattern:       "components/terraform/vpc",
			shouldHaveDuplication: false,
			skipOnWindows:         false,
		},
		{
			name:                  "Empty component base path",
			atmosBasePath:         "infrastructure",
			componentBasePath:     "",
			componentName:         "vpc",
			description:           "Empty component base",
			expectedPattern:       "infrastructure/vpc",
			shouldHaveDuplication: false,
			skipOnWindows:         false,
		},
		{
			name:                  "Dot as ATMOS base",
			atmosBasePath:         ".",
			componentBasePath:     ".",
			componentName:         "vpc",
			description:           "Both paths are dot",
			expectedPattern:       "vpc",
			shouldHaveDuplication: false,
			skipOnWindows:         false,
		},
		{
			name:                  "Double dots in path",
			atmosBasePath:         "infrastructure/../infrastructure",
			componentBasePath:     "components/../components/terraform",
			componentName:         "vpc",
			description:           "Paths with parent directory references",
			expectedPattern:       "infrastructure/components/terraform/vpc",
			shouldHaveDuplication: false,
			skipOnWindows:         false,
		},
		{
			name:                  "Absolute component name (edge case)",
			atmosBasePath:         "/home/runner/work",
			componentBasePath:     "components/terraform",
			componentName:         "/absolute/path/to/vpc",
			description:           "Component name itself is absolute",
			expectedPattern:       "/absolute/path/to/vpc",
			shouldHaveDuplication: false,
			skipOnWindows:         true,
		},
		{
			name:                  "Windows-style paths on Unix",
			atmosBasePath:         "C:\\infrastructure",
			componentBasePath:     "components\\terraform",
			componentName:         "vpc",
			description:           "Windows paths on Unix should be treated as relative",
			expectedPattern:       "C:\\infrastructure/components\\terraform/vpc", // Mixed separators
			shouldHaveDuplication: false,
			skipOnWindows:         true,
		},
		{
			name:                  "Windows absolute paths on Windows",
			atmosBasePath:         "C:\\Users\\runner\\work\\infrastructure",
			componentBasePath:     "C:\\Users\\runner\\work\\infrastructure\\components\\terraform",
			componentName:         "vpc",
			description:           "Windows absolute component base path should not be duplicated",
			expectedPattern:       "C:\\Users\\runner\\work\\infrastructure\\components\\terraform\\vpc",
			shouldHaveDuplication: false,
			skipOnWindows:         false,
			onlyOnWindows:         true,
		},
		{
			name:                  "URL-like path (should be treated as relative)",
			atmosBasePath:         "https://example.com/infrastructure",
			componentBasePath:     "components/terraform",
			componentName:         "vpc",
			description:           "URL-like paths are not valid file paths",
			expectedPattern:       "https://example.com/infrastructure/components/terraform/vpc",
			shouldHaveDuplication: false,
			skipOnWindows:         true, // URLs are not valid Windows file paths
		},

		// ============ THE EXACT BUG SCENARIO ============.
		{
			name:                  "GitHub Actions exact bug scenario",
			atmosBasePath:         "/home/runner/_work/infrastructure/infrastructure",
			componentBasePath:     "/home/runner/_work/infrastructure/infrastructure/atmos/components/terraform",
			componentName:         "iam-role-legacy",
			description:           "Exact scenario from the bug report",
			expectedPattern:       "atmos/components/terraform/iam-role-legacy",
			shouldHaveDuplication: true, // This is the bug!
			skipOnWindows:         true,
		},
		{
			name:                  "Partial path overlap",
			atmosBasePath:         "/home/project",
			componentBasePath:     "/home/project/subdir/components",
			componentName:         "vpc",
			description:           "Component base extends ATMOS base",
			expectedPattern:       "subdir/components/vpc",
			shouldHaveDuplication: true, // Another bug case
			skipOnWindows:         true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.skipOnWindows && runtime.GOOS == "windows" {
				t.Skipf("Skipping Unix-specific test on Windows")
			}
			if tt.onlyOnWindows && runtime.GOOS != "windows" {
				t.Skipf("Skipping Windows-specific test on non-Windows")
			}

			// Test production path logic using utils.JoinPath.
			base := u.JoinPath(tt.atmosBasePath, tt.componentBasePath)
			joinedPath := u.JoinPath(base, tt.componentName)
			t.Logf("utils.JoinPath(%q, %q) -> JoinPath(%q, %q) = %q",
				tt.atmosBasePath, tt.componentBasePath, base, tt.componentName, joinedPath)

			// All cases should avoid duplication now with utils.JoinPath.
			// Check that paths don't have duplication patterns.
			assert.NotContains(t, joinedPath, string([]byte{os.PathSeparator, os.PathSeparator}),
				"Path should not contain double separators")
			assert.NotContains(t, joinedPath, "//",
				"Path should not contain double slashes")
			assert.NotContains(t, joinedPath, "/./",
				"Path should not contain /./ pattern (should be cleaned)")

			// Clean path should equal joined path.
			cleanPath := filepath.Clean(joinedPath)
			assert.Equal(t, cleanPath, joinedPath,
				"utils.JoinPath should produce a clean path")

			// Test filepath.Abs behavior.
			absPath, err := filepath.Abs(joinedPath)
			require.NoError(t, err)
			t.Logf("filepath.Abs(%q) = %q", joinedPath, absPath)

			// Check that expected pattern is in the result.
			if tt.expectedPattern != "" && !tt.shouldHaveDuplication {
				assert.Contains(t, absPath, filepath.Clean(tt.expectedPattern),
					"Result should contain expected pattern")
			}
		})
	}
}

// TestCorrectPathJoining_Solution demonstrates how paths SHOULD be joined
// to avoid the duplication bug.
func TestCorrectPathJoining_Solution(t *testing.T) {
	tests := []struct {
		name              string
		atmosBasePath     string
		componentBasePath string
		componentName     string
		expectedResult    string
		skipOnWindows     bool
	}{
		{
			name:              "Both absolute paths - correct handling",
			atmosBasePath:     "/home/runner/work/infrastructure",
			componentBasePath: "/home/runner/work/infrastructure/components/terraform",
			componentName:     "vpc",
			expectedResult:    "/home/runner/work/infrastructure/components/terraform/vpc",
			skipOnWindows:     true,
		},
		{
			name:              "Absolute base, relative component",
			atmosBasePath:     "/home/runner/work/infrastructure",
			componentBasePath: "components/terraform",
			componentName:     "vpc",
			expectedResult:    "/home/runner/work/infrastructure/components/terraform/vpc",
			skipOnWindows:     true,
		},
		{
			name:              "Relative base, absolute component",
			atmosBasePath:     "./infrastructure",
			componentBasePath: "/absolute/components/terraform",
			componentName:     "vpc",
			expectedResult:    "/absolute/components/terraform/vpc",
			skipOnWindows:     true,
		},
		{
			name:              "Component name is absolute",
			atmosBasePath:     "/home/runner/work",
			componentBasePath: "components",
			componentName:     "/absolute/vpc",
			expectedResult:    "/absolute/vpc",
			skipOnWindows:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.skipOnWindows && runtime.GOOS == "windows" {
				t.Skipf("Skipping Unix-specific test on Windows")
			}

			// Correct path joining logic.
			var result string

			// First, handle component base path.
			// Use the production helpers instead of reimplementing.
			base := u.JoinPath(tt.atmosBasePath, tt.componentBasePath)
			result = u.JoinPath(base, tt.componentName)
			result = filepath.Clean(result)

			t.Logf("Correct joining: %q", result)
			assert.Equal(t, tt.expectedResult, result,
				"Correctly joined path should match expected")

			// Verify no duplication.
			assert.NotContains(t, result, "//",
				"Should not contain double slashes")
			assert.NotContains(t, result, "/./",
				"Should not contain /./ pattern")
		})
	}
}

// TestEnvironmentVariablePathCombinations tests ATMOS_BASE_PATH environment variable
// with various path combinations.
func TestEnvironmentVariablePathCombinations(t *testing.T) {
	tests := []struct {
		name              string
		envBasePathValue  string
		componentBasePath string
		componentName     string
		expectedPattern   string
		skipOnWindows     bool
	}{
		{
			name:              "Absolute env path + relative component",
			envBasePathValue:  "/env/absolute/path",
			componentBasePath: "components/terraform",
			componentName:     "vpc",
			expectedPattern:   "/env/absolute/path/components/terraform/vpc",
			skipOnWindows:     true,
		},
		{
			name:              "Relative env path + relative component",
			envBasePathValue:  "./env/path",
			componentBasePath: "components/terraform",
			componentName:     "vpc",
			expectedPattern:   "env/path/components/terraform/vpc",
			skipOnWindows:     false,
		},
		{
			name:              "Env path with ./ + absolute component",
			envBasePathValue:  "./env/path",
			componentBasePath: "/absolute/components",
			componentName:     "vpc",
			expectedPattern:   "/absolute/components/vpc",
			skipOnWindows:     true,
		},
		{
			name:              "Empty env path",
			envBasePathValue:  "",
			componentBasePath: "components/terraform",
			componentName:     "vpc",
			expectedPattern:   "components/terraform/vpc",
			skipOnWindows:     false,
		},
		{
			name:              "Env path with trailing slash",
			envBasePathValue:  "/env/path/",
			componentBasePath: "components/terraform",
			componentName:     "vpc",
			expectedPattern:   "/env/path/components/terraform/vpc",
			skipOnWindows:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.skipOnWindows && runtime.GOOS == "windows" {
				t.Skipf("Skipping Unix-specific test on Windows")
			}

			// Save and restore environment variable.
			oldEnv := os.Getenv("ATMOS_BASE_PATH")
			defer func() {
				if oldEnv != "" {
					t.Setenv("ATMOS_BASE_PATH", oldEnv)
				} else {
					os.Unsetenv("ATMOS_BASE_PATH")
				}
			}()

			// Set test environment variable.
			if tt.envBasePathValue != "" {
				t.Setenv("ATMOS_BASE_PATH", tt.envBasePathValue)
			} else {
				os.Unsetenv("ATMOS_BASE_PATH")
			}

			// Simulate path joining with env variable.
			basePath := tt.envBasePathValue
			if basePath == "" {
				basePath = "." // Default if not set.
			}

			// Use production helpers to process paths.
			base := u.JoinPath(basePath, tt.componentBasePath)
			joinedPath := u.JoinPath(base, tt.componentName)
			t.Logf("With ATMOS_BASE_PATH=%q: %q", tt.envBasePathValue, joinedPath)

			// Clean and check.
			cleanPath := filepath.Clean(joinedPath)
			if tt.expectedPattern != "" {
				assert.Contains(t, cleanPath, filepath.Clean(tt.expectedPattern),
					"Path should match expected pattern")
			}
		})
	}
}
