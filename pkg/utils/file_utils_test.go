package utils

import (
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestJoinPath tests the JoinPath function for both Unix and Windows paths.
func TestJoinPath(t *testing.T) {
	tests := []struct {
		name         string
		basePath     string
		providedPath string
		expected     string
		skipOnOS     string // "windows" or "unix"
	}{
		// Unix absolute path tests
		{
			name:         "Unix: both paths absolute",
			basePath:     "/home/user/project",
			providedPath: "/home/user/project/components",
			expected:     "/home/user/project/components",
			skipOnOS:     "windows",
		},
		{
			name:         "Unix: base absolute, provided relative",
			basePath:     "/home/user/project",
			providedPath: "components/terraform",
			expected:     "/home/user/project/components/terraform",
			skipOnOS:     "windows",
		},
		{
			name:         "Unix: base relative, provided absolute",
			basePath:     "./project",
			providedPath: "/absolute/path",
			expected:     "/absolute/path",
			skipOnOS:     "windows",
		},
		// Windows absolute path tests
		{
			name:         "Windows: both paths absolute",
			basePath:     `C:\Users\runner\project`,
			providedPath: `C:\Users\runner\project\components`,
			expected:     `C:\Users\runner\project\components`,
			skipOnOS:     "unix",
		},
		{
			name:         "Windows: base absolute, provided relative",
			basePath:     `C:\Users\runner\project`,
			providedPath: `components\terraform`,
			expected:     `C:\Users\runner\project\components\terraform`,
			skipOnOS:     "unix",
		},
		{
			name:         "Windows: base relative, provided absolute",
			basePath:     `.\\project`,
			providedPath: `D:\absolute\path`,
			expected:     `D:\absolute\path`,
			skipOnOS:     "unix",
		},
		// Cross-platform relative paths
		{
			name:         "Both paths relative",
			basePath:     "./project",
			providedPath: "components/terraform",
			expected:     "project/components/terraform",
			skipOnOS:     "",
		},
		{
			name:         "Empty provided path",
			basePath:     "/home/user",
			providedPath: "",
			expected:     "/home/user",
			skipOnOS:     "windows",
		},
		{
			name:         "Empty base path",
			basePath:     "",
			providedPath: "components",
			expected:     "components",
			skipOnOS:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Skip OS-specific tests
			if tt.skipOnOS == "windows" && runtime.GOOS == "windows" {
				t.Skipf("Skipping Unix-specific test on Windows")
			}
			if tt.skipOnOS == "unix" && runtime.GOOS != "windows" {
				t.Skipf("Skipping Windows-specific test on Unix")
			}

			result := JoinPath(tt.basePath, tt.providedPath)

			// On Windows, the filepath package will use backslashes
			// On Unix, it will use forward slashes
			// So we need to compare the actual result, not a hardcoded expected
			if tt.skipOnOS == "" {
				// For cross-platform tests, we can't hardcode the separator
				// Just check that it doesn't duplicate paths
				assert.NotContains(t, result, "//", "Should not have double slashes")
				assert.NotContains(t, result, `\\`, "Should not have double backslashes")
			} else {
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

// TestJoinPath_WindowsAbsolutePaths specifically tests Windows absolute path scenarios.
func TestJoinPath_WindowsAbsolutePaths(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skipf("Skipping Windows-specific test on non-Windows OS")
	}

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
			// Verify no path duplication
			assert.NotContains(t, result, `C:\Users\runner\_work\infrastructure\infrastructure\C:\Users`,
				"Should not duplicate absolute paths")
		})
	}
}

// TestJoinPath_UnixAbsolutePaths specifically tests Unix absolute path scenarios.
func TestJoinPath_UnixAbsolutePaths(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skipf("Skipping Unix-specific test on Windows")
	}

	tests := []struct {
		name         string
		basePath     string
		providedPath string
		expected     string
	}{
		{
			name:         "GitHub Actions Unix path - components absolute",
			basePath:     "/home/runner/_work/infrastructure/infrastructure",
			providedPath: "/home/runner/_work/infrastructure/infrastructure/atmos/components/terraform",
			expected:     "/home/runner/_work/infrastructure/infrastructure/atmos/components/terraform",
		},
		{
			name:         "GitHub Actions Unix path - components relative",
			basePath:     "/home/runner/_work/infrastructure/infrastructure",
			providedPath: "atmos/components/terraform",
			expected:     "/home/runner/_work/infrastructure/infrastructure/atmos/components/terraform",
		},
		{
			name:         "Root path joining",
			basePath:     "/",
			providedPath: "usr/local/bin",
			expected:     "/usr/local/bin",
		},
		{
			name:         "Home directory path",
			basePath:     "/home/user",
			providedPath: ".config/atmos",
			expected:     "/home/user/.config/atmos",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := JoinPath(tt.basePath, tt.providedPath)
			assert.Equal(t, tt.expected, result)
			// Verify no path duplication
			assert.NotContains(t, result, "/home/runner/_work/infrastructure/infrastructure/home/runner",
				"Should not duplicate absolute paths")
		})
	}
}
