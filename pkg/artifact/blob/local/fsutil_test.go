package local

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpenFile_ReturnsExistingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "object")
	require.NoError(t, os.WriteFile(path, []byte("payload"), 0o600))

	file, err := openFile(path)
	require.NoError(t, err)
	defer file.Close()
	data, err := io.ReadAll(file)
	require.NoError(t, err)
	assert.Equal(t, "payload", string(data))
}

func TestOpenFile_MissingFileReportsNotExistWithoutRetrying(t *testing.T) {
	file, err := openFile(filepath.Join(t.TempDir(), "absent"))
	require.ErrorIs(t, err, os.ErrNotExist)
	assert.Nil(t, file)
}

func TestReadFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "attrs")
	require.NoError(t, os.WriteFile(path, []byte(`{"a":1}`), 0o600))

	data, err := readFile(path)
	require.NoError(t, err)
	assert.JSONEq(t, `{"a":1}`, string(data))

	_, err = readFile(filepath.Join(t.TempDir(), "absent"))
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestOpenFileRetriesTransientFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "object")
	require.NoError(t, os.WriteFile(path, []byte("payload"), 0o600))
	attempts := 0
	file, err := openFileWith(path, 2, func(path string) (*os.File, error) {
		attempts++
		if attempts == 1 {
			return nil, os.ErrPermission
		}
		return os.Open(path)
	})
	require.NoError(t, err)
	defer file.Close()
	data, err := io.ReadAll(file)
	require.NoError(t, err)
	assert.Equal(t, "payload", string(data))
	assert.Equal(t, 2, attempts)
}
