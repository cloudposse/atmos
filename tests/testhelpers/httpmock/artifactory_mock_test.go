package httpmock

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestArtifactoryMockServer_Upload(t *testing.T) {
	mock := NewArtifactoryMockServer(t)

	// Upload a file.
	content := []byte(`{"key": "value"}`)
	req, err := http.NewRequest(http.MethodPut, mock.URL()+"/test-repo/path/to/file.json", bytes.NewReader(content))
	require.NoError(t, err)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusCreated, resp.StatusCode)

	// Verify file was stored.
	stored, exists := mock.GetFile("test-repo/path/to/file.json")
	assert.True(t, exists)
	assert.Equal(t, content, stored)
}

func TestArtifactoryMockServer_Download(t *testing.T) {
	mock := NewArtifactoryMockServer(t)

	// Pre-populate a file.
	content := []byte(`{"test": "data"}`)
	mock.SetFile("test-repo/my/file.json", content)

	// Download it.
	resp, err := http.Get(mock.URL() + "/test-repo/my/file.json")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, content, body)
}

func TestArtifactoryMockServer_DownloadNotFound(t *testing.T) {
	mock := NewArtifactoryMockServer(t)

	resp, err := http.Get(mock.URL() + "/nonexistent/file.json")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestArtifactoryMockServer_AQLSearch(t *testing.T) {
	mock := NewArtifactoryMockServer(t)

	// Pre-populate files.
	mock.SetFile("test-repo/prefix/dev/vpc/vpc_id", []byte(`"vpc-123"`))
	mock.SetFile("test-repo/prefix/dev/vpc/subnet_ids", []byte(`["subnet-1", "subnet-2"]`))
	mock.SetFile("test-repo/prefix/prod/vpc/vpc_id", []byte(`"vpc-456"`))

	// Perform AQL search.
	query := `items.find({"repo":"test-repo","path":"prefix/dev/vpc","name":"vpc_id"})`
	req, err := http.NewRequest(http.MethodPost, mock.URL()+"/api/search/aql", bytes.NewReader([]byte(query)))
	require.NoError(t, err)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var result map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&result)
	require.NoError(t, err)

	results, ok := result["results"].([]interface{})
	require.True(t, ok)
	assert.NotEmpty(t, results, "AQL search should return at least one result")
}

func TestArtifactoryMockServer_Clear(t *testing.T) {
	mock := NewArtifactoryMockServer(t)

	// Add files.
	mock.SetFile("repo/file1.json", []byte("data1"))
	mock.SetFile("repo/file2.json", []byte("data2"))

	// Clear.
	mock.Clear()

	// Verify empty.
	files := mock.ListFiles()
	assert.Empty(t, files)
}

func TestArtifactoryMockServer_ListFiles(t *testing.T) {
	mock := NewArtifactoryMockServer(t)

	mock.SetFile("repo/a.json", []byte("a"))
	mock.SetFile("repo/b.json", []byte("b"))

	files := mock.ListFiles()
	assert.Len(t, files, 2)
	assert.Contains(t, files, "repo/a.json")
	assert.Contains(t, files, "repo/b.json")
}
