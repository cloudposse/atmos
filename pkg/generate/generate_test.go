package generate

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateFiles_StringContent(t *testing.T) {
	tempDir := t.TempDir()

	generateSection := map[string]any{
		"readme.txt": "Hello {{ .name }}!",
	}

	templateContext := map[string]any{
		"name": "World",
	}

	config := GenerateConfig{
		DryRun: false,
		Clean:  false,
	}

	results, err := GenerateFiles(generateSection, tempDir, templateContext, config)
	require.NoError(t, err)
	require.Len(t, results, 1)

	assert.Equal(t, "readme.txt", results[0].Filename)
	assert.True(t, results[0].Created)
	assert.NoError(t, results[0].Error)

	// Verify file content.
	content, err := os.ReadFile(filepath.Join(tempDir, "readme.txt"))
	require.NoError(t, err)
	assert.Equal(t, "Hello World!", string(content))
}

func TestGenerateFiles_JSONContent(t *testing.T) {
	tempDir := t.TempDir()

	generateSection := map[string]any{
		"config.json": map[string]any{
			"name":    "test",
			"version": "1.0.0",
		},
	}

	templateContext := map[string]any{}

	config := GenerateConfig{
		DryRun: false,
		Clean:  false,
	}

	results, err := GenerateFiles(generateSection, tempDir, templateContext, config)
	require.NoError(t, err)
	require.Len(t, results, 1)

	assert.Equal(t, "config.json", results[0].Filename)
	assert.True(t, results[0].Created)

	// Verify file exists and contains JSON.
	content, err := os.ReadFile(filepath.Join(tempDir, "config.json"))
	require.NoError(t, err)
	assert.Contains(t, string(content), `"name": "test"`)
	assert.Contains(t, string(content), `"version": "1.0.0"`)
}

func TestGenerateFiles_YAMLContent(t *testing.T) {
	tempDir := t.TempDir()

	generateSection := map[string]any{
		"config.yaml": map[string]any{
			"name":    "test",
			"enabled": true,
		},
	}

	templateContext := map[string]any{}

	config := GenerateConfig{
		DryRun: false,
		Clean:  false,
	}

	results, err := GenerateFiles(generateSection, tempDir, templateContext, config)
	require.NoError(t, err)
	require.Len(t, results, 1)

	assert.Equal(t, "config.yaml", results[0].Filename)
	assert.True(t, results[0].Created)

	// Verify file exists and contains YAML.
	content, err := os.ReadFile(filepath.Join(tempDir, "config.yaml"))
	require.NoError(t, err)
	assert.Contains(t, string(content), "name: test")
	assert.Contains(t, string(content), "enabled: true")
}

func TestGenerateFiles_DryRun(t *testing.T) {
	tempDir := t.TempDir()

	generateSection := map[string]any{
		"test.txt": "content",
	}

	config := GenerateConfig{
		DryRun: true,
		Clean:  false,
	}

	results, err := GenerateFiles(generateSection, tempDir, nil, config)
	require.NoError(t, err)
	require.Len(t, results, 1)

	assert.True(t, results[0].Skipped)

	// Verify file was NOT created.
	_, err = os.Stat(filepath.Join(tempDir, "test.txt"))
	assert.True(t, os.IsNotExist(err))
}

func TestGenerateFiles_Clean(t *testing.T) {
	tempDir := t.TempDir()

	// Create a file first.
	testFile := filepath.Join(tempDir, "test.txt")
	err := os.WriteFile(testFile, []byte("content"), 0o644)
	require.NoError(t, err)

	generateSection := map[string]any{
		"test.txt": "content",
	}

	config := GenerateConfig{
		DryRun: false,
		Clean:  true,
	}

	results, err := GenerateFiles(generateSection, tempDir, nil, config)
	require.NoError(t, err)
	require.Len(t, results, 1)

	assert.True(t, results[0].Deleted)

	// Verify file was deleted.
	_, err = os.Stat(testFile)
	assert.True(t, os.IsNotExist(err))
}

func TestGenerateFiles_TemplateInMapValue(t *testing.T) {
	tempDir := t.TempDir()

	generateSection := map[string]any{
		"vars.json": map[string]any{
			"environment": "{{ .env }}",
			"region":      "{{ .region }}",
		},
	}

	templateContext := map[string]any{
		"env":    "prod",
		"region": "us-west-2",
	}

	config := GenerateConfig{
		DryRun: false,
		Clean:  false,
	}

	results, err := GenerateFiles(generateSection, tempDir, templateContext, config)
	require.NoError(t, err)
	require.Len(t, results, 1)

	content, err := os.ReadFile(filepath.Join(tempDir, "vars.json"))
	require.NoError(t, err)
	assert.Contains(t, string(content), `"environment": "prod"`)
	assert.Contains(t, string(content), `"region": "us-west-2"`)
}

func TestGenerateFiles_NilSection(t *testing.T) {
	tempDir := t.TempDir()

	results, err := GenerateFiles(nil, tempDir, nil, GenerateConfig{})
	require.NoError(t, err)
	assert.Nil(t, results)
}

func TestGenerateFiles_UpdateExistingFile(t *testing.T) {
	tempDir := t.TempDir()

	// Create file first.
	testFile := filepath.Join(tempDir, "test.txt")
	err := os.WriteFile(testFile, []byte("old content"), 0o644)
	require.NoError(t, err)

	generateSection := map[string]any{
		"test.txt": "new content",
	}

	config := GenerateConfig{
		DryRun: false,
		Clean:  false,
	}

	results, err := GenerateFiles(generateSection, tempDir, nil, config)
	require.NoError(t, err)
	require.Len(t, results, 1)

	// Should be an update, not a create.
	assert.False(t, results[0].Created)

	// Verify new content.
	content, err := os.ReadFile(testFile)
	require.NoError(t, err)
	assert.Equal(t, "new content", string(content))
}

func TestGetGenerateFilenames(t *testing.T) {
	generateSection := map[string]any{
		"file1.json": map[string]any{},
		"file2.yaml": "content",
		"file3.tf":   map[string]any{},
	}

	filenames := GetGenerateFilenames(generateSection)
	assert.Len(t, filenames, 3)
	assert.Contains(t, filenames, "file1.json")
	assert.Contains(t, filenames, "file2.yaml")
	assert.Contains(t, filenames, "file3.tf")
}

func TestGetGenerateFilenames_NilSection(t *testing.T) {
	filenames := GetGenerateFilenames(nil)
	assert.Nil(t, filenames)
}

func TestGenerateFiles_HCLContent(t *testing.T) {
	tempDir := t.TempDir()

	generateSection := map[string]any{
		"locals.tf": map[string]any{
			"locals": map[string]any{
				"environment": "prod",
				"enabled":     true,
			},
		},
	}

	templateContext := map[string]any{}

	config := GenerateConfig{
		DryRun: false,
		Clean:  false,
	}

	results, err := GenerateFiles(generateSection, tempDir, templateContext, config)
	require.NoError(t, err)
	require.Len(t, results, 1)

	content, err := os.ReadFile(filepath.Join(tempDir, "locals.tf"))
	require.NoError(t, err)
	// HCL output should have the block structure.
	contentStr := string(content)
	assert.Contains(t, contentStr, "locals")
	assert.Contains(t, contentStr, "environment")
	assert.Contains(t, contentStr, "prod")
}

func TestGenerateFiles_YAMLPrettyPrinted(t *testing.T) {
	tempDir := t.TempDir()

	// Create nested content to verify proper indentation.
	generateSection := map[string]any{
		"config.yaml": map[string]any{
			"database": map[string]any{
				"host": "localhost",
				"port": 5432,
				"credentials": map[string]any{
					"username": "admin",
					"password": "secret",
				},
			},
			"features": []any{"auth", "logging", "metrics"},
		},
	}

	templateContext := map[string]any{}

	config := GenerateConfig{
		DryRun: false,
		Clean:  false,
	}

	results, err := GenerateFiles(generateSection, tempDir, templateContext, config)
	require.NoError(t, err)
	require.Len(t, results, 1)

	content, err := os.ReadFile(filepath.Join(tempDir, "config.yaml"))
	require.NoError(t, err)
	contentStr := string(content)

	// Verify YAML is properly indented (2 spaces per level).
	assert.Contains(t, contentStr, "database:")
	assert.Contains(t, contentStr, "  host: localhost")
	assert.Contains(t, contentStr, "  credentials:")
	assert.Contains(t, contentStr, "    username: admin")
}

func TestGenerateFiles_JSONPrettyPrinted(t *testing.T) {
	tempDir := t.TempDir()

	// Create nested content to verify proper indentation.
	generateSection := map[string]any{
		"config.json": map[string]any{
			"server": map[string]any{
				"host": "0.0.0.0",
				"port": 8080,
			},
		},
	}

	templateContext := map[string]any{}

	config := GenerateConfig{
		DryRun: false,
		Clean:  false,
	}

	results, err := GenerateFiles(generateSection, tempDir, templateContext, config)
	require.NoError(t, err)
	require.Len(t, results, 1)

	content, err := os.ReadFile(filepath.Join(tempDir, "config.json"))
	require.NoError(t, err)
	contentStr := string(content)

	// Verify JSON is properly indented (2 spaces per level).
	assert.Contains(t, contentStr, "\"server\": {")
	assert.Contains(t, contentStr, "  \"host\": \"0.0.0.0\"")
}
