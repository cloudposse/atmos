package produce

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/opencontainers/go-digest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/archive"
)

func TestArchiveMemberStopsReadingAfterCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	members := withContext(ctx, []archive.Member{{Open: func() (io.ReadCloser, error) { return io.NopCloser(strings.NewReader("payload")), nil }}})
	reader, err := members[0].Open()
	require.NoError(t, err)
	defer reader.Close()
	buf := make([]byte, 1)
	_, err = reader.Read(buf)
	require.NoError(t, err)
	cancel()
	_, err = reader.Read(buf)
	require.True(t, errors.Is(err, context.Canceled))
}

func TestVerifyingReader_ClassifiesStreamFailures(t *testing.T) {
	body := []byte("payload")
	hash := digest.FromBytes(body)
	closedFile := func(t *testing.T) *os.File {
		t.Helper()
		file, err := os.CreateTemp(t.TempDir(), "closed-*")
		require.NoError(t, err)
		require.NoError(t, file.Close())
		return file
	}

	t.Run("a read failure keeps its cause and gains the packaging sentinel", func(t *testing.T) {
		reader := newVerifyingReader(closedFile(t), hash, int64(len(body)))
		_, err := reader.Read(make([]byte, 8))
		require.ErrorIs(t, err, errUtils.ErrArtifactPackagingInvalid)
		require.ErrorIs(t, err, os.ErrClosed)
	})
	t.Run("a close failure keeps its cause and gains the packaging sentinel", func(t *testing.T) {
		err := newVerifyingReader(closedFile(t), hash, int64(len(body))).Close()
		require.ErrorIs(t, err, errUtils.ErrArtifactPackagingInvalid)
		require.ErrorIs(t, err, os.ErrClosed)
	})
	t.Run("end of stream stays unchanged when the content matches", func(t *testing.T) {
		reader := newVerifyingReader(io.NopCloser(bytes.NewReader(body)), hash, int64(len(body)))
		got, err := io.ReadAll(reader)
		require.NoError(t, err)
		assert.Equal(t, body, got)
		require.NoError(t, reader.Close())
	})
	t.Run("a digest mismatch at the end keeps the integrity classification", func(t *testing.T) {
		reader := newVerifyingReader(io.NopCloser(bytes.NewReader([]byte("tampered"))), hash, int64(len(body)))
		_, err := io.ReadAll(reader)
		require.ErrorIs(t, err, errUtils.ErrArtifactIntegrityFailed)
		assert.NotErrorIs(t, err, errUtils.ErrArtifactPackagingInvalid)
	})
}
