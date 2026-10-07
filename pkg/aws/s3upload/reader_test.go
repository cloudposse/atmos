package s3upload

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
