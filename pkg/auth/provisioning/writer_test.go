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
	assert.Contains(t, writer.CacheDir, "atmos/aws")
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
