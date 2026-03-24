package artifact

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateTarArchive_RoundTrip(t *testing.T) {
	files := []FileEntry{
		{Name: "plan.tfplan", Data: strings.NewReader("plan data"), Size: -1},
		{Name: ".terraform.lock.hcl", Data: strings.NewReader("lock data"), Size: -1},
	}

	tarData, err := CreateTarArchive(files)
	require.NoError(t, err)
	require.NotEmpty(t, tarData)

	results, err := ExtractTarArchive(bytes.NewReader(tarData))
	require.NoError(t, err)
	require.Len(t, results, 2)

	// Verify first file.
	assert.Equal(t, "plan.tfplan", results[0].Name)
	content0, _ := io.ReadAll(results[0].Data)
	assert.Equal(t, "plan data", string(content0))

	// Verify second file.
	assert.Equal(t, ".terraform.lock.hcl", results[1].Name)
	content1, _ := io.ReadAll(results[1].Data)
	assert.Equal(t, "lock data", string(content1))
}

func TestCreateTarArchive_SingleFile(t *testing.T) {
	files := []FileEntry{
		{Name: "single.bin", Data: strings.NewReader("single file content"), Size: -1},
	}

	tarData, err := CreateTarArchive(files)
	require.NoError(t, err)

	results, err := ExtractTarArchive(bytes.NewReader(tarData))
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "single.bin", results[0].Name)

	content, _ := io.ReadAll(results[0].Data)
	assert.Equal(t, "single file content", string(content))
}

func TestCreateTarArchive_Empty(t *testing.T) {
	tarData, err := CreateTarArchive([]FileEntry{})
	require.NoError(t, err)

	results, err := ExtractTarArchive(bytes.NewReader(tarData))
	require.NoError(t, err)
	assert.Empty(t, results)
}

func TestExtractTarArchive_MultiFile(t *testing.T) {
	files := []FileEntry{
		{Name: "a.txt", Data: strings.NewReader("aaa"), Size: -1},
		{Name: "b.txt", Data: strings.NewReader("bbb"), Size: -1},
		{Name: "c.txt", Data: strings.NewReader("ccc"), Size: -1},
	}

	tarData, err := CreateTarArchive(files)
	require.NoError(t, err)

	results, err := ExtractTarArchive(bytes.NewReader(tarData))
	require.NoError(t, err)
	require.Len(t, results, 3)

	for i, r := range results {
		content, _ := io.ReadAll(r.Data)
		assert.Equal(t, files[i].Name, r.Name)
		expected, _ := io.ReadAll(strings.NewReader(string([]byte{byte('a' + i), byte('a' + i), byte('a' + i)})))
		assert.Equal(t, expected, content)
	}
}

func TestExtractTarArchive_ClosableResults(t *testing.T) {
	files := []FileEntry{
		{Name: "test.txt", Data: strings.NewReader("test"), Size: -1},
	}

	tarData, err := CreateTarArchive(files)
	require.NoError(t, err)

	results, err := ExtractTarArchive(bytes.NewReader(tarData))
	require.NoError(t, err)

	// Verify that Data implements io.ReadCloser and can be closed.
	for _, r := range results {
		assert.NoError(t, r.Data.Close())
	}
}
