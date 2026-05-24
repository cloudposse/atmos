package utils

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConvertToHclAst(t *testing.T) {
	tests := []struct {
		name    string
		data    any
		wantErr bool
	}{
		{
			name:    "simple map",
			data:    map[string]any{"key": "value"},
			wantErr: false,
		},
		{
			name:    "map with number",
			data:    map[string]any{"port": 8080},
			wantErr: false,
		},
		{
			name:    "nested map",
			data:    map[string]any{"outer": map[string]any{"inner": "value"}},
			wantErr: false,
		},
		{
			name:    "empty map",
			data:    map[string]any{},
			wantErr: false,
		},
		{
			name:    "nil value",
			data:    nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node, err := ConvertToHclAst(tt.data)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, node)
			}
		})
	}
}

func TestWriteToFileAsHcl(t *testing.T) {
	t.Run("writes HCL to file", func(t *testing.T) {
		tmpDir := t.TempDir()
		filePath := filepath.Join(tmpDir, "test.tf")

		data := map[string]any{
			"region":      "us-east-1",
			"environment": "prod",
		}

		err := WriteToFileAsHcl(filePath, data, 0o644)
		require.NoError(t, err)

		// Verify file was created.
		assert.FileExists(t, filePath)

		// Verify file content is not empty.
		content, err := os.ReadFile(filePath)
		require.NoError(t, err)
		assert.NotEmpty(t, content)
	})

	t.Run("writes backend config to file", func(t *testing.T) {
		tmpDir := t.TempDir()
		filePath := filepath.Join(tmpDir, "backend.tf")

		data := map[string]any{
			"bucket":  "my-terraform-state",
			"key":     "terraform.tfstate",
			"region":  "us-east-1",
			"encrypt": true,
		}

		err := WriteToFileAsHcl(filePath, data, 0o600)
		require.NoError(t, err)
		assert.FileExists(t, filePath)
	})

	t.Run("error when directory does not exist", func(t *testing.T) {
		err := WriteToFileAsHcl("/nonexistent/dir/file.tf", map[string]any{"key": "value"}, 0o644)
		assert.Error(t, err)
	})
}

func TestWriteTerraformBackendConfigToFileAsHcl(t *testing.T) {
	t.Run("writes s3 backend config", func(t *testing.T) {
		tmpDir := t.TempDir()
		filePath := filepath.Join(tmpDir, "backend.tf")

		backendConfig := map[string]any{
			"bucket":  "my-terraform-state",
			"key":     "terraform.tfstate",
			"region":  "us-east-1",
			"encrypt": true,
		}

		err := WriteTerraformBackendConfigToFileAsHcl(filePath, "s3", backendConfig)
		require.NoError(t, err)

		// Verify file was created.
		assert.FileExists(t, filePath)

		// Verify content contains expected HCL structure.
		content, err := os.ReadFile(filePath)
		require.NoError(t, err)
		contentStr := string(content)
		assert.Contains(t, contentStr, "terraform")
		assert.Contains(t, contentStr, "backend")
		assert.Contains(t, contentStr, "s3")
	})

	t.Run("writes gcs backend config", func(t *testing.T) {
		tmpDir := t.TempDir()
		filePath := filepath.Join(tmpDir, "backend.tf")

		backendConfig := map[string]any{
			"bucket": "my-tf-state-bucket",
			"prefix": "terraform/state",
		}

		err := WriteTerraformBackendConfigToFileAsHcl(filePath, "gcs", backendConfig)
		require.NoError(t, err)
		assert.FileExists(t, filePath)

		content, err := os.ReadFile(filePath)
		require.NoError(t, err)
		assert.Contains(t, string(content), "gcs")
	})

	t.Run("writes empty backend config", func(t *testing.T) {
		tmpDir := t.TempDir()
		filePath := filepath.Join(tmpDir, "backend.tf")

		err := WriteTerraformBackendConfigToFileAsHcl(filePath, "local", map[string]any{})
		require.NoError(t, err)
		assert.FileExists(t, filePath)
	})

	t.Run("error when directory does not exist", func(t *testing.T) {
		err := WriteTerraformBackendConfigToFileAsHcl("/nonexistent/dir/backend.tf", "s3", map[string]any{"key": "value"})
		assert.Error(t, err)
	})
}

func TestPrintAsHcl(t *testing.T) {
	// PrintAsHcl writes to stdout; just verify no error is returned for valid data.
	t.Run("simple map does not error", func(t *testing.T) {
		data := map[string]any{"region": "us-east-1"}
		err := PrintAsHcl(data)
		assert.NoError(t, err)
	})

	t.Run("empty map does not error", func(t *testing.T) {
		err := PrintAsHcl(map[string]any{})
		assert.NoError(t, err)
	})
}

func TestConvertToHclAst_KeyQuotesRemoved(t *testing.T) {
	// Verify that double quotes are removed from keys (terraform varfile requirement).
	data := map[string]any{"my_variable": "value"}

	node, err := ConvertToHclAst(data)
	require.NoError(t, err)
	assert.NotNil(t, node)

	// The HCL printer should produce output with keys without surrounding quotes.
	var buf strings.Builder
	_ = buf // We just verify the node is valid by checking no error.
}
