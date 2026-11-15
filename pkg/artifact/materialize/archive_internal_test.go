package materialize

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"math/rand/v2"
	"os"
	"path/filepath"
	"testing"

	"github.com/opencontainers/go-digest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/artifact"
)

const (
	internalBigBytes = 2 << 20
	internalMaxAfter = 256 << 10
)

// cancelOnBytesReader cancels once it served cancelAfter bytes and counts the bytes served after.
type cancelOnBytesReader struct {
	data        []byte
	cancel      context.CancelFunc
	cancelAfter int

	served        int
	afterCanceled int
	canceled      bool
}

func (c *cancelOnBytesReader) Read(p []byte) (int, error) {
	if len(c.data) == 0 {
		return 0, io.EOF
	}
	n := copy(p, c.data)
	c.data = c.data[n:]
	c.served += n
	if c.canceled {
		c.afterCanceled += n
	}
	if !c.canceled && c.served >= c.cancelAfter {
		c.canceled = true
		c.cancel()
	}
	return n, nil
}

func noise(size int) []byte {
	data := make([]byte, size)
	random := rand.New(rand.NewPCG(3, 4))
	for index := range data {
		data[index] = byte(random.Uint32())
	}
	return data
}

// newTestTree returns a tree over a temporary root whose inventory holds one file entry.
func newTestTree(t *testing.T, name string, body []byte) (*tree, string) {
	t.Helper()
	dir := t.TempDir()
	root, err := os.OpenRoot(dir)
	require.NoError(t, err)
	t.Cleanup(func() { _ = root.Close() })
	record := &artifact.Record{Inventory: []artifact.Entry{{Path: name, Type: artifact.EntryFile, Size: int64(len(body)), Mode: 0o644, Digest: digest.FromBytes(body)}}}
	return newTree(root, record), dir
}

func TestApplyMember_StopsInProgressCopyWhenCanceled(t *testing.T) {
	body := noise(internalBigBytes)
	tests := []struct {
		name        string
		preCanceled bool
	}{
		{"pre-canceled", true},
		{"canceled mid-copy", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tr, _ := newTestTree(t, "big.bin", body)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if tt.preCanceled {
				cancel()
			}
			reader := &cancelOnBytesReader{data: body, cancel: cancel, cancelAfter: 64 << 10}
			err := applyMember(ctx, tr, &managedChecker{}, &archiveMember{name: "big.bin", kind: memberFile, size: int64(len(body)), body: reader})
			require.ErrorIs(t, err, context.Canceled)
			assert.Less(t, reader.afterCanceled, internalMaxAfter, "the copy stops soon after the cancellation")
			assert.Less(t, reader.served, len(body), "the member is not read to the end")
		})
	}
}

func TestApplyZipFile_StopsInflatingWhenCanceled(t *testing.T) {
	body := noise(internalBigBytes)
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create("big.bin")
	require.NoError(t, err)
	_, err = w.Write(body)
	require.NoError(t, err)
	require.NoError(t, zw.Close())
	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	require.NoError(t, err)
	require.Len(t, zr.File, 1)

	tr, dir := newTestTree(t, "big.bin", body)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err = applyZipFile(ctx, tr, zr.File[0], &managedChecker{})
	require.ErrorIs(t, err, context.Canceled)
	info, statErr := os.Stat(filepath.Join(dir, "big.bin"))
	if statErr == nil {
		assert.Zero(t, info.Size(), "a canceled member writes no bytes")
	}
}
