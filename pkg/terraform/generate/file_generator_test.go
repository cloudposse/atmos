package generate

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGenerateFiles_Extensions tests file generation for different file extensions.
func TestGenerateFiles_Extensions(t *testing.T) {
	tests := []struct {
		name         string
		filename     string
		content      map[string]any
		wantContains []string
	}{
		{
			name:         "JSON extension",
			filename:     "config.json",
			content:      map[string]any{"name": "test", "version": "1.0.0"},
			wantContains: []string{`"name": "test"`, `"version": "1.0.0"`},
		},
		{
			name:         "YAML extension",
			filename:     "config.yaml",
			content:      map[string]any{"name": "test", "enabled": true},
			wantContains: []string{"name: test", "enabled: true"},
		},
		{
			name:         "YML extension",
			filename:     "config.yml",
			content:      map[string]any{"name": "test", "enabled": true},
			wantContains: []string{"name: test"},
		},
		{
			name:     "TF extension (HCL)",
			filename: "locals.tf",
			content: map[string]any{
				"locals": map[string]any{"environment": "prod", "enabled": true},
			},
			wantContains: []string{"locals", "environment", "prod"},
		},
		{
			name:     "HCL extension",
			filename: "backend.hcl",
			content: map[string]any{
				"backend": map[string]any{"type": "s3"},
			},
			wantContains: []string{"backend"},
		},
		{
			name:         "Unknown extension defaults to JSON",
			filename:     "config.xyz",
			content:      map[string]any{"key": "value"},
			wantContains: []string{`"key": "value"`},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir := t.TempDir()
			generateSection := map[string]any{tt.filename: tt.content}
			config := GenerateConfig{DryRun: false, Clean: false}

			results, err := GenerateFiles(generateSection, tempDir, nil, config)
			require.NoError(t, err)
			require.Len(t, results, 1)
			assert.True(t, results[0].Created)

			content, err := os.ReadFile(filepath.Join(tempDir, tt.filename))
			require.NoError(t, err)
			for _, want := range tt.wantContains {
				assert.Contains(t, string(content), want)
			}
		})
	}
}

// TestGenerateFiles_Modes tests different generation modes (normal, dry-run, clean).
func TestGenerateFiles_Modes(t *testing.T) {
	tests := []struct {
		name           string
		dryRun         bool
		clean          bool
		createFirst    bool
		wantSkipped    bool
		wantDeleted    bool
		wantFileExists bool
	}{
		{
			name:           "Normal mode creates file",
			dryRun:         false,
			clean:          false,
			createFirst:    false,
			wantSkipped:    false,
			wantDeleted:    false,
			wantFileExists: true,
		},
		{
			name:           "Dry-run mode skips creation",
			dryRun:         true,
			clean:          false,
			createFirst:    false,
			wantSkipped:    true,
			wantDeleted:    false,
			wantFileExists: false,
		},
		{
			name:           "Clean mode deletes existing file",
			dryRun:         false,
			clean:          true,
			createFirst:    true,
			wantSkipped:    false,
			wantDeleted:    true,
			wantFileExists: false,
		},
		{
			name:           "Clean dry-run mode skips deletion",
			dryRun:         true,
			clean:          true,
			createFirst:    true,
			wantSkipped:    true,
			wantDeleted:    false,
			wantFileExists: true,
		},
		{
			name:           "Clean mode with non-existent file skips",
			dryRun:         false,
			clean:          true,
			createFirst:    false,
			wantSkipped:    true,
			wantDeleted:    false,
			wantFileExists: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir := t.TempDir()
			testFile := filepath.Join(tempDir, "test.txt")

			if tt.createFirst {
				require.NoError(t, os.WriteFile(testFile, []byte("content"), 0o644))
			}

			generateSection := map[string]any{"test.txt": "content"}
			config := GenerateConfig{DryRun: tt.dryRun, Clean: tt.clean}

			results, err := GenerateFiles(generateSection, tempDir, nil, config)
			require.NoError(t, err)
			require.Len(t, results, 1)

			assert.Equal(t, tt.wantSkipped, results[0].Skipped)
			assert.Equal(t, tt.wantDeleted, results[0].Deleted)

			_, statErr := os.Stat(testFile)
			if tt.wantFileExists {
				assert.NoError(t, statErr)
			} else {
				assert.True(t, os.IsNotExist(statErr))
			}
		})
	}
}

// TestGenerateFiles_ContentTypes tests different content types (string, map, unsupported).
func TestGenerateFiles_ContentTypes(t *testing.T) {
	tests := []struct {
		name         string
		filename     string
		content      any
		context      map[string]any
		wantError    bool
		wantContains string
		wantExact    string
	}{
		{
			name:      "String content with template",
			filename:  "readme.txt",
			content:   "Hello {{ .name }}!",
			context:   map[string]any{"name": "World"},
			wantExact: "Hello World!",
		},
		{
			name:         "Map content as JSON",
			filename:     "config.json",
			content:      map[string]any{"key": "value"},
			wantContains: `"key": "value"`,
		},
		{
			name:      "Unsupported content type",
			filename:  "test.txt",
			content:   12345,
			wantError: true,
		},
		{
			name:      "Empty map",
			filename:  "empty.json",
			content:   map[string]any{},
			wantExact: "{}",
		},
		{
			name:         "Nil value in map",
			filename:     "null.json",
			content:      map[string]any{"value": nil},
			wantContains: "null",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir := t.TempDir()
			generateSection := map[string]any{tt.filename: tt.content}
			config := GenerateConfig{DryRun: false, Clean: false}

			results, err := GenerateFiles(generateSection, tempDir, tt.context, config)
			require.NoError(t, err)
			require.Len(t, results, 1)

			if tt.wantError {
				assert.Error(t, results[0].Error)
				assert.Contains(t, results[0].Error.Error(), "unsupported content type")
				return
			}

			content, err := os.ReadFile(filepath.Join(tempDir, tt.filename))
			require.NoError(t, err)

			if tt.wantExact != "" {
				assert.Equal(t, tt.wantExact, string(content))
			}
			if tt.wantContains != "" {
				assert.Contains(t, string(content), tt.wantContains)
			}
		})
	}
}

// TestGenerateFiles_Templates tests template rendering in various scenarios.
func TestGenerateFiles_Templates(t *testing.T) {
	tests := []struct {
		name         string
		filename     string
		content      any
		context      map[string]any
		wantError    bool
		wantContains []string
	}{
		{
			name:     "Template in map values",
			filename: "vars.json",
			content: map[string]any{
				"environment": "{{ .env }}",
				"region":      "{{ .region }}",
			},
			context:      map[string]any{"env": "prod", "region": "us-west-2"},
			wantContains: []string{`"environment": "prod"`, `"region": "us-west-2"`},
		},
		{
			name:     "Nested array templates",
			filename: "config.json",
			content: map[string]any{
				"items": []any{
					map[string]any{"name": "{{ .item1 }}"},
					map[string]any{"name": "{{ .item2 }}"},
				},
			},
			context:      map[string]any{"item1": "first", "item2": "second"},
			wantContains: []string{"first", "second"},
		},
		{
			name:      "Invalid template syntax",
			filename:  "invalid.txt",
			content:   "{{ .invalid | nonexistent }}",
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir := t.TempDir()
			generateSection := map[string]any{tt.filename: tt.content}
			config := GenerateConfig{DryRun: false, Clean: false}

			results, err := GenerateFiles(generateSection, tempDir, tt.context, config)
			require.NoError(t, err)
			require.Len(t, results, 1)

			if tt.wantError {
				assert.Error(t, results[0].Error)
				assert.Contains(t, results[0].Error.Error(), "template")
				return
			}

			assert.NoError(t, results[0].Error)
			content, err := os.ReadFile(filepath.Join(tempDir, tt.filename))
			require.NoError(t, err)
			for _, want := range tt.wantContains {
				assert.Contains(t, string(content), want)
			}
		})
	}
}

// TestGenerateFiles_HCLSpecialCases tests HCL-specific edge cases.
func TestGenerateFiles_HCLSpecialCases(t *testing.T) {
	tests := []struct {
		name         string
		filename     string
		content      map[string]any
		wantContains []string
	}{
		{
			name:     "Mixed types in HCL",
			filename: "mixed.tf",
			content: map[string]any{
				"locals": map[string]any{
					"mixed_list": []any{1, "two", true, 4.5},
					"mixed_map": map[string]any{
						"string_val": "hello",
						"int_val":    42,
						"bool_val":   false,
					},
				},
			},
			wantContains: []string{"locals", "mixed_list", "mixed_map"},
		},
		{
			name:     "Nested arrays in HCL",
			filename: "arrays.tf",
			content: map[string]any{
				"locals": map[string]any{
					"nested": []any{[]any{"a", "b"}, []any{"c", "d"}},
				},
			},
			wantContains: []string{"locals", "nested"},
		},
		{
			name:     "Empty slice in HCL",
			filename: "empty_array.tf",
			content: map[string]any{
				"locals": map[string]any{"empty_list": []any{}},
			},
			wantContains: []string{"locals", "empty_list"},
		},
		{
			name:     "Int64 value in HCL",
			filename: "numbers.tf",
			content: map[string]any{
				"locals": map[string]any{"big_number": int64(9223372036854775807)},
			},
			wantContains: []string{"locals", "big_number"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir := t.TempDir()
			generateSection := map[string]any{tt.filename: tt.content}
			config := GenerateConfig{DryRun: false, Clean: false}

			results, err := GenerateFiles(generateSection, tempDir, nil, config)
			require.NoError(t, err)
			require.Len(t, results, 1)
			assert.NoError(t, results[0].Error)

			content, err := os.ReadFile(filepath.Join(tempDir, tt.filename))
			require.NoError(t, err)
			for _, want := range tt.wantContains {
				assert.Contains(t, string(content), want)
			}
		})
	}
}

// TestGenerateFiles_PrettyPrinting tests proper indentation in output files.
func TestGenerateFiles_PrettyPrinting(t *testing.T) {
	t.Run("YAML pretty printed", func(t *testing.T) {
		tempDir := t.TempDir()
		generateSection := map[string]any{
			"config.yaml": map[string]any{
				"database": map[string]any{
					"host": "localhost",
					"credentials": map[string]any{
						"username": "admin",
					},
				},
			},
		}
		config := GenerateConfig{DryRun: false, Clean: false}

		results, err := GenerateFiles(generateSection, tempDir, nil, config)
		require.NoError(t, err)
		require.Len(t, results, 1)

		content, err := os.ReadFile(filepath.Join(tempDir, "config.yaml"))
		require.NoError(t, err)
		contentStr := string(content)
		assert.Contains(t, contentStr, "database:")
		assert.Contains(t, contentStr, "  host: localhost")
		assert.Contains(t, contentStr, "  credentials:")
		assert.Contains(t, contentStr, "    username: admin")
	})

	t.Run("JSON pretty printed", func(t *testing.T) {
		tempDir := t.TempDir()
		generateSection := map[string]any{
			"config.json": map[string]any{
				"server": map[string]any{"host": "0.0.0.0", "port": 8080},
			},
		}
		config := GenerateConfig{DryRun: false, Clean: false}

		results, err := GenerateFiles(generateSection, tempDir, nil, config)
		require.NoError(t, err)
		require.Len(t, results, 1)

		content, err := os.ReadFile(filepath.Join(tempDir, "config.json"))
		require.NoError(t, err)
		contentStr := string(content)
		assert.Contains(t, contentStr, "\"server\": {")
		assert.Contains(t, contentStr, "  \"host\": \"0.0.0.0\"")
	})
}

// TestGenerateFiles_EdgeCases tests edge cases and special scenarios.
func TestGenerateFiles_EdgeCases(t *testing.T) {
	t.Run("Nil generate section", func(t *testing.T) {
		tempDir := t.TempDir()
		results, err := GenerateFiles(nil, tempDir, nil, GenerateConfig{})
		require.NoError(t, err)
		assert.Nil(t, results)
	})

	t.Run("Update existing file", func(t *testing.T) {
		tempDir := t.TempDir()
		testFile := filepath.Join(tempDir, "test.txt")
		require.NoError(t, os.WriteFile(testFile, []byte("old content"), 0o644))

		generateSection := map[string]any{"test.txt": "new content"}
		config := GenerateConfig{DryRun: false, Clean: false}

		results, err := GenerateFiles(generateSection, tempDir, nil, config)
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.False(t, results[0].Created) // Update, not create.

		content, err := os.ReadFile(testFile)
		require.NoError(t, err)
		assert.Equal(t, "new content", string(content))
	})

	t.Run("Multiple files", func(t *testing.T) {
		tempDir := t.TempDir()
		generateSection := map[string]any{
			"file1.json": map[string]any{"key1": "value1"},
			"file2.yaml": map[string]any{"key2": "value2"},
			"file3.txt":  "plain text",
		}
		config := GenerateConfig{DryRun: false, Clean: false}

		results, err := GenerateFiles(generateSection, tempDir, nil, config)
		require.NoError(t, err)
		require.Len(t, results, 3)

		for _, result := range results {
			assert.True(t, result.Created)
			assert.NoError(t, result.Error)
		}
	})
}

// TestGetGenerateFilenames tests the GetGenerateFilenames function.
func TestGetGenerateFilenames(t *testing.T) {
	tests := []struct {
		name            string
		generateSection map[string]any
		wantNil         bool
		wantLen         int
		wantContains    []string
	}{
		{
			name:            "Nil section",
			generateSection: nil,
			wantNil:         true,
		},
		{
			name: "Multiple files",
			generateSection: map[string]any{
				"file1.json": map[string]any{},
				"file2.yaml": "content",
				"file3.tf":   map[string]any{},
			},
			wantLen:      3,
			wantContains: []string{"file1.json", "file2.yaml", "file3.tf"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filenames := GetGenerateFilenames(tt.generateSection)
			if tt.wantNil {
				assert.Nil(t, filenames)
				return
			}
			assert.Len(t, filenames, tt.wantLen)
			for _, want := range tt.wantContains {
				assert.Contains(t, filenames, want)
			}
		})
	}
}
