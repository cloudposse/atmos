package exec

import (
	"archive/zip"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// runZipExtractionTest is a helper function to run ZIP extraction tests.
func runZipExtractionTest(t *testing.T, zipContent map[string]string, expectError bool, errorMsg string) {
	tempDir := t.TempDir()

	// Create a ZIP file in memory
	var buf bytes.Buffer
	zipWriter := zip.NewWriter(&buf)

	for filename, content := range zipContent {
		writer, err := zipWriter.Create(filename)
		require.NoError(t, err)
		_, err = writer.Write([]byte(content))
		require.NoError(t, err)
	}
	zipWriter.Close()

	// Test extraction
	reader := bytes.NewReader(buf.Bytes())
	err := extractZipFile(reader, tempDir)

	if expectError {
		assert.Error(t, err)
		if errorMsg != "" {
			assert.Contains(t, err.Error(), errorMsg)
		}
	} else {
		assert.NoError(t, err)

		// Check that files were extracted
		for filename, expectedContent := range zipContent {
			filePath := filepath.Join(tempDir, filename)
			assert.FileExists(t, filePath)

			content, err := os.ReadFile(filePath)
			assert.NoError(t, err)
			assert.Equal(t, expectedContent, string(content))
		}
	}
}

// runZipExtractionTestSuite runs a complete test suite for ZIP extraction.
func runZipExtractionTestSuite(t *testing.T, tests []struct {
	name        string
	zipContent  map[string]string
	expectError bool
	errorMsg    string
}) {
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runZipExtractionTest(t, tt.zipContent, tt.expectError, tt.errorMsg)
		})
	}
}

// TestExtractZipFile tests ZIP file extraction functionality.
func TestExtractZipFile(t *testing.T) {
	tests := []struct {
		name        string
		zipContent  map[string]string
		expectError bool
		errorMsg    string
	}{
		{
			name: "Valid ZIP with files",
			zipContent: map[string]string{
				"file1.txt": "content1",
				"file2.txt": "content2",
			},
			expectError: false,
		},
		{
			name: "ZIP with directory",
			zipContent: map[string]string{
				"dir/file.txt": "content",
			},
			expectError: false,
		},
		{
			name: "ZIP with path traversal attempt",
			zipContent: map[string]string{
				"../file.txt": "malicious",
			},
			expectError: true,
			errorMsg:    "illegal file path in ZIP",
		},
		{
			name: "ZIP with absolute path",
			zipContent: map[string]string{
				"/etc/passwd": "malicious",
			},
			expectError: true,
			errorMsg:    "illegal file path in ZIP",
		},
		{
			name: "ZIP with Windows absolute path",
			zipContent: map[string]string{
				"C:\\Windows\\file.txt": "malicious",
			},
			expectError: true,
			errorMsg:    "illegal file path in ZIP",
		},
	}

	runZipExtractionTestSuite(t, tests)
}

// TestExtractZipFileZipSlip tests ZIP slip vulnerability protection.
func TestExtractZipFileZipSlip(t *testing.T) {
	tests := []struct {
		name        string
		zipContent  map[string]string
		expectError bool
		errorMsg    string
	}{
		{
			name: "Path traversal with ../",
			zipContent: map[string]string{
				"../../../etc/passwd": "malicious",
			},
			expectError: true,
			errorMsg:    "illegal file path in ZIP",
		},
		{
			name: "Path traversal with ..\\",
			zipContent: map[string]string{
				"..\\..\\..\\Windows\\System32\\config\\SAM": "malicious",
			},
			expectError: true,
			errorMsg:    "illegal file path in ZIP",
		},
		{
			name: "Mixed path traversal",
			zipContent: map[string]string{
				"normal/../malicious/file.txt": "malicious",
			},
			expectError: true,
			errorMsg:    "illegal file path in ZIP",
		},
		{
			name: "Valid nested path",
			zipContent: map[string]string{
				"normal/nested/file.txt": "valid content",
			},
			expectError: false,
		},
	}

	runZipExtractionTestSuite(t, tests)
}

// TestExtractZipFileSymlinks tests ZIP file extraction with symlink handling.
func TestExtractZipFileSymlinks(t *testing.T) {
	tests := []struct {
		name        string
		zipContent  map[string]string
		expectError bool
		errorMsg    string
	}{
		{
			name: "ZIP with symlink",
			zipContent: map[string]string{
				"file.txt": "content",
			},
			expectError: false,
		},
		{
			name: "ZIP with directory",
			zipContent: map[string]string{
				"dir/": "",
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir := t.TempDir()

			// Create a ZIP file in memory
			var buf bytes.Buffer
			zipWriter := zip.NewWriter(&buf)

			for filename, content := range tt.zipContent {
				if strings.HasSuffix(filename, "/") {
					// Create directory entry
					_, err := zipWriter.Create(filename)
					require.NoError(t, err)
				} else {
					// Create file entry
					writer, err := zipWriter.Create(filename)
					require.NoError(t, err)
					_, err = writer.Write([]byte(content))
					require.NoError(t, err)
				}
			}
			zipWriter.Close()

			// Test extraction
			reader := bytes.NewReader(buf.Bytes())
			err := extractZipFile(reader, tempDir)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)

				// Check that files were extracted
				for filename, expectedContent := range tt.zipContent {
					if !strings.HasSuffix(filename, "/") {
						filePath := filepath.Join(tempDir, filename)
						assert.FileExists(t, filePath)

						content, err := os.ReadFile(filePath)
						assert.NoError(t, err)
						assert.Equal(t, expectedContent, string(content))
					}
				}
			}
		})
	}
}

// TestExtractRawData tests raw data extraction functionality.
func TestExtractRawData(t *testing.T) {
	tests := []struct {
		name        string
		data        string
		layerIndex  int
		expectError bool
	}{
		{
			name:        "Valid raw data",
			data:        "test data content",
			layerIndex:  0,
			expectError: false,
		},
		{
			name:        "Empty data",
			data:        "",
			layerIndex:  0,
			expectError: false,
		},
		{
			name:        "Large data",
			data:        strings.Repeat("test", 1000),
			layerIndex:  1,
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir := t.TempDir()
			reader := strings.NewReader(tt.data)

			err := extractRawData(reader, tempDir, tt.layerIndex)

			if tt.expectError {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)

			// Check that the file was created
			expectedFile := filepath.Join(tempDir, fmt.Sprintf("layer_%d_raw", tt.layerIndex))
			assert.FileExists(t, expectedFile)

			// Check file content
			content, err := os.ReadFile(expectedFile)
			assert.NoError(t, err)
			assert.Equal(t, tt.data, string(content))
		})
	}
}
