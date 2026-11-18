//go:build unix

package cache_test

import (
	"testing"

	"github.com/cloudposse/atmos/pkg/artifact/discovery/cache"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

func TestOfflineRejectsFIFO(t *testing.T) {
	h := newHarness(t, "scope")
	source := h.source("repository")
	require.NoError(t, unix.Mkdir(h.store.Directory(), cache.DirectoryMode))
	require.NoError(t, unix.Mkfifo(h.store.Path("repository", "ref"), 0o600))
	_, _, err := h.load(t, source, cache.Mode{Offline: true})
	require.ErrorIs(t, err, errOffline)
}
