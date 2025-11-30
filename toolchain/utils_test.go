package toolchain

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseToolVersionArg(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		expectedTool  string
		expectedVer   string
		expectError   bool
		errorContains string
	}{
		{
			name:         "terraform@1.11.4",
			input:        "terraform@1.11.4",
			expectedTool: "terraform",
			expectedVer:  "1.11.4",
			expectError:  false,
		},
		{
			name:         "hashicorp/terraform@1.11.4",
			input:        "hashicorp/terraform@1.11.4",
			expectedTool: "hashicorp/terraform",
			expectedVer:  "1.11.4",
			expectError:  false,
		},
		{
			name:         "terraform",
			input:        "terraform",
			expectedTool: "terraform",
			expectedVer:  "",
			expectError:  false,
		},
		{
			name:          "terraform@",
			input:         "terraform@",
			expectedTool:  "",
			expectedVer:   "",
			expectError:   true,
			errorContains: "missing version after @",
		},
		{
			name:          "@1.11.4",
			input:         "@1.11.4",
			expectedTool:  "",
			expectedVer:   "",
			expectError:   true,
			errorContains: "missing tool name before @",
		},
		{
			name:          "empty string",
			input:         "",
			expectedTool:  "",
			expectedVer:   "",
			expectError:   true,
			errorContains: "empty tool argument",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tool, ver, err := ParseToolVersionArg(tt.input)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedTool, tool)
				assert.Equal(t, tt.expectedVer, ver)
			}
		})
	}
}

func TestIsSpecialVersion(t *testing.T) {
	testCases := []struct {
		version  string
		expected bool
	}{
		{"latest", true},
		{"system", true},
		{"1.11.4", false},
		{"v1.11.4", false},
		{"", false},
	}

	for _, tc := range testCases {
		t.Run(tc.version, func(t *testing.T) {
			result := isSpecialVersion(tc.version)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestSortVersionsSemver(t *testing.T) {
	tests := []struct {
		name     string
		input    []string
		expected []string
	}{
		{
			name:     "normal semver versions",
			input:    []string{"1.11.4", "1.9.8", "1.12.0", "1.8.0"},
			expected: []string{"1.8.0", "1.9.8", "1.11.4", "1.12.0"},
		},
		{
			name:     "with special versions",
			input:    []string{"latest", "1.11.4", "system", "1.9.8"},
			expected: []string{"1.9.8", "1.11.4", "latest", "system"},
		},
		{
			name:     "empty list",
			input:    []string{},
			expected: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sortVersionsSemver(tt.input)

			// Handle the case where we might get nil instead of empty slice
			if len(tt.expected) == 0 {
				assert.True(t, len(result) == 0, "Expected empty result, got %v", result)
			} else {
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestBasicFileOperations(t *testing.T) {
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test.txt")
	testContent := []byte("test content")

	// Test file operations
	err := os.WriteFile(testFile, testContent, defaultFileWritePermissions)
	require.NoError(t, err)

	// Verify file exists
	_, err = os.Stat(testFile)
	assert.NoError(t, err)

	// Test reading file
	content, err := os.ReadFile(testFile)
	assert.NoError(t, err)
	assert.Equal(t, testContent, content)

	// Test file size
	info, err := os.Stat(testFile)
	assert.NoError(t, err)
	assert.Equal(t, int64(len(testContent)), info.Size())
}

func TestBasicDirectoryOperations(t *testing.T) {
	tempDir := t.TempDir()
	newDir := filepath.Join(tempDir, "newdir")

	// Test creating directory
	err := os.Mkdir(newDir, defaultMkdirPermissions)
	assert.NoError(t, err)

	// Verify directory exists
	_, err = os.Stat(newDir)
	assert.NoError(t, err)

	// Test creating existing directory (should error).
	err = os.Mkdir(newDir, defaultMkdirPermissions)
	assert.Error(t, err) // Should error because directory already exists

	// Test creating directory with MkdirAll (should not error)
	err = os.MkdirAll(newDir, defaultMkdirPermissions)
	assert.NoError(t, err) // Should not error because MkdirAll is idempotent
}
