package filetype

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestExtractFilenameFromPath tests the filename extraction from paths and URLs.
func TestExtractFilenameFromPath(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		// Simple filenames
		{"simple filename", "file.json", "file.json"},
		{"filename with path", "/path/to/file.yaml", "file.yaml"},
		{"relative path", "relative/path/file.txt", "file.txt"},

		// URLs without query or fragment
		{"HTTP URL", "https://example.com/file.json", "file.json"},
		{"URL with path", "https://example.com/path/to/file.yaml", "file.yaml"},
		{"S3 URL", "s3://bucket/path/file.json", "file.json"},
		{"GCS URL", "gcs://bucket/folder/file.hcl", "file.hcl"},
		{"File URL", "file:///home/user/file.txt", "file.txt"},

		// URLs with query strings
		{"URL with single query param", "https://example.com/file.json?version=2", "file.json"},
		{"URL with multiple query params", "https://example.com/file.yaml?v=1&format=raw", "file.yaml"},
		{"URL with encoded query", "https://example.com/file.txt?filter=key%3Dvalue", "file.txt"},
		{"S3 with version ID", "s3://bucket/file.json?versionId=abc123", "file.json"},

		// URLs with fragments
		{"URL with fragment", "https://example.com/file.json#section", "file.json"},
		{"URL with complex fragment", "https://example.com/file.yaml#key.subkey", "file.yaml"},
		{"Local path with fragment", "/path/file.md#heading", "file.md"},

		// URLs with both query and fragment
		{"URL with query and fragment", "https://example.com/file.json?v=1#section", "file.json"},
		{"Complex URL", "https://api.example.com/v2/file.yaml?env=prod&v=2#database", "file.yaml"},
		{"GitHub raw URL", "https://raw.githubusercontent.com/org/repo/main/config.json?token=abc#L10", "config.json"},

		// Edge cases
		{"No extension", "README", "README"},
		{"Hidden file", ".env", ".env"},
		{"Hidden file with extension", ".hidden.json", ".hidden.json"},
		{"Multiple dots", "file.backup.2024.json", "file.backup.2024.json"},
		{"Question mark in filename", "is-this-json?.json", "is-this-json"},
		{"Hash in filename", "file#1.yaml", "file"},
		{"Mixed case", "CONFIG.JSON", "CONFIG.JSON"},
		{"Dots and special chars", "my.config.json.txt", "my.config.json.txt"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExtractFilenameFromPath(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestGetFileExtension tests the file extension extraction.
func TestGetFileExtension(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		// Standard extensions
		{"JSON file", "file.json", ".json"},
		{"YAML file", "file.yaml", ".yaml"},
		{"YML file", "file.yml", ".yml"},
		{"HCL file", "file.hcl", ".hcl"},
		{"TF file", "file.tf", ".tf"},
		{"TFVARS file", "file.tfvars", ".tfvars"},
		{"Text file", "file.txt", ".txt"},
		{"Markdown file", "README.md", ".md"},

		// Case sensitivity
		{"Uppercase JSON", "FILE.JSON", ".json"},
		{"Mixed case YAML", "File.YaMl", ".yaml"},
		{"Uppercase TXT", "README.TXT", ".txt"},

		// Multiple dots
		{"Multiple dots JSON", "file.backup.json", ".json"},
		{"Multiple dots TXT", "config.json.txt", ".txt"},
		{"Many dots", "file.v1.backup.2024.yaml", ".yaml"},

		// No extension
		{"No extension", "README", ""},
		{"No extension with path", "/path/to/README", ""},
		
		// Hidden files
		{"Hidden file no ext", ".env", ""},
		{"Hidden file with ext", ".hidden.json", ".json"},
		{"Hidden backup", ".backup.yaml", ".yaml"},

		// Edge cases
		{"Empty string", "", ""},
		{"Just dot", ".", ""},
		{"Just extension", ".json", ".json"},
		{"Trailing dot", "file.", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetFileExtension(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestParseByExtension tests the parsing logic based on extensions.
func TestParseByExtension(t *testing.T) {
	tests := []struct {
		name        string
		data        []byte
		ext         string
		filename    string
		expectType  string
		expectError bool
		validate    func(t *testing.T, result any)
	}{
		// JSON parsing
		{
			name:       "Valid JSON with .json extension",
			data:       []byte(`{"key": "value", "number": 42}`),
			ext:        ".json",
			filename:   "test.json",
			expectType: "map",
			validate: func(t *testing.T, result any) {
				m, ok := result.(map[string]any)
				require.True(t, ok)
				assert.Equal(t, "value", m["key"])
				assert.Equal(t, float64(42), m["number"])
			},
		},
		{
			name:        "Invalid JSON with .json extension",
			data:        []byte(`{invalid json}`),
			ext:         ".json",
			filename:    "test.json",
			expectError: true,
		},
		{
			name:       "JSON content with .txt extension returns raw",
			data:       []byte(`{"key": "value"}`),
			ext:        ".txt",
			filename:   "test.txt",
			expectType: "string",
			validate: func(t *testing.T, result any) {
				s, ok := result.(string)
				require.True(t, ok)
				assert.Equal(t, `{"key": "value"}`, s)
			},
		},

		// YAML parsing
		{
			name:       "Valid YAML with .yaml extension",
			data:       []byte("key: value\nnumber: 42"),
			ext:        ".yaml",
			filename:   "test.yaml",
			expectType: "map",
			validate: func(t *testing.T, result any) {
				m, ok := result.(map[string]any)
				require.True(t, ok)
				assert.Equal(t, "value", m["key"])
				assert.Equal(t, 42, m["number"])
			},
		},
		{
			name:       "Valid YAML with .yml extension",
			data:       []byte("enabled: true"),
			ext:        ".yml",
			filename:   "test.yml",
			expectType: "map",
			validate: func(t *testing.T, result any) {
				m, ok := result.(map[string]any)
				require.True(t, ok)
				assert.Equal(t, true, m["enabled"])
			},
		},
		{
			name:       "YAML content with .txt extension returns raw",
			data:       []byte("key: value"),
			ext:        ".txt",
			filename:   "test.txt",
			expectType: "string",
			validate: func(t *testing.T, result any) {
				s, ok := result.(string)
				require.True(t, ok)
				assert.Equal(t, "key: value", s)
			},
		},

		// HCL parsing
		{
			name:       "Valid HCL with .hcl extension",
			data:       []byte(`key = "value"`),
			ext:        ".hcl",
			filename:   "test.hcl",
			expectType: "map",
			validate: func(t *testing.T, result any) {
				m, ok := result.(map[string]any)
				require.True(t, ok)
				assert.Equal(t, "value", m["key"])
			},
		},
		{
			name:       "Valid HCL with .tf extension",
			data:       []byte(`variable = "test"`),
			ext:        ".tf",
			filename:   "test.tf",
			expectType: "map",
			validate: func(t *testing.T, result any) {
				m, ok := result.(map[string]any)
				require.True(t, ok)
				assert.Equal(t, "test", m["variable"])
			},
		},
		{
			name:       "Valid HCL with .tfvars extension",
			data:       []byte(`region = "us-east-1"`),
			ext:        ".tfvars",
			filename:   "terraform.tfvars",
			expectType: "map",
			validate: func(t *testing.T, result any) {
				m, ok := result.(map[string]any)
				require.True(t, ok)
				assert.Equal(t, "us-east-1", m["region"])
			},
		},

		// Raw string (default)
		{
			name:       "Unknown extension returns raw",
			data:       []byte("Some random content"),
			ext:        ".xyz",
			filename:   "test.xyz",
			expectType: "string",
			validate: func(t *testing.T, result any) {
				s, ok := result.(string)
				require.True(t, ok)
				assert.Equal(t, "Some random content", s)
			},
		},
		{
			name:       "No extension returns raw",
			data:       []byte("README content"),
			ext:        "",
			filename:   "README",
			expectType: "string",
			validate: func(t *testing.T, result any) {
				s, ok := result.(string)
				require.True(t, ok)
				assert.Equal(t, "README content", s)
			},
		},
		{
			name:       "Markdown extension returns raw",
			data:       []byte("# Heading\n\nContent"),
			ext:        ".md",
			filename:   "README.md",
			expectType: "string",
			validate: func(t *testing.T, result any) {
				s, ok := result.(string)
				require.True(t, ok)
				assert.Equal(t, "# Heading\n\nContent", s)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseByExtension(tt.data, tt.ext, tt.filename)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				if tt.validate != nil {
					tt.validate(t, result)
				}
			}
		})
	}
}

// TestParseFileByExtension tests the complete file parsing with extension detection.
func TestParseFileByExtension(t *testing.T) {
	tests := []struct {
		name        string
		filename    string
		fileContent string
		expectType  string
		expectError bool
		validate    func(t *testing.T, result any)
	}{
		// URLs with query strings
		{
			name:        "JSON URL with query string",
			filename:    "https://api.example.com/config.json?version=2&format=raw",
			fileContent: `{"api": "v2"}`,
			expectType:  "map",
			validate: func(t *testing.T, result any) {
				m, ok := result.(map[string]any)
				require.True(t, ok)
				assert.Equal(t, "v2", m["api"])
			},
		},
		{
			name:        "YAML URL with multiple query params",
			filename:    "https://example.com/settings.yaml?env=prod&region=us-east-1",
			fileContent: "environment: production",
			expectType:  "map",
			validate: func(t *testing.T, result any) {
				m, ok := result.(map[string]any)
				require.True(t, ok)
				assert.Equal(t, "production", m["environment"])
			},
		},
		{
			name:        "Text file URL with query string",
			filename:    "https://example.com/data.txt?download=true",
			fileContent: `{"this": "is not parsed"}`,
			expectType:  "string",
			validate: func(t *testing.T, result any) {
				s, ok := result.(string)
				require.True(t, ok)
				assert.Equal(t, `{"this": "is not parsed"}`, s)
			},
		},
		{
			name:        "JSON.txt with query string returns raw",
			filename:    "https://example.com/config.json.txt?version=1",
			fileContent: `{"json": "but as text"}`,
			expectType:  "string",
			validate: func(t *testing.T, result any) {
				s, ok := result.(string)
				require.True(t, ok)
				assert.Equal(t, `{"json": "but as text"}`, s)
			},
		},

		// S3 URLs
		{
			name:        "S3 URL with version ID",
			filename:    "s3://my-bucket/configs/app.json?versionId=abc123def456",
			fileContent: `{"bucket": "s3"}`,
			expectType:  "map",
			validate: func(t *testing.T, result any) {
				m, ok := result.(map[string]any)
				require.True(t, ok)
				assert.Equal(t, "s3", m["bucket"])
			},
		},

		// URLs with fragments
		{
			name:        "URL with fragment",
			filename:    "https://example.com/doc.yaml#section1",
			fileContent: "section: one",
			expectType:  "map",
		},
		{
			name:        "URL with query and fragment",
			filename:    "https://api.example.com/config.json?v=1&env=test#database",
			fileContent: `{"database": {"host": "localhost"}}`,
			expectType:  "map",
		},

		// Local files
		{
			name:        "Local JSON file",
			filename:    "/path/to/config.json",
			fileContent: `{"local": true}`,
			expectType:  "map",
		},
		{
			name:        "Local file with no extension",
			filename:    "/path/to/README",
			fileContent: "This is a readme",
			expectType:  "string",
			validate: func(t *testing.T, result any) {
				s, ok := result.(string)
				require.True(t, ok)
				assert.Equal(t, "This is a readme", s)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Mock file reader
			readFunc := func(path string) ([]byte, error) {
				// The function should receive the original filename
				assert.Equal(t, tt.filename, path)
				return []byte(tt.fileContent), nil
			}

			result, err := ParseFileByExtension(readFunc, tt.filename)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				if tt.validate != nil {
					tt.validate(t, result)
				}
			}
		})
	}
}

// TestParseFileRaw tests the raw file parsing function.
func TestParseFileRaw(t *testing.T) {
	tests := []struct {
		name        string
		filename    string
		fileContent string
		expectError bool
	}{
		{
			name:        "JSON file returned as raw",
			filename:    "config.json",
			fileContent: `{"key": "value"}`,
		},
		{
			name:        "YAML file returned as raw",
			filename:    "settings.yaml",
			fileContent: "key: value\nlist:\n  - item1",
		},
		{
			name:        "HCL file returned as raw",
			filename:    "terraform.tfvars",
			fileContent: `variable = "value"`,
		},
		{
			name:        "Text file returned as raw",
			filename:    "readme.txt",
			fileContent: "Plain text content",
		},
		{
			name:        "Binary-like content returned as raw",
			filename:    "data.bin",
			fileContent: string([]byte{0x01, 0x02, 0x03}),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			readFunc := func(path string) ([]byte, error) {
				if tt.expectError {
					return nil, errors.New("read error")
				}
				return []byte(tt.fileContent), nil
			}

			result, err := ParseFileRaw(readFunc, tt.filename)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				s, ok := result.(string)
				require.True(t, ok, "ParseFileRaw should always return a string")
				assert.Equal(t, tt.fileContent, s)
			}
		})
	}
}

// TestParseFileByExtensionReadError tests error handling when file reading fails.
func TestParseFileByExtensionReadError(t *testing.T) {
	readFunc := func(path string) ([]byte, error) {
		return nil, errors.New("file not found")
	}

	result, err := ParseFileByExtension(readFunc, "test.json")
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "file not found")
}

// TestParseFileRawReadError tests error handling for raw parsing when file reading fails.
func TestParseFileRawReadError(t *testing.T) {
	readFunc := func(path string) ([]byte, error) {
		return nil, errors.New("permission denied")
	}

	result, err := ParseFileRaw(readFunc, "test.txt")
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "permission denied")
}