package materialize_test

import (
	"context"
	"io"
	"math/rand/v2"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/fixtures"
	"github.com/cloudposse/atmos/pkg/artifact/materialize"
)

// The sizes below make the tests meaningful: the member is far larger than any read buffer, and
// the cancellation arrives long before the end of it.
const (
	bigMemberBytes      = 4 << 20
	cancelAfterBytes    = 64 << 10
	maxReadsAfterCancel = 256 << 10
)

// cancelingReader cancels a context once it served cancelAfter bytes and counts what it serves
// afterwards. When atEOF is set it cancels at the end of the stream instead.
type cancelingReader struct {
	inner       io.ReadCloser
	cancel      context.CancelFunc
	cancelAfter int
	atEOF       bool

	served        int
	servedCancel  int
	afterCanceled int
	canceled      bool
}

func (c *cancelingReader) Read(p []byte) (int, error) {
	n, err := c.inner.Read(p)
	c.served += n
	if c.canceled {
		c.afterCanceled += n
	}
	if !c.canceled && ((!c.atEOF && c.served >= c.cancelAfter) || (c.atEOF && err == io.EOF)) {
		c.canceled = true
		c.servedCancel = c.served
		c.cancel()
	}
	return n, err
}

func (c *cancelingReader) Close() error { return c.inner.Close() }

// noisyFile writes an incompressible file so the packaged payload stays large for every packaging.
func noisyFile(t *testing.T, src string) {
	t.Helper()
	data := make([]byte, bigMemberBytes)
	random := rand.New(rand.NewPCG(1, 2))
	for index := range data {
		data[index] = byte(random.Uint32())
	}
	fixtures.WriteFile(t, src, "big.bin", string(data), 0o644)
}

func payloadStream(t *testing.T, content *artifact.Content) *artifact.ContentStream {
	t.Helper()
	for index := range content.Streams {
		if content.Streams[index].Role == artifact.BlobPayload {
			return &content.Streams[index]
		}
	}
	require.FailNow(t, "the fixture must have a payload stream")
	return nil
}

func TestMaterialize_CancellationStopsInProgressArchiveMemberCopy(t *testing.T) {
	for _, packaging := range []artifact.Packaging{artifact.PackagingTar, artifact.PackagingTgz} {
		t.Run(string(packaging), func(t *testing.T) {
			src := t.TempDir()
			noisyFile(t, src)
			content := contentOf(t, prepared(t, src, packaging))
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			stream := payloadStream(t, content)
			reader := &cancelingReader{inner: stream.Reader, cancel: cancel, cancelAfter: cancelAfterBytes}
			stream.Reader = reader

			out := dest(t)
			err := materialize.Materialize(ctx, content, out)
			require.ErrorIs(t, err, context.Canceled)
			require.True(t, reader.canceled, "the fixture must reach the cancellation point")
			assert.Less(t, reader.afterCanceled, maxReadsAfterCancel, "reading must stop soon after the cancellation, not drain the member")
			_, statErr := os.Lstat(out)
			assert.True(t, os.IsNotExist(statErr), "a canceled extraction installs nothing")
			assertNoScratch(t, out)
		})
	}
}

func TestMaterialize_CancellationAtEndOfPayloadNeverInstalls(t *testing.T) {
	for _, packaging := range []artifact.Packaging{artifact.PackagingTar, artifact.PackagingTgz, artifact.PackagingZip} {
		t.Run(string(packaging), func(t *testing.T) {
			src := t.TempDir()
			fixtures.WriteFilePackage(t, src)
			content := contentOf(t, prepared(t, src, packaging))
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			stream := payloadStream(t, content)
			reader := &cancelingReader{inner: stream.Reader, cancel: cancel, atEOF: true}
			stream.Reader = reader

			out := dest(t)
			err := materialize.Materialize(ctx, content, out)
			require.True(t, reader.canceled, "the fixture must reach the end of the payload")
			require.ErrorIs(t, err, context.Canceled, "a context canceled while the last member finished must not install")
			_, statErr := os.Lstat(out)
			assert.True(t, os.IsNotExist(statErr))
			assertNoScratch(t, out)
		})
	}
}

func TestMaterialize_PreCanceledZipWritesNothing(t *testing.T) {
	// A zip member is inflated from the local spool; the in-package test covers the cancellation
	// arriving mid-member, this one the whole pipeline with a context that is already canceled.
	src := t.TempDir()
	noisyFile(t, src)
	content := contentOf(t, prepared(t, src, artifact.PackagingZip))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	out := dest(t)
	err := materialize.Materialize(ctx, content, out)
	require.ErrorIs(t, err, context.Canceled)
	_, statErr := os.Lstat(filepath.Join(out, "big.bin"))
	assert.True(t, os.IsNotExist(statErr))
}
