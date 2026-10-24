package filesystem

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestWriteFileRefusesExistingAndCancelledWrites(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "data")
	require.NoError(t, WriteFile(t.Context(), path, []byte("original"), WriteOptions{Mode: 0o600}))
	err := WriteFile(t.Context(), path, []byte("replacement"), WriteOptions{Mode: 0o600})
	require.ErrorIs(t, err, os.ErrExist)
	require.ErrorIs(t, err, errUtils.ErrWriteFile)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	err = WriteFile(ctx, path, []byte("replacement"), WriteOptions{Mode: 0o600, Overwrite: true})
	require.ErrorIs(t, err, context.Canceled)
	require.ErrorIs(t, err, errUtils.ErrWriteFile)
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "original", string(data))
	err = WriteFile(t.Context(), filepath.Dir(path), nil, WriteOptions{Mode: 0o600, Overwrite: true})
	require.ErrorIs(t, err, errUtils.ErrWriteFile)
}
