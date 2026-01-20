package exec

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestPrintOrWriteToFile(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			Terminal: schema.Terminal{
				TabWidth: 4,
			},
		},
	}

	// Test data to write
	testData := map[string]interface{}{
		"test": "data",
		"nested": map[string]interface{}{
			"key": "value",
		},
	}

	err := printOrWriteToFile(atmosConfig, "yaml", "", testData)
	assert.NoError(t, err)

	err = printOrWriteToFile(atmosConfig, "json", "", testData)
	assert.NoError(t, err)

	tempDir := t.TempDir()

	yamlFile := filepath.Join(tempDir, "test.yaml")
	err = printOrWriteToFile(atmosConfig, "yaml", yamlFile, testData)
	assert.NoError(t, err)

	_, err = os.Stat(yamlFile)
	assert.NoError(t, err)

	jsonFile := filepath.Join(tempDir, "test.json")
	err = printOrWriteToFile(atmosConfig, "json", jsonFile, testData)
	assert.NoError(t, err)

	_, err = os.Stat(jsonFile)
	assert.NoError(t, err)

	err = printOrWriteToFile(atmosConfig, "invalid", "", testData)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid 'format'")

	// Test with default tab width (when TabWidth is 0)
	atmosConfigDefaultTabWidth := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			Terminal: schema.Terminal{
				TabWidth: 0, // Should default to 2
			},
		},
	}
	err = printOrWriteToFile(atmosConfigDefaultTabWidth, "yaml", "", testData)
	assert.NoError(t, err)
}

func TestSanitizeFileName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
		skipOS   string // Skip test on this OS
	}{
		{
			name:     "simple filename",
			input:    "file.txt",
			expected: "file.txt",
		},
		{
			name:     "filename with path",
			input:    "/path/to/file.txt",
			expected: "file.txt",
		},
		{
			name:     "uri with query string",
			input:    "https://example.com/file.txt?version=1",
			expected: "file.txt",
		},
		{
			name:  "invalid windows chars - colon",
			input: "file:name.txt",
			// Colon makes it parse as URL scheme "file:", so path is empty, returns "."
			expected: ".",
		},
		{
			name:  "invalid windows chars - asterisk",
			input: "file*name.txt",
			expected: func() string {
				if runtime.GOOS == "windows" {
					return "file_name.txt"
				}
				return "file*name.txt"
			}(),
		},
		{
			name:  "invalid windows chars - question mark",
			input: "file?name.txt",
			// Question mark is query string delimiter, so returns base "file"
			expected: "file",
		},
		{
			name:  "invalid windows chars - quotes",
			input: `file"name.txt`,
			expected: func() string {
				if runtime.GOOS == "windows" {
					return "file_name.txt"
				}
				return `file"name.txt`
			}(),
		},
		{
			name:  "invalid windows chars - pipe",
			input: "file|name.txt",
			expected: func() string {
				if runtime.GOOS == "windows" {
					return "file_name.txt"
				}
				return "file|name.txt"
			}(),
		},
		{
			name:     "multiple slashes",
			input:    "path/to/nested/file.txt",
			expected: "file.txt",
		},
		{
			name:  "backslashes on windows",
			input: `path\to\file.txt`,
			// On Unix, backslashes are not path separators, so whole string is the basename
			// On Windows, they are path separators, so returns "file.txt"
			expected: func() string {
				if runtime.GOOS == "windows" {
					return "file.txt"
				}
				return `path\to\file.txt`
			}(),
		},
		{
			name:  "invalid url - fallback to base",
			input: "://invalid",
			// Falls back to filepath.Base which returns "invalid"
			expected: "invalid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.skipOS != "" && runtime.GOOS == tt.skipOS {
				t.Skipf("Skipping test on %s", tt.skipOS)
			}

			result := SanitizeFileName(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestToFileScheme(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
		skipOS   string // Skip test on this OS
	}{
		{
			name:     "unix absolute path",
			input:    "/tmp/foo.json",
			expected: "file:///tmp/foo.json",
			skipOS:   "windows", // Unix paths don't work correctly on Windows
		},
		{
			name:     "unix nested path",
			input:    "/path/to/nested/file.yaml",
			expected: "file:///path/to/nested/file.yaml",
			skipOS:   "windows", // Unix paths don't work correctly on Windows
		},
		{
			name:     "unix relative path",
			input:    "relative/path/file.txt",
			expected: "file:///relative/path/file.txt",
			skipOS:   "windows", // Relative paths handled differently on Windows
		},
	}

	// Add Windows-specific tests
	if runtime.GOOS == "windows" {
		tests = append(tests, []struct {
			name     string
			input    string
			expected string
			skipOS   string
		}{
			{
				name:     "windows absolute path",
				input:    `D:\Temp\foo.json`,
				expected: "file://D:/Temp/foo.json",
			},
			{
				name:     "windows path with forward slashes",
				input:    "D:/Temp/foo.json",
				expected: "file://D:/Temp/foo.json",
			},
			{
				name:     "windows UNC path",
				input:    `\\server\share\file.txt`,
				expected: "file://server/share/file.txt",
			},
		}...)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.skipOS != "" && runtime.GOOS == tt.skipOS {
				t.Skipf("Skipping test on %s: path format not applicable", tt.skipOS)
			}

			result := toFileScheme(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFixWindowsFileScheme(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expectError bool
		checkResult func(*testing.T, string)
	}{
		{
			name:        "valid http url - no changes",
			input:       "https://example.com/path",
			expectError: false,
			checkResult: func(t *testing.T, result string) {
				assert.Equal(t, "https://example.com/path", result)
			},
		},
		{
			name:        "file url on non-windows - no changes",
			input:       "file:///tmp/foo.json",
			expectError: false,
			checkResult: func(t *testing.T, result string) {
				if runtime.GOOS != "windows" {
					assert.Equal(t, "file:///tmp/foo.json", result)
				}
			},
		},
		{
			name:        "invalid url",
			input:       "://invalid",
			expectError: true,
		},
	}

	// Add Windows-specific tests
	if runtime.GOOS == "windows" {
		tests = append(tests, []struct {
			name        string
			input       string
			expectError bool
			checkResult func(*testing.T, string)
		}{
			{
				name:        "windows file url with host",
				input:       "file://D:/Temp/foo.json",
				expectError: false,
				checkResult: func(t *testing.T, result string) {
					assert.Equal(t, "file://D:/Temp/foo.json", result)
				},
			},
			{
				name:        "windows file url with leading slash",
				input:       "file:///D:/Temp/foo.json",
				expectError: false,
				checkResult: func(t *testing.T, result string) {
					assert.Equal(t, "file://D:/Temp/foo.json", result)
				},
			},
		}...)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := fixWindowsFileScheme(tt.input)

			if tt.expectError {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
			if tt.checkResult != nil {
				tt.checkResult(t, result.String())
			}
		})
	}
}

func TestRemoveTempDir(t *testing.T) {
	tests := []struct {
		name    string
		setup   func() string // Returns path to test with
		wantLog bool          // Should log a warning
	}{
		{
			name: "remove existing directory",
			setup: func() string {
				return t.TempDir()
			},
			wantLog: false,
		},
		{
			name: "remove non-existent directory - no error logged",
			setup: func() string {
				return filepath.Join(os.TempDir(), "atmos-non-existent-dir-12345")
			},
			wantLog: false, // os.RemoveAll doesn't error on non-existent paths
		},
		{
			name: "remove directory with files",
			setup: func() string {
				dir := t.TempDir()
				// Create a file inside
				file := filepath.Join(dir, "test.txt")
				err := os.WriteFile(file, []byte("test"), 0o644)
				assert.NoError(t, err)
				return dir
			},
			wantLog: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := tt.setup()

			// Call RemoveTempDir - it doesn't return anything
			RemoveTempDir(path)

			// Verify directory was removed
			_, err := os.Stat(path)
			assert.True(t, os.IsNotExist(err), "Directory should be removed")
		})
	}
}
