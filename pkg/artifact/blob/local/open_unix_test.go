//go:build unix

package local

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"

	"github.com/cloudposse/atmos/pkg/artifact/blob"
)

func TestGetAndStatRejectFIFOWithoutBlocking(t *testing.T) {
	store := newStore(t)
	require.NoError(t, mkdirAll(store.Root(), DefaultDirPerm))
	require.NoError(t, unix.Mkfifo(filepath.Join(store.Root(), "fifo"), 0o600))
	_, err := store.Get(context.Background(), "fifo", blob.GetOptions{})
	require.True(t, blob.IsNotFound(err))
	_, err = store.Stat(context.Background(), "fifo", blob.StatOptions{})
	require.True(t, blob.IsNotFound(err))
}
