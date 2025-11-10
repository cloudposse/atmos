package provisioning

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestWriter_Write(t *testing.T) {
	// Create temp directory for test.
	tempDir := t.TempDir()

	// Create writer with custom cache dir.
	writer := &Writer{
		CacheDir: tempDir,
	}

	// Create test result.
	result := &Result{
		Provider:      "test-provider",
		ProvisionedAt: time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC),
		Identities: map[string]*schema.Identity{
			"test-account/TestRole": {
				Provider: "test-provider",
				Principal: map[string]interface{}{
					"name": "TestRole",
					"account": map[string]interface{}{
						"name": "test-account",
						"id":   "123456789012",
					},
				},
			},
		},
		Metadata: Metadata{
			Source: "aws-sso",
			Counts: &Counts{
				Accounts:   1,
				Roles:      1,
				Identities: 1,
			},
			Extra: map[string]interface{}{
				"start_url": "https://test.awsapps.com/start",
				"region":    "us-east-1",
			},
		},
	}

	// Write result.
	filePath, err := writer.Write(result)
	require.NoError(t, err)

	// Verify file was created.
	assert.FileExists(t, filePath)

	// Verify file path structure.
	expectedPath := filepath.Join(tempDir, "test-provider", ProvisionedFileName)
	assert.Equal(t, expectedPath, filePath)

	// Read and verify content.
	content, err := os.ReadFile(filePath)
	require.NoError(t, err)
	assert.Contains(t, string(content), "test-account/TestRole")
	assert.Contains(t, string(content), "test-provider")
	assert.Contains(t, string(content), "aws-sso")
}

func TestWriter_Remove(t *testing.T) {
	// Create temp directory for test.
	tempDir := t.TempDir()

	// Create writer with custom cache dir.
	writer := &Writer{
		CacheDir: tempDir,
	}

	// Create test result and write.
	result := &Result{
		Provider:      "test-provider",
		ProvisionedAt: time.Now(),
		Identities: map[string]*schema.Identity{
			"test/role": {
				Provider: "test-provider",
			},
		},
		Metadata: Metadata{
			Source: "test",
			Counts: &Counts{
				Accounts:   1,
				Roles:      1,
				Identities: 1,
			},
		},
	}

	filePath, err := writer.Write(result)
	require.NoError(t, err)
	assert.FileExists(t, filePath)

	// Remove the file.
	err = writer.Remove("test-provider")
	require.NoError(t, err)

	// Verify file was removed.
	_, err = os.Stat(filePath)
	assert.True(t, os.IsNotExist(err))

	// Removing again should not error.
	err = writer.Remove("test-provider")
	require.NoError(t, err)
}

func TestWriter_GetProvisionedIdentitiesPath(t *testing.T) {
	tempDir := t.TempDir()

	writer := &Writer{
		CacheDir: tempDir,
	}

	path := writer.GetProvisionedIdentitiesPath("test-provider")
	expected := filepath.Join(tempDir, "test-provider", ProvisionedFileName)
	assert.Equal(t, expected, path)
}

func TestNewWriter(t *testing.T) {
	writer, err := NewWriter()
	require.NoError(t, err)
	assert.NotNil(t, writer)
	assert.NotEmpty(t, writer.CacheDir)
	assert.Contains(t, writer.CacheDir, filepath.Join("atmos", "auth"))
}

func TestWriter_Write_InvalidInput(t *testing.T) {
	tempDir := t.TempDir()

	writer := &Writer{
		CacheDir: tempDir,
	}

	// Test nil result.
	_, err := writer.Write(nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "result cannot be nil")

	// Test empty provider name.
	result := &Result{
		Provider: "",
	}
	_, err = writer.Write(result)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "provider name cannot be empty")
}

func TestWriter_Write_WithoutCounts(t *testing.T) {
	// Test writing provisioned identities without counts metadata.
	tempDir := t.TempDir()

	writer := &Writer{
		CacheDir: tempDir,
	}

	// Create test result without counts.
	result := &Result{
		Provider:      "test-provider",
		ProvisionedAt: time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC),
		Identities: map[string]*schema.Identity{
			"test-account/TestRole": {
				Provider: "test-provider",
			},
		},
		Metadata: Metadata{
			Source: "aws-sso",
			// Counts is nil.
		},
	}

	// Write result.
	filePath, err := writer.Write(result)
	require.NoError(t, err)
	assert.FileExists(t, filePath)

	// Read and verify content doesn't have counts.
	content, err := os.ReadFile(filePath)
	require.NoError(t, err)
	assert.Contains(t, string(content), "test-account/TestRole")
	assert.Contains(t, string(content), "aws-sso")
	// Should not contain counts section.
	assert.NotContains(t, string(content), "accounts:")
}

func TestWriter_Write_WithoutExtra(t *testing.T) {
	// Test writing provisioned identities without extra metadata.
	tempDir := t.TempDir()

	writer := &Writer{
		CacheDir: tempDir,
	}

	// Create test result without extra metadata.
	result := &Result{
		Provider:      "test-provider",
		ProvisionedAt: time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC),
		Identities: map[string]*schema.Identity{
			"test-account/TestRole": {
				Provider: "test-provider",
			},
		},
		Metadata: Metadata{
			Source: "aws-sso",
			Counts: &Counts{
				Accounts:   1,
				Roles:      1,
				Identities: 1,
			},
			// Extra is empty/nil.
		},
	}

	// Write result.
	filePath, err := writer.Write(result)
	require.NoError(t, err)
	assert.FileExists(t, filePath)

	// Read and verify content.
	content, err := os.ReadFile(filePath)
	require.NoError(t, err)
	assert.Contains(t, string(content), "test-account/TestRole")
	assert.Contains(t, string(content), "accounts: 1")
	// Should not contain extra section.
	assert.NotContains(t, string(content), "extra:")
}

func TestWriter_Write_WithAllMetadata(t *testing.T) {
	// Test writing provisioned identities with all metadata fields.
	tempDir := t.TempDir()

	writer := &Writer{
		CacheDir: tempDir,
	}

	// Create test result with all metadata.
	result := &Result{
		Provider:      "test-provider",
		ProvisionedAt: time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC),
		Identities: map[string]*schema.Identity{
			"test-account/TestRole": {
				Provider: "test-provider",
			},
		},
		Metadata: Metadata{
			Source: "aws-sso",
			Counts: &Counts{
				Accounts:   3,
				Roles:      10,
				Identities: 30,
			},
			Extra: map[string]interface{}{
				"start_url": "https://test.awsapps.com/start",
				"region":    "us-east-1",
				"custom":    "value",
			},
		},
	}

	// Write result.
	filePath, err := writer.Write(result)
	require.NoError(t, err)
	assert.FileExists(t, filePath)

	// Read and verify content has all fields.
	content, err := os.ReadFile(filePath)
	require.NoError(t, err)
	assert.Contains(t, string(content), "test-account/TestRole")
	assert.Contains(t, string(content), "aws-sso")
	assert.Contains(t, string(content), "accounts: 3")
	assert.Contains(t, string(content), "roles: 10")
	assert.Contains(t, string(content), "identities: 30")
	assert.Contains(t, string(content), "start_url")
	assert.Contains(t, string(content), "region")
	assert.Contains(t, string(content), "custom")
}

func TestWriter_Remove_NonExistentFile(t *testing.T) {
	// Test removing non-existent file (should succeed).
	tempDir := t.TempDir()

	writer := &Writer{
		CacheDir: tempDir,
	}

	// Remove file that doesn't exist - should not error.
	err := writer.Remove("non-existent-provider")
	require.NoError(t, err)
}

func TestWriter_GetProvisionedIdentitiesPath_Structure(t *testing.T) {
	// Test the file path structure.
	tempDir := t.TempDir()

	writer := &Writer{
		CacheDir: tempDir,
	}

	// Verify path structure.
	path := writer.GetProvisionedIdentitiesPath("my-provider")
	assert.Contains(t, path, "my-provider")
	assert.Contains(t, path, ProvisionedFileName)
	assert.Equal(t, filepath.Join(tempDir, "my-provider", ProvisionedFileName), path)
}

func TestBuildConfig_Structure(t *testing.T) {
	// Test the buildConfig function creates correct structure.
	result := &Result{
		Provider:      "test-provider",
		ProvisionedAt: time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC),
		Identities: map[string]*schema.Identity{
			"test-identity": {
				Provider: "test-provider",
			},
		},
		Metadata: Metadata{
			Source: "aws-sso",
			Counts: &Counts{
				Accounts:   1,
				Roles:      2,
				Identities: 3,
			},
			Extra: map[string]interface{}{
				"key": "value",
			},
		},
	}

	config := buildConfig(result)

	// Verify top-level structure.
	assert.Contains(t, config, "auth")
	authSection, ok := config["auth"].(map[string]interface{})
	require.True(t, ok)

	// Verify identities.
	assert.Contains(t, authSection, "identities")

	// Verify metadata.
	assert.Contains(t, authSection, "_metadata")
	metadata, ok := authSection["_metadata"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "2025-01-01T12:00:00Z", metadata["provisioned_at"])
	assert.Equal(t, "aws-sso", metadata["source"])
	assert.Equal(t, "test-provider", metadata["provider"])

	// Verify counts.
	assert.Contains(t, metadata, "counts")
	counts, ok := metadata["counts"].(map[string]int)
	require.True(t, ok)
	assert.Equal(t, 1, counts["accounts"])
	assert.Equal(t, 2, counts["roles"])
	assert.Equal(t, 3, counts["identities"])

	// Verify extra.
	assert.Contains(t, metadata, "extra")
	extra, ok := metadata["extra"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "value", extra["key"])
}

func TestGetDefaultCacheDir_XDG(t *testing.T) {
	// Test that XDG_CACHE_HOME is respected.
	tempDir := t.TempDir()

	// Set XDG_CACHE_HOME.
	originalXDG := os.Getenv("XDG_CACHE_HOME")
	defer func() {
		if originalXDG != "" {
			os.Setenv("XDG_CACHE_HOME", originalXDG)
		} else {
			os.Unsetenv("XDG_CACHE_HOME")
		}
	}()
	os.Setenv("XDG_CACHE_HOME", tempDir)

	// Get cache dir.
	cacheDir, err := getDefaultCacheDir()
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(tempDir, DefaultCacheDir), cacheDir)
}

func TestGetDefaultCacheDir_Fallback(t *testing.T) {
	// Test fallback to ~/.cache when XDG_CACHE_HOME not set.
	// Clear XDG_CACHE_HOME.
	originalXDG := os.Getenv("XDG_CACHE_HOME")
	defer func() {
		if originalXDG != "" {
			os.Setenv("XDG_CACHE_HOME", originalXDG)
		}
	}()
	os.Unsetenv("XDG_CACHE_HOME")

	// Get cache dir.
	cacheDir, err := getDefaultCacheDir()
	require.NoError(t, err)

	// Should contain .cache/atmos/auth.
	assert.Contains(t, cacheDir, ".cache")
	assert.Contains(t, cacheDir, "atmos")
	assert.Contains(t, cacheDir, "auth")
}
