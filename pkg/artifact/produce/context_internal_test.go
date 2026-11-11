package produce

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

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
