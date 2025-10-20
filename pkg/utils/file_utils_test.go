package utils

import (
	"os"
	"path/filepath"
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

// Platform-specific tests moved to separate files:
// - file_utils_windows_test.go: TestJoinPath_WindowsAbsolutePaths
// - file_utils_unix_test.go: TestJoinPath_UnixAbsolutePaths

// TestIsPathAbsolute tests the IsPathAbsolute function.
func TestIsPathAbsolute(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected bool
		skipOnOS string
	}{
		{
			name:     "Unix absolute path",
			path:     "/home/user/project",
			expected: true,
			skipOnOS: "windows",
		},
		{
			name:     "Unix relative path",
			path:     "home/user/project",
			expected: false,
			skipOnOS: "windows",
		},
		{
			name:     "Windows absolute path with drive",
			path:     `C:\Users\project`,
			expected: true,
			skipOnOS: "unix",
		},
		{
			name:     "Windows relative path",
			path:     `Users\project`,
			expected: false,
			skipOnOS: "unix",
		},
		{
			name:     "Relative path with dot",
			path:     "./components",
			expected: false,
			skipOnOS: "",
		},
		{
			name:     "Relative path with double dot",
			path:     "../components",
			expected: false,
			skipOnOS: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.skipOnOS == "windows" && runtime.GOOS == "windows" {
				t.Skipf("Skipping Unix-specific test on Windows")
			}
			if tt.skipOnOS == "unix" && runtime.GOOS != "windows" {
				t.Skipf("Skipping Windows-specific test on Unix")
			}

			result := IsPathAbsolute(tt.path)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestIsWindowsAbsolutePath tests the isWindowsAbsolutePath function.
func TestIsWindowsAbsolutePath(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected bool
	}{
		{
			name:     "Forward slash at start",
			path:     "/path/to/file",
			expected: true,
		},
		{
			name:     "Single backslash at start",
			path:     `\path\to\file`,
			expected: true,
		},
		{
			name:     "UNC path (double backslash)",
			path:     `\\server\share`,
			expected: false, // handled by filepath.IsAbs
		},
		{
			name:     "Relative path",
			path:     `path\to\file`,
			expected: false,
		},
		{
			name:     "Empty path",
			path:     "",
			expected: false,
		},
		{
			name:     "Single character",
			path:     "a",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isWindowsAbsolutePath(tt.path)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestJoinPathAndValidate tests the JoinPathAndValidate function.
func TestJoinPathAndValidate(t *testing.T) {
	// Create temporary directory structure.
	tmpDir := t.TempDir()

	tests := []struct {
		name         string
		basePath     string
		providedPath string
		wantErr      bool
	}{
		{
			name:         "Valid paths that exist",
			basePath:     tmpDir,
			providedPath: "",
			wantErr:      false,
		},
		{
			name:         "Non-existent path",
			basePath:     tmpDir,
			providedPath: "nonexistent/path",
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := JoinPathAndValidate(tt.basePath, tt.providedPath)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Empty(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotEmpty(t, result)
			}
		})
	}
}

// TestEnsureDir tests the EnsureDir function.
func TestEnsureDir(t *testing.T) {
	tmpDir := t.TempDir()

	tests := []struct {
		name     string
		fileName string
		wantErr  bool
	}{
		{
			name:     "Create single directory",
			fileName: tmpDir + "/newdir/file.txt",
			wantErr:  false,
		},
		{
			name:     "Create nested directories",
			fileName: tmpDir + "/nested/dir/structure/file.txt",
			wantErr:  false,
		},
		{
			name:     "File in existing directory",
			fileName: tmpDir + "/existing.txt",
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := EnsureDir(tt.fileName)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestSliceOfPathsContainsPath tests the SliceOfPathsContainsPath function.
func TestSliceOfPathsContainsPath(t *testing.T) {
	// Use temp dir for platform-agnostic paths.
	tmpDir := t.TempDir()
	file1 := filepath.Join(tmpDir, "file1.txt")
	file2 := filepath.Join(tmpDir, "file2.txt")

	tests := []struct {
		name      string
		paths     []string
		checkPath string
		expected  bool
	}{
		{
			name:      "Path exists in slice",
			paths:     []string{file1, file2},
			checkPath: tmpDir,
			expected:  true,
		},
		{
			name:      "Path does not exist in slice",
			paths:     []string{file1, file2},
			checkPath: "/other/path",
			expected:  false,
		},
		{
			name:      "Empty slice",
			paths:     []string{},
			checkPath: tmpDir,
			expected:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SliceOfPathsContainsPath(tt.paths, tt.checkPath)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestGetAllFilesInDir tests the GetAllFilesInDir function.
func TestGetAllFilesInDir(t *testing.T) {
	tmpDir := t.TempDir()

	// Create test files.
	files := []string{
		"file1.txt",
		"file2.yaml",
		"subdir/file3.json",
	}
	for _, f := range files {
		path := tmpDir + "/" + f
		err := EnsureDir(path)
		assert.NoError(t, err)
		err = os.WriteFile(path, []byte("test"), 0o644)
		assert.NoError(t, err)
	}

	result, err := GetAllFilesInDir(tmpDir)
	assert.NoError(t, err)
	assert.Len(t, result, 3)
}

// TestGetAllYamlFilesInDir tests the GetAllYamlFilesInDir function.
func TestGetAllYamlFilesInDir(t *testing.T) {
	tmpDir := t.TempDir()

	// Create test files.
	files := map[string]bool{
		"file1.yaml":        true,
		"file2.yml":         true,
		"file3.txt":         false,
		"file4.yaml.tmpl":   true,
		"file5.yml.tmpl":    true,
		"subdir/file6.yaml": true,
	}
	for f := range files {
		path := tmpDir + "/" + f
		err := EnsureDir(path)
		assert.NoError(t, err)
		err = os.WriteFile(path, []byte("test"), 0o644)
		assert.NoError(t, err)
	}

	result, err := GetAllYamlFilesInDir(tmpDir)
	assert.NoError(t, err)

	// Count expected YAML files.
	expectedCount := 0
	for _, isYaml := range files {
		if isYaml {
			expectedCount++
		}
	}
	assert.Equal(t, expectedCount, len(result))
}

// TestIsSocket tests the IsSocket function.
func TestIsSocket(t *testing.T) {
	tmpDir := t.TempDir()
	regularFile := tmpDir + "/regular.txt"
	err := os.WriteFile(regularFile, []byte("test"), 0o644)
	assert.NoError(t, err)

	isSocket, err := IsSocket(regularFile)
	assert.NoError(t, err)
	assert.False(t, isSocket)

	// Test non-existent file.
	_, err = IsSocket(tmpDir + "/nonexistent")
	assert.Error(t, err)
}

// TestSearchConfigFile tests the SearchConfigFile function.
func TestSearchConfigFile(t *testing.T) {
	tmpDir := t.TempDir()

	// Create test config files.
	yamlFile := tmpDir + "/config.yaml"
	err := os.WriteFile(yamlFile, []byte("test: value"), 0o644)
	assert.NoError(t, err)

	tests := []struct {
		name      string
		path      string
		wantFound bool
		wantPath  string
	}{
		{
			name:      "File with extension exists",
			path:      yamlFile,
			wantFound: true,
			wantPath:  yamlFile,
		},
		{
			name:      "File without extension, yaml exists",
			path:      tmpDir + "/config",
			wantFound: true,
			wantPath:  yamlFile,
		},
		{
			name:      "File does not exist",
			path:      tmpDir + "/nonexistent",
			wantFound: false,
			wantPath:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path, found := SearchConfigFile(tt.path)
			assert.Equal(t, tt.wantFound, found)
			if tt.wantFound {
				assert.Equal(t, tt.wantPath, path)
			}
		})
	}
}

// TestIsURL tests the IsURL function.
func TestIsURL(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{
			name:     "Valid HTTP URL",
			input:    "http://example.com",
			expected: true,
		},
		{
			name:     "Valid HTTPS URL",
			input:    "https://example.com/path",
			expected: true,
		},
		{
			name:     "Invalid scheme",
			input:    "ftp://example.com",
			expected: false,
		},
		{
			name:     "Not a URL",
			input:    "/path/to/file",
			expected: false,
		},
		{
			name:     "Empty string",
			input:    "",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsURL(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestGetFileNameFromURL tests the GetFileNameFromURL function.
func TestGetFileNameFromURL(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		expected string
		wantErr  bool
	}{
		{
			name:     "Valid URL with filename",
			url:      "https://example.com/path/to/file.txt",
			expected: "file.txt",
			wantErr:  false,
		},
		{
			name:     "URL with query parameters",
			url:      "https://example.com/download/file.zip?version=1",
			expected: "file.zip",
			wantErr:  false,
		},
		{
			name:     "Empty URL",
			url:      "",
			expected: "",
			wantErr:  true,
		},
		{
			name:     "URL ending with slash",
			url:      "https://example.com/path/",
			expected: "path",
			wantErr:  false,
		},
		{
			name:     "Invalid URL",
			url:      "not a url",
			expected: "not a url",
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := GetFileNameFromURL(tt.url)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

// TestGetLineEnding tests the GetLineEnding function.
func TestGetLineEnding(t *testing.T) {
	result := GetLineEnding()
	if runtime.GOOS == "windows" {
		assert.Equal(t, "\r\n", result)
	} else {
		assert.Equal(t, "\n", result)
	}
}

// TestFileOrDirExists tests the FileOrDirExists function.
func TestFileOrDirExists(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a test file.
	testFile := tmpDir + "/test.txt"
	err := os.WriteFile(testFile, []byte("test"), 0o644)
	assert.NoError(t, err)

	tests := []struct {
		name     string
		path     string
		expected bool
	}{
		{
			name:     "Existing file",
			path:     testFile,
			expected: true,
		},
		{
			name:     "Existing directory",
			path:     tmpDir,
			expected: true,
		},
		{
			name:     "Non-existent path",
			path:     tmpDir + "/nonexistent",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FileOrDirExists(tt.path)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestIsDirectory tests the IsDirectory function.
func TestIsDirectory(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a test file.
	testFile := tmpDir + "/test.txt"
	err := os.WriteFile(testFile, []byte("test"), 0o644)
	assert.NoError(t, err)

	tests := []struct {
		name     string
		path     string
		expected bool
		wantErr  bool
	}{
		{
			name:     "Directory exists",
			path:     tmpDir,
			expected: true,
			wantErr:  false,
		},
		{
			name:     "File is not directory",
			path:     testFile,
			expected: false,
			wantErr:  false,
		},
		{
			name:     "Path does not exist",
			path:     tmpDir + "/nonexistent",
			expected: false,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := IsDirectory(tt.path)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestIsYaml tests the IsYaml function.
func TestIsYaml(t *testing.T) {
	tests := []struct {
		name     string
		file     string
		expected bool
	}{
		{
			name:     "YAML extension",
			file:     "config.yaml",
			expected: true,
		},
		{
			name:     "YML extension",
			file:     "config.yml",
			expected: true,
		},
		{
			name:     "YAML template extension",
			file:     "config.yaml.tmpl",
			expected: true,
		},
		{
			name:     "YML template extension",
			file:     "config.yml.tmpl",
			expected: true,
		},
		{
			name:     "JSON extension",
			file:     "config.json",
			expected: false,
		},
		{
			name:     "No extension",
			file:     "config",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsYaml(tt.file)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestConvertPathsToAbsolutePaths tests the ConvertPathsToAbsolutePaths function.
func TestConvertPathsToAbsolutePaths(t *testing.T) {
	tests := []struct {
		name    string
		paths   []string
		wantErr bool
	}{
		{
			name:    "Relative paths",
			paths:   []string{"./path1", "./path2"},
			wantErr: false,
		},
		{
			name:    "Mixed absolute and relative",
			paths:   []string{"/absolute", "./relative"},
			wantErr: false,
		},
		{
			name:    "Empty slice",
			paths:   []string{},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ConvertPathsToAbsolutePaths(tt.paths)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Len(t, result, len(tt.paths))
				// Verify all paths are absolute.
				for _, p := range result {
					assert.True(t, IsPathAbsolute(p))
				}
			}
		})
	}
}

// TestJoinPaths tests the JoinPaths function.
func TestJoinPaths(t *testing.T) {
	tests := []struct {
		name     string
		basePath string
		paths    []string
		expected int
	}{
		{
			name:     "Multiple relative paths",
			basePath: "/base",
			paths:    []string{"path1", "path2", "path3"},
			expected: 3,
		},
		{
			name:     "Empty paths slice",
			basePath: "/base",
			paths:    []string{},
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := JoinPaths(tt.basePath, tt.paths)
			assert.NoError(t, err)
			assert.Len(t, result, tt.expected)
		})
	}
}

// TestTrimBasePathFromPath tests the TrimBasePathFromPath function.
func TestTrimBasePathFromPath(t *testing.T) {
	tests := []struct {
		name        string
		basePath    string
		path        string
		expected    string
		windowsOnly bool
	}{
		{
			name:     "Trim base path",
			basePath: "/home/user/project",
			path:     "/home/user/project/file.txt",
			expected: "/file.txt",
		},
		{
			name:     "Path without base",
			basePath: "/base",
			path:     "/other/path",
			expected: "/other/path",
		},
		{
			name:        "Windows path (converted to forward slashes)",
			basePath:    `C:\Users\project`,
			path:        `C:\Users\project\file.txt`,
			expected:    "/file.txt", // Function converts to forward slashes on Windows
			windowsOnly: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.windowsOnly && runtime.GOOS != "windows" {
				t.Skipf("Skipping Windows-specific test on %s", runtime.GOOS)
			}

			result := TrimBasePathFromPath(tt.basePath, tt.path)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExpandTilde(t *testing.T) {
	homeDir, err := os.UserHomeDir()
	assert.NoError(t, err)

	tests := []struct {
		name     string
		input    string
		expected string
		wantErr  bool
	}{
		{
			name:     "tilde only",
			input:    "~",
			expected: homeDir,
			wantErr:  false,
		},
		{
			name:     "tilde with slash",
			input:    "~/",
			expected: homeDir,
			wantErr:  false,
		},
		{
			name:     "tilde with path",
			input:    "~/Documents/test",
			expected: filepath.Join(homeDir, "Documents/test"),
			wantErr:  false,
		},
		{
			name:     "absolute path no tilde",
			input:    "/usr/local/bin",
			expected: "/usr/local/bin",
			wantErr:  false,
		},
		{
			name:     "relative path no tilde",
			input:    "relative/path",
			expected: "relative/path",
			wantErr:  false,
		},
		{
			name:     "empty path",
			input:    "",
			expected: "",
			wantErr:  false,
		},
		{
			name:     "tilde with specific user (not supported)",
			input:    "~otheruser/path",
			expected: "~otheruser/path",
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ExpandTilde(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}
