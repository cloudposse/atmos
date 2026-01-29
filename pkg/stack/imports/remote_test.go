package imports

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestRemoteImporter_Download_HTTP(t *testing.T) {
	// Create a mock HTTP server.
	content := `
components:
  terraform:
    vpc:
      vars:
        name: "test-vpc"
`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/yaml")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(content))
	}))
	defer server.Close()

	// Create temp cache directory.
	cacheDir := t.TempDir()

	// Create RemoteImporter with custom cache dir.
	atmosConfig := &schema.AtmosConfiguration{}
	importer, err := NewRemoteImporter(atmosConfig, WithCacheDir(cacheDir))
	require.NoError(t, err)

	// Download the file.
	localPath, err := importer.Download(server.URL + "/config.yaml")
	require.NoError(t, err)
	assert.NotEmpty(t, localPath)

	// Verify the file exists and has correct content.
	data, err := os.ReadFile(localPath)
	require.NoError(t, err)
	assert.Equal(t, content, string(data))

	// Download again - should hit cache.
	localPath2, err := importer.Download(server.URL + "/config.yaml")
	require.NoError(t, err)
	assert.Equal(t, localPath, localPath2, "should return cached path")
}

func TestRemoteImporter_Download_NotFound(t *testing.T) {
	// Create a mock HTTP server that returns 404.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("Not Found"))
	}))
	defer server.Close()

	// Create temp cache directory.
	cacheDir := t.TempDir()

	// Create RemoteImporter.
	atmosConfig := &schema.AtmosConfiguration{}
	importer, err := NewRemoteImporter(atmosConfig, WithCacheDir(cacheDir))
	require.NoError(t, err)

	// Download should fail.
	_, err = importer.Download(server.URL + "/nonexistent.yaml")
	assert.Error(t, err)
}

func TestRemoteImporter_Download_LocalPath_Error(t *testing.T) {
	// Create temp cache directory.
	cacheDir := t.TempDir()

	// Create RemoteImporter.
	atmosConfig := &schema.AtmosConfiguration{}
	importer, err := NewRemoteImporter(atmosConfig, WithCacheDir(cacheDir))
	require.NoError(t, err)

	// Trying to download a local path should fail.
	_, err = importer.Download("catalog/vpc.yaml")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid remote import")
}

func TestRemoteImporter_ClearCache(t *testing.T) {
	// Create a mock HTTP server.
	content := "test content"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(content))
	}))
	defer server.Close()

	// Create temp cache directory.
	cacheDir := t.TempDir()

	// Create RemoteImporter.
	atmosConfig := &schema.AtmosConfiguration{}
	importer, err := NewRemoteImporter(atmosConfig, WithCacheDir(cacheDir))
	require.NoError(t, err)

	// Download a file.
	localPath, err := importer.Download(server.URL + "/config.yaml")
	require.NoError(t, err)

	// Verify the file exists.
	_, err = os.Stat(localPath)
	require.NoError(t, err)

	// Clear the cache.
	err = importer.ClearCache()
	require.NoError(t, err)

	// Verify the file no longer exists.
	_, err = os.Stat(localPath)
	assert.True(t, os.IsNotExist(err))
}

func TestProcessImportPath_Local(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	basePath := "/stacks"

	tests := []struct {
		name       string
		importPath string
		expected   string
	}{
		{"catalog path", "catalog/vpc", filepath.Join("/stacks", "catalog/vpc")},
		{"mixins path", "mixins/region", filepath.Join("/stacks", "mixins/region")},
		{"relative dot", "./local", filepath.Join("/stacks", "./local")},
		{"relative parent", "../shared", filepath.Join("/stacks", "../shared")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ProcessImportPath(atmosConfig, basePath, tt.importPath)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestProcessImportPath_Remote(t *testing.T) {
	// Create a mock HTTP server.
	content := "remote: content"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(content))
	}))
	defer server.Close()

	// Reset the global importer for this test.
	globalImporterOnce = sync.Once{}
	globalImporter = nil
	globalImporterErr = nil

	atmosConfig := &schema.AtmosConfiguration{}
	basePath := "/stacks"

	// Process a remote import path.
	result, err := ProcessImportPath(atmosConfig, basePath, server.URL+"/config.yaml")
	require.NoError(t, err)
	assert.NotEmpty(t, result)
	assert.NotEqual(t, filepath.Join(basePath, server.URL+"/config.yaml"), result, "should not join remote URL with basePath")

	// Verify the downloaded file has correct content.
	data, err := os.ReadFile(result)
	require.NoError(t, err)
	assert.Equal(t, content, string(data))
}
