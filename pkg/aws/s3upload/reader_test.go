package s3upload

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUploadReaderHashesEntireStreamFromNonzeroOffset(t *testing.T) {
	t.Parallel()
	const content = "complete artifact"
	reader := strings.NewReader(content)
	_, err := reader.Seek(9, 0)
	require.NoError(t, err)
	client := newMemoryS3()
	opts := Options{Source: "artifact.bin", Destination: "s3://bucket/artifact.bin"}

	changed, err := UploadReader(t.Context(), client, reader, int64(len(content)), opts)
	require.NoError(t, err)
	assert.True(t, changed)
	assert.Equal(t, content, client.bodies["artifact.bin"])
	digest := sha256.Sum256([]byte(content))
	assert.Equal(t, hex.EncodeToString(digest[:]), client.objects["artifact.bin"].Metadata[checksumMetadata])

	unchanged, err := UploadReader(t.Context(), client, reader, int64(len(content)), opts)
	require.NoError(t, err)
	assert.False(t, unchanged)
	assert.Equal(t, 1, client.writes)
}

func TestUploadReaderStopsHashingOnCancellation(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	client := newMemoryS3()
	_, err := UploadReader(ctx, client, strings.NewReader("data"), 4, Options{Source: "artifact.bin", Destination: "s3://bucket/artifact.bin"})
	require.ErrorIs(t, err, context.Canceled)
	assert.Zero(t, client.reads)
	assert.Zero(t, client.writes)
}

func TestUploadReaderRejectsInvalidInputBeforeAWS(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, destination string
		size              int64
	}{
		{"negative size", "s3://bucket/key", -1},
		{"oversized", "s3://bucket/key", maxObjectSize + 1},
		{"invalid destination", "https://bucket/key", 4},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			client := newMemoryS3()
			_, err := UploadReader(t.Context(), client, strings.NewReader("data"), tc.size, Options{Source: "file", Destination: tc.destination})
			require.Error(t, err)
			assert.Zero(t, client.reads)
			assert.Zero(t, client.writes)
		})
	}
}

func TestUploadReaderFailsOnUnreadableSource(t *testing.T) {
	t.Parallel()
	filename := filepath.Join(t.TempDir(), "file")
	require.NoError(t, os.WriteFile(filename, []byte("data"), 0o600))
	file, err := os.Open(filename)
	require.NoError(t, err)
	require.NoError(t, file.Close())
	client := newMemoryS3()
	_, err = UploadReader(t.Context(), client, file, 4, Options{Source: "file", Destination: "s3://bucket/key"})
	require.ErrorIs(t, err, os.ErrClosed)
	assert.Zero(t, client.reads)
	assert.Zero(t, client.writes)
}
