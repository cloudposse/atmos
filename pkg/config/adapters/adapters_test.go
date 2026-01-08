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

// TestLocalAdapter_AbsolutePath tests resolution of absolute paths.
func TestLocalAdapter_AbsolutePath(t *testing.T) {
	tempDir := t.TempDir()

	// Create a valid YAML file.
	validContent := `settings:
  key: value
`
	validPath := filepath.Join(tempDir, "absolute.yaml")
	err := os.WriteFile(validPath, []byte(validContent), 0o644)
	assert.NoError(t, err)

	adapter := &LocalAdapter{}
	ctx := context.Background()

	// Use the absolute path directly.
	paths, err := adapter.Resolve(ctx, validPath, tempDir, tempDir, 1, 10)
	assert.NoError(t, err)
	assert.NotEmpty(t, paths)
	assert.Equal(t, validPath, paths[0].FilePath)
}

// TestLocalAdapter_OutsideBasePath tests paths outside the base directory.
func TestLocalAdapter_OutsideBasePath(t *testing.T) {
	parentDir := t.TempDir()
	subDir := filepath.Join(parentDir, "subdir")
	err := os.Mkdir(subDir, 0o755)
	assert.NoError(t, err)

	// Create a config in parent directory.
	parentConfig := filepath.Join(parentDir, "parent.yaml")
	err = os.WriteFile(parentConfig, []byte("key: value\n"), 0o644)
	assert.NoError(t, err)

	adapter := &LocalAdapter{}
	ctx := context.Background()

	// Import from subdir pointing to parent.
	paths, err := adapter.Resolve(ctx, "../parent.yaml", subDir, parentDir, 1, 10)
	assert.NoError(t, err)
	assert.NotEmpty(t, paths)
	assert.Equal(t, parentConfig, paths[0].FilePath)
}

// TestLocalAdapter_NestedImports tests processing of nested imports.
func TestLocalAdapter_NestedImports(t *testing.T) {
	tempDir := t.TempDir()

	// Create nested config file.
	nestedContent := `settings:
  nested: true
`
	nestedPath := filepath.Join(tempDir, "nested.yaml")
	err := os.WriteFile(nestedPath, []byte(nestedContent), 0o644)
	assert.NoError(t, err)

	// Create main config with import.
	mainContent := `import:
  - nested.yaml
settings:
  main: true
`
	mainPath := filepath.Join(tempDir, "main.yaml")
	err = os.WriteFile(mainPath, []byte(mainContent), 0o644)
	assert.NoError(t, err)

	// Setup test adapters to avoid circular import.
	config.ResetImportAdapterRegistry()
	config.SetDefaultAdapter(&LocalAdapter{})

	adapter := &LocalAdapter{}
	ctx := context.Background()

	paths, err := adapter.Resolve(ctx, "main.yaml", tempDir, tempDir, 1, 10)
	assert.NoError(t, err)
	assert.GreaterOrEqual(t, len(paths), 1) // At least the main file.
}

// TestLocalAdapter_NestedImportsWithCustomBasePath tests nested imports with custom base_path.
func TestLocalAdapter_NestedImportsWithCustomBasePath(t *testing.T) {
	tempDir := t.TempDir()
	subDir := filepath.Join(tempDir, "configs")
	err := os.Mkdir(subDir, 0o755)
	assert.NoError(t, err)

	// Create nested config in subdirectory.
	nestedContent := `settings:
  nested: true
`
	nestedPath := filepath.Join(subDir, "nested.yaml")
	err = os.WriteFile(nestedPath, []byte(nestedContent), 0o644)
	assert.NoError(t, err)

	// Create main config with import and custom base_path.
	mainContent := `base_path: ./configs
import:
  - nested.yaml
settings:
  main: true
`
	mainPath := filepath.Join(tempDir, "main.yaml")
	err = os.WriteFile(mainPath, []byte(mainContent), 0o644)
	assert.NoError(t, err)

	// Setup test adapters.
	config.ResetImportAdapterRegistry()
	config.SetDefaultAdapter(&LocalAdapter{})

	adapter := &LocalAdapter{}
	ctx := context.Background()

	paths, err := adapter.Resolve(ctx, "main.yaml", tempDir, tempDir, 1, 10)
	assert.NoError(t, err)
	assert.NotEmpty(t, paths)
}

// TestLocalAdapter_NoNestedImports tests file without nested imports.
func TestLocalAdapter_NoNestedImports(t *testing.T) {
	tempDir := t.TempDir()

	// Create config without imports.
	content := `settings:
  key: value
`
	configPath := filepath.Join(tempDir, "config.yaml")
	err := os.WriteFile(configPath, []byte(content), 0o644)
	assert.NoError(t, err)

	config.ResetImportAdapterRegistry()
	config.SetDefaultAdapter(&LocalAdapter{})

	adapter := &LocalAdapter{}
	ctx := context.Background()

	paths, err := adapter.Resolve(ctx, "config.yaml", tempDir, tempDir, 1, 10)
	assert.NoError(t, err)
	assert.Len(t, paths, 1)
}

// TestGoGetterAdapter_AllSchemes tests that all expected schemes are present.
func TestGoGetterAdapter_AllSchemes(t *testing.T) {
	adapter := &GoGetterAdapter{}
	schemes := adapter.Schemes()

	expectedSchemes := []string{
		"http://", "https://",
		"git::", "git@",
		"s3::", "s3://",
		"gcs::", "gcs://",
		"oci://",
		"file://",
		"hg::",
		"scp://", "sftp://",
		"github.com/", "bitbucket.org/",
	}

	for _, expected := range expectedSchemes {
		assert.Contains(t, schemes, expected, "Should contain scheme: %s", expected)
	}
}

// TestDownloadRemoteConfig_NilContext tests downloadRemoteConfig with nil context.
func TestDownloadRemoteConfig_NilContext(t *testing.T) {
	tempDir := t.TempDir()

	// Test with TODO context - should work with default timeout.
	// Use an invalid URL to trigger error (we're testing the context handling).
	_, err := downloadRemoteConfig(context.TODO(), "https://nonexistent.invalid/config.yaml", tempDir)
	assert.Error(t, err) // Should fail but not panic.
}

// TestDownloadRemoteConfig_ValidContext tests downloadRemoteConfig with valid context.
func TestDownloadRemoteConfig_ValidContext(t *testing.T) {
	tempDir := t.TempDir()
	ctx := context.Background()

	// Use an invalid URL to trigger error.
	_, err := downloadRemoteConfig(ctx, "https://nonexistent.invalid/config.yaml", tempDir)
	assert.Error(t, err)
}

// TestMockAdapter_NestedImports tests mock adapter with nested imports.
func TestMockAdapter_NestedImports(t *testing.T) {
	tempDir := t.TempDir()

	// Setup adapters.
	config.ResetImportAdapterRegistry()
	mockAdapter := &MockAdapter{
		MockData: map[string]string{
			"parent": `import:
  - mock://child
settings:
  parent: true
`,
			"child": `settings:
  child: true
`,
		},
	}
	config.RegisterImportAdapter(mockAdapter)
	config.SetDefaultAdapter(&LocalAdapter{})

	ctx := context.Background()

	paths, err := mockAdapter.Resolve(ctx, "mock://parent", tempDir, tempDir, 1, 10)
	assert.NoError(t, err)
	assert.NotEmpty(t, paths)
}

// TestMockAdapter_WriteError tests mock adapter when temp directory doesn't exist.
func TestMockAdapter_WriteError(t *testing.T) {
	adapter := &MockAdapter{}
	ctx := context.Background()

	// Use non-existent temp directory.
	_, err := adapter.Resolve(ctx, "mock://test", "/nonexistent/temp/dir", "/base", 1, 10)
	assert.Error(t, err)
}

// TestGoGetterAdapter_ResolveWithLocalFile tests Resolve with a valid local file.
func TestGoGetterAdapter_ResolveWithLocalFile(t *testing.T) {
	tempDir := t.TempDir()

	// Create a valid local YAML config file.
	validContent := `settings:
  key: value
`
	validPath := filepath.Join(tempDir, "config.yaml")
	err := os.WriteFile(validPath, []byte(validContent), 0o644)
	assert.NoError(t, err)

	// Setup adapters.
	config.ResetImportAdapterRegistry()
	config.SetDefaultAdapter(&LocalAdapter{})

	adapter := &GoGetterAdapter{}
	ctx := context.Background()

	// Use file:// URL to download local file.
	fileURL := "file://" + validPath
	paths, err := adapter.Resolve(ctx, fileURL, tempDir, tempDir, 1, 10)
	assert.NoError(t, err)
	assert.NotEmpty(t, paths)
	assert.Equal(t, config.REMOTE, paths[0].ImportType)
}

// TestGoGetterAdapter_ResolveWithInvalidYAML tests Resolve with invalid YAML content.
func TestGoGetterAdapter_ResolveWithInvalidYAML(t *testing.T) {
	tempDir := t.TempDir()

	// Create an invalid YAML config file.
	invalidContent := `invalid: [yaml: content`
	invalidPath := filepath.Join(tempDir, "invalid.yaml")
	err := os.WriteFile(invalidPath, []byte(invalidContent), 0o644)
	assert.NoError(t, err)

	adapter := &GoGetterAdapter{}
	ctx := context.Background()

	// Use file:// URL to download the invalid file.
	fileURL := "file://" + invalidPath
	_, err = adapter.Resolve(ctx, fileURL, tempDir, tempDir, 1, 10)
	assert.Error(t, err) // Should fail when reading invalid YAML.
}

// TestGoGetterAdapter_ResolveWithNestedImports tests Resolve with nested imports.
func TestGoGetterAdapter_ResolveWithNestedImports(t *testing.T) {
	tempDir := t.TempDir()

	// Create nested config file.
	nestedContent := `settings:
  nested: true
`
	nestedPath := filepath.Join(tempDir, "nested.yaml")
	err := os.WriteFile(nestedPath, []byte(nestedContent), 0o644)
	assert.NoError(t, err)

	// Create parent config with import.
	parentContent := `import:
  - nested.yaml
settings:
  parent: true
`
	parentPath := filepath.Join(tempDir, "parent.yaml")
	err = os.WriteFile(parentPath, []byte(parentContent), 0o644)
	assert.NoError(t, err)

	// Setup adapters.
	config.ResetImportAdapterRegistry()
	config.SetDefaultAdapter(&LocalAdapter{})

	adapter := &GoGetterAdapter{}
	ctx := context.Background()

	// Use file:// URL to download parent file.
	fileURL := "file://" + parentPath
	paths, err := adapter.Resolve(ctx, fileURL, tempDir, tempDir, 1, 10)
	assert.NoError(t, err)
	assert.GreaterOrEqual(t, len(paths), 1) // At least the parent file.
}

// TestGoGetterAdapter_ResolveWithCustomBasePath tests Resolve with custom base_path.
func TestGoGetterAdapter_ResolveWithCustomBasePath(t *testing.T) {
	tempDir := t.TempDir()
	subDir := filepath.Join(tempDir, "subdir")
	err := os.Mkdir(subDir, 0o755)
	assert.NoError(t, err)

	// Create nested config in subdirectory.
	nestedContent := `settings:
  nested: true
`
	nestedPath := filepath.Join(subDir, "nested.yaml")
	err = os.WriteFile(nestedPath, []byte(nestedContent), 0o644)
	assert.NoError(t, err)

	// Create parent config with custom base_path.
	parentContent := `base_path: ` + subDir + `
import:
  - nested.yaml
settings:
  parent: true
`
	parentPath := filepath.Join(tempDir, "parent.yaml")
	err = os.WriteFile(parentPath, []byte(parentContent), 0o644)
	assert.NoError(t, err)

	// Setup adapters.
	config.ResetImportAdapterRegistry()
	config.SetDefaultAdapter(&LocalAdapter{})

	adapter := &GoGetterAdapter{}
	ctx := context.Background()

	// Use file:// URL to download parent file.
	fileURL := "file://" + parentPath
	paths, err := adapter.Resolve(ctx, fileURL, tempDir, tempDir, 1, 10)
	assert.NoError(t, err)
	assert.NotEmpty(t, paths)
}

// TestGoGetterAdapter_ResolveNoImports tests Resolve with config that has no imports.
func TestGoGetterAdapter_ResolveNoImports(t *testing.T) {
	tempDir := t.TempDir()

	// Create a config without imports.
	content := `settings:
  key: value
components:
  terraform:
    vpc: {}
`
	configPath := filepath.Join(tempDir, "no-imports.yaml")
	err := os.WriteFile(configPath, []byte(content), 0o644)
	assert.NoError(t, err)

	// Setup adapters.
	config.ResetImportAdapterRegistry()
	config.SetDefaultAdapter(&LocalAdapter{})

	adapter := &GoGetterAdapter{}
	ctx := context.Background()

	fileURL := "file://" + configPath
	paths, err := adapter.Resolve(ctx, fileURL, tempDir, tempDir, 1, 10)
	assert.NoError(t, err)
	assert.Len(t, paths, 1) // Only the downloaded file.
	assert.Equal(t, config.REMOTE, paths[0].ImportType)
}

// TestDownloadRemoteConfig_LocalFile tests downloading a local file via file:// URL.
func TestDownloadRemoteConfig_LocalFile(t *testing.T) {
	tempDir := t.TempDir()
	destDir := t.TempDir()

	// Create a source file.
	content := `key: value`
	sourcePath := filepath.Join(tempDir, "source.yaml")
	err := os.WriteFile(sourcePath, []byte(content), 0o644)
	assert.NoError(t, err)

	ctx := context.Background()
	fileURL := "file://" + sourcePath

	// Download the file.
	resultPath, err := downloadRemoteConfig(ctx, fileURL, destDir)
	assert.NoError(t, err)
	assert.NotEmpty(t, resultPath)

	// Verify content was downloaded.
	downloadedContent, err := os.ReadFile(resultPath)
	assert.NoError(t, err)
	assert.Equal(t, content, string(downloadedContent))
}
