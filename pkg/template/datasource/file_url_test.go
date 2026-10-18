package datasource

import (
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestToFileScheme(t *testing.T) {
	t.Parallel()

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
			t.Parallel()
			if tt.skipOS != "" && runtime.GOOS == tt.skipOS {
				t.Skipf("Skipping test on %s: path format not applicable", tt.skipOS)
			}

			result := toFileScheme(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFixWindowsFileScheme(t *testing.T) {
	t.Parallel()

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
			t.Parallel()
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
