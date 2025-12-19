package adapters

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/config"
)

// TestGoGetterAdapter_Schemes tests that GoGetterAdapter returns expected schemes.
func TestGoGetterAdapter_Schemes(t *testing.T) {
	adapter := &GoGetterAdapter{}
	schemes := adapter.Schemes()

	assert.Contains(t, schemes, "http://")
	assert.Contains(t, schemes, "https://")
	assert.Contains(t, schemes, "git::")
	assert.Contains(t, schemes, "s3::")
	assert.Contains(t, schemes, "oci://")
}

// TestGoGetterAdapter_DownloadError tests error handling in GoGetterAdapter.
func TestGoGetterAdapter_DownloadError(t *testing.T) {
	tempDir := t.TempDir()

	adapter := &GoGetterAdapter{}
	ctx := context.Background()

	_, err := adapter.Resolve(ctx, "https://nonexistent.invalid/config.yaml", tempDir, tempDir, 1, 10)
	assert.Error(t, err) // Download should fail
}

// TestGoGetterAdapter_ReadConfigError tests viper read error handling.
func TestGoGetterAdapter_ReadConfigError(t *testing.T) {
	tempDir := t.TempDir()

	// Create a local file with invalid YAML to simulate download success but read failure.
	invalidYAML := `invalid: [yaml: content`
	invalidPath := filepath.Join(tempDir, "invalid.yaml")
	err := os.WriteFile(invalidPath, []byte(invalidYAML), 0o644)
	assert.NoError(t, err)

	// Simulate what the adapter does with a malformed file.
	v := viper.New()
	v.SetConfigFile(invalidPath)
	err = v.ReadInConfig()
	assert.Error(t, err) // Should fail to read invalid YAML
}

// TestLocalAdapter_Schemes tests that LocalAdapter returns nil (default fallback).
func TestLocalAdapter_Schemes(t *testing.T) {
	adapter := &LocalAdapter{}
	schemes := adapter.Schemes()

	assert.Nil(t, schemes) // LocalAdapter is the default fallback.
}

// TestLocalAdapter_EmptyImportPath tests error path for empty import path.
func TestLocalAdapter_EmptyImportPath(t *testing.T) {
	tempDir := t.TempDir()

	adapter := &LocalAdapter{}
	ctx := context.Background()

	_, err := adapter.Resolve(ctx, "", tempDir, tempDir, 1, 10)
	assert.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrImportPathRequired)
}

// TestLocalAdapter_SearchConfigError tests error path for non-existent path.
func TestLocalAdapter_SearchConfigError(t *testing.T) {
	tempDir := t.TempDir()

	nonExistentPath := filepath.Join(tempDir, "nonexistent", "config.yaml")

	adapter := &LocalAdapter{}
	ctx := context.Background()

	_, err := adapter.Resolve(ctx, nonExistentPath, tempDir, tempDir, 1, 10)
	assert.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrResolveLocal)
}

// TestLocalAdapter_ReadConfigError tests error path for invalid YAML.
func TestLocalAdapter_ReadConfigError(t *testing.T) {
	tempDir := t.TempDir()

	// Create invalid YAML file.
	invalidContent := `invalid: [yaml: content`
	invalidPath := filepath.Join(tempDir, "invalid.yaml")
	err := os.WriteFile(invalidPath, []byte(invalidContent), 0o644)
	assert.NoError(t, err)

	adapter := &LocalAdapter{}
	ctx := context.Background()

	paths, err := adapter.Resolve(ctx, "invalid.yaml", tempDir, tempDir, 1, 10)
	// Should not error but should skip files that fail to load.
	assert.NoError(t, err)
	_ = paths
}

// TestLocalAdapter_ValidFile tests successful resolution of a valid file.
func TestLocalAdapter_ValidFile(t *testing.T) {
	tempDir := t.TempDir()

	// Create a valid YAML file.
	validContent := `settings:
  key: value
`
	validPath := filepath.Join(tempDir, "valid.yaml")
	err := os.WriteFile(validPath, []byte(validContent), 0o644)
	assert.NoError(t, err)

	adapter := &LocalAdapter{}
	ctx := context.Background()

	paths, err := adapter.Resolve(ctx, "valid.yaml", tempDir, tempDir, 1, 10)
	assert.NoError(t, err)
	assert.NotEmpty(t, paths)
	assert.Equal(t, validPath, paths[0].FilePath)
	assert.Equal(t, config.LOCAL, paths[0].ImportType)
}

// TestMockAdapter_Schemes tests that MockAdapter returns the mock:// scheme.
func TestMockAdapter_Schemes(t *testing.T) {
	adapter := &MockAdapter{}
	schemes := adapter.Schemes()

	assert.Equal(t, []string{"mock://"}, schemes)
}

// TestMockAdapter_Empty tests mock://empty path.
func TestMockAdapter_Empty(t *testing.T) {
	tempDir := t.TempDir()
	adapter := &MockAdapter{}
	ctx := context.Background()

	paths, err := adapter.Resolve(ctx, "mock://empty", tempDir, tempDir, 1, 10)
	assert.NoError(t, err)
	assert.Len(t, paths, 1)
	assert.Equal(t, config.ADAPTER, paths[0].ImportType)
}

// TestMockAdapter_Error tests mock://error path.
func TestMockAdapter_Error(t *testing.T) {
	tempDir := t.TempDir()
	adapter := &MockAdapter{}
	ctx := context.Background()

	_, err := adapter.Resolve(ctx, "mock://error", tempDir, tempDir, 1, 10)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "mock error")
}

// TestMockAdapter_CustomData tests custom mock data injection.
func TestMockAdapter_CustomData(t *testing.T) {
	tempDir := t.TempDir()
	adapter := &MockAdapter{
		MockData: map[string]string{
			"custom/path": "custom_key: custom_value\n",
		},
	}
	ctx := context.Background()

	paths, err := adapter.Resolve(ctx, "mock://custom/path", tempDir, tempDir, 1, 10)
	assert.NoError(t, err)
	assert.Len(t, paths, 1)

	// Verify the file contains the custom content.
	content, err := os.ReadFile(paths[0].FilePath)
	assert.NoError(t, err)
	assert.Equal(t, "custom_key: custom_value\n", string(content))
}

// TestMockAdapter_DefaultPath tests default mock path handling.
func TestMockAdapter_DefaultPath(t *testing.T) {
	tempDir := t.TempDir()
	adapter := &MockAdapter{}
	ctx := context.Background()

	paths, err := adapter.Resolve(ctx, "mock://some/path", tempDir, tempDir, 1, 10)
	assert.NoError(t, err)
	assert.Len(t, paths, 1)

	// Verify the file contains default mock config.
	content, err := os.ReadFile(paths[0].FilePath)
	assert.NoError(t, err)
	assert.Contains(t, string(content), "mock_path: \"some/path\"")
}

// TestFindImportAdapter tests adapter registry routing.
func TestFindImportAdapter(t *testing.T) {
	tests := []struct {
		name            string
		path            string
		isLocalFallback bool
	}{
		{"http URL", "http://example.com/config.yaml", false},
		{"https URL", "https://example.com/config.yaml", false},
		{"git URL", "git::https://github.com/user/repo.git", false},
		{"s3 URL", "s3://bucket/path/config.yaml", false},
		{"mock URL", "mock://test", false},
		{"local path", "local/path/config.yaml", true},
		{"absolute path", "/absolute/path/config.yaml", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adapter := config.FindImportAdapter(tt.path)
			assert.NotNil(t, adapter)
			if tt.isLocalFallback {
				// Local paths should get LocalAdapter (default fallback).
				schemes := adapter.Schemes()
				assert.Nil(t, schemes) // LocalAdapter returns nil for schemes.
			}
		})
	}
}

// TestGlobalMockAdapter tests the global mock adapter singleton.
func TestGlobalMockAdapter(t *testing.T) {
	// Clear any existing mock data.
	ClearMockData()

	// Set new mock data.
	SetMockData(map[string]string{
		"test/path": "test: data\n",
	})

	adapter := GetGlobalMockAdapter()
	assert.NotNil(t, adapter)
	assert.Equal(t, "test: data\n", adapter.MockData["test/path"])

	// Clear and verify.
	ClearMockData()
	assert.Empty(t, adapter.MockData)
}
