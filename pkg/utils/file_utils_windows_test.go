//go:build windows
// +build windows

package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestJoinPath_WindowsEdgeCases tests comprehensive Windows path edge cases.
func TestJoinPath_WindowsEdgeCases(t *testing.T) {
	tests := []struct {
		name         string
		basePath     string
		providedPath string
		expected     string
		description  string
	}{
		// ============ STANDARD WINDOWS PATHS ============
		{
			name:         "GitHub Actions Windows - both absolute",
			basePath:     `C:\Users\runner\_work\infrastructure\infrastructure`,
			providedPath: `C:\Users\runner\_work\infrastructure\infrastructure\atmos\components\terraform`,
			expected:     `C:\Users\runner\_work\infrastructure\infrastructure\atmos\components\terraform`,
			description:  "Should not duplicate when both paths are absolute",
		},
		{
			name:         "GitHub Actions Windows - relative component",
			basePath:     `C:\Users\runner\_work\infrastructure\infrastructure`,
			providedPath: `atmos\components\terraform`,
			expected:     `C:\Users\runner\_work\infrastructure\infrastructure\atmos\components\terraform`,
			description:  "Should join properly with relative path",
		},

		// ============ UNC PATHS (NETWORK SHARES) ============
		{
			name:         "UNC path as base with relative",
			basePath:     `\\server\share\project`,
			providedPath: `components\terraform`,
			expected:     `\\server\share\project\components\terraform`,
			description:  "UNC paths should work as base",
		},
		{
			name:         "UNC path as base with absolute C: drive",
			basePath:     `\\server\share\project`,
			providedPath: `C:\components\terraform`,
			expected:     `C:\components\terraform`,
			description:  "Absolute path should override UNC base",
		},
		{
			name:         "Both paths are UNC",
			basePath:     `\\server1\share1\project`,
			providedPath: `\\server2\share2\components`,
			expected:     `\\server2\share2\components`,
			description:  "Second UNC should override first",
		},
		{
			name:         "UNC with IP address",
			basePath:     `\\192.168.1.100\share\project`,
			providedPath: `components\terraform`,
			expected:     `\\192.168.1.100\share\project\components\terraform`,
			description:  "IP-based UNC paths should work",
		},

		// ============ DRIVE LETTER VARIATIONS ============
		{
			name:         "Different drive letters",
			basePath:     `C:\project`,
			providedPath: `D:\components`,
			expected:     `D:\components`,
			description:  "Should use absolute path from different drive",
		},
		{
			name:         "Drive root only",
			basePath:     `C:\`,
			providedPath: `components\terraform`,
			expected:     `C:\components\terraform`,
			description:  "Should join to drive root",
		},
		{
			name:         "Drive without backslash",
			basePath:     `C:`,
			providedPath: `components\terraform`,
			expected:     `C:components\terraform`,
			description:  "Drive letter without backslash (current directory on drive)",
		},
		{
			name:         "Lowercase drive letter",
			basePath:     `c:\project`,
			providedPath: `components\terraform`,
			expected:     `c:\project\components\terraform`,
			description:  "Lowercase drive letters should work",
		},
		{
			name:         "Mixed case drive override",
			basePath:     `c:\project`,
			providedPath: `C:\Components`,
			expected:     `C:\Components`,
			description:  "Case shouldn't matter for drive letters",
		},

		// ============ PATHS WITH SPACES AND SPECIAL CHARS ============
		{
			name:         "Path with spaces",
			basePath:     `C:\Program Files\My Project`,
			providedPath: `components\terraform module`,
			expected:     `C:\Program Files\My Project\components\terraform module`,
			description:  "Spaces in paths should be preserved",
		},
		{
			name:         "Path with parentheses",
			basePath:     `C:\Program Files (x86)\project`,
			providedPath: `components\terraform`,
			expected:     `C:\Program Files (x86)\project\components\terraform`,
			description:  "Parentheses should be handled",
		},
		{
			name:         "Path with dots",
			basePath:     `C:\project.v1.2.3`,
			providedPath: `components.new\terraform`,
			expected:     `C:\project.v1.2.3\components.new\terraform`,
			description:  "Dots in directory names",
		},
		{
			name:         "Path with dashes and underscores",
			basePath:     `C:\my-project_2024`,
			providedPath: `components-prod\terraform_modules`,
			expected:     `C:\my-project_2024\components-prod\terraform_modules`,
			description:  "Special characters in names",
		},
		{
			name:         "Path with @ symbol",
			basePath:     `C:\projects\team@company`,
			providedPath: `components\terraform`,
			expected:     `C:\projects\team@company\components\terraform`,
			description:  "@ symbol in path",
		},

		// ============ TRAILING AND MULTIPLE SEPARATORS ============
		{
			name:         "Trailing backslash on base",
			basePath:     `C:\project\`,
			providedPath: `components\terraform`,
			expected:     `C:\project\components\terraform`,
			description:  "Trailing backslash should be handled",
		},
		{
			name:         "Leading backslash on provided",
			basePath:     `C:\project`,
			providedPath: `\components\terraform`,
			expected:     `\components\terraform`,
			description:  "Leading backslash indicates absolute path on Windows",
		},
		{
			name:         "Multiple backslashes (UNC-like path)",
			basePath:     `C:\\project\\`,
			providedPath: `\\components\\terraform`,
			expected:     `\\components\\terraform`,
			description:  "Double backslash at start treated as UNC-like absolute path",
		},

		// ============ MIXED FORWARD AND BACKWARD SLASHES ============
		{
			name:         "Forward slashes in Windows path",
			basePath:     `C:/project/folder`,
			providedPath: `components/terraform`,
			expected:     `C:\project\folder\components\terraform`,
			description:  "filepath.Join normalizes to OS separator",
		},
		{
			name:         "Mixed slashes",
			basePath:     `C:\project`,
			providedPath: `components/terraform/module`,
			expected:     `C:\project\components\terraform\module`,
			description:  "filepath.Join normalizes to OS separator",
		},
		{
			name:         "Unix-style absolute on Windows",
			basePath:     `C:\project`,
			providedPath: `/usr/local/bin`,
			expected:     `/usr/local/bin`,
			description:  "Unix absolute path on Windows returns unchanged",
		},

		// ============ WINDOWS LONG PATH SUPPORT ============
		{
			name:         "Long path prefix",
			basePath:     `\\?\C:\very\long\path\project`,
			providedPath: `components\terraform`,
			expected:     `\\?\C:\very\long\path\project\components\terraform`,
			description:  "Windows extended-length path prefix",
		},
		{
			name:         "Long UNC path",
			basePath:     `\\?\UNC\server\share\project`,
			providedPath: `components\terraform`,
			expected:     `\\?\UNC\server\share\project\components\terraform`,
			description:  "Extended-length UNC path",
		},

		// ============ RELATIVE PATH COMPONENTS ============
		{
			name:         "Current directory dot",
			basePath:     `C:\project`,
			providedPath: `.\components\terraform`,
			expected:     `C:\project\components\terraform`,
			description:  "filepath.Join cleans dot for current directory",
		},
		{
			name:         "Parent directory dots",
			basePath:     `C:\project\subfolder`,
			providedPath: `..\components\terraform`,
			expected:     `C:\project\components\terraform`,
			description:  "filepath.Join resolves parent directory navigation",
		},
		{
			name:         "Multiple parent directories",
			basePath:     `C:\a\b\c\d`,
			providedPath: `..\..\components`,
			expected:     `C:\a\b\components`,
			description:  "filepath.Join resolves multiple parent directory navigations",
		},

		// ============ ENVIRONMENT VARIABLES (NOT EXPANDED) ============
		{
			name:         "Environment variable in path",
			basePath:     `%USERPROFILE%\projects`,
			providedPath: `components\terraform`,
			expected:     `%USERPROFILE%\projects\components\terraform`,
			description:  "Environment variables not expanded",
		},
		{
			name:         "System drive variable",
			basePath:     `%SystemDrive%\project`,
			providedPath: `components\terraform`,
			expected:     `%SystemDrive%\project\components\terraform`,
			description:  "System drive variable preserved",
		},
		{
			name:         "Program files variable",
			basePath:     `%ProgramFiles%\MyApp`,
			providedPath: `config\settings`,
			expected:     `%ProgramFiles%\MyApp\config\settings`,
			description:  "Program files variable",
		},

		// ============ EDGE CASES ============
		{
			name:         "Empty provided path",
			basePath:     `C:\project`,
			providedPath: ``,
			expected:     `C:\project`,
			description:  "Empty provided returns base",
		},
		{
			name:         "Empty base path",
			basePath:     ``,
			providedPath: `C:\components`,
			expected:     `C:\components`,
			description:  "Empty base with absolute provided",
		},
		{
			name:         "Both paths empty",
			basePath:     ``,
			providedPath: ``,
			expected:     ``,
			description:  "Both empty returns empty string",
		},
		{
			name:         "Same absolute paths",
			basePath:     `C:\project\components`,
			providedPath: `C:\project\components`,
			expected:     `C:\project\components`,
			description:  "Identical paths return as-is",
		},
		{
			name:         "Reserved names",
			basePath:     `C:\project`,
			providedPath: `CON\components`,
			expected:     `C:\project\CON\components`,
			description:  "Windows reserved names (CON, PRN, AUX, etc.)",
		},
		{
			name:         "Path with colon (invalid but test behavior)",
			basePath:     `C:\project`,
			providedPath: `time:12:30\log`,
			expected:     `C:\project\time:12:30\log`,
			description:  "Colon in path component",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := JoinPath(tt.basePath, tt.providedPath)
			assert.Equal(t, tt.expected, result, tt.description)

			// Additional validation for path duplication
			if tt.basePath != "" && tt.providedPath != "" {
				// Check for the specific GitHub Actions duplication bug
				if tt.basePath == `C:\Users\runner\_work\infrastructure\infrastructure` &&
					tt.providedPath == `C:\Users\runner\_work\infrastructure\infrastructure\atmos\components\terraform` {
					assert.NotContains(t, result, `C:\Users\runner\_work\infrastructure\infrastructure\C:\Users`,
						"Should not duplicate the GitHub Actions path")
				}
			}
		})
	}
}

// TestJoinPath_WindowsPathNormalization tests how paths are normalized on Windows.
func TestJoinPath_WindowsPathNormalization(t *testing.T) {
	tests := []struct {
		name         string
		basePath     string
		providedPath string
		description  string
		checkFunc    func(t *testing.T, result string)
	}{
		{
			name:         "Preserve exact separators",
			basePath:     `C:\project`,
			providedPath: `components\terraform`,
			description:  "Should use backslashes on Windows",
			checkFunc: func(t *testing.T, result string) {
				assert.Equal(t, `C:\project\components\terraform`, result)
				assert.Contains(t, result, `\`)
				assert.NotContains(t, result, `/`)
			},
		},
		{
			name:         "Normalize multiple slashes",
			basePath:     `C:\\project\\`,
			providedPath: `\\components\\`,
			description:  "filepath.Join normalizes multiple slashes",
			checkFunc: func(t *testing.T, result string) {
				// Double backslash at start is treated as UNC path by filepath.IsAbs.
				// So it returns as absolute, but filepath.Join would clean it.
				assert.Equal(t, `\\components\\`, result)
			},
		},
		{
			name:         "Case preservation",
			basePath:     `C:\Project\MyFolder`,
			providedPath: `Components\Terraform`,
			description:  "Windows preserves case even though it's case-insensitive",
			checkFunc: func(t *testing.T, result string) {
				assert.Equal(t, `C:\Project\MyFolder\Components\Terraform`, result)
				assert.Contains(t, result, `MyFolder`)
				assert.Contains(t, result, `Components`)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := JoinPath(tt.basePath, tt.providedPath)
			tt.checkFunc(t, result)
		})
	}
}

// TestJoinPath_WindowsAbsolutePaths specifically tests Windows absolute path scenarios.
func TestJoinPath_WindowsAbsolutePaths(t *testing.T) {
	tests := []struct {
		name         string
		basePath     string
		providedPath string
		expected     string
	}{
		{
			name:         "GitHub Actions Windows path - components absolute",
			basePath:     `C:\Users\runner\_work\infrastructure\infrastructure`,
			providedPath: `C:\Users\runner\_work\infrastructure\infrastructure\atmos\components\terraform`,
			expected:     `C:\Users\runner\_work\infrastructure\infrastructure\atmos\components\terraform`,
		},
		{
			name:         "GitHub Actions Windows path - components relative",
			basePath:     `C:\Users\runner\_work\infrastructure\infrastructure`,
			providedPath: `atmos\components\terraform`,
			expected:     `C:\Users\runner\_work\infrastructure\infrastructure\atmos\components\terraform`,
		},
		{
			name:         "Windows UNC path",
			basePath:     `\\server\share\project`,
			providedPath: `components\terraform`,
			expected:     `\\server\share\project\components\terraform`,
		},
		{
			name:         "Different drive letters",
			basePath:     `C:\project`,
			providedPath: `D:\components`,
			expected:     `D:\components`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := JoinPath(tt.basePath, tt.providedPath)
			assert.Equal(t, tt.expected, result)
			// Verify no path duplication.
			assert.NotContains(t, result, `C:\Users\runner\_work\infrastructure\infrastructure\C:\Users`,
				"Should not duplicate absolute paths")
		})
	}
}
